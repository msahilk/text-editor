package main

import (
	"flag"
	"github.com/gorilla/websocket"
	"net/http"
	"net/url"
	"os"
	"text-editor/crdt"
	"time"
)

type Flags struct {
	Server string
	Login  bool
	File   string
	Debug  bool
	Scroll bool
}

func parseFlags() Flags {
	serverAddress := flag.String("server", "localhost:8080", "server address")
	enableDebug := flag.Bool("debug", false, "enable debug mode")
	enableLogin := flag.Bool("login", false, "enable login mode")
	file := flag.String("file", "", "file path to load")
	enableScroll := flag.Bool("scroll", false, "enable scroll mode")

	flag.Parse()

	return Flags{
		Server: *serverAddress,
		Login:  *enableLogin,
		File:   *file,
		Debug:  *enableDebug,
		Scroll: *enableScroll,
	}

}

func Connect(flags Flags) (*websocket.Conn, *http.Response, error) {

	var _url url.URL

	_url = url.URL{Scheme: "ws", Host: flags.Server, Path: "/"}

	dialer := websocket.Dialer{
		HandshakeTimeout: 30 * time.Second,
	}

	return dialer.Dial(_url.String(), nil)
}

func checkDirExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}

	err = os.Mkdir(path, 0700)
	if err != nil {
		return false, err
	}
	return true, nil
}

func printDoc(doc crdt.Document) {
}
