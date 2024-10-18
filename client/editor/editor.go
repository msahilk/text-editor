package editor

import (
	"fmt"
	"sync"

	"github.com/mattn/go-runewidth"
	"github.com/nsf/termbox-go"
)

type EditorConfig struct {
	ScrollEnabled bool
}

// Editor encapsulates the core structure of the text editor.
// It consists of two primary components:
// 1. A text area for user interaction and content editing.
// 2. A status bar for displaying various event notifications and user information.
type Editor struct {
	// Text stores the editor's content.
	Text []rune

	// Cursor indicates the current editing position.
	Cursor int

	// Width denotes the terminal's horizontal character capacity.
	Width int

	// Height represents the terminal's vertical character capacity.
	Height int

	// ColOff tracks the horizontal scroll position.
	ColOff int

	// RowOff tracks the vertical scroll position.
	RowOff int

	// ShowMsg toggles the visibility of the status bar.
	ShowMsg bool

	// StatusMsg contains the text to be shown in the status bar.
	StatusMsg string

	// StatusChan facilitates communication of status messages.
	StatusChan chan string

	// StatusMu ensures thread-safe access to status bar information.
	StatusMu sync.Mutex

	// Users maintains a list of connected users for display.
	Users []string

	// ScrollEnabled determines if scrolling beyond the initial view is allowed.
	ScrollEnabled bool

	// IsConnected indicates the current server connection status.
	IsConnected bool

	// DrawChan facilitates signaling for display updates.
	DrawChan chan int

	// mu ensures thread-safe access to the editor's state.
	mu sync.RWMutex
}

var userColors = []termbox.Attribute{
	termbox.ColorGreen,
	termbox.ColorYellow,
	termbox.ColorBlue,
	termbox.ColorMagenta,
	termbox.ColorCyan,
	termbox.ColorLightYellow,
	termbox.ColorLightMagenta,
	termbox.ColorLightGreen,
	termbox.ColorLightRed,
	termbox.ColorRed,
}

// NewEditor initializes and returns a fresh editor instance.
func NewEditor(conf EditorConfig) *Editor {
	return &Editor{
		ScrollEnabled: conf.ScrollEnabled,
		StatusChan:    make(chan string, 100),
		DrawChan:      make(chan int, 10000),
	}
}

// GetText retrieves the current content of the editor.
func (e *Editor) GetText() []rune {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.Text
}

// SetText updates the editor's content with the provided text.
func (e *Editor) SetText(text string) {
	e.mu.Lock()
	e.Text = []rune(text)
	e.mu.Unlock()
}

// GetX retrieves the horizontal component of the cursor's position.
func (e *Editor) GetX() int {
	x, _ := e.calcXY(e.Cursor)
	return x
}

// SetX updates the horizontal component of the cursor's position.
func (e *Editor) SetX(x int) {
	e.Cursor = x
}

// GetY retrieves the vertical component of the cursor's position.
func (e *Editor) GetY() int {
	_, y := e.calcXY(e.Cursor)
	return y
}

// GetWidth retrieves the editor's horizontal character capacity.
func (e *Editor) GetWidth() int {
	return e.Width
}

// GetHeight retrieves the editor's vertical character capacity.
func (e *Editor) GetHeight() int {
	return e.Height
}

// SetSize updates the editor's dimensions to the specified width and height.
func (e *Editor) SetSize(w, h int) {
	e.Width = w
	e.Height = h
}

// GetRowOff retrieves the current vertical scroll position.
func (e *Editor) GetRowOff() int {
	return e.RowOff
}

// GetColOff retrieves the current horizontal scroll position.
func (e *Editor) GetColOff() int {
	return e.ColOff
}

// IncRowOff adjusts the vertical scroll position by the specified increment.
func (e *Editor) IncRowOff(inc int) {
	e.RowOff += inc
}

// IncColOff adjusts the horizontal scroll position by the specified increment.
func (e *Editor) IncColOff(inc int) {
	e.ColOff += inc
}

// SendDraw signals the drawLoop to update the display, ensuring thread-safe rendering.
func (e *Editor) SendDraw() {
	e.DrawChan <- 1
}

// Draw refreshes the UI by populating cells with the editor's content.
func (e *Editor) Draw() {
	_ = termbox.Clear(termbox.ColorDefault, termbox.ColorDefault)

	e.mu.RLock()
	cursor := e.Cursor
	e.mu.RUnlock()

	cx, cy := e.calcXY(cursor)

	// Adjust cursor x position for horizontal scroll
	if cx-e.GetColOff() > 0 {
		cx -= e.GetColOff()
	}

	// Adjust cursor y position for vertical scroll
	if cy-e.GetRowOff() > 0 {
		cy -= e.GetRowOff()
	}

	termbox.SetCursor(cx-1, cy-1)

	// Determine visible area boundaries
	yStart := e.GetRowOff()
	yEnd := yStart + e.GetHeight() - 1 // Account for status bar
	xStart := e.GetColOff()

	x, y := 0, 0
	for i := 0; i < len(e.Text) && y < yEnd; i++ {
		if e.Text[i] == rune('\n') {
			x = 0
			y++
		} else {
			// Render visible content
			setY := y - yStart
			setX := x - xStart
			termbox.SetCell(setX, setY, e.Text[i], termbox.ColorDefault, termbox.ColorDefault)

			// Advance horizontal position
			x = x + runewidth.RuneWidth(e.Text[i])
		}
	}

	e.DrawStatusBar()

	// Apply changes to display
	termbox.Flush()
}

// DrawStatusBar renders status and debug information at the bottom of the editor.
func (e *Editor) DrawStatusBar() {
	e.StatusMu.Lock()
	showMsg := e.ShowMsg
	e.StatusMu.Unlock()
	if showMsg {
		e.DrawStatusMsg()
	} else {
		e.DrawInfoBar()
	}

	// Display connection status indicator
	if e.IsConnected {
		termbox.SetBg(e.Width-1, e.Height-1, termbox.ColorGreen)
	} else {
		termbox.SetBg(e.Width-1, e.Height-1, termbox.ColorRed)
	}
}

// DrawStatusMsg displays the current status message at the bottom of the editor.
func (e *Editor) DrawStatusMsg() {
	e.StatusMu.Lock()
	statusMsg := e.StatusMsg
	e.StatusMu.Unlock()
	for i, r := range []rune(statusMsg) {
		termbox.SetCell(i, e.Height-1, r, termbox.ColorDefault, termbox.ColorDefault)
	}
}

// DrawInfoBar presents debug information and active user list at the bottom of the editor.
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
			colorIdx := i % len(userColors)
			termbox.SetCell(x, e.Height-1, r, userColors[colorIdx], termbox.ColorDefault)
			x++
		}
		termbox.SetCell(x, e.Height-1, ' ', termbox.ColorDefault, termbox.ColorDefault)
		x++
	}

	e.mu.RLock()
	cursor := e.Cursor
	e.mu.RUnlock()

	cx, cy := e.calcXY(cursor)
	debugInfo := fmt.Sprintf(" x=%d, y=%d, cursor=%d, len(text)=%d", cx, cy, e.Cursor, length)

	for _, r := range debugInfo {
		termbox.SetCell(x, e.Height-1, r, termbox.ColorDefault, termbox.ColorDefault)
		x++
	}
}

// MoveCursor updates the cursor position based on the given horizontal and vertical increments.
// Positive values move right and down, respectively.
// This function is invoked by the UI layer in response to user input.
func (e *Editor) MoveCursor(x, y int) {
	if len(e.Text) == 0 && e.Cursor == 0 {
		return
	}
	// Adjust horizontal cursor position
	newCursor := e.Cursor + x

	// Adjust vertical cursor position
	if y > 0 {
		newCursor = e.calcCursorDown()
	}

	if y < 0 {
		newCursor = e.calcCursorUp()
	}

	if e.ScrollEnabled {
		cx, cy := e.calcXY(newCursor)

		// Adjust view window based on cursor movement
		rowStart := e.GetRowOff()
		rowEnd := e.GetRowOff() + e.GetHeight() - 1

		if cy <= rowStart { // Scroll up
			e.IncRowOff(cy - rowStart - 1)
		}

		if cy > rowEnd { // Scroll down
			e.IncRowOff(cy - rowEnd)
		}

		colStart := e.GetColOff()
		colEnd := e.GetColOff() + e.GetWidth()

		if cx <= colStart { // Scroll left
			e.IncColOff(cx - (colStart + 1))
		}

		if cx > colEnd { // Scroll right
			e.IncColOff(cx - colEnd)
		}
	}

	// Ensure cursor remains within text bounds
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

// The calcCursorUp and calcCursorDown functions locate newline characters by scanning backwards and forwards from the current cursor position.
// These characters define the "start" and "end" of the current line.
// The cursor's offset from the line start is calculated and used to determine its final position on the target line, considering the target line's length.
// "pos" serves as a temporary cursor position.

// calcCursorUp computes the new cursor position when moving up one line.
func (e *Editor) calcCursorUp() int {
	pos := e.Cursor
	offset := 0

	// Adjust initial cursor position if out of bounds or on a newline
	if pos == len(e.Text) || e.Text[pos] == '\n' {
		offset++
		pos--
	}

	if pos < 0 {
		pos = 0
	}

	start, end := pos, pos

	// Locate start of current line
	for start > 0 && e.Text[start] != '\n' {
		start--
	}

	// Return to text beginning if already on first line
	if start == 0 {
		return 0
	}

	// Locate end of current line
	for end < len(e.Text) && e.Text[end] != '\n' {
		end++
	}

	// Locate start of previous line
	prevStart := start - 1
	for prevStart >= 0 && e.Text[prevStart] != '\n' {
		prevStart--
	}

	// Calculate cursor offset from line start
	offset += pos - start
	if offset <= start-prevStart {
		return prevStart + offset
	} else {
		return start
	}
}

// calcCursorDown computes the new cursor position when moving down one line.
func (e *Editor) calcCursorDown() int {
	pos := e.Cursor
	offset := 0

	// Adjust initial cursor position if out of bounds or on a newline
	if pos == len(e.Text) || e.Text[pos] == '\n' {
		offset++
		pos--
	}

	if pos < 0 {
		pos = 0
	}

	start, end := pos, pos

	// Locate start of current line
	for start > 0 && e.Text[start] != '\n' {
		start--
	}

	// Handle first line case (no leading newline)
	if start == 0 && e.Text[start] != '\n' {
		offset++
	}

	// Locate end of current line
	for end < len(e.Text) && e.Text[end] != '\n' {
		end++
	}

	// Handle newline case
	if e.Text[pos] == '\n' && e.Cursor != 0 {
		end++
	}

	// Move to text end if already on last line
	if end == len(e.Text) {
		return len(e.Text)
	}

	// Locate end of next line
	nextEnd := end + 1
	for nextEnd < len(e.Text) && e.Text[nextEnd] != '\n' {
		nextEnd++
	}

	// Calculate cursor offset from line start
	offset += pos - start
	if offset < nextEnd-end {
		return end + offset
	} else {
		return nextEnd
	}
}

// calcXY determines the display coordinates for the given text index.
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
