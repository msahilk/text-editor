package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"text-editor/crdt"

	"github.com/gorilla/websocket"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/writer"
)

// Flags represents the available command-line options for the editor's client.
type Flags struct {
	Server string
	Login  bool
	File   string
	Debug  bool
	Scroll bool
}

// parseFlags retrieves and processes the command-line arguments.
func parseFlags() Flags {
	serverAddr := flag.String("server", "localhost:8080", "The network address of the server")
	enableDebug := flag.Bool("debug", false, "Enable debugging mode to show more verbose logs")
	enableLogin := flag.Bool("login", false, "Enable the login prompt for the server")
	file := flag.String("file", "", "The file to load the editor content from")
	enableScroll := flag.Bool("scroll", true, "Enable scrolling with the cursor")

	flag.Parse()

	return Flags{
		Server: *serverAddr,
		Debug:  *enableDebug,
		Login:  *enableLogin,
		File:   *file,
		Scroll: *enableScroll,
	}
}

// createConn sets up a WebSocket connection using the provided flags.
func createConn(flags Flags) (*websocket.Conn, *http.Response, error) {
	var u url.URL

	u = url.URL{Scheme: "ws", Host: flags.Server, Path: "/"}

	// Set up the WebSocket connection.
	dialer := websocket.Dialer{
		HandshakeTimeout: 2 * time.Minute,
	}

	return dialer.Dial(u.String(), nil)
}

// ensureDirExists checks if a directory exists, creating it if it doesn't.
func ensureDirExists(path string) (bool, error) {
	// Check if the directory exists
	if _, err := os.Stat(path); err == nil {
		return true, nil
	}

	// Create the directory
	err := os.Mkdir(path, 0700)
	if err != nil {
		return false, err
	}

	return true, nil
}

// setupLogger configures the logging system for the client using logrus.
func setupLogger(logger *logrus.Logger) (*os.File, *os.File, error) {
	// Define log file paths relative to the home directory.
	logPath := "editor.log"
	debugLogPath := "editor-debug.log"

	// Get the home directory.
	homeDirExists := true
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDirExists = false
	}

	editorDir := filepath.Join(homeDir, ".edito")

	dirExists, err := ensureDirExists(editorDir)
	if err != nil {
		return nil, nil, err
	}

	// Update log paths if home directory is available.
	if dirExists && homeDirExists {
		logPath = filepath.Join(editorDir, "editor.log")
		debugLogPath = filepath.Join(editorDir, "editor-debug.log")
	}

	// Open or create the main log file.
	logFile, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) // skipcq: GSC-G302
	if err != nil {
		fmt.Printf("Logger error, exiting: %s", err)
		return nil, nil, err
	}

	// Open or create a separate file for detailed logs.
	debugLogFile, err := os.OpenFile(debugLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644) // skipcq: GSC-G302
	if err != nil {
		fmt.Printf("Logger error, exiting: %s", err)
		return nil, nil, err
	}

	logger.SetOutput(io.Discard)
	logger.SetFormatter(&logrus.JSONFormatter{})
	logger.AddHook(&writer.Hook{
		Writer: logFile,
		LogLevels: []logrus.Level{
			logrus.WarnLevel,
			logrus.ErrorLevel,
			logrus.FatalLevel,
			logrus.PanicLevel,
		},
	})
	logger.AddHook(&writer.Hook{
		Writer: debugLogFile,
		LogLevels: []logrus.Level{
			logrus.TraceLevel,
			logrus.DebugLevel,
			logrus.InfoLevel,
		},
	})

	return logFile, debugLogFile, nil
}

// closeLogFiles closes the log files opened by the client.
// This function is intended to be used with defer statements.
func closeLogFiles(logFile, debugLogFile *os.File) {
	if err := logFile.Close(); err != nil {
		fmt.Printf("Failed to close log file: %s", err)
		return
	}

	if err := debugLogFile.Close(); err != nil {
		fmt.Printf("Failed to close debug log file: %s", err)
		return
	}
}

// printDoc outputs the current document state for debugging purposes.
func printDoc(doc crdt.Document) {
	if flags.Debug {
		logger.Infof("---DOCUMENT STATE---")
		for i, c := range doc.Characters {
			logger.Infof("index: %v  value: %s  ID: %v  IDPrev: %v  IDNext: %v  ", i, c.Value, c.ID, c.IDPrevious, c.IDNext)
		}
	}
}
