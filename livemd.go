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
	"text/template"
)

var (
	htmlBuffer []byte
	target     string
	sockets    []*websocket.Conn
)

func updateBuffer() {
	var mdBuffer []byte
	var err error
	if mdBuffer, err = ioutil.ReadFile(target); err != nil {
		log.Fatal("Error reading from ", target, ": ", err)
	}

	htmlBuffer = blackfriday.Run(mdBuffer)

	for _, c := range sockets {
		err = c.WriteMessage(websocket.TextMessage, htmlBuffer)
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
	c.WriteMessage(websocket.TextMessage, htmlBuffer)
}

func main() {
	if len(os.Args)-1 != 1 {
		fmt.Printf("livemd\n\n")
		fmt.Printf("Usage:\n")
		fmt.Printf("\t%s <file>\n", os.Args[0])
		return
	}

	var err error
	target, err = filepath.Abs(os.Args[1])
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

	log.Print("Watching \"", os.Args[1], "\" for changes")

	err = watcher.Add(".")
	if err != nil {
		log.Fatal(err)
	}

	tmpl := template.Must(template.ParseFiles("templates/view.html"))

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data := struct {
			Rendered string
			WsUrl    string
		}{
			string(htmlBuffer),
			"ws://" + r.Host + "/update",
		}
		tmpl.Execute(w, data)
	})

	http.HandleFunc("/update", registerUpdate)

	log.Print("Serving on port 8081")
	log.Fatal(http.ListenAndServe(":8081", nil))
}
