//
// livemd.go
// Copyright (C) 2018 Christoph Böhmwalder <christoph@boehmwalder.at>
//
// Distributed under terms of the GPLv3 license.
//

package main

import (
	"flag"
	"fmt"
	_ "github.com/chrboe/livemd/statik"
	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
	"github.com/rakyll/statik/fs"
	"github.com/skratchdot/open-golang/open"
	"gopkg.in/russross/blackfriday.v2"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"text/template"
	"github.com/microcosm-cc/bluemonday"
)

var (
	messageBuf htmlMessage
	sockets    []*websocket.Conn
	titleRegex *regexp.Regexp = regexp.MustCompile(`(?s)^\s*<h1>(.*)</h1>.*$`)
	pwd        string
	upgrader   = websocket.Upgrader{}
)

const (
	templatePath = "/view.html"
)

type htmlMessage struct {
	Title string
	Html  string
}

func guessTitle(htmlBuffer string) string {
	// guess the title based on a dumb heuristic: is the first tag a <h1> tag?
	match := titleRegex.FindStringSubmatch(htmlBuffer)

	if match != nil {
		return match[1]
	}

	return "livemd"
}

func renderMarkdown(markdown []byte) []byte {
	unsafe := blackfriday.Run(markdown, blackfriday.WithNoExtensions())
	html := bluemonday.UGCPolicy().SanitizeBytes(unsafe)
	return html
}

func updateBuffer(target string) {
	mdBuffer, err := ioutil.ReadFile(target)

	if err != nil {
		log.Fatal("Error reading from ", target, ": ", err)
	}

	messageBuf.Html = string(renderMarkdown(mdBuffer))
	messageBuf.Title = guessTitle(messageBuf.Html)

	for _, c := range sockets {
		err = c.WriteJSON(&messageBuf)
		if err != nil {
			log.Println("Error", err)
		}
	}
}

func setupWatch(target string) (*fsnotify.Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return watcher, err
	}

	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if event.Op&fsnotify.Write == fsnotify.Write {
					abs, err := filepath.Abs(event.Name)
					if err != nil {
						log.Fatal("Error getting absolute path of ", event.Name, ": ", err)
					}

					if abs == target {
						updateBuffer(target)
					}
				}
			case err := <-watcher.Errors:
				log.Println("error:", err)
			}
		}
	}()

	err = watcher.Add(".")
	if err != nil {
		log.Fatal(err)
	}

	return watcher, nil
}

func registerUpdate(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}

	sockets = append(sockets, c)
	c.WriteJSON(messageBuf)
}

func loadTemplate(path string) (*template.Template, error) {
	// load statikFS
	sfs, err := fs.New()
	if err != nil {
		return nil, err
	}

	tmplFile, err := sfs.Open(path)
	if err != nil {
		return nil, err
	}

	tmplBin, err := ioutil.ReadAll(tmplFile)
	if err != nil {
		return nil, err
	}

	tmpl, err := template.New("view").Parse(string(tmplBin))
	if err != nil {
		return nil, err
	}

	return tmpl, nil
}

func handleHttpRequest(w http.ResponseWriter, r *http.Request, tmpl *template.Template) {
	data := struct {
		Title    string
		Rendered string
		WsUrl    string
	}{
		messageBuf.Title,
		string(messageBuf.Html),
		"ws://" + r.Host + "/update",
	}

	tmpl.Execute(w, data)
}

// print usage information
func usage() {
	fmt.Fprintf(os.Stderr, "livemd\n\n")
	fmt.Fprintf(os.Stderr, "Usage:\n")
	fmt.Fprintf(os.Stderr, "\t%s [-b] <file>\n", os.Args[0])
	fmt.Fprintf(os.Stderr, "\nOptions:\n")
	fmt.Fprintf(os.Stderr, "\t-b\tOpen a browser window with the markdown document\n")
	fmt.Fprintf(os.Stderr, "\t-p\tWhich port to start the webserver on (default: 8081)\n")
}

func main() {
	flag.Usage = usage

	// define flags
	openBrowser := flag.Bool("b", false, "")
	port := flag.Int("p", 8081, "")

	// parse flags
	flag.Parse()

	// check if there is exactly one trailing argument (filename)
	if len(flag.Args()) != 1 {
		flag.Usage()
		return
	}

	// get target filename
	relTarget := flag.Args()[0]

	target, err := filepath.Abs(relTarget)
	if err != nil {
		log.Fatal(err)
		return
	}

	// convert the document first
	updateBuffer(target)

	// setup the file watch
	watcher, err := setupWatch(target)
	if err != nil {
		log.Fatal(err)
		return
	}
	defer watcher.Close()

	log.Print("Watching \"", relTarget, "\" for changes")

	// load the template
	tmpl, err := loadTemplate(templatePath)
	if err != nil {
		log.Fatal(err)
		return
	}

	// register http handlers
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		handleHttpRequest(w, r, tmpl)
	})

	http.HandleFunc("/update", registerUpdate)

	// listen (, open browser,) and serve
	log.Printf("Serving on port %d\n", *port)

	l, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatal(err)
	}

	if *openBrowser {
		open.Start(fmt.Sprintf("http://localhost:%d", *port))
	}

	log.Fatal(http.Serve(l, nil))
}
