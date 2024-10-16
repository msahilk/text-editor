package main

import (
	"github.com/sirupsen/logrus"
	"text-editor/client/editor"
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
