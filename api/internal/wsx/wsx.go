// Package wsx is the WebSocket plumbing shared by live routes: upgrading,
// serving a connection against the Runtime's channels, and handshake logs.
package wsx

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"github.com/coder/websocket"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/livewire"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

// Subprotocol is the only WebSocket subprotocol live connections speak.
const Subprotocol = "playhoot.v1"

var errSubprotocolNotOffered = errors.New("client did not offer the " + Subprotocol + " subprotocol")

// maxMessageBytes only protects the edge; the Runtime enforces its own
// limits per kind of message.
const maxMessageBytes = 32 << 10

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
	conn.SetReadLimit(maxMessageBytes)
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

// Serve moves messages between conn and connection until the connection
// ends, and returns once Inbound is closed. Frames that are not client
// messages end the connection without reaching the Runtime.
func Serve(ctx context.Context, conn *websocket.Conn, connection runtime.Connection) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		// Cancel before closing Inbound, so the Runtime closing Outbound in
		// response is not mistaken for an unexpected end.
		defer close(connection.Inbound)
		defer cancel()
		readClient(ctx, conn, connection.Inbound)
	}()

	writeClient(ctx, conn, connection.Outbound)
	cancel()
	<-clientDone
}

func readClient(ctx context.Context, conn *websocket.Conn, inbound chan<- runtime.ClientMessage) {
	for {
		messageType, data, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if messageType != websocket.MessageText {
			_ = conn.Close(websocket.StatusUnsupportedData, "only text messages are accepted")
			return
		}
		message, err := livewire.DecodeClientMessage(data)
		if errors.Is(err, livewire.ErrUnknownMessageType) {
			_ = conn.Close(websocket.StatusPolicyViolation, "unknown message type")
			return
		}
		if err != nil {
			_ = conn.Close(websocket.StatusInvalidFramePayloadData, "malformed message")
			return
		}
		select {
		case inbound <- message:
		case <-ctx.Done():
			return
		}
	}
}

func writeClient(ctx context.Context, conn *websocket.Conn, outbound <-chan runtime.ServerMessage) {
	for {
		select {
		case <-ctx.Done():
			return
		case message, ok := <-outbound:
			if !ok {
				if ctx.Err() == nil {
					logFailure(ctx, "runtime closed the connection without a close message")
					Abort(conn)
				}
				return
			}
			if closing, isClose := message.(runtime.Close); isClose {
				_ = conn.Close(closeStatus(closing.Reason), string(closing.Reason))
				return
			}
			data, err := livewire.EncodeServerMessage(message)
			if err != nil {
				logFailure(ctx, err.Error())
				Abort(conn)
				return
			}
			if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
				return
			}
		}
	}
}

func closeStatus(reason runtime.CloseReason) websocket.StatusCode {
	switch reason {
	case runtime.CloseAdmissionRejected:
		return websocket.StatusPolicyViolation
	default:
		return websocket.StatusInternalError
	}
}

// logFailure logs a server-side failure after the handshake log was flushed,
// sharing the connection's trace.
func logFailure(ctx context.Context, failure string) {
	failureCtx := logging.Start(ctx)
	logging.LogFields(failureCtx, logging.Field("connection_failure", failure))
	logging.FinishRequestLog(failureCtx, slog.Default(), "live connection failure")
}

// Abort closes conn as a server failure; it is a no-op on a closed conn.
func Abort(conn *websocket.Conn) {
	_ = conn.Close(websocket.StatusInternalError, "internal error")
}

type handshakeContextKey struct{}

type handshakeLog struct {
	once    sync.Once
	ctx     context.Context
	message string
}

// StartHandshake starts the handshake's own log as a child span of ctx.
func StartHandshake(ctx context.Context, pattern string) context.Context {
	handshakeCtx := logging.Start(ctx)
	return context.WithValue(handshakeCtx, handshakeContextKey{}, &handshakeLog{ctx: handshakeCtx, message: pattern + " handshake"})
}

// FinishHandshake flushes the handshake log; only the first call does.
func FinishHandshake(ctx context.Context) {
	handshake, ok := ctx.Value(handshakeContextKey{}).(*handshakeLog)
	if !ok {
		return
	}
	handshake.once.Do(func() {
		logging.FinishRequestLog(handshake.ctx, slog.Default(), handshake.message)
	})
}
