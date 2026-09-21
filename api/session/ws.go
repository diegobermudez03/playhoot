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
//
// This goroutine has no owning request context - it drains sends that can
// originate from another connection's Coordinator.Deliver call as easily
// as from this connection's own read pump - so a write failure is logged
// standalone rather than through the per-request logging.Start/Step chain.
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

// handleWebSocket performs Join-and-upgrade as its own logged step, then -
// only once that succeeded - runs the read pump for the rest of the
// connection's life. Join-and-upgrade must flush its own log as soon as it
// completes, not when the connection eventually closes: readPump blocks
// for as long as the client stays connected, which would otherwise delay
// the join log far past when it actually happened.
func (h *Handler) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	wc, sessionUUID, userUUID, unbind, ok := h.joinAndUpgrade(w, r)
	if !ok {
		return
	}

	go wc.writePump()

	// Blocks until the client disconnects or sends an unreadable message.
	h.readPump(r, wc, sessionUUID, userUUID)

	// unbind first, so no further Deliver call is accepted for this
	// connection, then stop the write pump, then close the socket - in
	// that order, so a Coordinator call already in flight from another
	// connection can only ever observe this connection as gone, never
	// panic against a closed channel.
	unbind()
	wc.close()
	wc.conn.Close()
	logConnectionEvent(r.Context(), string(sessionUUID), string(userUUID), "api.session.WebSocketDisconnected")
}

// joinAndUpgrade performs Join and, only when it results in an active
// Participant (a fresh JOINED, or an idempotent-replay-shaped
// ALREADY_JOINED - the same reconnect path a client retrying with a new
// idempotency token after a drop takes), upgrades the connection and
// binds it - one client-facing operation with one outcome, never a Join
// that silently leaves the caller unconnected or a connection bound to a
// Session the caller never actually joined. Logged and flushed as its own
// single entry, separate from the connection's subsequent per-message log
// entries.
//
// A decline (LOBBY_EXPIRED, LOBBY_FULL) or an error is reported over
// plain HTTP without ever upgrading, since nothing worth connecting for
// happened. Bind requires an actual live socket, so upgrading has to
// follow a successful Join, not precede it - Join is validated first, the
// socket is only opened once it is known to be worth opening.
func (h *Handler) joinAndUpgrade(w http.ResponseWriter, r *http.Request) (wc *wsConn, sessionUUID play.SessionUUID, userUUID play.UserUUID, unbind func(), ok bool) {
	ctx := logging.Start(r.Context())
	defer logging.FinishRequestLog(ctx, slog.Default(), "api.session.WebSocketJoin")

	joinCode, userUUIDStr, displayName, idempotencyKey, valid := parseJoinQuery(r)
	if !valid {
		httpx.WriteError(w, http.StatusBadRequest, "join_code, user_uuid, display_name, and idempotency_key query parameters are required, and join_code must be a number")
		return nil, "", "", nil, false
	}
	logging.LogFields(ctx,
		logging.Field("join_code", joinCode),
		logging.Field("user_uuid", userUUIDStr),
	)

	result, err := h.coord.Join(ctx, joinCode, userUUIDStr, displayName, idempotencyKey)
	if err != nil {
		logging.LogError(ctx, err)
		writeDomainError(w, err)
		return nil, "", "", nil, false
	}
	logging.LogFields(ctx, logging.Field("outcome", string(result.Outcome)))
	if result.Outcome != play.JoinOutcomeJoined && result.Outcome != play.JoinOutcomeAlreadyJoined {
		httpx.WriteJSON(w, http.StatusOK, joinDeclineResponse{Outcome: string(result.Outcome)})
		return nil, "", "", nil, false
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		logging.LogError(ctx, err)
		return nil, "", "", nil, false
	}

	newConn := newWSConn(conn)
	unbind = h.coord.Bind(result.SessionUUID, play.UserUUID(userUUIDStr), newConn)
	_ = newConn.enqueue(outboundMessage{Type: outboundTypeJoinResult, Outcome: string(result.Outcome), SessionUUID: string(result.SessionUUID)})

	return newConn, result.SessionUUID, play.UserUUID(userUUIDStr), unbind, true
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
