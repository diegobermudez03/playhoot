// Package livewire maps the Runtime's live messages to and from JSON text
// frames whose "type" field names the message.
package livewire

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

var (
	// ErrMalformedMessage means a frame is not a JSON object naming its type.
	ErrMalformedMessage = errors.New("malformed message")

	// ErrUnknownMessageType means the type is not in the catalog for this
	// direction.
	ErrUnknownMessageType = errors.New("unknown message type")
)

type envelope struct {
	Type string `json:"type"`
}

type rejectedMessage struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

// DecodeClientMessage reads one frame a client sent.
func DecodeClientMessage(data []byte) (runtime.ClientMessage, error) {
	var header envelope
	if err := json.Unmarshal(data, &header); err != nil || header.Type == "" {
		return nil, ErrMalformedMessage
	}
	// The client catalog is empty until its first message is needed.
	return nil, fmt.Errorf("%w: %q", ErrUnknownMessageType, header.Type)
}

// EncodeServerMessage writes one message for a client. runtime.Close is not
// encodable: it ends the connection instead of travelling as a message.
func EncodeServerMessage(message runtime.ServerMessage) ([]byte, error) {
	switch m := message.(type) {
	case runtime.Rejected:
		return json.Marshal(rejectedMessage{Type: "rejected", Reason: string(m.Reason)})
	default:
		return nil, fmt.Errorf("%w: %T", ErrUnknownMessageType, message)
	}
}
