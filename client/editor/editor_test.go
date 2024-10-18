package editor

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestEditor_CalcXY(t *testing.T) {
	tests := []struct {
		description string
		cursor      int
		expectedX   int
		expectedY   int
	}{
		{"initial position", 0, 1, 1},
		{"negative index", -1, 1, 1},
		{"normal editing", 6, 7, 1},
		{"after new line", 10, 3, 2},
		{"large number", 100000, 5, 2},
	}

	e := NewEditor(EditorConfig{})

	e.Text = []rune("content\ntest")

	for _, tc := range tests {
		e.Cursor = tc.cursor
		x, y := e.calcXY(e.Cursor)

		res := []int{x, y}

		expected := []int{tc.expectedX, tc.expectedY}

		if !cmp.Equal(res, expected) {
			t.Errorf("(%s) got != expected, diff: %v", tc.description, cmp.Diff(res, expected))
		}
	}

}

func TestEditor_MoveCursor(t *testing.T) {

	tests := []struct {
		description    string
		cursor         int
		x              int
		y              int
		expectedCursor int
		text           []rune
	}{
		// Test horizontal
		{"move forward (empty doc)", 0, 1, 0, 0, []rune("")},
		{"move backward (empty doc)", 0, -1, 0, 0, []rune("")},
		{"move forward", 0, 1, 0, 1, []rune("test\n")},
		{"move backward", 1, -1, 0, 0, []rune("test\n")},
		{"move forward (oob)", 4, 3, 0, 5, []rune("test\n")},
		{"move backward (oob)", 0, -10, 0, 0, []rune("test\n")},

		// Test vertical
		{"move up", 6, 0, -1, 2, []rune("tes\nter")},
		{"move down", 1, 0, 1, 5, []rune("tes\nter")},
		{"move up (empty)", 0, 0, -1, 0, []rune("")},
		{"move down (empty)", 0, 0, 1, 0, []rune("")},
		{"move up (line 1)", 1, 0, -1, 0, []rune("test\ning")},
		{"move down (last line)", 7, 0, 1, 8, []rune("test\ning")},
		{"move up (middle line)", 6, 0, -1, 2, []rune("tes\nting\ncase")},
		{"move down (middle line)", 6, 0, 1, 11, []rune("tes\nting\ncase")},
		{"move up (on new line)", 4, 0, -1, 0, []rune("tes\nting\ncase")},
		{"move down (on new line)", 4, 0, 1, 9, []rune("tes\nting\ncase")},
		{"move up (short to long)", 6, 0, -1, 1, []rune("test\ning\ncase")},
		{"move down (short to long)", 6, 0, 1, 10, []rune("test\ning\ncase")},
		{"move up (long to short)", 6, 0, -1, 2, []rune("tes\nting\nyes")},
		{"move down (long to short)", 6, 0, 1, 11, []rune("tes\nting\nyes")},
		{"move up (from empty line)", 3, 0, -1, 0, []rune("tes\n\nting\n")},
		{"move down (from empty line)", 3, 0, 1, 4, []rune("tes\n\nting\n")},
		{"move up (from empty to empty)", 6, 0, -1, 5, []rune("test\n\n\n")},
		{"move down (from empty to empty)", 6, 0, 1, 7, []rune("test\n\n\n")},
		{"move up (from empty last line to empty)", 7, 0, -1, 6, []rune("test\n\n\n")},
		{"move down (from empty first line to empty)", 0, 0, 1, 1, []rune("\n\n\n")},
		{"move up (from empty last)", 7, 0, -1, 2, []rune("\n\ntest\n")},
		{"move down (from empty first)", 0, 0, 1, 1, []rune("\ntest\n")},
		{"move up (from empty first)", 0, 0, -1, 0, []rune("\ntest\n")},
		{"move down (from empty last)", 6, 0, 1, 6, []rune("\ntest\n")},
		{"move up (from last to empty)", 2, 0, -1, 1, []rune("\n\ntest")},
		{"move up (from first to empty)", 2, 0, 1, 5, []rune("test\n\n")},
	}

	e := NewEditor(EditorConfig{})

	for _, tc := range tests {
		e.Cursor = tc.cursor
		e.Text = tc.text
		e.MoveCursor(tc.x, tc.y)

		res := e.Cursor
		expected := tc.expectedCursor

		if !cmp.Equal(res, expected) {
			t.Errorf("(%s) got != expected, diff: %v", tc.description, cmp.Diff(res, expected))
		}
	}

}

// Test scrolling
func TestScroll(t *testing.T) {
	{
		tests := []struct {
			description    string
			x              int
			y              int
			colOff         int
			expectedColOff int
			rowOff         int
			expectedRowOff int
			cursor         int
			expectedCursor int
			text           []rune
		}{
			{description: "scroll down",
				y:      1,
				colOff: 0, expectedColOff: 0,
				rowOff: 0, expectedRowOff: 1,
				cursor: 6, expectedCursor: 8,
				text: []rune("a\nb\nc\nd\ne")},

			{description: "scroll up",
				y:      -1,
				colOff: 0, expectedColOff: 0,
				rowOff: 1, expectedRowOff: 0,
				cursor: 2, expectedCursor: 0,
				text: []rune("a\nb\nc\nd\ne")},

			{description: "scroll right",
				x:      1,
				colOff: 0, expectedColOff: 1,
				rowOff: 0, expectedRowOff: 0,
				cursor: 4, expectedCursor: 5,
				text: []rune("abcde")},

			{description: "scroll left",
				x:      -1,
				colOff: 1, expectedColOff: 0,
				rowOff: 0, expectedRowOff: 0,
				cursor: 1, expectedCursor: 0,
				text: []rune("abcde")},

			{description: "horizontal jump backwards",
				x:      1,
				colOff: 4, expectedColOff: 0,
				rowOff: 0, expectedRowOff: 0,
				cursor: 8, expectedCursor: 9,
				text: []rune("abcdefgh\nijk")},

			{description: "horizontal jump forwards",
				x:      -1,
				colOff: 0, expectedColOff: 4,
				rowOff: 0, expectedRowOff: 0,
				cursor: 9, expectedCursor: 8,
				text: []rune("abcdefgh\nijk")},
		}

		e := NewEditor(EditorConfig{
			ScrollEnabled: true,
		})
		e.Width = 5
		e.Height = 5

		for _, tc := range tests {
			e.ColOff = tc.colOff
			e.RowOff = tc.rowOff
			e.Cursor = tc.cursor
			e.Text = tc.text

			e.MoveCursor(tc.x, tc.y)

			gotCursor := e.Cursor
			expectedCursor := tc.expectedCursor

			if !cmp.Equal(gotCursor, expectedCursor) {
				t.Errorf("(%s) Wrong cursor: got != expected, diff: %v\n", tc.description, cmp.Diff(gotCursor, expectedCursor))
			}

			gotRowOff := e.RowOff
			expectedRowOff := tc.expectedRowOff

			if !cmp.Equal(gotRowOff, expectedRowOff) {
				t.Errorf("(%s) Wrong row offset: got != expected, diff: %v\n", tc.description, cmp.Diff(gotRowOff, expectedRowOff))
			}

			gotColOff := e.ColOff
			expectedColOff := tc.expectedColOff

			if !cmp.Equal(gotColOff, expectedColOff) {
				t.Errorf("(%s) Wrong col offset: got != expected, diff: %v\n", tc.description, cmp.Diff(gotColOff, expectedColOff))
			}
		}
	}
}
