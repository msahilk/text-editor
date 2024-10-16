package commons

import (
	"github.com/google/uuid"
	"text-editor/crdt"
)

type Message struct {
	Username string `json:"username"`

	Text string `json:"text"`

	Type MessageType `json:"type"`

	ID uuid.UUID `json:"ID"`

	Operation Operation `json:"operation"`

	Document crdt.Document `json:"document"`
}

type MessageType string

const (
	DocSyncMessage MessageType = "docSync"
	DocReqMessage  MessageType = "docReq"
	SiteIDMessage  MessageType = "SiteID"
	JoinMessage    MessageType = "join"
	UsersMessage   MessageType = "users"
)
