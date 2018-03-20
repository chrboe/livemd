# livemd

## 📝 A live-updating Markdown viewer

This is a small tool that allows you to instantly see changes you make to a
Markdown document. It is written in [Go](https://www.golang.org) and uses the
[Blackfriday](https://github.com/russross/blackfriday) library for parsing the
Markdown.

## Installation

```
$ go get -u github.com/chrboe/livemd
$ cd $GOPATH/src/github.com/chrboe/livemd
$ go build
```

(Sorry, no binaries yet)

## Usage

```
$ ./livemd somefile.md
```

livemd will start a webserver locally and deploy the rendered Markdown view there.
To view your document, navigate to http://localhost:8081 in your favorite browser.
The site will automatically update whenever you save the Markdown file.
