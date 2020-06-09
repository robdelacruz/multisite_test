package main

import (
	"database/sql"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type PrintFunc func(format string, a ...interface{}) (n int, err error)

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

	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("./static"))))
	http.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) { http.ServeFile(w, r, "./static/coffee.ico") })
	http.HandleFunc("/", indexHandler(db))

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

// Helper function to make fmt.Fprintf(w, ...) calls shorter.
// Ex.
// Replace:
//   fmt.Fprintf(w, "<p>Some text %s.</p>", str)
//   fmt.Fprintf(w, "<p>Some other text %s.</p>", str)
// with the shorter version:
//   P := makeFprintf(w)
//   P("<p>Some text %s.</p>", str)
//   P("<p>Some other text %s.</p>", str)
func makePrintFunc(w io.Writer) func(format string, a ...interface{}) (n int, err error) {
	return func(format string, a ...interface{}) (n int, err error) {
		return fmt.Fprintf(w, format, a...)
	}
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
		"CREATE TABLE page (page_id INTEGER PRIMARY KEY NOT NULL, book_id INTEGER NOT NULL, body TEXT)",
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

func printMenuCol(P PrintFunc) {
	P("  <section class=\"col-menu flex-shrink flex flex-col text-xs px-4\">\n")
	P("    <div class=\"flex flex-col mb-4\">\n")
	P("      <h1 class=\"text-lg text-bold\">Site Name here</h1>\n")
	P("      <div class=\"\">\n")
	P("        <a class=\"text-gray-800 bg-gray-400 rounded px-2 mr-1\" href=\"#\">robdelacruz</a>\n")
	P("        <a class=\"text-blue-900\" href=\"#\">logout</a>\n")
	P("      </div>\n")
	P("    </div>\n")
	P("    <ul class=\"list-none mb-2\">\n")
	P("      <li><p class=\"border-b mb-1\">Content</p></li>\n")
	P("      <li><a class=\"text-blue-900\" href=\"#\">Create page</a></li>\n")
	P("      <li><a class=\"text-blue-900\" href=\"#\">Upload file</a></li>\n")
	P("    </ul>\n")
	P("  </section>\n")
}

func printSidebarCol(P PrintFunc) {
	P("  <section class=\"col-sidebar flex-shrink flex flex-col text-xs px-8 page\">\n")
	P("    <p>Lorem ipsum dolor sit amet, consectetur adipiscing elit. Etiam mattis volutpat libero a sodales. Sed a sagittis est. Sed eros nunc, maximus id lectus nec, tempor tincidunt felis. Cras viverra arcu ut tellus sagittis, et pharetra arcu ornare. Cras euismod turpis id auctor posuere. Nunc euismod molestie est, nec congue velit vestibulum rutrum. Etiam vitae consectetur mauris.</p>\n")
	P("    <p>Etiam sodales neque sit amet erat ullamcorper placerat. Curabitur sit amet sapien ac sem convallis efficitur. Class aptent taciti sociosqu ad litora torquent per conubia nostra, per inceptos himenaeos. Cras maximus felis dolor, ac ultricies mauris varius scelerisque. Proin vitae velit a odio eleifend tristique sit amet vitae risus. Curabitur varius sapien ut viverra suscipit. Integer suscipit lectus vel velit rhoncus, eget condimentum neque imperdiet. Morbi dapibus condimentum convallis. Suspendisse potenti. Aenean fermentum nisi mauris, rhoncus malesuada enim semper semper.</p>\n")
	//	P("    <p>Suspendisse sit amet molestie nisl, id egestas nulla. Pellentesque eget orci consequat, fermentum lorem eget, condimentum nibh. Vivamus sit amet odio maximus, lobortis nulla vel, luctus nisi. In commodo vel risus id auctor. Duis vulputate euismod mauris a congue. Pellentesque semper dolor id metus eleifend tempus. Ut maximus, nisl in convallis consequat, metus urna tempor mi, non laoreet orci magna sed eros. Etiam sed felis facilisis, fermentum diam fermentum, mattis nunc. Aliquam sed volutpat mauris. Cras pellentesque aliquam nisl non vulputate. Vestibulum tempor quam velit. Curabitur auctor mattis diam non pretium. Integer sagittis nunc in metus luctus, vel tempus mauris mattis. Sed placerat ligula fringilla libero vestibulum, id lacinia lacus suscipit.</p>\n")
	//	P("    <p>Pellentesque habitant morbi tristique senectus et netus et malesuada fames ac turpis egestas. Vestibulum vitae egestas dui. Etiam euismod et quam sed auctor. Donec vehicula lacus justo, aliquet suscipit tortor tristique sit amet. Etiam nec sem vel orci laoreet ornare id et ipsum. Nulla eu elementum sem. Nam hendrerit ligula diam, nec porttitor dui blandit nec.</p>\n")
	//	P("    <p>Maecenas molestie ante justo, sed consequat dui sagittis id. Praesent quis erat porta, consequat nulla eu, interdum ex. Etiam elit est, facilisis eget urna ac, accumsan tincidunt tellus. Quisque ut malesuada velit. Aliquam at urna ut mauris faucibus sagittis. Donec quis nulla eget ipsum feugiat blandit. Aenean dapibus consectetur faucibus. Nullam egestas sagittis metus. In sollicitudin augue bibendum lacus lacinia, eget rhoncus est lobortis.</p>\n")
	P("  </section>\n")
}

func printNav(P PrintFunc) {
	P("<nav class=\"flex flex-row justify-between border-b border-gray-500 pb-1 mb-4\">\n")
	P("  <div>\n")
	P("    <span class=\"font-bold mr-1\">Rob's Notebook</span> &gt;\n")
	P("    <span class=\"ml-1\">Intro</span>\n")
	P("  </div>\n")
	P("  <div>\n")
	P("    <a class=\"inline italic text-xs link-3 no-underline self-center text-blue-900\" href=\"#\">Intro</a>\n")
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
		w.Header().Set("Content-Type", "text/html")
		P := makePrintFunc(w)
		printHead(P, nil, nil, "Title")
		printMenuCol(P)

		P("<section class=\"col-content flex-grow flex flex-col px-8\">\n")
		printNav(P)
		printPage(P)
		P("</section>\n")

		printSidebarCol(P)
		printFoot(P)
	}
}
