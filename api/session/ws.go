package session

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"sync"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/gorilla/websocket"
)

// sendBufferSize bounds a connection's outbound message buffer. A full
// buffer means the client is not reading fast enough; that delivery
// failure is only ever logged here, never retried/queued further or fed
// back into Session Runtime.
const sendBufferSize = 16

var (
	errSendBufferFull = errors.New("send buffer full")
	errConnClosed     = errors.New("connection closed")
)

var upgrader = websocket.Upgrader{
	// No cross-origin restriction: no browser-facing deployment/CORS
	// policy is defined yet, and no independent credential verification
	// happens anywhere in this path either.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// wsConn owns one upgraded *websocket.Conn. A gorilla websocket.Conn
// supports exactly one concurrent writer, so every outbound write goes
// through one buffered channel drained by a single write-pump goroutine,
// never conn.WriteJSON directly from more than one goroutine. Today the
// only writer is this connection's own read pump (there is no fan-out
// mechanism yet - see this package's doc comment); the buffered-channel/
// single-writer shape is kept because a rebuilt fan-out mechanism will
// need it again, not because anything currently writes concurrently.
type wsConn struct {
	conn *websocket.Conn
	send chan outboundMessage

	mu     sync.Mutex
	closed bool
}

func newWSConn(conn *websocket.Conn) *wsConn {
	return &wsConn{conn: conn, send: make(chan outboundMessage, sendBufferSize)}
}

// enqueue buffers msg for the write pump, guarded against a concurrent
// close.
func (c *wsConn) enqueue(msg outboundMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errConnClosed
	}
	select {
	case c.send <- msg:
		return nil
	default:
		return errSendBufferFull
	}
}

// close stops accepting further sends and closes send, stopping the write
// pump. Safe to call more than once.
func (c *wsConn) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	close(c.send)
}

// writePump drains send and writes each message to the socket, until send
// is closed. The one goroutine ever allowed to call conn.WriteJSON.
func (c *wsConn) writePump() {
	for msg := range c.send {
		if err := c.conn.WriteJSON(msg); err != nil {
			logging.LogStandalone(slog.Default(), []string{"websocket", "session"}, "api.session.WSWriteFailed",
				logging.Field("error", err.Error()),
			)
			return
		}
	}
}

// handleWebSocket performs the join-and-upgrade handshake as its own
// logged step, then - only once that succeeded - runs the read pump for
// the rest of the connection's life. It logs nothing about the
// connection's own open/close lifecycle itself - Server's wsObservability
// wrapper already logs that generically, sharing one trace ID with every
// log this handler produces (see server.go's doc comment).
func (h *Handler) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	wc, sessionUUID, userUUID, ok := h.joinAndUpgrade(w, r)
	if !ok {
		return
	}

	go wc.writePump()

	// Blocks until the client disconnects or sends an unreadable message.
	h.readPump(r, wc, sessionUUID, userUUID)

	wc.close()
	wc.conn.Close()
}

// joinAndUpgrade parses the join handshake's query parameters and
// upgrades the connection. It does not call any domain package today -
// there is no Join to perform yet (see this package's doc comment) - so
// it always proceeds to upgrade once the query parses; a client can still
// open a real WebSocket connection through this path, which is the
// mechanic being deliberately kept working. Sending back a real
// JOIN_RESULT, and declining a request that does not actually result in
// an active Participant, returns once a real Join call exists again.
func (h *Handler) joinAndUpgrade(w http.ResponseWriter, r *http.Request) (wc *wsConn, sessionUUID, userUUID string, ok bool) {
	ctx := logging.Start(r.Context())
	defer logging.FinishRequestLog(ctx, slog.Default(), "api.session.WebSocketJoin")

	joinCode, userUUIDStr, displayName, idempotencyKey, valid := parseJoinQuery(r)
	if !valid {
		httpx.WriteError(w, http.StatusBadRequest, "join_code, user_uuid, display_name, and idempotency_key query parameters are required, and join_code must be a number")
		return nil, "", "", false
	}
	logging.LogFields(ctx,
		logging.Field("join_code", joinCode),
		logging.Field("user_uuid", userUUIDStr),
		logging.Field("display_name", displayName),
	)

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logging.LogError(ctx, err)
		return nil, "", "", false
	}

	newConn := newWSConn(conn)
	_ = newConn.enqueue(outboundMessage{Type: outboundTypeJoinResult, Outcome: "NOT_IMPLEMENTED"})
	_ = idempotencyKey

	return newConn, "", userUUIDStr, true
}

// parseJoinQuery reads and validates GET /ws's required query parameters.
func parseJoinQuery(r *http.Request) (joinCode uint, userUUID, displayName, idempotencyKey string, ok bool) {
	query := r.URL.Query()
	userUUID = query.Get("user_uuid")
	displayName = query.Get("display_name")
	idempotencyKey = query.Get("idempotency_key")
	if userUUID == "" || displayName == "" || idempotencyKey == "" {
		return 0, "", "", "", false
	}
	parsed, err := strconv.ParseUint(query.Get("join_code"), 10, 64)
	if err != nil {
		return 0, "", "", "", false
	}
	return uint(parsed), userUUID, displayName, idempotencyKey, true
}

// readPump blocks reading inbound command messages until conn closes or
// sends an unreadable message, dispatching each to handleInboundMessage.
func (h *Handler) readPump(r *http.Request, wc *wsConn, sessionUUID, userUUID string) {
	for {
		var msg inboundMessage
		if err := wc.conn.ReadJSON(&msg); err != nil {
			return
		}
		h.handleInboundMessage(r.Context(), wc, sessionUUID, userUUID, msg)
	}
}

// handleInboundMessage logs one decoded WS command as its own single
// request-scoped entry. No message type has a real implementation yet -
// every message currently answers ERROR/"unknown message type" (see this
// package's doc comment) - message-specific dispatch depended entirely on
// the now-removed play.Coordinator and returns once it is rebuilt.
func (h *Handler) handleInboundMessage(reqCtx context.Context, wc *wsConn, sessionUUID, userUUID string, msg inboundMessage) {
	ctx := logging.Start(reqCtx)
	defer logging.FinishRequestLog(ctx, slog.Default(), "api.session.WSMessage")

	logging.LogFields(ctx,
		logging.Field("session_uuid", sessionUUID),
		logging.Field("user_uuid", userUUID),
		logging.Field("message_type", msg.Type),
	)

	logging.LogFields(ctx, logging.Field("error", "unknown message type"))
	_ = wc.enqueue(outboundMessage{Type: outboundTypeError, Message: "unknown message type"})
}
