package editor

import (
	"fmt"
	"github.com/mattn/go-runewidth"
	"github.com/nsf/termbox-go"
	"sync"
)

type Config struct {
	ScrollEnabled bool
}

type Editor struct {
	Text []rune

	Cursor int

	Width int

	Height int

	ColOffset int

	RowOffset int

	ShowMsg bool

	StatusMsg string

	StatusChan chan string

	StatusMu sync.Mutex

	Users []string

	ScrollEnabled bool

	IsConnected bool

	DrawChan chan int

	mu sync.RWMutex
}

var userColors = []termbox.Attribute{
	termbox.ColorGreen,
	termbox.ColorYellow,
	termbox.ColorRed,
	termbox.ColorCyan,
}

func NewEditor(conf Config) *Editor {
	return &Editor{
		ScrollEnabled: conf.ScrollEnabled,
		StatusChan:    make(chan string, 100),
		DrawChan:      make(chan int, 10000),
	}
}

func (e *Editor) GetText() []rune {

	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Text
}

func (e *Editor) SetText(text string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Text = []rune(text)
}

func (e *Editor) GetX() int {
	x, _ := e.calcXY(e.Cursor)
	return x
}

func (e *Editor) SetX(x int) {
	e.Cursor = x
}

func (e *Editor) GetY() int {
	_, y := e.calcXY(e.Cursor)
	return y
}

func (e *Editor) GetWidth() int {
	return e.Width
}

func (e *Editor) GetHeight() int {
	return e.Height
}

func (e *Editor) SetSize(w, h int) {

	e.Width = w
	e.Height = h
}

func (e *Editor) GetRowOffset() int {
	return e.RowOffset
}

func (e *Editor) GetColOffset() int {
	return e.ColOffset
}

func (e *Editor) IncRowOffset(inc int) {
	e.RowOffset += inc
}

func (e *Editor) IncColOffset(inc int) {
	e.ColOffset += inc
}

func (e *Editor) SendDraw() {
	e.DrawChan <- 1
}

func (e *Editor) Draw() {

	_ = termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)

	e.mu.RLock()
	cursor := e.Cursor
	e.mu.RUnlock()

	cx, cy := e.calcXY(cursor)

	if cx-e.GetColOffset() > 0 {
		cx -= e.GetColOffset()
	}

	if cy-e.GetRowOffset() > 0 {
		cy -= e.GetRowOffset()
	}

	termbox.SetCursor(cx-1, cy-1)

	yStart := e.GetRowOffset()
	yEnd := yStart + e.GetHeight()

	xStart := e.GetColOffset()

	x, y := 0, 0
	for i := 0; i < len(e.Text) && y < yEnd; i++ {
		if e.Text[i] == rune('\n') {
			x = 0
			y++
		} else {
			setY := y - yStart
			setX := x - xStart

			termbox.SetCell(setX, setY, e.Text[i], termbox.ColorDefault, termbox.ColorDefault)

			x += runewidth.RuneWidth(e.Text[i])
		}
	}

	e.DrawStatusBar()

	err := termbox.Flush()
	if err != nil {
		return
	}
}

func (e *Editor) DrawStatusBar() {

	e.StatusMu.Lock()
	showMsg := e.ShowMsg
	e.StatusMu.Unlock()

	if showMsg {
		e.DrawStatusMsg()
	} else {
		e.DrawInfoBar()
	}

	if e.IsConnected {
		termbox.SetBg(e.Width-1, e.Height-1, termbox.ColorGreen)
	} else {
		termbox.SetBg(e.Width-1, e.Height-1, termbox.ColorRed)
	}
}

func (e *Editor) DrawStatusMsg() {
	e.StatusMu.Lock()
	statusMsg := e.StatusMsg
	e.StatusMu.Unlock()

	for i, r := range []rune(statusMsg) {
		termbox.SetCell(i, e.Height-1, r, termbox.ColorDefault, termbox.ColorDefault)
	}
}

// Debug info that replaces status bar
func (e *Editor) DrawInfoBar() {
	e.StatusMu.Lock()
	users := e.Users
	e.StatusMu.Unlock()

	e.mu.RLock()
	length := len(e.Text)
	e.mu.RUnlock()

	x := 0
	for i, user := range users {
		for _, r := range user {
			color := i % len(userColors)
			termbox.SetCell(x, e.Height-1, r, userColors[color], termbox.ColorDefault)

		}
		termbox.SetCell(x, e.Height-1, ' ', termbox.ColorDefault, termbox.ColorDefault)
		x++
	}

	e.mu.RLock()
	cursor := e.Cursor
	e.mu.RUnlock()

	cx, cy := e.calcXY(cursor)

	debugInfo := fmt.Sprintf(" x = %d, y = %d, cursor = %d, len(text) = %d", cx, cy, e.Cursor, length)

	for _, r := range debugInfo {
		termbox.SetCell(x, e.Height-1, r, termbox.ColorDefault, termbox.ColorDefault)
		x++
	}
}

func (e *Editor) MoveCursor(x, y int) {

	if len(e.Text) == 0 && e.Cursor == 0 {
		return
	}

	newCursor := e.Cursor + x

	if y > 0 {
		newCursor = e.calcCursorDown()
	}

	if y < 0 {
		newCursor = e.calcCursorUp()
	}

	if e.ScrollEnabled {
		cx, cy := e.calcXY(newCursor)

		rowStart := e.GetRowOffset()
		rowEnd := rowStart + e.GetHeight() - 1

		if cy <= rowStart {
			e.IncRowOffset(cy - rowStart - 1)
		}

		if cy > rowEnd {
			e.IncRowOffset(cy - rowEnd)
		}

		colStart := e.GetColOffset()
		colEnd := colStart + e.GetWidth()

		if cx <= colStart {
			e.IncColOffset(cx - colStart - 1)
		}

		if cx > colEnd {
			e.IncColOffset(cx - colEnd)
		}
	}

	if newCursor > len(e.Text) {
		newCursor = len(e.Text)
	}

	if newCursor < 0 {
		newCursor = 0
	}

	e.mu.Lock()
	e.Cursor = newCursor
	e.mu.Unlock()

}

func (e *Editor) calcCursorUp() int {
	pos := e.Cursor
	offset := 0

	if pos == len(e.Text) || e.Text[pos] == '\n' {
		offset++
		pos--
	}

	if pos < 0 {
		pos = 0
	}

	currLineStart := pos

	for currLineStart > 0 && e.Text[currLineStart] != '\n' {
		currLineStart--
	}

	if currLineStart == 0 {
		return 0
	}

	prevLineStart := currLineStart - 1
	for prevLineStart >= 0 && e.Text[prevLineStart] != '\n' {
		prevLineStart--
	}

	offset += pos - currLineStart

	if offset <= currLineStart-prevLineStart {
		return prevLineStart + offset
	} else {
		return currLineStart
	}
}

func (e *Editor) calcCursorDown() int {

	pos := e.Cursor
	offset := 0

	if pos == len(e.Text) || e.Text[pos] == '\n' {
		offset++
		pos--

	}

	if pos < 0 {
		pos = 0
	}

	currLineStart, currLineEnd := pos, pos

	for currLineStart > 0 && e.Text[currLineStart] != '\n' {
		currLineStart--
	}

	if currLineStart == 0 && e.Text[currLineStart] != '\n' {
		offset++
	}

	for currLineEnd < len(e.Text) && e.Text[currLineEnd] != '\n' {
		currLineEnd++
	}

	// Check this
	if e.Text[pos] == '\n' && e.Cursor != 0 {
		currLineEnd++
	}

	if currLineEnd == len(e.Text) {
		return len(e.Text)
	}

	nextLineEnd := currLineEnd + 1
	for nextLineEnd < len(e.Text) && e.Text[nextLineEnd] != '\n' {
		nextLineEnd++
	}

	offset += pos - currLineStart
	if offset < nextLineEnd-currLineEnd {
		return currLineEnd + offset
	} else {
		return nextLineEnd
	}

}

func (e *Editor) calcXY(index int) (int, int) {
	x := 1
	y := 1

	if index < 0 {
		return x, y
	}
	e.mu.RLock()
	length := len(e.Text)
	e.mu.RUnlock()

	if index > length {
		index = length
	}

	for i := 0; i < index; i++ {
		e.mu.RLock()
		r := e.Text[i]
		e.mu.RUnlock()
		if r == rune('\n') {
			x = 1
			y++
		} else {
			x = x + runewidth.RuneWidth(r)
		}
	}
	return x, y
}
