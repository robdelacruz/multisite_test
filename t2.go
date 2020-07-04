package main

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"

	_ "github.com/mattn/go-sqlite3"
	"gopkg.in/russross/blackfriday.v2"
)

type User struct {
	Userid   int64
	Username string
	Active   bool
	Email    string
}
type Site struct {
	Siteid   int64
	Sitename string
	Desc     string
}
type Page struct {
	Pageid int64
	Title  string
	Body   string
}

var _loremipsum string

func init() {
	_loremipsum = `<p>Lorem ipsum dolor sit amet, consectetur adipiscing elit. Etiam mattis volutpat libero a sodales. Sed a sagittis est. Sed eros nunc, maximus id lectus nec, tempor tincidunt felis. Cras viverra arcu ut tellus sagittis, et pharetra arcu ornare. Cras euismod turpis id auctor posuere. Nunc euismod molestie est, nec congue velit vestibulum rutrum. Etiam vitae consectetur mauris.</p>
<blockquote>
<p>Lorem ipsum dolor sit amet, consectetur adipiscing elit. Etiam mattis volutpat libero a sodales. Sed a sagittis est. Sed eros nunc, maximus id lectus nec, tempor tincidunt felis. Cras viverra arcu ut tellus sagittis, et pharetra arcu ornare. Cras euismod turpis id auctor posuere. Nunc euismod molestie est, nec congue velit vestibulum rutrum. Etiam vitae consectetur mauris.</p>
<p>Etiam sodales neque sit amet erat ullamcorper placerat. Curabitur sit amet sapien ac sem convallis efficitur. Class aptent taciti sociosqu ad litora torquent per conubia nostra, per inceptos himenaeos. Cras maximus felis dolor, ac ultricies mauris varius scelerisque. Proin vitae velit a odio eleifend tristique sit amet vitae risus. Curabitur varius sapien ut viverra suscipit.</p>
</blockquote>
<p>Etiam sodales neque sit amet erat ullamcorper placerat. Curabitur sit amet sapien ac sem convallis efficitur. Class aptent taciti sociosqu ad litora torquent per conubia nostra, per inceptos himenaeos. Cras maximus felis dolor, ac ultricies mauris varius scelerisque. Proin vitae velit a odio eleifend tristique sit amet vitae risus. Curabitur varius sapien ut viverra suscipit. Integer suscipit lectus vel velit rhoncus, eget condimentum neque imperdiet. Morbi dapibus condimentum convallis. Suspendisse potenti. Aenean fermentum nisi mauris, rhoncus malesuada enim semper semper.</p>`
}

type PrintFunc func(format string, a ...interface{}) (n int, err error)

func createTables(newfile string) {
	if fileExists(newfile) {
		s := fmt.Sprintf("File '%s' already exists. Can't initialize it.\n", newfile)
		fmt.Printf(s)
		os.Exit(1)
	}

	db, err := sql.Open("sqlite3", newfile)
	if err != nil {
		fmt.Printf("Error opening '%s' (%s)\n", newfile, err)
		os.Exit(1)
	}

	ss := []string{
		"CREATE TABLE user (user_id INTEGER PRIMARY KEY NOT NULL, username TEXT UNIQUE, password TEXT, active INTEGER NOT NULL, email TEXT);",
		"CREATE TABLE site (site_id INTEGER PRIMARY KEY NOT NULL, sitename TEXT UNIQUE, desc TEXT);",
		"INSERT INTO user (user_id, username, password, active, email) VALUES (1, 'admin', '', 1, '');",
	}

	tx, err := db.Begin()
	if err != nil {
		log.Printf("DB error (%s)\n", err)
		os.Exit(1)
	}
	for _, s := range ss {
		_, err := txexec(tx, s)
		if err != nil {
			tx.Rollback()
			log.Printf("DB error (%s)\n", err)
			os.Exit(1)
		}
	}
	err = tx.Commit()
	if err != nil {
		log.Printf("DB error (%s)\n", err)
		os.Exit(1)
	}

	site := Site{
		Sitename: "main",
		Desc:     "This is the main website",
	}
	_, err = createSite(db, &site)
	if err != nil {
		log.Printf("Error creating site (%s)\n", err)
		os.Exit(1)
	}
}

func main() {
	os.Args = os.Args[1:]
	sw, parms := parseArgs(os.Args)

	// [-i new_file]  Create and initialize db file
	if sw["i"] != "" {
		dbfile := sw["i"]
		if fileExists(dbfile) {
			s := fmt.Sprintf("File '%s' already exists. Can't initialize it.\n", dbfile)
			fmt.Printf(s)
			os.Exit(1)
		}
		createTables(dbfile)
		os.Exit(0)
	}

	// Need to specify a db file as first parameter.
	if len(parms) == 0 {
		s := `Usage:

Start webservice using database file:
	t2 <sites.db>

Initialize new database file:
	t2 -i <sites.db>
`
		fmt.Printf(s)
		os.Exit(0)
	}

	// Exit if db file doesn't exist.
	dbfile := parms[0]
	if !fileExists(dbfile) {
		s := fmt.Sprintf(`Sites database file '%s' doesn't exist. Create one using:
	wb -i <notes.db>
`, dbfile)
		fmt.Printf(s)
		os.Exit(1)
	}

	db, err := sql.Open("sqlite3", dbfile)
	if err != nil {
		fmt.Printf("Error opening '%s' (%s)\n", dbfile, err)
		os.Exit(1)
	}

	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "./static/coffee.ico") })
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	http.HandleFunc("/", indexHandler(db))
	http.HandleFunc("/createsite/", createsiteHandler(db))
	http.HandleFunc("/editsite/", editsiteHandler(db))
	http.HandleFunc("/createpage/", createpageHandler(db))
	http.HandleFunc("/editpage/", editpageHandler(db))

	port := "8000"
	fmt.Printf("Listening on %s...\n", port)
	err = http.ListenAndServe(fmt.Sprintf(":%s", port), nil)
	log.Fatal(err)
}

//*** DB functions ***
func sqlstmt(db *sql.DB, s string) *sql.Stmt {
	stmt, err := db.Prepare(s)
	if err != nil {
		log.Fatalf("db.Prepare() sql: '%s'\nerror: '%s'", s, err)
	}
	return stmt
}
func sqlexec(db *sql.DB, s string, pp ...interface{}) (sql.Result, error) {
	stmt := sqlstmt(db, s)
	defer stmt.Close()
	return stmt.Exec(pp...)
}
func txstmt(tx *sql.Tx, s string) *sql.Stmt {
	stmt, err := tx.Prepare(s)
	if err != nil {
		log.Fatalf("tx.Prepare() sql: '%s'\nerror: '%s'", s, err)
	}
	return stmt
}
func txexec(tx *sql.Tx, s string, pp ...interface{}) (sql.Result, error) {
	stmt := txstmt(tx, s)
	defer stmt.Close()
	return stmt.Exec(pp...)
}

func queryUserById(db *sql.DB, userid int64) *User {
	var u User
	s := "SELECT user_id, username, active, email FROM user WHERE user_id = ?"
	row := db.QueryRow(s, userid)
	err := row.Scan(&u.Userid, &u.Username, &u.Active, &u.Email)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		fmt.Printf("queryUser() db error (%s)\n", err)
		return nil
	}
	return &u
}
func querySiteById(db *sql.DB, siteid int64) *Site {
	var site Site
	s := "SELECT site_id, sitename, desc FROM site WHERE site_id = ?"
	row := db.QueryRow(s, siteid)
	err := row.Scan(&site.Siteid, &site.Sitename, &site.Desc)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		fmt.Printf("querySiteById() db error (%s)\n", err)
		return nil
	}
	return &site
}
func querySiteBySitename(db *sql.DB, sitename string) *Site {
	var site Site
	s := "SELECT site_id, sitename, desc FROM site WHERE sitename = ?"
	row := db.QueryRow(s, sitename)
	err := row.Scan(&site.Siteid, &site.Sitename, &site.Desc)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		fmt.Printf("querySiteBySitename() db error (%s)\n", err)
		return nil
	}
	return &site
}
func pagetblName(siteid int64) string {
	return fmt.Sprintf("pages_%d", siteid)
}
func queryPageById(db *sql.DB, siteid int64, pageid int64) *Page {
	var p Page
	pagetbl := pagetblName(siteid)
	s := fmt.Sprintf("SELECT page_id, title, body FROM %s WHERE page_id = ?", pagetbl)
	row := db.QueryRow(s, pageid)
	err := row.Scan(&p.Pageid, &p.Title, &p.Body)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		fmt.Printf("queryPageById() db error (%s)\n", err)
		return nil
	}
	return &p
}
func queryPageByTitle(db *sql.DB, siteid int64, title string) *Page {
	var p Page
	pagetbl := pagetblName(siteid)
	s := fmt.Sprintf("SELECT page_id, title, body FROM %s WHERE title = ?", pagetbl)
	row := db.QueryRow(s, title)
	err := row.Scan(&p.Pageid, &p.Title, &p.Body)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		fmt.Printf("queryPageByTitle() db error (%s)\n", err)
		return nil
	}
	return &p
}
func createSite(db *sql.DB, site *Site) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	s := "INSERT INTO site (sitename, desc) VALUES (?, ?)"
	result, err := txexec(tx, s, site.Sitename, site.Desc)
	if handleTxErr(tx, err) {
		return 0, err
	}
	siteid, err := result.LastInsertId()
	if handleTxErr(tx, err) {
		return 0, err
	}

	pagetbl := pagetblName(siteid)
	s = fmt.Sprintf("CREATE TABLE %s (page_id INTEGER PRIMARY KEY NOT NULL, title TEXT UNIQUE, body TEXT)", pagetbl)
	_, err = txexec(tx, s)
	if handleTxErr(tx, err) {
		return 0, err
	}

	err = tx.Commit()
	if handleTxErr(tx, err) {
		return 0, err
	}
	return siteid, nil
}
func createPage(db *sql.DB, site *Site, p *Page) (int64, error) {
	s := fmt.Sprintf("INSERT INTO %s (title, body) VALUES (?, ?)", pagetblName(site.Siteid))
	result, err := sqlexec(db, s, p.Title, p.Body)
	if err != nil {
		return 0, err
	}
	pageid, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	return pageid, nil
}

//*** Helper functions ***
func listContains(ss []string, v string) bool {
	for _, s := range ss {
		if v == s {
			return true
		}
	}
	return false
}
func fileExists(file string) bool {
	_, err := os.Stat(file)
	if err != nil && os.IsNotExist(err) {
		return false
	}
	return true
}
func makePrintFunc(w io.Writer) func(format string, a ...interface{}) (n int, err error) {
	// Return closure enclosing io.Writer.
	return func(format string, a ...interface{}) (n int, err error) {
		return fmt.Fprintf(w, format, a...)
	}
}
func atoi(s string) int {
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
func idtoi(sid string) int64 {
	return int64(atoi(sid))
}

func parseArgs(args []string) (map[string]string, []string) {
	switches := map[string]string{}
	parms := []string{}

	standaloneSwitches := []string{}
	definitionSwitches := []string{"i", "import"}
	fNoMoreSwitches := false
	curKey := ""

	for _, arg := range args {
		if fNoMoreSwitches {
			// any arg after "--" is a standalone parameter
			parms = append(parms, arg)
		} else if arg == "--" {
			// "--" means no more switches to come
			fNoMoreSwitches = true
		} else if strings.HasPrefix(arg, "--") {
			switches[arg[2:]] = "y"
			curKey = ""
		} else if strings.HasPrefix(arg, "-") {
			if listContains(definitionSwitches, arg[1:]) {
				// -a "val"
				curKey = arg[1:]
				continue
			}
			for _, ch := range arg[1:] {
				// -a, -b, -ab
				sch := string(ch)
				if listContains(standaloneSwitches, sch) {
					switches[sch] = "y"
				}
			}
		} else if curKey != "" {
			switches[curKey] = arg
			curKey = ""
		} else {
			// standalone parameter
			parms = append(parms, arg)
		}
	}

	return switches, parms
}

func unescape(s string) string {
	s2, err := url.QueryUnescape(s)
	if err != nil {
		return s
	}
	return s2
}
func escape(s string) string {
	return url.QueryEscape(s)
}

func parsePageUrl(url string) (string, string) {
	url = strings.Trim(url, "/")
	ss := strings.Split(url, "/")
	sslen := len(ss)
	if sslen == 0 {
		return "", ""
	} else if sslen == 1 {
		return unescape(ss[0]), ""
	}
	return unescape(ss[0]), unescape(ss[1])
}

func getLoginUser(r *http.Request, db *sql.DB) *User {
	c, err := r.Cookie("userid")
	if err != nil {
		return nil
	}
	userid := idtoi(c.Value)
	if userid == 0 {
		return nil
	}
	return queryUserById(db, userid)
}

func validateLogin(w http.ResponseWriter, login *User) bool {
	if login.Userid == -1 {
		http.Error(w, "Not logged in.", 401)
		return false
	}
	if !login.Active {
		http.Error(w, "Not an active user.", 401)
		return false
	}
	return true
}
func handleDbErr(w http.ResponseWriter, err error, sfunc string) bool {
	if err == sql.ErrNoRows {
		http.Error(w, "Not found.", 404)
		return true
	}
	if err != nil {
		log.Printf("%s: database error (%s)\n", sfunc, err)
		http.Error(w, "Server database error.", 500)
		return true
	}
	return false
}
func handleTxErr(tx *sql.Tx, err error) bool {
	if err != nil {
		tx.Rollback()
		return true
	}
	return false
}
func parseMarkdown(s string) string {
	return string(blackfriday.Run([]byte(s), blackfriday.WithExtensions(blackfriday.HardLineBreak|blackfriday.BackslashLineBreak)))
	//return string(blackfriday.Run([]byte(s), blackfriday.WithNoExtensions()))
}

//*** Html menu template functions ***
func printSectionMenuHead(P PrintFunc, sitename string, login *User) {
	P("<section class=\"col-menu flex flex-col text-xs px-4\">\n")
	P("   <div class=\"flex flex-col mb-4\">\n")
	P("     <h1 class=\"text-lg text-bold\"><a href=\"/\">%s</a></h1>\n", sitename)
	P("     <div class=\"\">\n")
	if login != nil {
		P("       <a class=\"text-gray-800 bg-gray-400 rounded px-2 mr-1\" href=\"#\">%s</a>\n", login.Username)
		P("       <a class=\"text-blue-900\" href=\"/logout\">logout</a>\n")
	} else {
		P("       <a class=\"text-blue-900\" href=\"/login\">login</a>\n")
	}
	P("    </div>\n")
	P("  </div>\n")
}
func printSectionMenuFoot(P PrintFunc) {
	P("  </section>\n")
}
func printMenuHead(P PrintFunc, title string) {
	P("<ul class=\"list-none mb-2\">\n")
	if title != "" {
		P("  <li><p class=\"border-b mb-1\">%s</p></li>\n", title)
	}
}
func printMenuFoot(P PrintFunc) {
	P("</ul>\n")
}
func printMenuLine(P PrintFunc, href, text string) {
	P("  <li><a class=\"text-blue-900\" href=\"%s\">%s</a></li>\n", href, text)
}

//*** Html form template functions ***
func printFormHead(P PrintFunc, action string) {
	P("<form class=\"max-w-2xl\" method=\"post\" action=\"%s\">\n", action)
}
func printFormFoot(P PrintFunc) {
	P("</form>\n")
}
func printFormTitle(P PrintFunc, title string) {
	P("<h1 class=\"border-b border-gray-500 pb-1 mb-4\">%s</h1>\n", title)
}
func printFormControlHead(P PrintFunc) {
	P("<div class=\"mb-2\">\n")
}
func printFormControlFoot(P PrintFunc) {
	P("</div>\n")
}
func printFormLabel(P PrintFunc, sfor, lbl string) {
	P("<label class=\"lbl\" for=\"%s\">%s</label>\n", sfor, lbl)
}
func printFormInput(P PrintFunc, sid, val string, size int) {
	P("<input class=\"input w-full\" id=\"%s\" name=\"%s\" type=\"text\" size=\"%d\" value=\"%s\">\n", sid, sid, size, val)
}
func printFormTextarea(P PrintFunc, sid, val string, rows int) {
	P("<textarea class=\"input w-full\" id=\"%s\" name=\"%s\" rows=\"%d\">%s</textarea>\n", sid, sid, rows, val)
}
func printFormButton(P PrintFunc, sid, lbl, stype string) {
	P("<button class=\"btn\" id=\"%s\" name=\"%s\" type=\"%s\">%s</button>\n", sid, sid, stype, lbl)
}
func printFormSubmitButton(P PrintFunc, sid, lbl string) {
	printFormButton(P, sid, lbl, "submit")
}
func printFormControlError(P PrintFunc, errmsg string) {
	if errmsg != "" {
		printFormControlHead(P)
		P("<p class=\"text-red-500 italic\">%s</p>\n", errmsg)
		printFormControlFoot(P)
	}
}
func printFormControlInput(P PrintFunc, sid, lbl, val string, size int) {
	printFormControlHead(P)
	printFormLabel(P, sid, lbl)
	printFormInput(P, sid, val, size)
	printFormControlFoot(P)
}
func printFormControlTextarea(P PrintFunc, sid, lbl, val string, rows int) {
	printFormControlHead(P)
	printFormLabel(P, sid, lbl)
	printFormTextarea(P, sid, val, rows)
	printFormControlFoot(P)
}
func printFormControlButton(P PrintFunc, sid, lbl, stype string) {
	printFormControlHead(P)
	printFormButton(P, sid, lbl, stype)
	printFormControlFoot(P)
}
func printFormControlSubmitButton(P PrintFunc, sid, lbl string) {
	printFormControlButton(P, sid, lbl, "submit")
}

//*** Other html template functions ***
func printHead(P PrintFunc, jsurls []string, cssurls []string, title string) {
	P("<!DOCTYPE html>\n")
	P("<html>\n")
	P("<head>\n")
	P("<meta charset=\"utf-8\">\n")
	P("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">\n")
	P("<title>%s</title>\n", title)
	P("<link rel=\"stylesheet\" type=\"text/css\" href=\"/static/style.css\">\n")
	for _, cssurl := range cssurls {
		P("<link rel=\"stylesheet\" type=\"text/css\" href=\"%s\">\n", cssurl)
	}
	for _, jsurl := range jsurls {
		P("<script src=\"%s\" defer></script>\n", jsurl)
	}
	P("</head>\n")
	P("<body class=\"text-black bg-white text-sm\">\n")
	P("  <section class=\"flex flex-row py-4 mx-auto\">\n")
}
func printFoot(P PrintFunc) {
	P("  </section>\n")
	P("</body>\n")
	P("</html>\n")
}
func printSidebar(P PrintFunc) {
	P("<section class=\"col-sidebar flex flex-col text-xs px-8\">\n")
	printContentDiv(P, _loremipsum)
	P("</section>\n")
}
func printMainHead(P PrintFunc) {
	P("<section class=\"col-content flex-grow flex flex-col px-8\">\n")
}
func printMainFoot(P PrintFunc) {
	P("</section>\n")
}
func printContentDiv(P PrintFunc, markup string) {
	// Print html markup wrapped in a <div class="content"> container.
	P("<div class=\"content\">\n")
	P(markup)
	P("</div>\n")
}

func printPageNav(P PrintFunc, site *Site, pageTitle string) {
	if site == nil {
		return
	}

	P("<nav class=\"flex flex-row justify-between border-b border-gray-500 pb-1 mb-4\">\n")
	P("  <div>\n")
	P("    <span class=\"font-bold mr-1\"><a href=\"/?siteid=%d\">%s</a></span>\n", site.Siteid, site.Sitename)
	if pageTitle != "" {
		P(" &gt; <span class=\"ml-1\">%s</span>\n", pageTitle)
	}
	P("  </div>\n")

	if pageTitle != "" {
		P("  <div>\n")
		P("    <a class=\"inline italic text-xs no-underline self-center text-blue-900\" href=\"#\">%s</a>\n", pageTitle)
		P("  </div>\n")
	}
	P("</nav>\n")
}

func indexHandler(db *sql.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		login := getLoginUser(r, db)
		qsiteid := idtoi(r.FormValue("siteid"))
		qtitle := r.FormValue("title")

		var site *Site
		var p *Page
		site = querySiteById(db, qsiteid)
		if site != nil {
			p = queryPageByTitle(db, qsiteid, qtitle)
		}
		p = p

		w.Header().Set("Content-Type", "text/html")
		P := makePrintFunc(w)
		printHead(P, nil, nil, "t2")

		printSectionMenu(P, db, site, p, qtitle, login)
		printMain(P, db, site, p, qtitle, login)

		printSidebar(P)
		printFoot(P)
	}
}

func printSectionMenu(P PrintFunc, db *sql.DB, site *Site, p *Page, qtitle string, login *User) {
	printSectionMenuHead(P, "t2 portal", login)
	defer printSectionMenuFoot(P)

	if site == nil {
		printSitesMenu(P, db)

		printMenuHead(P, "Actions")
		printMenuLine(P, "/createsite/", "Create Site")
		printMenuFoot(P)
		return
	}

	if qtitle == "" {
		printMenuHead(P, "Actions")
		printMenuLine(P, fmt.Sprintf("/editsite?siteid=%d", site.Siteid), "Site Settings")
		printMenuLine(P, fmt.Sprintf("/createpage?siteid=%d", site.Siteid), "Create Page")
		printMenuFoot(P)
		return
	}

	if p == nil {
		printMenuHead(P, "Actions")
		printMenuLine(P, fmt.Sprintf("/editsite?siteid=%d", site.Siteid), "Site Settings")
		href := fmt.Sprintf("/createpage?siteid=%d&title=%s", site.Siteid, escape(qtitle))
		link := fmt.Sprintf("Create page '%s'", qtitle)
		printMenuLine(P, href, link)
		printMenuFoot(P)
		return
	}

	printMenuHead(P, "Actions")
	printMenuLine(P, fmt.Sprintf("/editsite?siteid=%d", site.Siteid), "Site Settings")
	printMenuLine(P, fmt.Sprintf("/createpage?siteid=%d", site.Siteid), "Create Page")
	href := fmt.Sprintf("/editpage?siteid=%d&pageid=%d", site.Siteid, p.Pageid)
	link := fmt.Sprintf("Edit Page '%s'", p.Title)
	printMenuLine(P, href, link)
	printMenuFoot(P)
}

func printMain(P PrintFunc, db *sql.DB, site *Site, p *Page, qtitle string, login *User) {
	printMainHead(P)
	defer printMainFoot(P)

	if site == nil {
		P("<nav class=\"flex flex-row justify-between border-b border-gray-500 pb-1 mb-4\">\n")
		P("  <span class=\"font-bold mr-1\">t2 portal start</span>\n")
		P("</nav>\n")
		return
	}

	printPageNav(P, site, qtitle)

	if qtitle == "" {
		printContentDiv(P, parseMarkdown(site.Desc))
		return
	}

	if p == nil {
		printContentDiv(P, "<p class=\"italic\">(Page not found)</p>\n")
		return
	}

	printContentDiv(P, parseMarkdown(p.Body))
}

func printSitesMenu(P PrintFunc, db *sql.DB) {
	printMenuHead(P, "Select Site")
	s := "SELECT site_id, sitename, desc FROM site ORDER BY site_id"
	rows, err := db.Query(s)
	if err != nil {
		log.Printf("printSitesMenu() db err (%s)\n", err)
		return
	}
	var site Site
	for rows.Next() {
		rows.Scan(&site.Siteid, &site.Sitename, &site.Desc)
		href := fmt.Sprintf("/?siteid=%d", site.Siteid)
		printMenuLine(P, href, site.Sitename)
	}
	printMenuFoot(P)
}

func createsiteHandler(db *sql.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var errmsg string
		var site Site

		login := getLoginUser(r, db)
		if !validateLogin(w, login) {
			return
		}

		if r.Method == "POST" {
			site.Sitename = strings.TrimSpace(r.FormValue("sitename"))
			site.Desc = strings.TrimSpace(r.FormValue("desc"))
			for {
				if site.Sitename == "" {
					errmsg = "Please enter a site name."
					break
				}
				_, err := createSite(db, &site)
				if err != nil {
					log.Printf("Error creating site (%s)\n", err)
					errmsg = "A problem occured. Please try again."
					break
				}
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
		}

		w.Header().Set("Content-Type", "text/html")
		P := makePrintFunc(w)
		printHead(P, nil, nil, "Create Site")

		printSectionMenuHead(P, "Site name here", login)
		printSectionMenuFoot(P)

		printMainHead(P)
		printFormHead(P, "/createsite/")
		printFormTitle(P, "Create site")
		printFormControlError(P, errmsg)
		printFormControlInput(P, "sitename", "Sitename (enter a unique site name)", site.Sitename, 10)
		printFormControlTextarea(P, "desc", "Description", site.Desc, 10)
		printFormControlSubmitButton(P, "create", "Create")
		printFormFoot(P)
		printMainFoot(P)

		printSidebar(P)

		printFoot(P)
	}
}

func editsiteHandler(db *sql.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var errmsg string
		login := getLoginUser(r, db)
		if !validateLogin(w, login) {
			return
		}

		qsiteid := idtoi(r.FormValue("siteid"))
		site := querySiteById(db, qsiteid)
		if site == nil {
			http.Error(w, fmt.Sprintf("siteid %d not found.", qsiteid), 404)
			return
		}

		if r.Method == "POST" {
			site.Sitename = strings.TrimSpace(r.FormValue("sitename"))
			site.Desc = strings.TrimSpace(r.FormValue("desc"))
			for {
				if site.Sitename == "" {
					errmsg = "Please enter a site name."
					break
				}

				s := "UPDATE site SET sitename = ?, desc = ? WHERE site_id = ?"
				_, err := sqlexec(db, s, site.Sitename, site.Desc, qsiteid)
				if err != nil {
					log.Printf("Error updating site (%s)\n", err)
					errmsg = "A problem occured. Please try again."
					break
				}
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
		}

		w.Header().Set("Content-Type", "text/html")
		P := makePrintFunc(w)
		printHead(P, nil, nil, "Edit Site")

		printSectionMenuHead(P, "Site name here", login)
		printSectionMenuFoot(P)

		printMainHead(P)
		printFormHead(P, fmt.Sprintf("/editsite/?siteid=%d", qsiteid))
		printFormTitle(P, "Edit site")
		printFormControlError(P, errmsg)
		printFormControlInput(P, "sitename", "Sitename (unique sitename required)", site.Sitename, 60)
		printFormControlTextarea(P, "desc", "Description", site.Desc, 10)
		printFormControlSubmitButton(P, "update", "Update")
		printFormFoot(P)
		printMainFoot(P)

		printSidebar(P)

		printFoot(P)
	}
}

func createpageHandler(db *sql.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var errmsg string
		var p Page

		login := getLoginUser(r, db)
		if !validateLogin(w, login) {
			return
		}

		qsiteid := idtoi(r.FormValue("siteid"))
		site := querySiteById(db, qsiteid)
		if site == nil {
			http.Error(w, fmt.Sprintf("siteid %d not found.", qsiteid), 404)
			return
		}

		p.Title = strings.TrimSpace(r.FormValue("title"))
		if r.Method == "POST" {
			p.Body = r.FormValue("body")
			for {
				if p.Title == "" {
					errmsg = "Please enter a page title."
					break
				}
				_, err := createPage(db, site, &p)
				if err != nil {
					log.Printf("Error creating page (%s)\n", err)
					errmsg = "A problem occured. Please try again."
					break
				}
				http.Redirect(w, r, fmt.Sprintf("/?siteid=%d&title=%s", qsiteid, escape(p.Title)), http.StatusSeeOther)
				return
			}
		}

		w.Header().Set("Content-Type", "text/html")
		P := makePrintFunc(w)
		printHead(P, nil, nil, "Create Page")

		printSectionMenuHead(P, "Site name here", login)
		printSectionMenuFoot(P)

		printMainHead(P)
		printFormHead(P, fmt.Sprintf("/createpage/?siteid=%d", qsiteid))
		printFormTitle(P, "Create Page")
		printFormControlError(P, errmsg)
		printFormControlInput(P, "title", "Title", p.Title, 10)
		printFormControlTextarea(P, "body", "Body", p.Body, 25)
		printFormControlSubmitButton(P, "create", "Create")
		printFormFoot(P)
		printMainFoot(P)

		printSidebar(P)

		printFoot(P)
	}
}

func editpageHandler(db *sql.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var errmsg string

		login := getLoginUser(r, db)
		if !validateLogin(w, login) {
			return
		}

		qsiteid := idtoi(r.FormValue("siteid"))
		qpageid := idtoi(r.FormValue("pageid"))
		site := querySiteById(db, qsiteid)
		if site == nil {
			http.Error(w, fmt.Sprintf("siteid %d not found.", qsiteid), 404)
			return
		}
		p := queryPageById(db, qsiteid, qpageid)
		if p == nil {
			http.Error(w, fmt.Sprintf("pageid %d not found.", qpageid), 404)
			return
		}

		if r.Method == "POST" {
			p.Title = strings.TrimSpace(r.FormValue("title"))
			p.Body = r.FormValue("body")
			for {
				if p.Title == "" {
					errmsg = "Please enter a page title."
					break
				}

				s := fmt.Sprintf("UPDATE %s SET title = ?, body = ? WHERE page_id = ?", pagetblName(qsiteid))
				_, err := sqlexec(db, s, p.Title, p.Body, qpageid)
				if err != nil {
					log.Printf("Error updating page (%s)\n", err)
					errmsg = "A problem occured. Please try again."
					break
				}
				http.Redirect(w, r, fmt.Sprintf("/?siteid=%d&title=%s", qsiteid, escape(p.Title)), http.StatusSeeOther)
				return
			}
		}

		w.Header().Set("Content-Type", "text/html")
		P := makePrintFunc(w)
		printHead(P, nil, nil, "Edit Page")

		printSectionMenuHead(P, "Site name here", login)
		printSectionMenuFoot(P)

		printMainHead(P)
		printFormHead(P, fmt.Sprintf("/editpage/?siteid=%d&pageid=%d", qsiteid, qpageid))
		printFormTitle(P, "Edit Page")
		printFormControlError(P, errmsg)
		printFormControlInput(P, "title", "Title", p.Title, 10)
		printFormControlTextarea(P, "body", "Body", p.Body, 25)
		printFormControlSubmitButton(P, "update", "Update")
		printFormFoot(P)
		printMainFoot(P)

		printSidebar(P)

		printFoot(P)
	}
}
