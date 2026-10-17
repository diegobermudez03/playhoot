package play

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/coder/websocket"

	"github.com/diegobermudez03/playhoot/api/internal/wsx"
	"github.com/diegobermudez03/playhoot/logging"
)

// closeCause is why a served connection ended, as logged when it closes.
type closeCause string

const (
	causeClientClosed       closeCause = "client_closed"
	causeBinaryFrame        closeCause = "binary_frame"
	causeMalformedMessage   closeCause = "malformed_message"
	causeUnknownMessageType closeCause = "unknown_message_type"
	causeRuntimeEnded       closeCause = "runtime_ended"
	causeServerShutdown     closeCause = "server_shutdown"
	causeServerFailure      closeCause = "server_failure"
	causeWriteFailed        closeCause = "write_failed"
)

// connectionEnd is how a served connection ended.
type connectionEnd struct {
	cause closeCause
	// detail is the Runtime's end reason or the server failure.
	detail string
}

// liveLoop runs one direction of a live connection until ctx is done or it
// ends the connection itself, which it reports with ended.
type liveLoop func(ctx context.Context) (end connectionEnd, ended bool)

type loopResult struct {
	end   connectionEnd
	ended bool
}

// serveLive runs loops on conn until one ends the connection or shutdown is
// cancelled, then stops the rest, calls disconnect, and logs the end under
// ctx's trace. ctx must not be the request's context, which ends at the
// upgrade.
func serveLive(ctx context.Context, shutdown context.Context, conn *websocket.Conn, disconnect func(), loops ...liveLoop) {
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

	end, decided, pending := connectionEnd{}, false, len(loops)
	for !decided && pending > 0 {
		select {
		case result := <-results:
			pending--
			end, decided = result.end, result.ended
		case <-shutdown.Done():
			_ = conn.Close(websocket.StatusGoingAway, "server shutting down")
			end, decided = connectionEnd{cause: causeServerShutdown}, true
		}
	}
	if !decided {
		end = connectionEnd{cause: causeServerFailure, detail: "every loop stopped without ending the connection"}
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

// readMessage reads the client's next message and its type. When the client
// is gone, or the frame cannot be a message, it closes conn as needed and
// reports the end; ended is false when ctx was cancelled instead.
func readMessage(ctx context.Context, conn *websocket.Conn) (messageType string, data []byte, end connectionEnd, ended bool) {
	frameType, data, err := conn.Read(ctx)
	if err != nil {
		if ctx.Err() != nil {
			return "", nil, connectionEnd{}, false
		}
		return "", nil, connectionEnd{cause: causeClientClosed}, true
	}
	if frameType != websocket.MessageText {
		_ = conn.Close(websocket.StatusUnsupportedData, "only text messages are accepted")
		return "", nil, connectionEnd{cause: causeBinaryFrame}, true
	}
	var envelope struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil || envelope.Type == "" {
		return "", nil, closeMalformed(conn), true
	}
	return envelope.Type, data, connectionEnd{}, false
}

func closeMalformed(conn *websocket.Conn) connectionEnd {
	_ = conn.Close(websocket.StatusInvalidFramePayloadData, "malformed message")
	return connectionEnd{cause: causeMalformedMessage}
}

func closeUnknownType(conn *websocket.Conn, messageType string) connectionEnd {
	_ = conn.Close(websocket.StatusPolicyViolation, "unknown message type")
	return connectionEnd{cause: causeUnknownMessageType, detail: messageType}
}

func closeServerFailure(conn *websocket.Conn, err error) connectionEnd {
	wsx.Abort(conn)
	return connectionEnd{cause: causeServerFailure, detail: err.Error()}
}

// writeMessage sends message as a JSON text frame. ended is false when ctx was
// cancelled instead.
func writeMessage(ctx context.Context, conn *websocket.Conn, message any) (connectionEnd, bool) {
	data, err := json.Marshal(message)
	if err != nil {
		return closeServerFailure(conn, err), true
	}
	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		if ctx.Err() != nil {
			return connectionEnd{}, false
		}
		return connectionEnd{cause: causeWriteFailed}, true
	}
	return connectionEnd{}, false
}

func logConnectionEnd(ctx context.Context, end connectionEnd, duration time.Duration) {
	endCtx := logging.Start(ctx)
	logging.LogFields(endCtx,
		logging.Field("close_cause", string(end.cause)),
		logging.Field("connection_duration_ms", duration.Milliseconds()),
	)
	if end.detail != "" {
		logging.LogFields(endCtx, logging.Field("close_detail", end.detail))
	}
	logging.FinishRequestLog(endCtx, slog.Default(), "live connection closed")
}
