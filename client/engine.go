package main

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"text-editor/commons"
	"text-editor/crdt"

	"github.com/gorilla/websocket"
	"github.com/nsf/termbox-go"
	"github.com/sirupsen/logrus"
)

// handleTermboxEvent processes keyboard input, updates the local CRDT document,
// and transmits a message via WebSocket.
func handleTermboxEvent(ev termbox.Event, conn *websocket.Conn) error {
	// Focus on termbox key events (EventKey) exclusively.
	if ev.Type == termbox.EventKey {
		switch ev.Key {

		// Esc and Ctrl+C serve as the standard session termination keys.
		case termbox.KeyEsc, termbox.KeyCtrlC:
			// Generate an error with the "editor" prefix for exit handling.
			return errors.New("editor: exiting")

		// Ctrl+S is designated as the default key for content preservation.
		case termbox.KeyCtrlS:
			// Assign a default filename if none is provided.
			if fileName == "" {
				fileName = "editor-content.txt"
			}

			// Persist the CRDT to a file.
			err := crdt.Save(fileName, &doc)
			if err != nil {
				logrus.Errorf("Failed to save to %s", fileName)
				e.StatusChan <- fmt.Sprintf("Failed to save to %s", fileName)
				return err
			}

			// Update the status bar.
			e.StatusChan <- fmt.Sprintf("Saved document to %s", fileName)

		// Ctrl+L is set as the default key for file content retrieval.
		case termbox.KeyCtrlL:
			if fileName != "" {
				logger.Log(logrus.InfoLevel, "LOADING DOCUMENT")
				newDoc, err := crdt.Load(fileName)
				if err != nil {
					logrus.Errorf("failed to load file %s", fileName)
					e.StatusChan <- fmt.Sprintf("Failed to load %s", fileName)
					return err
				}
				e.StatusChan <- fmt.Sprintf("Loading %s", fileName)
				doc = newDoc
				e.SetX(0)
				e.SetText(crdt.Content(doc))

				logger.Log(logrus.InfoLevel, "SENDING DOCUMENT")
				docMsg := commons.Message{Type: commons.DocSyncMessage, Document: doc}
				_ = conn.WriteJSON(&docMsg)
			} else {
				e.StatusChan <- "No file to load!"
			}

		// Left arrow and Ctrl+B are configured for leftward cursor movement.
		case termbox.KeyArrowLeft, termbox.KeyCtrlB:
			e.MoveCursor(-1, 0)

		// Right arrow and Ctrl+F facilitate rightward cursor movement.
		case termbox.KeyArrowRight, termbox.KeyCtrlF:
			e.MoveCursor(1, 0)

		// Up arrow and Ctrl+P enable upward cursor movement.
		case termbox.KeyArrowUp, termbox.KeyCtrlP:
			e.MoveCursor(0, -1)

		// Down arrow and Ctrl+N allow downward cursor movement.
		case termbox.KeyArrowDown, termbox.KeyCtrlN:
			e.MoveCursor(0, 1)

		// Home key repositions the cursor to the line's start (X=0).
		case termbox.KeyHome:
			e.SetX(0)

		// End key shifts the cursor to the line's end (X = text length).
		case termbox.KeyEnd:
			e.SetX(len(e.Text))

		// Backspace and Delete are assigned for character removal.
		case termbox.KeyBackspace, termbox.KeyBackspace2:
			performOperation(OperationDelete, ev, conn)
		case termbox.KeyDelete:
			performOperation(OperationDelete, ev, conn)

		// Tab key inserts 4 spaces to emulate a tab character.
		case termbox.KeyTab:
			for i := 0; i < 4; i++ {
				ev.Ch = ' '
				performOperation(OperationInsert, ev, conn)
			}

		// Enter key adds a newline character to the content.
		case termbox.KeyEnter:
			ev.Ch = '\n'
			performOperation(OperationInsert, ev, conn)

		// Space key introduces a space character to the content.
		case termbox.KeySpace:
			ev.Ch = ' '
			performOperation(OperationInsert, ev, conn)

		// Any other key is considered for insertion.
		default:
			if ev.Ch != 0 {
				performOperation(OperationInsert, ev, conn)
			}
		}
	}

	e.SendDraw()
	return nil
}

const (
	OperationInsert = iota
	OperationDelete
)

// performOperation executes a CRDT insert or delete action on the local document
// and dispatches a message via WebSocket.
func performOperation(opType int, ev termbox.Event, conn *websocket.Conn) {
	// Retrieve position and value.
	ch := string(ev.Ch)

	var msg commons.Message

	// Adjust local state (CRDT) initially.
	switch opType {
	case OperationInsert:
		logger.Infof("LOCAL INSERT: %s at cursor position %v\n", ch, e.Cursor)

		char, err := doc.GenerateInsertChar(e.Cursor+1, ch)
		if err != nil {
			logger.Errorf("CRDT error: %v\n", err)
			return
		}
		e.SetText(crdt.Content(doc))

		e.MoveCursor(1, 0)
		msg = commons.Message{Type: "operation", Operation: commons.Operation{Type: "insert", Position: e.Cursor, Value: ch, Char: char}}
		logOp(msg.Operation)

	case OperationDelete:
		logger.Infof("LOCAL DELETE: cursor position %v\n", e.Cursor)

		if e.Cursor-1 < 0 {
			e.Cursor = 0
		}

		char, ok := doc.GenerateDeleteChar(e.Cursor)
		if !ok {
			return
		}
		e.SetText(crdt.Content(doc))

		msg = commons.Message{Type: "operation", Operation: commons.Operation{Type: "delete", Position: e.Cursor, Char: char}}
		logOp(msg.Operation)
		e.MoveCursor(-1, 0)
	}

	// Transmit the message.
	if e.IsConnected {
		err := conn.WriteJSON(msg)
		if err != nil {
			e.IsConnected = false
			e.StatusChan <- "lost connection!"
		}
	}
}

// getTermboxChan yields a channel of termbox Events, continuously awaiting user input.
func getTermboxChan() chan termbox.Event {
	termboxChan := make(chan termbox.Event)

	go func() {
		for {
			termboxChan <- termbox.PollEvent()
		}
	}()

	return termboxChan
}

// handleMsg refreshes the CRDT document with the message contents.
func handleMsg(msg commons.Message, conn *websocket.Conn) {
	switch msg.Type {
	case commons.DocSyncMessage:
		logger.Infof("DOCSYNC RECEIVED, updating local doc %+v\n", msg.Document)

		doc = msg.Document

		// Replay edits made or received while the snapshot was in flight.
		// Integration is idempotent, so edits already in the snapshot are skipped.
		synced = true
		for _, op := range preSyncLog {
			applyRemoteOp(op)
		}
		preSyncLog = nil

		e.SetText(crdt.Content(doc))

	case commons.DocReqMessage:
		logger.Infof("DOCREQ RECEIVED, sending local document to %v\n", msg.ID)

		docMsg := commons.Message{Type: commons.DocSyncMessage, Document: doc, ID: msg.ID}
		_ = conn.WriteJSON(&docMsg)

	case commons.SiteIDMessage:
		siteID, err := strconv.Atoi(msg.Text)
		if err != nil {
			logger.Errorf("failed to set siteID, err: %v\n", err)
		}

		crdt.SiteID = siteID
		logger.Infof("SITE ID %v, INTENDED SITE ID: %v", crdt.SiteID, siteID)

	case commons.JoinMessage:
		e.StatusChan <- fmt.Sprintf("%s has joined the session!", msg.Username)

	case commons.UsersMessage:
		e.StatusMu.Lock()
		e.Users = strings.Split(msg.Text, ",")
		e.StatusMu.Unlock()

	default:
		switch msg.Operation.Type {
		case "insert", "delete":
			// Remember where the cursor-relative edit happened so the cursor
			// keeps pointing at the same text.
			var before int
			if msg.Operation.Type == "delete" {
				before = doc.VisiblePosition(msg.Operation.Char.ID)
			}

			logOp(msg.Operation)
			applyRemoteOp(msg.Operation)
			e.SetText(crdt.Content(doc))

			switch msg.Operation.Type {
			case "insert":
				if p := doc.VisiblePosition(msg.Operation.Char.ID); p != -1 && p <= e.Cursor {
					e.MoveCursor(1, 0)
				}
				logger.Infof("REMOTE INSERT: %s (id %s)\n", msg.Operation.Char.Value, msg.Operation.Char.ID)
			case "delete":
				if before != -1 && before <= e.Cursor {
					e.MoveCursor(-1, 0)
				}
				logger.Infof("REMOTE DELETE: id %s\n", msg.Operation.Char.ID)
			}
		}
	}

	// printDoc aids in debugging. Avoid commenting this out.
	// It can be activated via the `-debug` flag.
	// By default, printDoc doesn't log anything.
	// This ensures debug logs don't consume excessive space on the user's system,
	// and can be enabled as needed.
	printDoc(doc)

	e.SendDraw()
}

// getMsgChan returns a message channel that continuously reads from a websocket connection.
func getMsgChan(conn *websocket.Conn) chan commons.Message {
	messageChan := make(chan commons.Message)
	go func() {
		for {
			var msg commons.Message

			// Retrieve message.
			err := conn.ReadJSON(&msg)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					logger.Errorf("websocket error: %v", err)
				}
				e.IsConnected = false
				e.StatusChan <- "lost connection!"
				break
			}

			logger.Infof("message received: %+v\n", msg)

			// Transmit message through channel
			messageChan <- msg

		}
	}()
	return messageChan
}

// handleStatusMsg asynchronously waits for messages from e.StatusChan and
// renders the message upon arrival.
func handleStatusMsg() {
	for msg := range e.StatusChan {
		e.StatusMu.Lock()
		e.StatusMsg = msg
		e.ShowMsg = true
		e.StatusMu.Unlock()

		logger.Infof("got status message: %s", e.StatusMsg)

		e.SendDraw()
		time.Sleep(3 * time.Second)

		e.StatusMu.Lock()
		e.ShowMsg = false
		e.StatusMu.Unlock()

		e.SendDraw()
	}

}

func drawLoop() {
	for {
		<-e.DrawChan
		e.Draw()
	}
}

var (
	// pendingOps holds remote edits whose dependencies have not arrived yet.
	pendingOps []commons.Operation

	// synced reports whether this client has received the document snapshot.
	synced bool

	// preSyncLog records every edit until the snapshot arrives, so none are
	// lost when the snapshot replaces the local document.
	preSyncLog []commons.Operation
)

// logOp records an edit until the initial document snapshot has been applied.
func logOp(op commons.Operation) {
	if !synced {
		preSyncLog = append(preSyncLog, op)
	}
}

// applyRemoteOp integrates a remote edit into the local document. Edits that
// depend on characters not yet received are parked and retried later.
func applyRemoteOp(op commons.Operation) {
	pendingOps = append(pendingOps, op)

	for progress := true; progress; {
		progress = false
		remaining := pendingOps[:0]
		for _, p := range pendingOps {
			var err error
			switch p.Type {
			case "insert":
				err = doc.RemoteInsert(p.Char)
			case "delete":
				err = doc.RemoteDelete(p.Char)
			}
			if errors.Is(err, crdt.ErrNotReady) {
				remaining = append(remaining, p)
				continue
			}
			if err != nil {
				logger.Errorf("failed to apply %s: %v", p.Type, err)
			}
			progress = true
		}
		pendingOps = remaining
	}
}
