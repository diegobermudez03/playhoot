package api

import (
	"errors"
	"log"
	"net/http"
	"sync"

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
			log.Printf("api: writing WS message: %v", err)
			return
		}
	}
}

// handleWebSocket upgrades the connection, binds it to (session_uuid,
// user_uuid) - both supplied directly by the client as query parameters,
// trusted as-is - and runs its read pump until the connection closes.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	sessionUUID := r.URL.Query().Get("session_uuid")
	userUUID := r.URL.Query().Get("user_uuid")
	if sessionUUID == "" || userUUID == "" {
		http.Error(w, "session_uuid and user_uuid query parameters are required", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("api: WS upgrade failed: %v", err)
		return
	}

	wc := newWSConn(conn)
	unbind := s.coord.Bind(play.SessionUUID(sessionUUID), play.UserUUID(userUUID), wc)

	go wc.writePump()

	// Blocks until the client disconnects or sends an unreadable message.
	s.readPump(r, wc, play.SessionUUID(sessionUUID), play.UserUUID(userUUID))

	// unbind first, so no further Deliver call is accepted for this
	// connection, then stop the write pump, then close the socket - in
	// that order, so a Coordinator call already in flight from another
	// connection can only ever observe this connection as gone, never
	// panic against a closed channel.
	unbind()
	wc.close()
	conn.Close()
}

// readPump blocks reading inbound command messages until conn closes or
// sends an unreadable message, dispatching each to Coordinator.
func (s *Server) readPump(r *http.Request, wc *wsConn, sessionUUID play.SessionUUID, userUUID play.UserUUID) {
	ctx := r.Context()
	for {
		var msg inboundMessage
		if err := wc.conn.ReadJSON(&msg); err != nil {
			return
		}

		switch msg.Type {
		case inboundTypeStart:
			outcome, events, err := s.coord.Start(ctx, sessionUUID, userUUID, msg.IdempotencyKey)
			if err != nil {
				_ = wc.enqueue(outboundMessage{Type: outboundTypeError, Message: err.Error()})
				continue
			}
			// This connection's own direct reply is enqueued before the
			// fan-out, so it is always the first of the two this same
			// connection can see - Deliver has no way to know that
			// ordering matters, so api sequences it explicitly.
			_ = wc.enqueue(outboundMessage{Type: outboundTypeStartResult, Outcome: string(outcome)})
			s.coord.Deliver(sessionUUID, events)

		case inboundTypeAnswerInteraction:
			outcome, events, err := s.coord.AnswerInteraction(ctx, sessionUUID, play.InteractionUUID(msg.InteractionID), userUUID, []byte(msg.Answer))
			if err != nil {
				_ = wc.enqueue(outboundMessage{Type: outboundTypeError, Message: err.Error()})
				continue
			}
			_ = wc.enqueue(outboundMessage{Type: outboundTypeAnswerResult, Outcome: string(outcome), InteractionID: msg.InteractionID})
			s.coord.Deliver(sessionUUID, events)

		default:
			_ = wc.enqueue(outboundMessage{Type: outboundTypeError, Message: "unknown message type"})
		}
	}
}
