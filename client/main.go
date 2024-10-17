package main

import (
	"bufio"
	"fmt"
	"github.com/Pallinder/go-randomdata"
	"github.com/sirupsen/logrus"
	"os"
	"strings"
	"text-editor/client/editor"
	"text-editor/commons"
	"text-editor/crdt"
)

var (
	// Local document containing content.
	doc = crdt.New()

	logger = logrus.New()

	// termbox-based editor.
	e = editor.NewEditor(editor.Config{})

	// The name of the file to load from and save to.
	fileName string

	// Parsed flags.
	flags Flags
)

func main() {
	flags = parseFlags()

	scan := bufio.NewScanner(os.Stdin)

	name := randomdata.SillyName()

	if flags.Login {
		fmt.Print("Enter your name: ")
		scan.Scan()
		name = scan.Text()
	}

	conn, _, err := Connect(flags)
	if err != nil {
		fmt.Printf("Error connecting to server: %s\n", err)
		return
	}
	defer conn.Close()

	msg := commons.Message{
		Username: name,
		Text:     "has joined.",
		Type:     commons.JoinMessage,
	}
	_ = conn.WriteJSON(msg)

	if flags.File != "" {
		if doc, err = crdt.Load(flags.File); err != nil {
			fmt.Printf("Error loading document: %s\n", err)
			return
		}

	}

	uiConfig := UIConfig{
		editor.Config{ScrollEnabled: flags.Scroll},
	}

	err = initUI(conn, uiConfig)
	if err != nil {
		if strings.HasPrefix(err.Error(), "editor") {
			fmt.Println("Exiting session")
			return
		}
		fmt.Printf("Error initializing UI: %s\n", err)
		return
	}

}
