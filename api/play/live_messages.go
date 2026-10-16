package play

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

// Live messages travel as JSON text frames whose "type" field names them.
var (
	// errMalformedMessage means a frame is not a JSON object naming its type.
	errMalformedMessage = errors.New("malformed message")

	// errUnknownMessageType means the type is not in the catalog for this
	// direction.
	errUnknownMessageType = errors.New("unknown message type")
)

type envelope struct {
	Type string `json:"type"`
}

type rejectedMessage struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

// decodeClientMessage reads one frame a client sent.
func decodeClientMessage(data []byte) (runtime.ClientMessage, error) {
	var header envelope
	if err := json.Unmarshal(data, &header); err != nil || header.Type == "" {
		return nil, errMalformedMessage
	}
	// The client catalog is empty until its first message is needed.
	return nil, fmt.Errorf("%w: %q", errUnknownMessageType, header.Type)
}

// encodeServerMessage writes one message for a client. runtime.Close is not
// encodable: it ends the connection instead of travelling as a message.
func encodeServerMessage(message runtime.ServerMessage) ([]byte, error) {
	switch m := message.(type) {
	case runtime.Rejected:
		return json.Marshal(rejectedMessage{Type: "rejected", Reason: string(m.Reason)})
	default:
		return nil, fmt.Errorf("%w: %T", errUnknownMessageType, message)
	}
}
