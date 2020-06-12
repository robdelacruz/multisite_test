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
)

type User struct {
	Userid   int64
	Username string
	Active   bool
	Email    string
}
type Book struct {
	Bookid int64
	Name   string
	Desc   string
}
type Page struct {
	Pageid   int64
	Title    string
	Body     string
	Bookid   int64
	BookName string
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
		"CREATE TABLE user (user_id INTEGER PRIMARY KEY NOT NULL, username TEXT, password TEXT, active INTEGER NOT NULL, email TEXT, CONSTRAINT unique_username UNIQUE (username));",
		"CREATE TABLE book (book_id INTEGER PRIMARY KEY NOT NULL, name TEXT, desc TEXT);",
		"CREATE TABLE page (page_id INTEGER PRIMARY KEY NOT NULL, book_id INTEGER NOT NULL, title TEXT UNIQUE, body TEXT)",
		"INSERT INTO user (user_id, username, password, active, email) VALUES (1, 'admin', '', 1, '');",
		"INSERT INTO book (book_id, name, desc) VALUES (1, 'main', '');",
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

Start webservice using notes database file:
	mn <notes.db>

Initialize new notes database file:
	mn -i <notes.db>
`
		fmt.Printf(s)
		os.Exit(0)
	}

	// Exit if db file doesn't exist.
	dbfile := parms[0]
	if !fileExists(dbfile) {
		s := fmt.Sprintf(`Notes database file '%s' doesn't exist. Create one using:
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
	http.HandleFunc("/createbook/", createbookHandler(db))

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
func queryBookById(db *sql.DB, bookid int64) *Book {
	var b Book
	s := "SELECT book_id, name, desc WHERE book_id = ?"
	row := db.QueryRow(s, bookid)
	err := row.Scan(&b.Bookid, &b.Name, &b.Desc)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		fmt.Printf("queryBookById() db error (%s)\n", err)
		return nil
	}
	return &b
}
func queryPageById(db *sql.DB, pageid int64) *Page {
	var p Page
	s := "SELECT page_id, book_id, title, body, b.name FROM page p INNER JOIN book b ON p.book_id = b.book_id WHERE page_id = ?"
	row := db.QueryRow(s, pageid)
	err := row.Scan(&p.Pageid, &p.Bookid, &p.Title, &p.Body, &p.BookName)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		fmt.Printf("queryPageById() db error (%s)\n", err)
		return nil
	}
	return &p
}
func createBook(db *sql.DB, b *Book) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	s := "INSERT INTO book (name, desc) VALUES (?, ?)"
	result, err := txexec(tx, s, b.Name, b.Desc)
	if handleTxErr(tx, err) {
		return 0, err
	}
	err = tx.Commit()
	if handleTxErr(tx, err) {
		return 0, err
	}
	bookid, err := result.LastInsertId()
	if handleTxErr(tx, err) {
		return 0, err
	}

	err = tx.Commit()
	if handleTxErr(tx, err) {
		return 0, err
	}
	return bookid, nil
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

func parsePageUrl(url string) (string, string) {
	url = strings.Trim(url, "/")
	ss := strings.Split(url, "/")
	sslen := len(ss)
	if sslen == 0 {
		return "", ""
	} else if sslen == 1 {
		return ss[0], ""
	}
	return ss[0], ss[1]
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

func printMenuHead(P PrintFunc) {
	P("  <section class=\"col-menu flex flex-col text-xs px-4\">\n")
	P("    <div class=\"flex flex-col mb-4\">\n")
	P("      <h1 class=\"text-lg text-bold\">Site Name here</h1>\n")
	P("      <div class=\"\">\n")
	P("        <a class=\"text-gray-800 bg-gray-400 rounded px-2 mr-1\" href=\"#\">robdelacruz</a>\n")
	P("        <a class=\"text-blue-900\" href=\"#\">logout</a>\n")
	P("      </div>\n")
	P("    </div>\n")
}
func printMenuFoot(P PrintFunc) {
	P("  </section>\n")
}

func printSidebar(P PrintFunc) {
	P("  <section class=\"col-sidebar flex flex-col text-xs px-8 page\">\n")
	P("    <p>Lorem ipsum dolor sit amet, consectetur adipiscing elit. Etiam mattis volutpat libero a sodales. Sed a sagittis est. Sed eros nunc, maximus id lectus nec, tempor tincidunt felis. Cras viverra arcu ut tellus sagittis, et pharetra arcu ornare. Cras euismod turpis id auctor posuere. Nunc euismod molestie est, nec congue velit vestibulum rutrum. Etiam vitae consectetur mauris.</p>\n")
	P("    <p>Etiam sodales neque sit amet erat ullamcorper placerat. Curabitur sit amet sapien ac sem convallis efficitur. Class aptent taciti sociosqu ad litora torquent per conubia nostra, per inceptos himenaeos. Cras maximus felis dolor, ac ultricies mauris varius scelerisque. Proin vitae velit a odio eleifend tristique sit amet vitae risus. Curabitur varius sapien ut viverra suscipit. Integer suscipit lectus vel velit rhoncus, eget condimentum neque imperdiet. Morbi dapibus condimentum convallis. Suspendisse potenti. Aenean fermentum nisi mauris, rhoncus malesuada enim semper semper.</p>\n")
	//	P("    <p>Suspendisse sit amet molestie nisl, id egestas nulla. Pellentesque eget orci consequat, fermentum lorem eget, condimentum nibh. Vivamus sit amet odio maximus, lobortis nulla vel, luctus nisi. In commodo vel risus id auctor. Duis vulputate euismod mauris a congue. Pellentesque semper dolor id metus eleifend tempus. Ut maximus, nisl in convallis consequat, metus urna tempor mi, non laoreet orci magna sed eros. Etiam sed felis facilisis, fermentum diam fermentum, mattis nunc. Aliquam sed volutpat mauris. Cras pellentesque aliquam nisl non vulputate. Vestibulum tempor quam velit. Curabitur auctor mattis diam non pretium. Integer sagittis nunc in metus luctus, vel tempus mauris mattis. Sed placerat ligula fringilla libero vestibulum, id lacinia lacus suscipit.</p>\n")
	//	P("    <p>Pellentesque habitant morbi tristique senectus et netus et malesuada fames ac turpis egestas. Vestibulum vitae egestas dui. Etiam euismod et quam sed auctor. Donec vehicula lacus justo, aliquet suscipit tortor tristique sit amet. Etiam nec sem vel orci laoreet ornare id et ipsum. Nulla eu elementum sem. Nam hendrerit ligula diam, nec porttitor dui blandit nec.</p>\n")
	//	P("    <p>Maecenas molestie ante justo, sed consequat dui sagittis id. Praesent quis erat porta, consequat nulla eu, interdum ex. Etiam elit est, facilisis eget urna ac, accumsan tincidunt tellus. Quisque ut malesuada velit. Aliquam at urna ut mauris faucibus sagittis. Donec quis nulla eget ipsum feugiat blandit. Aenean dapibus consectetur faucibus. Nullam egestas sagittis metus. In sollicitudin augue bibendum lacus lacinia, eget rhoncus est lobortis.</p>\n")
	P("  </section>\n")
}

func printMainHead(P PrintFunc) {
	P("<section class=\"col-content flex-grow flex flex-col px-8\">\n")
}
func printMainFoot(P PrintFunc) {
	P("</section>\n")
}

func printPageNav(P PrintFunc, bookName, pageTitle string) {
	P("<nav class=\"flex flex-row justify-between border-b border-gray-500 pb-1 mb-4\">\n")
	P("  <div>\n")
	P("    <span class=\"font-bold mr-1\">%s</span> &gt;\n", bookName)
	if pageTitle != "" {
		P("    <span class=\"ml-1\">%s</span>\n", pageTitle)
	}
	P("  </div>\n")
	P("  <div>\n")
	if pageTitle != "" {
		P("    <a class=\"inline italic text-xs link-3 no-underline self-center text-blue-900\" href=\"#\">%s</a>\n", pageTitle)
	}
	P("  </div>\n")
	P("</nav>\n")
}
func printPage(P PrintFunc) {
	P("<article class=\"page\">\n")
	P("  <p>You are the commander of Space Rescue Emergency Vessel III. You have spent almost six months alone in space, and your only companion is your computer, Henry. You are steering your ship through a meteorite shower when an urgent signal comes from headquarters- a ship in your sector is under attack by space pirates!</p>\n")
	P("  <p>You are the commander of the Lacoonian System Rapid Force response team, in charge of protecting all planets in the System. You learn that the Evil Power Master has zeroed in on three planets and plans to destroy them. The safety of the Lacoonian System depends on you!</p>\n")
	P("</article>\n")
}

func indexHandler(db *sql.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		bookName, pageTitle := parsePageUrl(r.URL.Path)

		headTitle := pageTitle
		if headTitle == "" {
			headTitle = bookName
		}

		w.Header().Set("Content-Type", "text/html")
		P := makePrintFunc(w)
		printHead(P, nil, nil, headTitle)

		printMenuHead(P)
		P("    <ul class=\"list-none mb-2\">\n")
		P("      <li><p class=\"border-b mb-1\">Actions</p></li>\n")
		if bookName == "" {
			P("      <li><a class=\"text-blue-900\" href=\"/createbook\">Create new book</a></li>\n")
		} else {
			P("      <li><a class=\"text-blue-900\" href=\"/createpage\">Create new page</a></li>\n")
		}
		if pageTitle != "" {
			P("      <li><a class=\"text-blue-900\" href=\"/editpage\">Edit page</a></li>\n")
		}
		P("    </ul>\n")

		if bookName == "" {
			P("    <ul class=\"list-none mb-2\">\n")
			P("      <li><p class=\"border-b mb-1\">Select Book</p></li>\n")
			P("      <li><a class=\"text-blue-900\" href=\"#\">book 1</a></li>\n")
			P("      <li><a class=\"text-blue-900\" href=\"#\">book 2</a></li>\n")
			P("    </ul>\n")
		} else if pageTitle == "" {
			P("    <ul class=\"list-none mb-2\">\n")
			P("      <li><p class=\"border-b mb-1\">Select Page</p></li>\n")
			P("      <li><a class=\"text-blue-900\" href=\"#\">page 1</a></li>\n")
			P("      <li><a class=\"text-blue-900\" href=\"#\">page 2</a></li>\n")
			P("      <li><a class=\"text-blue-900\" href=\"#\">page 3</a></li>\n")
			P("    </ul>\n")
		}

		printMenuFoot(P)

		printMainHead(P)
		printPageNav(P, bookName, pageTitle)
		printPage(P)
		printMainFoot(P)

		printSidebar(P)

		printFoot(P)
	}
}

func createbookHandler(db *sql.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var errmsg string
		var b Book

		login := getLoginUser(r, db)
		if !validateLogin(w, login) {
			return
		}

		if r.Method == "POST" {
			b.Name = strings.TrimSpace(r.FormValue("name"))
			b.Desc = strings.TrimSpace(r.FormValue("desc"))
			for {
				if b.Name == "" {
					errmsg = "Please enter a book name."
					break
				}
				_, err := createBook(db, &b)
				if err != nil {
					log.Printf("Error creating book (%s)\n", err)
					errmsg = "A problem occured. Please try again."
					break
				}
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
		}

		w.Header().Set("Content-Type", "text/html")
		P := makePrintFunc(w)
		printHead(P, nil, nil, "Create Page")

		printMenuHead(P)
		P("    <ul class=\"list-none mb-2\">\n")
		P("    </ul>\n")
		printMenuFoot(P)

		printMainHead(P)
		printMainFoot(P)

		printSidebar(P)

		printFoot(P)

		errmsg = errmsg
	}
}

func editbookHandler(db *sql.DB) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		bookid := idtoi(r.FormValue("bookid"))

		b := queryBookById(db, bookid)
		if b == nil {
			http.Error(w, fmt.Sprintf("Book %d not found.", bookid), 404)
			return
		}
	}
}
