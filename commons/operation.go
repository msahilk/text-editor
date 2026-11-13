package commons

import "text-editor/crdt"

type Operation struct {
	Type string `json:"type"`

	// Position and Value describe the edit for logging/debugging only.
	// Remote replicas integrate Char, never the position, because positions
	// shift when users edit concurrently.
	Position int `json:"position"`

	Value string `json:"value"`

	// Char is the CRDT character that was inserted or deleted.
	Char crdt.Character `json:"char"`
}
