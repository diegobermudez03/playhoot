// Package live runs the API edge's live connections: it owns their goroutines
// so they can be ended on shutdown, drives each connection's loops, reads and
// writes its JSON messages, and measures them. It knows no domain.
package live

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
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
	CauseInvalidGameData    Cause = "invalid_game_data"
	CauseUnknownMessageType Cause = "unknown_message_type"
	CauseRateLimited        Cause = "rate_limited"
	CauseEndedByOwner       Cause = "ended_by_owner"
	CauseServerShutdown     Cause = "server_shutdown"
	CauseServerFailure      Cause = "server_failure"
	CauseWriteFailed        Cause = "write_failed"
	CauseSlowConsumer       Cause = "slow_consumer"
	CauseUnresponsive       Cause = "unresponsive"
)

// writeTimeout bounds how long one message may take to reach a client; a
// client that cannot take it in that time is too slow to keep.
var writeTimeout = 10 * time.Second

// pingInterval is how often an idle-looking client is asked to prove it is
// still there, and pongTimeout how long it has to answer, so a half-open
// connection is noticed instead of held.
var (
	pingInterval = 15 * time.Second
	pongTimeout  = 10 * time.Second
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

// serve runs loops on conn until one ends the connection or shutdown is
// cancelled, then stops the rest, calls disconnect, and logs and measures the
// end under ctx's trace. ctx must not be the request's context, which ends at
// the upgrade.
func serve(ctx context.Context, shutdown context.Context, conn *websocket.Conn, disconnect func(), loops ...Loop) {
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
	// The keepalive never stops by itself, so it is not one of the loops
	// whose stopping means nothing serves the connection anymore.
	alive := make(chan loopResult, 1)
	go func() {
		end, ended := keepalive(conn)(ctx)
		alive <- loopResult{end: end, ended: ended}
	}()
	aliveStopped := false
	defer func() {
		if !aliveStopped {
			<-alive
		}
	}()

	end, decided, pending := End{}, false, len(loops)
	for !decided && pending > 0 {
		var aliveResult <-chan loopResult
		if !aliveStopped {
			aliveResult = alive
		}
		select {
		case result := <-results:
			pending--
			end, decided = result.end, result.ended
		case result := <-aliveResult:
			aliveStopped = true
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
	defer droppedConnections.Delete(conn)
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
		if end, dropped := droppedConnections.Load(conn); dropped {
			return "", nil, end.(End), true
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
	return closeAs(conn, websocket.StatusInvalidFramePayloadData, "malformed message", End{Cause: CauseMalformedMessage})
}

// CloseInvalidGameData closes conn because the client relayed data its game
// does not declare: a well-formed message whose content is the fault of the
// game's frontend, not of the client. reason is what was wrong, for the log.
func CloseInvalidGameData(conn *websocket.Conn, reason string) End {
	return closeAs(conn, websocket.StatusInvalidFramePayloadData, "invalid_game_data", End{Cause: CauseInvalidGameData, Detail: reason})
}

// CloseUnknownType closes conn because the client sent a message type the
// connection does not accept.
func CloseUnknownType(conn *websocket.Conn, messageType string) End {
	return closeAs(conn, websocket.StatusPolicyViolation, "unknown message type", End{Cause: CauseUnknownMessageType, Detail: messageType})
}

// CloseRateLimited closes conn because the client sent more than its limits
// allow.
func CloseRateLimited(conn *websocket.Conn) End {
	return closeAs(conn, websocket.StatusPolicyViolation, "rate limited", End{Cause: CauseRateLimited})
}

// closeAs closes conn with status and reason as end. It is recorded first, so
// the reader, which sees the client answer the close before the close returns
// here, reports this end and not the client's.
func closeAs(conn *websocket.Conn, status websocket.StatusCode, reason string, end End) End {
	droppedConnections.Store(conn, end)
	_ = conn.Close(status, reason)
	return end
}

// CloseServerFailure closes conn because the server could not go on.
func CloseServerFailure(conn *websocket.Conn, err error) End {
	wsx.Abort(conn)
	return End{Cause: CauseServerFailure, Detail: err.Error()}
}

// WriteMessage sends message as a JSON text frame. A client that does not take
// it within writeTimeout is dropped as too slow. ended is false when ctx was
// cancelled instead.
func WriteMessage(ctx context.Context, conn *websocket.Conn, message any) (End, bool) {
	data, err := json.Marshal(message)
	if err != nil {
		return CloseServerFailure(conn, err), true
	}
	// The server drops the client itself, after recording why, so a reader
	// failing on the dropped socket reports the same cause.
	var slow atomic.Bool
	tooSlow := time.AfterFunc(writeTimeout, func() {
		slow.Store(true)
		droppedConnections.Store(conn, End{Cause: CauseSlowConsumer})
		_ = conn.CloseNow()
	})
	err = conn.Write(ctx, websocket.MessageText, data)
	tooSlow.Stop()
	switch {
	case slow.Load():
		return End{Cause: CauseSlowConsumer}, true
	case err == nil:
		return End{}, false
	case ctx.Err() != nil:
		return End{}, false
	}
	return End{Cause: CauseWriteFailed}, true
}

// droppedConnections are the connections the server dropped or closed, with
// how they ended, until served connections are done with them.
var droppedConnections sync.Map

// keepalive pings the client every pingInterval and ends the connection when
// a pong does not come back within pongTimeout. It relies on the connection
// being read, which Dispatch always does.
func keepalive(conn *websocket.Conn) Loop {
	return func(ctx context.Context) (End, bool) {
		ticker := time.NewTicker(pingInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
			case <-ctx.Done():
				return End{}, false
			}
			pingCtx, cancel := context.WithTimeout(ctx, pongTimeout)
			err := conn.Ping(pingCtx)
			timedOut := pingCtx.Err() != nil && ctx.Err() == nil
			cancel()
			switch {
			case timedOut:
				// Closed by serve once it takes this end, so a reader failing
				// on the closed socket cannot report it first.
				return End{Cause: CauseUnresponsive}, true
			case err != nil:
				// Closed by whatever ended it, which reports why.
				return End{}, false
			}
		}
	}
}

func logConnectionEnd(ctx context.Context, end End, duration time.Duration) {
	endCtx := logging.Start(ctx)
	connectionsActive.Dec(endCtx)
	connectionsClosed.Inc(endCtx, causeLabel.Value(string(end.Cause)))
	connectionDuration.ObserveDuration(endCtx, duration)
	logging.LogFields(endCtx,
		logging.Field("close_cause", string(end.Cause)),
		logging.Field("connection_duration_ms", duration.Milliseconds()),
	)
	if end.Detail != "" {
		logging.LogFields(endCtx, logging.Field("close_detail", end.Detail))
	}
	logging.FinishRequestLog(endCtx, slog.Default(), "live connection closed")
}
