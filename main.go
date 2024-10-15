package main

import (
	"fmt"
	"github.com/nsf/termbox-go"
	"log"
)

//TIP To run your code, right-click the code and select <b>Run</b>. Alternatively, click
// the <icon src="AllIcons.Actions.Execute"/> icon in the gutter and select the <b>Run</b> menu item from here.

type term struct {
	x int
	y int
}

func main() {
	//TIP Press <shortcut actionId="ShowIntentionActions"/> when your caret is at the underlined or highlighted text
	// to see how GoLand suggests fixing it.
	s := "gopher"
	fmt.Println("Hello and welcome, %s!", s)
	termBox := term{0, 0}
	err := termbox.Init()
	if err != nil {
		log.Fatal(err)
	}
	defer termbox.Close()

	renderText("Welcome to Collaborative Editor", &termBox)

	for {
		switch ev := termbox.PollEvent(); ev.Type {
		case termbox.EventKey:
			if ev.Key == termbox.KeyEsc {
				return
			}
			handleKeyPress(ev.Ch, &termBox)
		}
	}
}

func handleKeyPress(ch rune, box *term) {
	renderText(string(ch), box)
}

func renderText(text string, box *term) {
	for _, ch := range text {
		termbox.SetCell(box.x, box.y, ch, termbox.ColorDefault, termbox.ColorDefault)
		box.x += 1
		if box.x > 50 {
			box.x = 0
			box.y += 1
		}
	}
	termbox.Flush()
}

//TIP See GoLand help at <a href="https://www.jetbrains.com/help/go/">jetbrains.com/help/go/</a>.
// Also, you can try interactive lessons for GoLand by selecting 'Help | Learn IDE Features' from the main menu.
