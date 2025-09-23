package types

type MessageType string

const (
	MessageTypeData  MessageType = "data"
	MessageTypeClose MessageType = "close"
)

type Message struct {
	Type       MessageType `json:"type"`
	StreamID   string      `json:"stream_id"`
	SourceAddr string      `json:"source_addr"`
	Data       []byte      `json:"data,omitempty"`
}
