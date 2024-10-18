package main

import (
	"text-editor/client/editor"
	"text-editor/crdt"

	"github.com/gorilla/websocket"
	"github.com/nsf/termbox-go"
)

type UIConfig struct {
	EditorConfig editor.EditorConfig
}

// The text user interface is constructed using termbox-go.
// termbox enables us to assign content to individual cells, making the cell the fundamental unit of the editor.

// initUI establishes a new editor view and initiates the primary loop.
func initUI(conn *websocket.Conn, conf UIConfig) error {
	err := termbox.Init()
	if err != nil {
		return err
	}
	defer termbox.Close()

	e = editor.NewEditor(conf.EditorConfig)
	e.SetSize(termbox.Size())
	e.SetText(crdt.Content(doc))
	e.SendDraw()
	e.IsConnected = true

	go handleStatusMsg()

	go drawLoop()

	err = mainLoop(conn)
	if err != nil {
		return err
	}

	return nil
}

// mainLoop serves as the primary update cycle for the user interface.
func mainLoop(conn *websocket.Conn) error {
	// termboxChan facilitates the transmission and reception of termbox events.
	termboxChan := getTermboxChan()

	// msgChan enables the sending and receiving of messages.
	msgChan := getMsgChan(conn)

	for {
		select {
		case termboxEvent := <-termboxChan:
			err := handleTermboxEvent(termboxEvent, conn)
			if err != nil {
				return err
			}
		case msg := <-msgChan:
			handleMsg(msg, conn)
		}
	}
}
