package main

import (
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/nsf/termbox-go"
	"github.com/sirupsen/logrus"
	"strconv"
	"strings"
	"text-editor/commons"
	"text-editor/crdt"
	"time"
)

// handleTermboxEvent processes terminal key events and triggers appropriate actions.
func handleTermboxEvent(ev termbox.Event, conn *websocket.Conn) error {
	// Only handle key events
	if ev.Type == termbox.EventKey {
		switch ev.Key {

		// Exit on Escape or Ctrl+C
		case termbox.KeyEsc, termbox.KeyCtrlC:
			return errors.New("exiting") // Error returned as an exit event

		// Save document on Ctrl+S
		case termbox.KeyCtrlS:
			if fileName == "" {
				fileName = "editor-content.txt" // Default file name
			}

			// Save CRDT document to file
			err := crdt.Save(fileName, &doc)
			if err != nil {
				logrus.Errorf("Failed to save to %s", fileName)
				e.StatusChan <- fmt.Sprintf("Failed to save to %s", fileName)
				return err
			}

			// Show status message of successful save
			e.StatusChan <- fmt.Sprintf("Saved to %s", fileName)

		// Load document on Ctrl+L
		case termbox.KeyCtrlL:
			if fileName != "" {
				logger.Log(logrus.InfoLevel, "LOADING")
				newDoc, err := crdt.Load(fileName)
				if err != nil {
					logrus.Errorf("Failed to load from %s", fileName)
					e.StatusChan <- fmt.Sprintf("Failed to load from %s", fileName)
					return err
				}

				// Load document and send it over the WebSocket connection
				e.StatusChan <- fmt.Sprintf("Loaded from %s", fileName)
				doc = newDoc
				e.SetX(0) // Reset cursor to start
				e.SetText(crdt.Content(doc))
				docMsg := commons.Message{Type: commons.DocSyncMessage, Document: doc}
				_ = conn.WriteJSON(docMsg)
			} else {
				e.StatusChan <- "No file to load"
			}

		// Move left (arrow or Ctrl+B)
		case termbox.KeyArrowLeft, termbox.KeyCtrlB:
			e.MoveCursor(-1, 0)

		// Move right (arrow or Ctrl+F)
		case termbox.KeyArrowRight, termbox.KeyCtrlF:
			e.MoveCursor(1, 0)

		// Move up (arrow or Ctrl+P)
		case termbox.KeyArrowUp, termbox.KeyCtrlP:
			e.MoveCursor(0, -1)

		// Move down (arrow or Ctrl+N)
		case termbox.KeyArrowDown, termbox.KeyCtrlN:
			e.MoveCursor(0, 1)

		// Move to start of line on Home key
		case termbox.KeyHome:
			e.SetX(0)

		// Move to end of text on End key
		case termbox.KeyEnd:
			e.SetX(len(e.Text))

		// Delete (backspace or delete key)
		case termbox.KeyBackspace, termbox.KeyBackspace2, termbox.KeyDelete:
			performOperation(OperationDelete, ev, conn)

		// Tab inserts 4 spaces
		case termbox.KeyTab:
			for i := 0; i < 4; i++ {
				ev.Ch = ' '
				performOperation(OperationInsert, ev, conn)
			}

		// Enter key inserts newline
		case termbox.KeyEnter:
			ev.Ch = '\n'
			performOperation(OperationInsert, ev, conn)

		// Space key inserts space
		case termbox.KeySpace:
			ev.Ch = ' '
			performOperation(OperationInsert, ev, conn)

		// Insert any other character typed
		default:
			if ev.Ch != 0 {
				performOperation(OperationInsert, ev, conn)
			}
		}
	}

	// Send a signal to redraw the editor
	e.SendDraw()
	return nil
}

const (
	OperationInsert = 1
	OperationDelete = 2
)

// performOperation processes insert/delete operations and updates local CRDT state.
func performOperation(operation int, ev termbox.Event, conn *websocket.Conn) {
	ch := string(ev.Ch) // Convert key event to string character

	var msg commons.Message // Message to be sent over WebSocket

	// Perform insert operation
	switch operation {
	case OperationInsert:
		logger.Infof("LOCAL INSERT: %s at cursor position %v\n", ch, e.Cursor)
		// Insert character at the current cursor position in the local CRDT document
		text, err := doc.Insert(e.Cursor+1, ch)
		if err != nil {
			e.SetText(text)
			logger.Errorf("CRDT error: %v", err)
		}
		e.SetText(text)
		e.MoveCursor(1, 0) // Move cursor right after inserting

		// Create operation message to send over WebSocket
		msg = commons.Message{Type: "operation", Operation: commons.Operation{Type: "insert", Position: e.Cursor, Value: ch}}

	// Perform delete operation
	case OperationDelete:
		logger.Infof("LOCAL DELETE: %s at cursor position %v\n", ch, e.Cursor)

		// Ensure cursor doesn't go out of bounds
		if e.Cursor-1 < 0 {
			e.Cursor = 0
		}

		// Delete character at the current cursor position
		text := doc.Delete(e.Cursor)
		e.SetText(text)

		// Create delete operation message
		msg = commons.Message{Type: "operation", Operation: commons.Operation{Type: "delete", Position: e.Cursor}}
		e.MoveCursor(-1, 0) // Move cursor left after deletion
	}

	// Send message if connected to the server
	if e.IsConnected {
		err := conn.WriteJSON(msg)
		if err != nil {
			e.IsConnected = false
			e.StatusChan <- "Lost connection"
		}
	}
}

// getTermboxChan returns a channel of termbox Events.
func getTermboxChan() chan termbox.Event {
	termboxChan := make(chan termbox.Event)

	// Poll for terminal events and send them through the channel
	go func() {
		for {
			termboxChan <- termbox.PollEvent()
		}
	}()

	return termboxChan
}

// handleMsg handles incoming WebSocket messages and updates the local CRDT document accordingly.
func handleMsg(msg commons.Message, conn *websocket.Conn) {

	switch msg.Type {

	// Sync local document with remote document received
	case commons.DocSyncMessage:
		logger.Infof("DOCSYNC RECEIVED, UPDATING LOCAL DOC %+v\n", msg.Document)
		doc = msg.Document
		e.SetText(crdt.Content(doc))

	// Respond to document request by sending local document
	case commons.DocReqMessage:
		logger.Infof("DOCREQ RECEIVED, SENDING LOCAL DOC TO %v\n", msg.ID)
		docMsg := commons.Message{Type: commons.DocSyncMessage, Document: doc, ID: msg.ID}
		_ = conn.WriteJSON(&docMsg)

	// Set site ID for the CRDT (for distributed editing)
	case commons.SiteIDMessage:
		siteID, err := strconv.Atoi(msg.Text)
		if err != nil {
			logger.Errorf("failed to set siteID, err: %v\n", err)
		}
		crdt.SiteID = siteID
		logger.Infof("SITE ID %v, INTENDED: %v", crdt.SiteID, siteID)

	// Handle when a user joins the session
	case commons.JoinMessage:
		e.StatusChan <- fmt.Sprintf("%v joined", msg.Username)

	// Update the list of connected users
	case commons.UsersMessage:
		e.StatusMu.Lock()
		e.Users = strings.Split(msg.Text, ",")
		e.StatusMu.Unlock()

	// Handle insert or delete operation messages
	default:
		switch msg.Operation.Type {

		// Insert character into the local document and update UI
		case "insert":
			_, err := doc.Insert(msg.Operation.Position, msg.Operation.Value)
			if err != nil {
				logger.Errorf("failed to insert, err: %v\n", err)
			}
			e.SetText(crdt.Content(doc))
			if msg.Operation.Position-1 <= e.Cursor {
				e.MoveCursor(len(msg.Operation.Value), 0)
			}
			logger.Infof("REMOTE INSERT: %s at position %v\n", msg.Operation.Value, msg.Operation.Position)

		// Delete character from the local document and update UI
		case "delete":
			_ = doc.Delete(msg.Operation.Position)
			e.SetText(crdt.Content(doc))
			if msg.Operation.Position-1 <= e.Cursor {
				e.MoveCursor(-len(msg.Operation.Value), 0)
			}
			logger.Infof("REMOTE DELETE: position %v\n", msg.Operation.Position)

		}
	}

	// Debugging output for the document
	printDoc(doc)

	// Send redraw signal to update the UI
	e.SendDraw()

}

// getMsgChan returns a message channel that reads from a WebSocket connection.
func getMsgChan(conn *websocket.Conn) chan commons.Message {
	msgChan := make(chan commons.Message)

	// Continuously read WebSocket messages and send them to the channel
	go func() {
		for {
			var msg commons.Message

			// Read JSON from WebSocket
			err := conn.ReadJSON(&msg)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					logger.Errorf("websocket error: %v", err)
				}
				// Handle disconnection
				e.IsConnected = false
				e.StatusChan <- "Lost connection"
				break
			}
			// Send the received message to the message channel
			msgChan <- msg
		}
	}()

	return msgChan
}

// handleStatusMsg asynchronously waits for status messages and displays them.
func handleStatusMsg() {
	for msg := range e.StatusChan {
		// Lock the status message and display it
		e.StatusMu.Lock()
		e.StatusMsg = msg
		e.ShowMsg = true
		e.StatusMu.Unlock()

		logger.Infof("got status message: %s", e.StatusMsg)

		// Redraw the UI to display the message
		e.SendDraw()
		time.Sleep(3 * time.Second)

		// Hide the status message after 3 seconds
		e.StatusMu.Lock()
		e.ShowMsg = false
		e.StatusMu.Unlock()

		// Redraw the UI to remove the message
		e.SendDraw()
	}
}

// drawLoop listens for draw signals and updates the UI when necessary.
func drawLoop() {
	for {
		<-e.DrawChan // Wait for the signal to draw
		e.Draw()     // Trigger the draw function to update the UI
	}
}
