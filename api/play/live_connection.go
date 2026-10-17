package play

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/coder/websocket"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/live"
	"github.com/diegobermudez03/playhoot/api/internal/wsx"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

// Wire messages every kind of live connection sends: every frame is a JSON
// object whose "type" names it.
const (
	liveRejected    = "rejected"
	liveLobbyUpdate = "lobby_update"
	liveLobbyClosed = "lobby_closed"
	livePlayerView  = "player_view"
)

type rejectedMessage struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

type lobbyUpdateMessage struct {
	Type    string               `json:"type"`
	Members []lobbyMemberMessage `json:"members"`
}

type lobbyMemberMessage struct {
	UserUUID string `json:"user_uuid"`
}

type lobbyClosedMessage struct {
	Type string `json:"type"`
}

type outcomeMessage struct {
	Type    string `json:"type"`
	Outcome string `json:"outcome"`
}

// routedConnection is an upgraded live request that passed authorization and
// owner routing.
type routedConnection struct {
	// ctx keeps the request's values (trace, request log) but not its
	// lifetime, which ends at the upgrade.
	ctx         context.Context
	conn        *websocket.Conn
	userUUID    string
	sessionUUID string
	ownerEpoch  int64
}

// acceptRouted upgrades a request its route's middlewares authorized and
// routed here. On false the request was answered or the connection closed.
func (h *Handler) acceptRouted(w http.ResponseWriter, r *http.Request) (routedConnection, bool) {
	ctx := r.Context()

	userUUID, callerOK := auth.UserUUID(ctx)
	sessionUUID, ownerEpoch, routedOK := routedSessionFrom(ctx)
	if !callerOK || !routedOK {
		// Only possible if the route was registered without its middlewares.
		logging.LogFields(ctx, logging.Field("wiring_error", "live join reached without an authorized caller or a routed session"))
		httpx.WriteError(w, http.StatusInternalServerError, "joining session failed")
		return routedConnection{}, false
	}

	conn, err := wsx.Accept(w, r, h.webSocketOriginPatterns)
	if err != nil {
		// Accept already answered the request.
		logging.LogError(ctx, err)
		return routedConnection{}, false
	}
	return routedConnection{
		ctx:         context.WithoutCancel(ctx),
		conn:        conn,
		userUUID:    userUUID,
		sessionUUID: sessionUUID,
		ownerEpoch:  ownerEpoch,
	}, true
}

// admitted reports whether the Runtime accepted the connection. Otherwise it
// sent the rejection, or failed the connection, and closed it.
func admitted(ctx context.Context, conn *websocket.Conn, outcome runtime.AttachOutcome, rejection runtime.RejectReason) bool {
	logging.LogFields(ctx, logging.Field("attach_outcome", string(outcome)))
	switch outcome {
	case runtime.AttachAccepted:
		return true
	case runtime.AttachRejected:
		logging.LogFields(ctx, logging.Field("reject_reason", string(rejection)))
		_, _ = live.WriteMessage(ctx, conn, rejectedMessage{Type: liveRejected, Reason: string(rejection)})
		_ = conn.Close(websocket.StatusPolicyViolation, "admission rejected")
	default:
		logging.LogFields(ctx, logging.Field("attach_error", "unknown admission outcome"))
		wsx.Abort(conn)
	}
	return false
}

// liveSession is what every served live connection shares: its socket, the
// lock held while answering, and the Runtime streams still delivering.
type liveSession struct {
	conn *websocket.Conn

	// answering is read-held by each lane while it answers and write-held to
	// close, so lanes never wait for each other and an answer is never cut.
	answering sync.RWMutex
	streams   sync.WaitGroup
}

func newLiveSession(conn *websocket.Conn) *liveSession {
	return &liveSession{conn: conn}
}

// serveRequests returns the loops handling the client's messages: one reader
// and one lane per message type, so each type only waits for its own earlier
// messages. Every other type ends the connection.
func (s *liveSession) serveRequests(handlers map[string]live.Handler) []live.Loop {
	lanes := make(map[string]*live.Lane, len(handlers))
	loops := make([]live.Loop, 0, len(handlers)+1)
	for messageType, handle := range handlers {
		lane := live.NewLane(func(ctx context.Context, data []byte) (live.End, bool) {
			s.answering.RLock()
			defer s.answering.RUnlock()
			return handle(ctx, data)
		})
		lanes[messageType] = lane
		loops = append(loops, lane.Run)
	}
	return append(loops, live.Dispatch(s.conn, lanes))
}

// sendStream returns a loop that sends, as message, every item next produces
// until next reports none or message marks one as the last.
func sendStream[T any](s *liveSession, next func(ctx context.Context) (T, bool), message func(item T) (wire any, last bool)) live.Loop {
	// Counted when the loop is built, so closeOnRuntimeEnd can never miss it.
	s.streams.Add(1)
	return func(ctx context.Context) (live.End, bool) {
		defer s.streams.Done()
		for {
			item, ok := next(ctx)
			if !ok {
				return live.End{}, false
			}
			wire, last := message(item)
			if end, ended := live.WriteMessage(ctx, s.conn, wire); ended || ctx.Err() != nil || last {
				return end, ended
			}
		}
	}
}

// sendLobbyUpdates sends every lobby update, ending with lobby_closed once the
// lobby closes.
func (s *liveSession) sendLobbyUpdates(next func(ctx context.Context) (runtime.LobbyUpdate, bool)) live.Loop {
	return sendStream(s, next, func(update runtime.LobbyUpdate) (any, bool) {
		if update.Closed {
			return lobbyClosedMessage{Type: liveLobbyClosed}, true
		}
		members := make([]lobbyMemberMessage, 0, len(update.Members))
		for _, member := range update.Members {
			members = append(members, lobbyMemberMessage{UserUUID: member.UserUUID})
		}
		return lobbyUpdateMessage{Type: liveLobbyUpdate, Members: members}, false
	})
}

// closeOnRuntimeEnd closes the connection once the Runtime ends it, after every
// stream and any answer in progress were delivered.
func (s *liveSession) closeOnRuntimeEnd(ended func(ctx context.Context) (runtime.ConnectionEnd, bool)) live.Loop {
	return func(ctx context.Context) (live.End, bool) {
		end, ok := ended(ctx)
		if !ok {
			return live.End{}, false
		}
		delivered := make(chan struct{})
		go func() {
			s.streams.Wait()
			close(delivered)
		}()
		select {
		case <-delivered:
		case <-ctx.Done():
			return live.End{}, false
		}
		s.answering.Lock()
		defer s.answering.Unlock()

		status := websocket.StatusInternalError
		switch end.Reason {
		case runtime.EndSessionEnded, runtime.EndDisconnected, runtime.EndLeft:
			status = websocket.StatusNormalClosure
		case runtime.EndOwnerChanged:
			// Tells the client to reconnect, which routes it to the current owner.
			status = websocket.StatusServiceRestart
		}
		_ = s.conn.Close(status, string(end.Reason))
		return live.End{Cause: live.CauseEndedByOwner, Detail: string(end.Reason)}, true
	}
}

// viewState is a view's game state on the wire; JSON null when there is none.
func gameJSON(state []byte) json.RawMessage {
	if len(state) == 0 {
		return nil
	}
	return state
}
