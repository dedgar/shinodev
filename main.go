package main

import (
	"bufio"
	"bytes"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"./models"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	asession "github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/postgres"
	"github.com/labstack/echo"
	"github.com/labstack/echo-contrib/session"
	"golang.org/x/crypto/bcrypt"

	"github.com/gorilla/sessions"
	"github.com/labstack/echo/middleware"
)

const (
	Sender    = "contact@shinobu.ninja"
	Recipient = "contact@shinobu.ninja"
	Subject   = "dedgar contact form submission"
	CharSet   = "UTF-8"
)

var (
	host     = os.Getenv("POSTGRESQL_SERVICE_HOST")
	port     = os.Getenv("POSTGRESQL_SERVICE_PORT")
	dbuser   = os.Getenv("POSTGRESQL_USER")
	dbpass   = os.Getenv("POSTGRESQL_PASSWORD")
	dbname   = os.Getenv("POSTGRESQL_DATABASE")
	certacc  = os.Getenv("CERT_ACC")
	postmap  = make(map[string]string)
	psqlInfo = fmt.Sprintf("host=%s port=%s user=%s "+"password=%s dbname=%s sslmode=disable", host, port, dbuser, dbpass, dbname)
	DB, _    = gorm.Open("postgres", psqlInfo)
)

type Contact struct {
	Name    string //`json:"name" form:"name"`
	Email   string //`json:"email" form:"email"`
	Message string //`json:"message" form:"message"`
}

type Template struct {
	templates *template.Template
}

func (t *Template) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

// only return true if the url maps to a file in our specific hierarchy
// can be replaced with a
func availableVids(show string, season string, episode string) bool {
	if _, err := os.Stat("./static/vid/" + show + "/" + season + "/" + episode + ".mp4"); err == nil {
		return true
	}
	return false
}

// GET /
func getMain(c echo.Context) error {
	return c.Render(http.StatusOK, "main.html", postmap)
}

// handle any error by attempting to render a custom page for it
func custom404Handler(err error, c echo.Context) {
	code := http.StatusInternalServerError
	if he, ok := err.(*echo.HTTPError); ok {
		code = he.Code
	}
	errorPage := fmt.Sprintf("%d.html", code)
	if err := c.Render(code, errorPage, code); err != nil {
		c.Logger().Error(err)
	}
	c.Logger().Error(err)
}

func getCert(c echo.Context) error {
	response := c.Param("response")
	return c.String(http.StatusOK, response+"."+certacc)
}

// GET /about
func getAbout(c echo.Context) error {
	return c.Render(http.StatusOK, "about.html", nil)
}

// GET /contact
func getContact(c echo.Context) error {
	return c.Render(http.StatusOK, "contact.html", nil)
}

// GET /login
func getLogin(c echo.Context) error {
	return c.Render(http.StatusOK, "login.html", nil)
}

// GET /login
func getRegister(c echo.Context) error {
	return c.Render(http.StatusOK, "register.html", nil)
}

// GET /privacy
func getPrivacy(c echo.Context) error {
	return c.Render(http.StatusOK, "privacy.html", nil)
}

// GET /dev
func getDev(c echo.Context) error {
	return c.Render(http.StatusOK, "dev.html", nil)
}

// POST /post-contact
func postContact(c echo.Context) error {

	if strings.Contains(c.FormValue("message"), "http") && strings.Contains(c.FormValue("message"), "dedgar.com/") == false {
		return c.String(http.StatusOK, "Form submitted")
	}

	TextBody := c.FormValue("name") + "\n" + c.FormValue("email") + "\n" + c.FormValue("message")

	sess, err := asession.NewSession(&aws.Config{
		Region: aws.String("us-west-2")},
	)

	svc := ses.New(sess)

	input := &ses.SendEmailInput{
		Destination: &ses.Destination{
			CcAddresses: []*string{},
			ToAddresses: []*string{
				aws.String(Recipient),
			},
		},
		Message: &ses.Message{
			Body: &ses.Body{
				Text: &ses.Content{
					Charset: aws.String(CharSet),
					Data:    aws.String(TextBody),
				},
			},
			Subject: &ses.Content{
				Charset: aws.String(CharSet),
				Data:    aws.String(Subject),
			},
		},
		Source: aws.String(Sender),
	}

	result, err := svc.SendEmail(input)

	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case ses.ErrCodeMessageRejected:
				fmt.Println(ses.ErrCodeMessageRejected, aerr.Error())
			case ses.ErrCodeMailFromDomainNotVerifiedException:
				fmt.Println(ses.ErrCodeMailFromDomainNotVerifiedException, aerr.Error())
			case ses.ErrCodeConfigurationSetDoesNotExistException:
				fmt.Println(ses.ErrCodeConfigurationSetDoesNotExistException, aerr.Error())
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			fmt.Println(err.Error())
		}

	}
	fmt.Println(c.FormValue("name"))
	fmt.Println(c.FormValue("email"))
	fmt.Println(c.FormValue("message"))
	fmt.Println("Email Sent to address: " + Recipient)
	fmt.Println(result)
	return c.String(http.StatusOK, "Form submitted")
}

// GET /post/:postname
func getPost(c echo.Context) error {
	post := c.Param("postname")
	if _, ok := postmap[post]; ok {
		return c.Render(http.StatusOK, post+".html", post)
	}
	return c.Render(http.StatusNotFound, "404.html", "404 Post not found")
}

// GET /post
func getPostView(c echo.Context) error {
	return c.Render(http.StatusOK, "post_view.html", postmap)
}

func findSummary(fpath string) string {
	file, err := os.Open(fpath + "_summary")
	if err != nil {
		return "No summary"
	}
	defer file.Close()

	var buffer bytes.Buffer
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		buffer.WriteString(line)
		//		if line == "<!--more-->" {
		//			break
		//		}
		//fmt.Println(scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		log.Fatal(err)
	}
	return buffer.String()
}

// Populates a map of postnames that gets checked every call to GET /post/:postname.
// We're running in a container, so populating this on startup works fine as we won't be adding
// any new posts while the container is running.
func findPosts(dirpath string, extension string) map[string]string {
	if err := filepath.Walk(dirpath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Println(err)
		}
		if strings.HasSuffix(path, extension) {
			postname := strings.Split(path, extension)[0]
			summary := findSummary(postname)
			//fmt.Println(summary)
			//fmt.Println(fmt.Sprintf("%T", summary))
			postmap[filepath.Base(postname)] = summary
		}
		return err
	}); err != nil {
		panic(err)
	}
	return postmap
}

func getOauth(filepath string) (id, key string) {
	filebytes, err := ioutil.ReadFile(filepath)
	if err != nil {
		fmt.Println(err)
	}
	file_str := string(filebytes)

	id, key = strings.Split(file_str, "\n")[0], strings.Split(file_str, "\n")[1]

	return id, key
}

func HashPass(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

func createUser(eName, uName, pWord string) {
	hashed_pw, err := HashPass(pWord)

	if err != nil {
		log.Fatal(err)
	}

	new_user := models.User{Email: eName, UName: uName, Password: hashed_pw}
	DB.NewRecord(new_user)
	DB.Create(&new_user)
}

// POST /register
func postRegister(c echo.Context) error {
	TextBody := c.FormValue("login") + "\n" + c.FormValue("password")
	fmt.Println(TextBody)

	if !checkUser(c.FormValue("username")) || checkEmail(c.FormValue("email")) {
		return c.String(http.StatusOK, "Email address or username already taken, try again!")
	}

	createUser(c.FormValue("email"), c.FormValue("username"), c.FormValue("password"))

	return c.Redirect(http.StatusPermanentRedirect, "/login")
}

func checkEmail(eName string) bool {
	var user models.User
	var found_e models.User

	DB.Where(&models.User{Email: eName}).First(&user).Scan(&found_e)

	if found_e.Email != "" {
		fmt.Println("Email already taken!")
		return false
	}

	fmt.Println("Email not taken!")
	return true
}
func checkUser(uName string) bool {
	var user models.User
	var found_u models.User

	DB.Where(&models.User{UName: uName}).First(&user).Scan(&found_u)

	if found_u.UName != "" {
		fmt.Println("Username already taken!")
		return false
	}

	fmt.Println("Username not taken!")
	return true
}

func findUser(uName, pWord string) bool {
	var user models.User
	var found_u models.User

	hashed_pw, err := HashPass(pWord)

	if err != nil {
		log.Fatal(err)
	}

	DB.Where(&models.User{UName: uName, Password: hashed_pw}).First(&user).Scan(&found_u)
	fmt.Println(found_u)
	fmt.Println(found_u.UName, found_u.Password)
	if found_u.UName == "" || found_u.Password == "" {
		fmt.Println("Invalid username or password!")
		return false
	}

	fmt.Println("found the name!")
	return true
}

// POST /login
func postLogin(c echo.Context) error {
	if checkUser(c.FormValue("username")) {
		return c.String(http.StatusOK, "Username not found!")
	}

	if !findUser(c.FormValue("username"), c.FormValue("password")) {
		sess, _ := session.Get("session", c)
		sess.Values["dude_logged_in"] = c.FormValue("username")
		sess.Values["yepyepyep"] = "true"
		sess.Save(c.Request(), c.Response())

		return c.Redirect(http.StatusPermanentRedirect, "/")
	}

	return c.Render(http.StatusUnauthorized, "404.html", "401 not authenticated")
}

func checkDB() {
	if !DB.HasTable(&models.User{}) {
		fmt.Println("Creating users table")
		DB.CreateTable(&models.User{})
	}
}

func ServerHeader() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			//c.Response().Header().Set(echo.HeaderServer, "Echo/3.0")
			mainCookie(c)
			fmt.Println("serverheader /admin")
			sess, _ := session.Get("session", c)

			if sess.Values["authenticated"] == "true" {
				fmt.Println("in if block")
				fmt.Println(sess.Values)
				return next(c)
			}

			return next(c)
		}
	}
}

func getTrial(c echo.Context) error {
	sess, _ := session.Get("session", c)
	logged_in_dude := sess.Values["dude_logged_in"].(string)
	return c.String(http.StatusOK, logged_in_dude)
}

func ServerTet(http.Handler) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			fmt.Println("servertest /auth")
			//c.Response().Header().Set(echo.HeaderServer, "Echo/3.0")
			return next(c)
		}
	}
}

func mainCookie(c echo.Context) { //error {
	sess, _ := session.Get("session", c)
	sess.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   86400 * 7,
		HttpOnly: true,
	}
	sess.Values["foo"] = "bar"
	sess.Values["authenticated"] = "true"
	sess.Save(c.Request(), c.Response())
}

func main() {
	t := &Template{
		templates: func() *template.Template {
			tmpl := template.New("")
			if err := filepath.Walk("./tmpl", func(path string, info os.FileInfo, err error) error {
				if strings.HasSuffix(path, ".html") {
					_, err = tmpl.ParseFiles(path)
					if err != nil {
						log.Println(err)
					}
				}
				return err
			}); err != nil {
				panic(err)
			}
			return tmpl
		}(),
	}
	e := echo.New()
	e.Static("/", "static")
	e.Renderer = t
	//e.HTTPErrorHandler = custom404Handler
	//	e.Pre(middleware.HTTPSWWWRedirect())
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	//admin_group := e.Group("/posts", ServerHeader())
	//admin_group.Use(ServerHeader())
	e.Use(session.Middleware(sessions.NewCookieStore([]byte("supersecret"))))
	e.Use(ServerHeader())

	checkDB()

	//g_id, g_key := getOauth("/secrets/google_auth_creds")

	//		return req.RemoteAddr == "127.0.0.1" || (currentUser.(*models.User) != nil && currentUser.(*models.User).Role == "admin")

	findPosts("./tmpl/posts", ".html")
	//fmt.Println(findPosts("./tmpl/posts", ".html"))
	e.GET("/", getMain)
	e.POST("/", getMain)
	e.GET("/about", getAbout)
	e.GET("/register", getRegister)
	e.POST("/register", postRegister)
	e.GET("/login", getLogin)
	e.POST("/login", postLogin)
	e.GET("/about-us", getAbout)
	e.GET("/trial", getTrial)
	e.GET("/contact", getContact)
	e.GET("/contact-us", getContact)
	e.GET("/privacy-policy", getPrivacy)
	e.GET("/privacy", getPrivacy)
	e.GET("/dev", getDev)
	e.POST("/post-contact", postContact)
	e.GET("/post", getPostView)
	e.GET("/post/", getPostView)
	e.GET("/posts", getPostView)
	e.GET("/posts/", getPostView)
	e.GET("/post/:postname", getPost)
	e.GET("/posts/:postname", getPost)
	e.GET("/.well-known/acme-challenge/test", getCert)
	e.GET("/.well-known/acme-challenge/test/", getCert)
	e.GET("/.well-known/acme-challenge/:response", getCert)
	e.GET("/.well-known/acme-challenge/:response/", getCert)
	e.GET("/well-known/acme-challenge/:response", getCert)
	e.GET("/well-known/acme-challenge/:response/", getCert)
	e.File("/robots.txt", "static/public/robots.txt")
	e.File("/sitemap.xml", "static/public/sitemap.xml")
	e.Logger.Info(e.Start(":8080"))
	//	e.Logger.Info(e.StartAutoTLS(":443"))
}
