package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"text-editor/client/editor"
	"text-editor/commons"
	"text-editor/crdt"

	"github.com/Pallinder/go-randomdata"
	"github.com/sirupsen/logrus"
)

var (
	// doc stores the local content of the document
	doc = crdt.New()

	// logger is the centralized logging system
	logger = logrus.New()

	// e is the termbox-based text editor
	e = editor.NewEditor(editor.EditorConfig{})

	// fileName specifies the file for loading and saving
	fileName string

	// flags contain the parsed command-line arguments
	flags Flags
)

func main() {
	// Initialize flags from command-line arguments
	flags = parseFlags()

	s := bufio.NewScanner(os.Stdin)

	// Generate a random username for the user
	name := randomdata.SillyName()

	// If login is enabled, prompt for a custom username
	if flags.Login {
		fmt.Print("Enter your name: ")
		s.Scan()
		name = s.Text()
	}

	conn, _, err := createConn(flags)
	if err != nil {
		fmt.Printf("Connection error, exiting: %s\n", err)
		return
	}
	defer conn.Close()

	// Notify other users about the new participant
	msg := commons.Message{Username: name, Text: "has joined the session.", Type: commons.JoinMessage}
	_ = conn.WriteJSON(msg)

	logFile, debugLogFile, err := setupLogger(logger)
	if err != nil {
		fmt.Printf("Failed to setup logger, exiting: %s\n", err)
		return
	}
	defer closeLogFiles(logFile, debugLogFile)

	if flags.File != "" {
		if doc, err = crdt.Load(flags.File); err != nil {
			fmt.Printf("failed to load document: %s\n", err)
			return
		}
	}

	uiConfig := UIConfig{
		EditorConfig: editor.EditorConfig{
			ScrollEnabled: flags.Scroll,
		},
	}

	err = initUI(conn, uiConfig)
	if err != nil {

		// Check if the error is related to editor events (e.g., exiting)
		if strings.HasPrefix(err.Error(), "editor") {
			fmt.Println("exiting session.")
			return
		}

		// Display error message for actual errors
		fmt.Printf("TUI error, exiting: %s\n", err)
		return
	}
}
