package crdt

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
)

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

	SiteID = 0

	LocalClock = 0

	StartChar = Character{ID: "start", Visible: false, Value: "", IDPrevious: "", IDNext: "end"}

	EndChar = Character{ID: "end", Visible: false, Value: "", IDPrevious: "start", IDNext: ""}

	ErrPosOutOfRange    = errors.New("position out of range")
	ErrEmptyCharacter   = errors.New("empty character ID")
	ErrBoundsNotPresent = errors.New("subsequence bound(s) not present")
)

func New() Document {
	return Document{Characters: []Character{StartChar, EndChar}}
}

func Load(fileName string) (Document, error) {
	doc := New()

	content, err := os.ReadFile(fileName)

	if err != nil {
		return doc, err
	}

	lines := strings.Split(string(content), "\n")
	pos := 1
	for i, line := range lines {
		for _, character := range line {
			_, err = doc.Insert(pos, string(character))
			if err != nil {
				return doc, err
			}

			pos++
		}
		if i < len(lines)-1 {
			_, err := doc.Insert(pos, "\n")
			if err != nil {
				return doc, err
			}
		}
	}
	return doc, err
}

func Save(fileName string, doc *Document) error {
	return os.WriteFile(fileName, []byte(Content(*doc)), 0644)
}

// Utils

func (doc *Document) SetText(newDoc Document) {
	for _, char := range newDoc.Characters {
		c := Character{ID: char.ID, Visible: char.Visible, Value: char.Value, IDPrevious: char.ID, IDNext: char.ID}
		doc.Characters = append(doc.Characters, c)
	}
}

func Content(doc Document) string {
	value := ""

	for _, char := range doc.Characters {
		if char.Visible {
			value += char.Value
		}
	}
	return value
}

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

func (doc *Document) Length() int {
	return len(doc.Characters)
}

func (doc *Document) ElementAt(position int) (Character, error) {
	if position < 0 || position >= doc.Length() {
		return Character{}, ErrPosOutOfRange
	}

	return doc.Characters[position], nil
}

func (doc *Document) Position(charID string) int {
	for i, char := range doc.Characters {
		if char.ID == charID {
			return i + 1
		}
	}
	return -1
}

func (doc *Document) Left(charID string) string {
	i := doc.Position(charID)

	if i <= 0 {
		return doc.Characters[i].ID
	}
	return doc.Characters[i-1].ID
}

func (doc *Document) Right(charID string) string {
	i := doc.Position(charID)
	if i >= len(doc.Characters)-1 {
		return doc.Characters[i-1].ID
	}
	return doc.Characters[i+1].ID
}

func (doc *Document) Contains(charID string) bool {
	position := doc.Position(charID)
	return position != -1
}

func (doc *Document) Find(id string) Character {
	for _, char := range doc.Characters {
		if char.ID == id {
			return char
		}
	}

	return Character{ID: "-1"}
}

func (doc *Document) Subsequence(characterStart Character, characterEnd Character) ([]Character, error) {

	startPos := doc.Position(characterStart.ID)
	endPos := doc.Position(characterEnd.ID)

	if startPos == -1 || endPos == -1 {
		return doc.Characters, ErrBoundsNotPresent
	}

	if startPos > endPos {
		return doc.Characters, ErrBoundsNotPresent
	}

	if startPos == endPos {
		return []Character{}, nil
	}

	return doc.Characters[startPos : endPos-1], nil
}

// Operations

func (doc *Document) InsertLocal(char Character, position int) (*Document, error) {

	if position <= 0 || position >= doc.Length() {
		return doc, ErrPosOutOfRange
	}

	if char.ID == "" {
		return doc, ErrEmptyCharacter
	}
	// Insert character between [:position] and [position:]
	before := doc.Characters[:position]
	after := doc.Characters[position:]

	inserted := []Character{char}

	before = append(before, inserted...)

	doc.Characters = append(before, after...)

	doc.Characters[position-1].IDNext = char.ID
	doc.Characters[position+1].IDPrevious = char.ID

	return doc, nil
}

func (doc *Document) IntegrateInsert(char, charPrev, charNext Character) (*Document, error) {

	subsequence, err := doc.Subsequence(charPrev, charNext)
	if err != nil {
		return doc, err
	}

	position := doc.Position(charNext.ID)
	position--

	if len(subsequence) == 0 {
		return doc.InsertLocal(char, position)
	}

	if len(subsequence) == 1 {
		return doc.InsertLocal(char, position-1)
	}

	i := 1

	for i < len(subsequence)-1 && subsequence[i].ID < char.ID {
		i++
	}
	return doc.IntegrateInsert(char, subsequence[i-1], subsequence[i])
}

func (doc *Document) GenerateInsert(position int, value string) (*Document, error) {
	mu.Lock()
	LocalClock++
	mu.Unlock()

	charPrev := IthVisible(*doc, position-1)
	charNext := IthVisible(*doc, position)

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

func (doc *Document) IntegrateDelete(char Character) *Document {
	position := doc.Position(char.ID)

	if position == -1 {
		return doc
	}

	doc.Characters[position-1].Visible = false

	return doc

}

func (doc *Document) GenerateDelete(position int) *Document {
	char := IthVisible(*doc, position)

	return doc.IntegrateDelete(char)

}

// Implement CRDT interface

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
