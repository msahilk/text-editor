package crdt

import (
	"testing"
)

func TestDocument(t *testing.T) {
	doc := New()

	// A new document must have at least 2 characters (start and end).
	got := doc.Length()
	want := 2

	if got != want {
		t.Errorf("got != want; got = %v, expected = %v\n", got, want)
	}
}
