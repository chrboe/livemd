//
// livemd.go
// Copyright (C) 2018 Christoph Böhmwalder <christoph@boehmwalder.at>
//
// Distributed under terms of the GPLv3 license.
//

package main

import (
	"fmt"
	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
	"gopkg.in/russross/blackfriday.v2"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"text/template"
	"github.com/rakyll/statik/fs"
	_ "github.com/chrboe/livemd/statik"
	"flag"
	"github.com/skratchdot/open-golang/open"
	"net"
)

var (
	messageBuf htmlMessage
	target     string
	sockets    []*websocket.Conn
	titleRegex *regexp.Regexp = regexp.MustCompile(`(?s)^\s*<h1>(.*)</h1>.*$`)
	pwd string
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
	return blackfriday.Run(markdown, blackfriday.WithNoExtensions())
}

func updateBuffer() {
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

func setupWatch() (*fsnotify.Watcher, error) {
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
						updateBuffer()
					}
				}
			case err := <-watcher.Errors:
				log.Println("error:", err)
			}
		}
	}()

	return watcher, nil
}

var upgrader = websocket.Upgrader{}

func registerUpdate(w http.ResponseWriter, r *http.Request) {
	c, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}

	sockets = append(sockets, c)
	c.WriteJSON(messageBuf)
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "livemd\n\n")
		fmt.Fprintf(os.Stderr, "Usage:\n")
		fmt.Fprintf(os.Stderr, "\t%s [-b] <file>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nOptions:\n")
		fmt.Fprintf(os.Stderr, "\t-b\tOpen a browser window with the markdown document\n")
	}

	openBrowser := flag.Bool("b", false, "")

	flag.Parse()

	if len(flag.Args()) != 1 {
		flag.Usage()
		return
	}

	var err error

	statikFS, err := fs.New()
	if err != nil {
		log.Fatal(err)
	}

	relTarget := flag.Args()[0]
	target, err = filepath.Abs(relTarget)
	if err != nil {
		log.Fatal(err)
		return
	}

	updateBuffer()

	watcher, err := setupWatch()
	if err != nil {
		log.Fatal(err)
		return
	}
	defer watcher.Close()

	log.Print("Watching \"", relTarget, "\" for changes")

	err = watcher.Add(".")
	if err != nil {
		log.Fatal(err)
	}

	tmplFile, err := statikFS.Open("/view.html")
	if err != nil {
		panic(err)
	}

	tmplBin, err := ioutil.ReadAll(tmplFile)
	if err != nil {
		panic(err)
	}

	tmpl, err := template.New("view").Parse(string(tmplBin))
	if err != nil {
		panic(err)
	}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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
	})

	http.HandleFunc("/update", registerUpdate)

	log.Print("Serving on port 8081")

	l, err := net.Listen("tcp", "localhost:8081")
	if err != nil {
		log.Fatal(err)
	}

	if *openBrowser {
		open.Start("http://localhost:8081")
	}

	log.Fatal(http.Serve(l, nil))
}
