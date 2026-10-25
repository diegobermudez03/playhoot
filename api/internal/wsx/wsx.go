// Package wsx is the API edge's WebSocket transport mechanics: upgrading a
// request into a live connection. It knows no domain.
package wsx

import (
	"errors"
	"net/http"
	"strings"

	"github.com/coder/websocket"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
)

// Subprotocol is the only WebSocket subprotocol live connections speak.
const Subprotocol = "playhoot.v1"

// MaxMessageBytes is the largest message a client may send on a live
// connection; each connection's owner enforces its own limits per message.
const MaxMessageBytes = 32 << 10

var errSubprotocolNotOffered = errors.New("client did not offer the " + Subprotocol + " subprotocol")

// OriginPatterns are the patterns Accept takes for origins, each an origin in
// canonical form (scheme://host[:port]) or "*" for any origin.
func OriginPatterns(origins []string) []string {
	escape := strings.NewReplacer(`\`, `\\`, "[", `\[`, "]", `\]`, "*", `\*`, "?", `\?`)
	patterns := make([]string, len(origins))
	for i, origin := range origins {
		patterns[i] = origin
		if origin != "*" {
			patterns[i] = escape.Replace(origin)
		}
	}
	return patterns
}

// Accept upgrades the request to a WebSocket speaking Subprotocol, from the
// server's own origin or one matching originPatterns. On error the request
// has already been answered.
func Accept(w http.ResponseWriter, r *http.Request, originPatterns []string) (*websocket.Conn, error) {
	if !offersSubprotocol(r) {
		httpx.WriteError(w, http.StatusBadRequest, "the "+Subprotocol+" WebSocket subprotocol is required")
		return nil, errSubprotocolNotOffered
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		Subprotocols:   []string{Subprotocol},
		OriginPatterns: originPatterns,
	})
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(MaxMessageBytes)
	return conn, nil
}

func offersSubprotocol(r *http.Request) bool {
	for _, header := range r.Header.Values("Sec-WebSocket-Protocol") {
		for _, protocol := range strings.Split(header, ",") {
			if strings.TrimSpace(protocol) == Subprotocol {
				return true
			}
		}
	}
	return false
}

// Abort closes conn as a server failure; it is a no-op on a closed conn.
func Abort(conn *websocket.Conn) {
	_ = conn.Close(websocket.StatusInternalError, "internal error")
}
