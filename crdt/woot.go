package crdt

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

// DONE
// Document is a slice of characters
type Document struct {
	Characters []Character
}

type Character struct {
	ID         string
	Visible    bool
	Value      string
	IDPrevious string
	IDNext     string
}

var (
	mu sync.Mutex

	// Unique variable per user to generate identifiers for characters in the document.
	SiteID = 0

	// Incremented whenever an insert operation takes place. Used to generate unique IDs for characters.
	LocalClock = 0

	// StartChar is placed at the start.
	StartChar = Character{ID: "start", Visible: false, Value: "", IDPrevious: "", IDNext: "end"}

	// EndChar is placed at the end.
	EndChar = Character{ID: "end", Visible: false, Value: "", IDPrevious: "start", IDNext: ""}

	ErrPositionOutOfBounds = errors.New("position out of bounds")
	ErrEmptyWCharacter     = errors.New("empty char ID provided")
	ErrBoundsNotPresent    = errors.New("subsequence bound(s) not present")
)

// New returns a new document with the start and end characters.
func New() Document {
	return Document{Characters: []Character{StartChar, EndChar}}
}

// Load creates a new CRDTdocument from a file.
func Load(fileName string) (Document, error) {
	doc := New()
	content, err := os.ReadFile(fileName)
	if err != nil {
		return doc, err
	}
	lines := strings.Split(string(content), "\n")
	pos := 1
	for i := 0; i < len(lines); i++ {
		for j := 0; j < len(lines[i]); j++ {
			_, err := doc.Insert(pos, string(lines[i][j]))
			if err != nil {
				return doc, err
			}
			pos++
		}
		if i < len(lines)-1 { // don't insert '\n' on last line
			_, err := doc.Insert(pos, "\n")
			if err != nil {
				return doc, err
			}
			pos++
		}
	}
	return doc, err
}

// Save writes the document to a file. Overwrites the file if it exists.
func Save(fileName string, doc *Document) error {
	return os.WriteFile(fileName, []byte(Content(*doc)), 0644)
}

// Utility functions

// SetText sets the document to be equal to the passed document.
func (doc *Document) SetText(newDoc Document) {
	for _, char := range newDoc.Characters {
		c := Character{ID: char.ID, Visible: char.Visible, Value: char.Value, IDPrevious: char.IDPrevious, IDNext: char.IDNext}
		doc.Characters = append(doc.Characters, c)
	}
}

// Content returns the content of the document.
func Content(doc Document) string {
	value := ""
	for _, char := range doc.Characters {
		if char.Visible {
			value += char.Value
		}
	}
	return value
}

// IthVisible returns the ith visible character in the document.
func IthVisible(doc Document, position int) Character {
	count := 0

	for _, char := range doc.Characters {
		if char.Visible {
			if count == position-1 {
				return char
			}
			count++
		}
	}

	return Character{ID: "-1"}
}

// Length returns the length of the document.
func (doc *Document) Length() int {
	return len(doc.Characters)
}

// ElementAt returns the character at the given position.
func (doc *Document) ElementAt(position int) (Character, error) {
	if position < 0 || position >= doc.Length() {
		return Character{}, ErrPositionOutOfBounds
	}

	return doc.Characters[position], nil
}

// Position returns the position of the given character.
func (doc *Document) Position(charID string) int {
	for position, char := range doc.Characters {
		if charID == char.ID {
			return position + 1
		}
	}

	return -1
}

// Left returns the ID of the character to the left of the given character.
func (doc *Document) Left(charID string) string {
	i := doc.Position(charID)
	if i <= 0 {
		return doc.Characters[i].ID
	}
	return doc.Characters[i-1].ID
}

// Right returns the ID of the character to the right of the given character.
func (doc *Document) Right(charID string) string {
	i := doc.Position(charID)
	if i >= len(doc.Characters)-1 {
		return doc.Characters[i-1].ID
	}
	return doc.Characters[i+1].ID
}

// Contains checks if a character is present in the document.
func (doc *Document) Contains(charID string) bool {
	position := doc.Position(charID)
	return position != -1
}

// Find returns the character at the ID.
func (doc *Document) Find(id string) Character {
	for _, char := range doc.Characters {
		if char.ID == id {
			return char
		}
	}

	return Character{ID: "-1"}
}

// Subsequence returns the content between the positions.
func (doc *Document) Subsequence(wcharacterStart, wcharacterEnd Character) ([]Character, error) {
	startPosition := doc.Position(wcharacterStart.ID)
	endPosition := doc.Position(wcharacterEnd.ID)

	if startPosition == -1 || endPosition == -1 {
		return doc.Characters, ErrBoundsNotPresent
	}

	if startPosition > endPosition {
		return doc.Characters, ErrBoundsNotPresent
	}

	if startPosition == endPosition {
		return []Character{}, nil
	}

	return doc.Characters[startPosition : endPosition-1], nil
}

// Operations

// LocalInsert inserts the character into the document.
func (doc *Document) LocalInsert(char Character, position int) (*Document, error) {
	if position <= 0 || position >= doc.Length() {
		return doc, ErrPositionOutOfBounds
	}

	if char.ID == "" {
		return doc, ErrEmptyWCharacter
	}

	doc.Characters = append(doc.Characters[:position],
		append([]Character{char}, doc.Characters[position:]...)...,
	)

	// Update next and previous pointers.
	doc.Characters[position-1].IDNext = char.ID
	doc.Characters[position+1].IDPrevious = char.ID

	return doc, nil
}

// IntegrateInsert inserts the given Character into the Document
// Characters based off of the previous & next Character
func (doc *Document) IntegrateInsert(char, charPrev, charNext Character) (*Document, error) {
	// Get the subsequence.

	// Handle invalid subsequence.
	subsequence, err := doc.Subsequence(charPrev, charNext)
	if err != nil {
		return doc, err
	}

	// Get the position of the next character.
	position := doc.Position(charNext.ID)
	position--

	// Handle empty subsequence (Insert at current position)
	if len(subsequence) == 0 {
		return doc.LocalInsert(char, position)
	}

	// Handle single character subsequence (Insert at previous position)
	if len(subsequence) == 1 {
		return doc.LocalInsert(char, position-1)
	}

	// Find the correct position to insert the character.
	i := 1
	for i < len(subsequence)-1 && subsequence[i].ID < char.ID {
		i++
	}
	// Insert the character at the correct position.
	return doc.IntegrateInsert(char, subsequence[i-1], subsequence[i])
}

// GenerateInsert generates an insert operation for the given position and value.
func (doc *Document) GenerateInsert(position int, value string) (*Document, error) {
	// Increment local clock.
	mu.Lock()
	LocalClock++
	mu.Unlock()

	// Get previous and next characters.
	charPrev := IthVisible(*doc, position-1)
	charNext := IthVisible(*doc, position)

	// Use defaults.
	if charPrev.ID == "-1" {
		charPrev = doc.Find("start")
	}
	if charNext.ID == "-1" {
		charNext = doc.Find("end")
	}

	char := Character{
		ID:         fmt.Sprint(SiteID) + fmt.Sprint(LocalClock),
		Visible:    true,
		Value:      value,
		IDPrevious: charPrev.ID,
		IDNext:     charNext.ID,
	}

	return doc.IntegrateInsert(char, charPrev, charNext)
}

// IntegrateDelete marks the given character for deletion.
func (doc *Document) IntegrateDelete(char Character) *Document {
	position := doc.Position(char.ID)
	if position == -1 {
		return doc
	}

	// This is how deletion is done.
	doc.Characters[position-1].Visible = false

	return doc
}

// GenerateDelete generates a delete operation for the given position.
func (doc *Document) GenerateDelete(position int) *Document {
	char := IthVisible(*doc, position)
	return doc.IntegrateDelete(char)
}

// Implement the CRDT interface

func (doc *Document) Insert(position int, value string) (string, error) {
	newDoc, err := doc.GenerateInsert(position, value)
	if err != nil {
		return Content(*doc), err
	}

	return Content(*newDoc), nil
}

func (doc *Document) Delete(position int) string {
	newDoc := doc.GenerateDelete(position)
	return Content(*newDoc)
}
