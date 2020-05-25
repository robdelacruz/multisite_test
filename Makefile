all: t2 static/style.css

dep:
	sudo apt update
	sudo apt install curl software-properties-common
	curl -sL https://deb.nodesource.com/setup_13.x | sudo bash -
	sudo apt install nodejs
	sudo npm install -g npx
	go get github.com/mattn/go-sqlite3
	go get golang.org/x/crypto/bcrypt
	go get gopkg.in/russross/blackfriday.v2

webtools:
	npm install tailwindcss
	npm install postcss-cli
	npm install cssnano --save-dev

static/style.css: twsrc.css
	#npx tailwind build twsrc.css -o twsrc.o 1>/dev/null
	#npx postcss twsrc.o > static/style.css
	npx tailwind build twsrc.css -o static/style.css 1>/dev/null

t2: t2.go
	go build -o t2 t2.go

clean:
	rm -rf t2 *.o static/style.css

