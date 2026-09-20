package session

import (
	"context"
	"errors"
	"log"
	"log/slog"
	"net/http"
	"sync"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play"
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

// wsConn adapts one upgraded *websocket.Conn to play.Conn. A gorilla
// websocket.Conn supports exactly one concurrent writer, so every outbound
// write - both fanned-out Events (via Deliver) and this same connection's
// own command results/errors (via enqueue) - goes through one buffered
// channel drained by a single write-pump goroutine, never conn.WriteJSON
// directly from more than one goroutine.
type wsConn struct {
	conn *websocket.Conn
	send chan outboundMessage

	mu     sync.Mutex
	closed bool
}

func newWSConn(conn *websocket.Conn) *wsConn {
	return &wsConn{conn: conn, send: make(chan outboundMessage, sendBufferSize)}
}

// Deliver satisfies play.Conn, called from whichever connection's own
// read-pump goroutine drove the committing call - never this connection's.
func (c *wsConn) Deliver(e play.Event) error {
	msg := outboundMessage{InteractionID: string(e.InteractionID)}
	switch e.Kind {
	case play.EventKindInteractionOpened:
		msg.Type = outboundTypeInteractionOpened
		msg.Question = e.Question
		msg.Arguments = e.Arguments
	case play.EventKindInteractionClosed:
		msg.Type = outboundTypeInteractionClosed
	default:
		return nil
	}
	return c.enqueue(msg)
}

// enqueue buffers msg for the write pump, guarded against a concurrent
// close (this connection's own read pump exiting while another
// connection's Coordinator call is still fanning an Event out to it).
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
			log.Printf("api/session: writing WS message: %v", err)
			return
		}
	}
}

// handleWebSocket upgrades the connection, binds it to (session_uuid,
// user_uuid) - both supplied directly by the client as query parameters,
// trusted as-is - and runs its read pump until the connection closes.
func (h *Handler) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	sessionUUID := r.URL.Query().Get("session_uuid")
	userUUID := r.URL.Query().Get("user_uuid")
	if sessionUUID == "" || userUUID == "" {
		http.Error(w, "session_uuid and user_uuid query parameters are required", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("api/session: WS upgrade failed: %v", err)
		return
	}

	wc := newWSConn(conn)
	unbind := h.coord.Bind(play.SessionUUID(sessionUUID), play.UserUUID(userUUID), wc)
	logConnectionEvent(r.Context(), sessionUUID, userUUID, "api.session.WebSocketConnected")

	go wc.writePump()

	// Blocks until the client disconnects or sends an unreadable message.
	h.readPump(r, wc, play.SessionUUID(sessionUUID), play.UserUUID(userUUID))

	// unbind first, so no further Deliver call is accepted for this
	// connection, then stop the write pump, then close the socket - in
	// that order, so a Coordinator call already in flight from another
	// connection can only ever observe this connection as gone, never
	// panic against a closed channel.
	unbind()
	wc.close()
	conn.Close()
	logConnectionEvent(r.Context(), sessionUUID, userUUID, "api.session.WebSocketDisconnected")
}

// logConnectionEvent records one connection-lifecycle event (as opposed to
// a per-message exchange) as its own single-line structured log entry.
func logConnectionEvent(ctx context.Context, sessionUUID, userUUID, message string) {
	ctx = logging.Start(ctx)
	logging.LogFields(ctx,
		logging.Field("session_uuid", sessionUUID),
		logging.Field("user_uuid", userUUID),
	)
	logging.FinishRequestLog(ctx, slog.Default(), message)
}

// readPump blocks reading inbound command messages until conn closes or
// sends an unreadable message, dispatching each to Coordinator.
func (h *Handler) readPump(r *http.Request, wc *wsConn, sessionUUID play.SessionUUID, userUUID play.UserUUID) {
	for {
		var msg inboundMessage
		if err := wc.conn.ReadJSON(&msg); err != nil {
			return
		}
		h.handleInboundMessage(r.Context(), wc, sessionUUID, userUUID, msg)
	}
}

// handleInboundMessage dispatches one decoded WS command and logs it as its
// own single request-scoped entry, covering the message received through
// to the direct reply sent back on this same connection - a long-lived
// connection carries many such entries, one per exchange, rather than one
// entry for the connection's entire lifetime.
func (h *Handler) handleInboundMessage(reqCtx context.Context, wc *wsConn, sessionUUID play.SessionUUID, userUUID play.UserUUID, msg inboundMessage) {
	ctx := logging.Start(reqCtx)
	defer logging.FinishRequestLog(ctx, slog.Default(), "api.session.WSMessage")

	logging.LogFields(ctx,
		logging.Field("session_uuid", string(sessionUUID)),
		logging.Field("user_uuid", string(userUUID)),
		logging.Field("message_type", msg.Type),
	)

	switch msg.Type {
	case inboundTypeStart:
		outcome, events, err := h.coord.Start(ctx, sessionUUID, userUUID, msg.IdempotencyKey)
		if err != nil {
			logging.LogError(ctx, err)
			_ = wc.enqueue(outboundMessage{Type: outboundTypeError, Message: err.Error()})
			return
		}
		logging.LogFields(ctx, logging.Field("outcome", string(outcome)))
		// This connection's own direct reply is enqueued before the
		// fan-out, so it is always the first of the two this same
		// connection can see - Deliver has no way to know that
		// ordering matters, so this handler sequences it explicitly.
		_ = wc.enqueue(outboundMessage{Type: outboundTypeStartResult, Outcome: string(outcome)})
		h.coord.Deliver(sessionUUID, events)

	case inboundTypeAnswerInteraction:
		logging.LogFields(ctx, logging.Field("interaction_id", msg.InteractionID))
		outcome, events, err := h.coord.AnswerInteraction(ctx, sessionUUID, play.InteractionUUID(msg.InteractionID), userUUID, []byte(msg.Answer))
		if err != nil {
			logging.LogError(ctx, err)
			_ = wc.enqueue(outboundMessage{Type: outboundTypeError, Message: err.Error()})
			return
		}
		logging.LogFields(ctx, logging.Field("outcome", string(outcome)))
		_ = wc.enqueue(outboundMessage{Type: outboundTypeAnswerResult, Outcome: string(outcome), InteractionID: msg.InteractionID})
		h.coord.Deliver(sessionUUID, events)

	default:
		logging.LogFields(ctx, logging.Field("error", "unknown message type"))
		_ = wc.enqueue(outboundMessage{Type: outboundTypeError, Message: "unknown message type"})
	}
}
