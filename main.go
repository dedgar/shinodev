package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ses"
	"github.com/jinzhu/gorm"
	_ "github.com/jinzhu/gorm/dialects/sqlite"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	_ "github.com/lib/pq"
	"github.com/qor/admin"
	"github.com/qor/auth"
	"github.com/qor/auth/auth_identity"
	"github.com/qor/auth/providers/google"
	"github.com/qor/auth_themes/clean"
	"github.com/qor/session/manager"
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
	user     = os.Getenv("POSTGRESQL_USER")
	password = os.Getenv("POSTGRESQL_PASSWORD")
	dbname   = os.Getenv("POSTGRESQL_DATABASE")
	certacc  = os.Getenv("CERT_ACC")
	postmap  = make(map[string]string)
)

type Contact struct {
	Name    string //`json:"name" form:"name"`
	Email   string //`json:"email" form:"email"`
	Message string //`json:"message" form:"message"`
}

// Define a GORM-backend model
type User struct {
	gorm.Model
	Name string
}

// Define another GORM-backend model
type Product struct {
	gorm.Model
	Name        string
	Description string
}

type Template struct {
	templates *template.Template
}

func initDatabase() {
	fmt.Println("tet")
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

// GET kanjitainer
func getContainer(c echo.Context) error {
	return c.Render(http.StatusOK, "container.html", "container")
}

// GET /watch/:show/:season/:episode
func getShow(c echo.Context) error {
	show := c.Param("show")
	season := c.Param("season")
	episode := c.Param("episode")

	vid_list := availableVids(show, season, episode)
	if vid_list {

		return c.Render(http.StatusOK, "episode_view.html", map[string]interface{}{
			"show":    show,
			"season":  season,
			"episode": episode,
		})
	}
	return c.Render(http.StatusNotFound, "404.html", "404 Video not found")
}

// GET /kanji
func getJapanese(c echo.Context) error {
	return c.Render(http.StatusOK, "level_selection.html", "level_selection")
}

// GET /kanji/:selection/:level
func getLevel(c echo.Context) error {
	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sql.Open("postgres", psqlInfo)

	if err != nil {
		log.Fatal(err)
	}

	var sqlQuery string

	switch c.Param("selection") {
	case "grade":
		sqlQuery = "SELECT kanj, von, vkun, transl, roma, rememb, jlpt, school FROM info WHERE school = $1"
	case "jlpt":
		sqlQuery = "SELECT kanj, von, vkun, transl, roma, rememb, jlpt, school FROM info WHERE jlpt = $1"
	}
	rows, err := db.Query(sqlQuery, c.Param("level"))

	if err != nil {
		log.Fatal(err)
	}

	defer rows.Close()

	var entry []string

	for rows.Next() {
		var kanj string
		var von string
		var vkun string
		var transl string
		var roma string
		var rememb string
		var jlpt string
		var school string

		if err := rows.Scan(&kanj, &von, &vkun, &transl, &roma, &rememb, &jlpt, &school); err != nil {
			log.Fatal(err)
		}
		entry = append(entry, kanj)
	}

	if err := rows.Err(); err != nil {
		log.Fatal(err)

	}

	selection := c.Param("selection")
	level := c.Param("level")
	entrymap := map[string]interface{}{"entry": entry, "selection": selection, "level": level}

	return c.Render(http.StatusOK, "kanji_list.html", entrymap) //map[string]interface{}{
	//	"entry":     entry,
	//	"selection": selection,
	//	"level":     level,
	//})

}

// GET /:selection/:level/:kanji
func getKanji(c echo.Context) error {
	psqlInfo := fmt.Sprintf("host=%s port=%s user=%s "+
		"password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	db, err := sql.Open("postgres", psqlInfo)

	if err != nil {
		log.Fatal(err)
	}

	// ensure :kanji isn't used as an escaped query like "%e9%9b%a8"
	uni_kanj, err := url.QueryUnescape(c.Param("kanji"))

	// start list of all in level get

	var sqlQuery string

	switch c.Param("selection") {
	case "grade":
		sqlQuery = "SELECT kanj, von, vkun, transl, roma, rememb, jlpt, school FROM info WHERE school = $1"
	case "jlpt":
		sqlQuery = "SELECT kanj, von, vkun, transl, roma, rememb, jlpt, school FROM info WHERE jlpt = $1"
	}
	rows, err := db.Query(sqlQuery, c.Param("level"))

	if err != nil {
		log.Fatal(err)
	}

	defer rows.Close()

	other_kanj := make(map[string]int)
	kanj_index := make(map[int]string)

	k_index := 0

	for rows.Next() {
		var kanj string
		var von string
		var vkun string
		var transl string
		var roma string
		var rememb string
		var jlpt string
		var school string

		switch err := rows.Scan(&kanj, &von, &vkun, &transl, &roma, &rememb, &jlpt, &school); err {
		case sql.ErrNoRows:
			return c.Render(http.StatusNotFound, "404.html", "No rows were found")
		case nil:
			//fmt.Println(kanj, von, vkun, transl, roma, rememb, jlpt, school)
		default:
			log.Fatal(err)
		}

		other_kanj[kanj] = k_index
		kanj_index[k_index] = kanj
		k_index++
	}

	if err := rows.Err(); err != nil {
		log.Fatal(err)
	}

	// start single kanji definition get

	if err != nil {
		log.Fatal(err)
	}

	singleQuery := "SELECT kanj, von, vkun, transl, roma, rememb, jlpt, school FROM info WHERE kanj = $1"
	row := db.QueryRow(singleQuery, uni_kanj)

	if err != nil {
		log.Fatal(err)
	}

	var kanj string
	var von string
	var vkun string
	var transl string
	var roma string
	var rememb string
	var jlpt string
	var school string
	var p_index int
	var n_index int
	var p_kanj string
	var n_kanj string
	var u_level string
	var u_selection string

	switch err := row.Scan(&kanj, &von, &vkun, &transl, &roma, &rememb, &jlpt, &school); err {
	case sql.ErrNoRows:
		// use a 404 here
		fmt.Println("No rows were returned!")
	case nil:
		//		fmt.Println(kanj, von, vkun, transl, roma, rememb, jlpt, school)
	default:
		log.Fatal(err)
	}

	num_items := len(other_kanj)

	p_index = other_kanj[uni_kanj] - 1
	n_index = other_kanj[uni_kanj] + 1

	// if we're at the beginning of the map, previous should be the last item
	if p_index < 0 {
		p_kanj = kanj_index[num_items-1]
	} else {
		p_kanj = kanj_index[p_index]
	}

	// if we reach the end of the map, next should cycle back to the beginning
	if n_index == num_items {
		n_kanj = kanj_index[0]
	} else {
		n_kanj = kanj_index[n_index]
	}

	u_level = c.Param("level")
	u_selection = c.Param("selection")

	entry := map[string]string{
		"kanj":        kanj,
		"von":         von,
		"vkun":        vkun,
		"transl":      transl,
		"roma":        roma,
		"rememb":      rememb,
		"jlpt":        jlpt,
		"school":      school,
		"p_kanj":      p_kanj,
		"n_kanj":      n_kanj,
		"u_level":     u_level,
		"u_selection": u_selection,
	}

	// TODO regex checking on values of :level and :selection
	return c.Render(http.StatusOK, "flashcard.html", entry)
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

	sess, err := session.NewSession(&aws.Config{
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

//func qorMiddleware() echo.MiddlewareFunc {
//    return func(next echo.HandlerFunc) echo.HandlerFunc {
//      return func(c echo.Context) err error {
//         e.Set(manager.SessionManager.Middleware(mux)
//        return next(c)
//   }
//   }
//}

//func qorFunc(next echo.HandlerFunc) echo.HandlerFunc {
//	return func(c echo.Context) error {
//		next.ServeHTTP(manager.SessionManager.Middleware(mux))
//		return next(c)
//	}
//}

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

	DB, _ := gorm.Open("sqlite3", "demo.db")
	DB.AutoMigrate(&User{}, &Product{})

	//	config := auth.Config{}

	Auth := clean.New(&auth.Config{
		DB: DB,
		//		Render:    config.View,
		//		Mailer:    config.Mailer,
		//		UserModel: models.User{},
	})

	DB.AutoMigrate(&auth_identity.AuthIdentity{})

	g_id, g_key := getOauth("/secrets/google_auth_creds")

	Auth.RegisterProvider(google.New(&google.Config{
		ClientID:     g_id,
		ClientSecret: g_key,
	}))

	Admin := admin.New(&admin.AdminConfig{DB: DB})

	Admin.AddResource(&User{})
	Admin.AddResource(&Product{})

	mux := http.NewServeMux()

	//Admin.SetAuth
	//manager.Sessionmanager.Middleware()
	//e.Use(manager.SessionManager.Middleware(mux))
	//e.Use(qorMiddleware())
	Admin.MountTo("/admin", mux)

	e.Any("/admin/*", echo.WrapHandler(mux))
	e.Any("/auth/*", echo.WrapHandler(Auth.NewServeMux()))

	e.Use(echo.WrapMiddleware(manager.SessionManager.Middleware(mux)))

	findPosts("./tmpl/posts", ".html")
	//fmt.Println(findPosts("./tmpl/posts", ".html"))
	e.GET("/", getMain)
	e.GET("/about", getAbout)
	e.GET("/about-us", getAbout)
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
	e.GET("/watch/:show/:season/:episode", getShow)
	//	e.GET("/grade/:level", getLevel)
	e.GET("/kanji", getJapanese)
	e.GET("/kanji/", getJapanese)
	e.GET("/kanjitainer", getContainer)
	e.GET("/kanjitainer/", getContainer)
	e.GET("/kanji/:selection/:level", getLevel)
	e.GET("/kanji/:selection/:level/:kanji", getKanji)
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
