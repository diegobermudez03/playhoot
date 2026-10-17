// Package live runs the API edge's live connections: it owns their goroutines
// so they can be ended on shutdown, drives each connection's loops, and reads
// and writes its JSON messages. It knows no domain.
package live

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/coder/websocket"

	"github.com/diegobermudez03/playhoot/api/internal/wsx"
	"github.com/diegobermudez03/playhoot/logging"
)

// Cause is why a served connection ended, as logged when it closes.
type Cause string

const (
	CauseClientClosed       Cause = "client_closed"
	CauseBinaryFrame        Cause = "binary_frame"
	CauseMalformedMessage   Cause = "malformed_message"
	CauseUnknownMessageType Cause = "unknown_message_type"
	CauseEndedByOwner       Cause = "ended_by_owner"
	CauseServerShutdown     Cause = "server_shutdown"
	CauseServerFailure      Cause = "server_failure"
	CauseWriteFailed        Cause = "write_failed"
)

// End is how a served connection ended.
type End struct {
	Cause Cause
	// Detail is the owner's end reason or the server failure.
	Detail string
}

// Loop runs one direction of a live connection until ctx is done or it
// ends the connection itself, which it reports with ended.
type Loop func(ctx context.Context) (end End, ended bool)

type loopResult struct {
	end   End
	ended bool
}

// Serve runs loops on conn until one ends the connection or shutdown is
// cancelled, then stops the rest, calls disconnect, and logs the end under
// ctx's trace. ctx must not be the request's context, which ends at the
// upgrade.
func Serve(ctx context.Context, shutdown context.Context, conn *websocket.Conn, disconnect func(), loops ...Loop) {
	started := time.Now()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	results := make(chan loopResult, len(loops))
	for _, loop := range loops {
		go func() {
			end, ended := loop(ctx)
			results <- loopResult{end: end, ended: ended}
		}()
	}

	end, decided, pending := End{}, false, len(loops)
	for !decided && pending > 0 {
		select {
		case result := <-results:
			pending--
			end, decided = result.end, result.ended
		case <-shutdown.Done():
			_ = conn.Close(websocket.StatusGoingAway, "server shutting down")
			end, decided = End{Cause: CauseServerShutdown}, true
		}
	}
	if !decided {
		end = End{Cause: CauseServerFailure, Detail: "every loop stopped without ending the connection"}
		wsx.Abort(conn)
	}

	cancel()
	disconnect()
	// Releases the socket if the ending loop did not close it; a no-op otherwise.
	_ = conn.CloseNow()
	for ; pending > 0; pending-- {
		<-results
	}
	logConnectionEnd(ctx, end, time.Since(started))
}

// ReadMessage reads the client's next message and its type. When the client
// is gone, or the frame cannot be a message, it closes conn as needed and
// reports the end; ended is false when ctx was cancelled instead.
func ReadMessage(ctx context.Context, conn *websocket.Conn) (messageType string, data []byte, end End, ended bool) {
	frameType, data, err := conn.Read(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return "", nil, End{}, false
		}
		return "", nil, End{Cause: CauseClientClosed}, true
	}
	if frameType != websocket.MessageText {
		_ = conn.Close(websocket.StatusUnsupportedData, "only text messages are accepted")
		return "", nil, End{Cause: CauseBinaryFrame}, true
	}
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || envelope.Type == "" {
		return "", nil, CloseMalformed(conn), true
	}
	return envelope.Type, data, End{}, false
}

// CloseMalformed closes conn because the client sent an invalid message.
func CloseMalformed(conn *websocket.Conn) End {
	_ = conn.Close(websocket.StatusInvalidFramePayloadData, "malformed message")
	return End{Cause: CauseMalformedMessage}
}

// CloseUnknownType closes conn because the client sent a message type the
// connection does not accept.
func CloseUnknownType(conn *websocket.Conn, messageType string) End {
	_ = conn.Close(websocket.StatusPolicyViolation, "unknown message type")
	return End{Cause: CauseUnknownMessageType, Detail: messageType}
}

// CloseServerFailure closes conn because the server could not go on.
func CloseServerFailure(conn *websocket.Conn, err error) End {
	wsx.Abort(conn)
	return End{Cause: CauseServerFailure, Detail: err.Error()}
}

// WriteMessage sends message as a JSON text frame. ended is false when ctx was
// cancelled instead.
func WriteMessage(ctx context.Context, conn *websocket.Conn, message any) (End, bool) {
	data, err := json.Marshal(message)
	if err != nil {
		return CloseServerFailure(conn, err), true
	}
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		if ctx.Err() != nil {
			return End{}, false
		}
		return End{Cause: CauseWriteFailed}, true
	}
	return End{}, false
}

func logConnectionEnd(ctx context.Context, end End, duration time.Duration) {
	endCtx := logging.Start(ctx)
	logging.LogFields(endCtx,
		logging.Field("close_cause", string(end.Cause)),
		logging.Field("connection_duration_ms", duration.Milliseconds()),
	)
	if end.Detail != "" {
		logging.LogFields(endCtx, logging.Field("close_detail", end.Detail))
	}
	logging.FinishRequestLog(endCtx, slog.Default(), "live connection closed")
}
