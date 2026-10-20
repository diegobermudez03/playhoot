package play

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/live"
	"github.com/diegobermudez03/playhoot/api/internal/wsx"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/runtime"
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

// connectionLimits bound everything one client sends on a live connection;
// a client beyond them is closed (1008). They sit above what each message
// type allows, so only a misbehaving client reaches them.
var connectionLimits = live.ConnectionLimits{
	Messages: []ratelimit.Limit{{Count: 120, Per: time.Second}, {Count: 6000, Per: time.Minute}},
	Bytes:    []ratelimit.Limit{{Count: 256 << 10, Per: time.Second}, {Count: 4 << 20, Per: time.Minute}},
}

// Error codes of a live connection refused before its upgrade.
const (
	codeServerBusy         = "server_busy"
	codeTooManyConnections = "too_many_connections"
)

// Wire messages every kind of live connection sends: every frame is a JSON
// object whose "type" names it.
const (
	liveRejected       = "rejected"
	liveLobbyUpdate    = "lobby_update"
	liveLobbyClosed    = "lobby_closed"
	liveSessionStarted = "session_started"
	liveSessionEnded   = "session_ended"
	livePlayerView     = "player_view"
)

type rejectedMessage struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

type lobbyUpdateMessage struct {
	Type        string               `json:"type"`
	SessionUUID string               `json:"session_uuid"`
	Members     []lobbyMemberMessage `json:"members"`
}

type lobbyMemberMessage struct {
	UserUUID string `json:"user_uuid"`
}

// sessionEndMessage tells a client how its Session ended: as lobby_closed
// when it never started for the client, as session_ended otherwise.
type sessionEndMessage struct {
	Type     string `json:"type"`
	Status   string `json:"status"`
	EndCause string `json:"end_cause"`
}

type participantMessage struct {
	ParticipantID string `json:"participant_id"`
	UserUUID      string `json:"user_uuid"`
}

type frontendMessage struct {
	URL       string    `json:"url"`
	SHA256    string    `json:"sha256"`
	ExpiresAt time.Time `json:"expires_at"`
}

// frontendJSON is a started Session's game frontend, on the wire.
func frontendJSON(start runtime.SessionStart) frontendMessage {
	return frontendMessage{URL: start.Frontend.URL, SHA256: start.Frontend.SHA256, ExpiresAt: start.Frontend.ExpiresAt}
}

// participantsJSON is who plays a started Session, on the wire.
func participantsJSON(start runtime.SessionStart) []participantMessage {
	participants := make([]participantMessage, 0, len(start.Participants))
	for _, participant := range start.Participants {
		participants = append(participants, participantMessage{ParticipantID: participant.ParticipantID, UserUUID: participant.UserUUID})
	}
	return participants
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
	// release gives back the room the connection holds on this instance; it
	// may be called more than once.
	release func()
}

// acceptRouted upgrades a request its route's middlewares authorized and
// routed here, once this instance and the caller have room for one more live
// connection. On false the request was answered or the connection closed.
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

	release, err := h.live.Reserve(ctx, userUUID)
	switch {
	case errors.Is(err, live.ErrCallerFull):
		httpx.WriteCodedError(w, http.StatusTooManyRequests, codeTooManyConnections, "too many live connections open")
		return routedConnection{}, false
	case err != nil:
		logging.LogFields(ctx, logging.Field("live_connection_refused", err.Error()))
		httpx.WriteCodedError(w, http.StatusServiceUnavailable, codeServerBusy, "server busy, try again later")
		return routedConnection{}, false
	}

	conn, err := wsx.Accept(w, r, h.webSocketOriginPatterns)
	if err != nil {
		release()
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
		release:     release,
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
	// started is set once the client was sent the Session's start.
	started atomic.Bool
}

func newLiveSession(conn *websocket.Conn) *liveSession {
	return &liveSession{conn: conn}
}

// request is how a connection handles one type of client message, and how
// often and how many of it it takes.
type request struct {
	handle live.Handler
	limits live.LaneLimits
}

// serveRequests returns the loops handling the client's messages: one reader
// and one lane per message type, so each type only waits for its own earlier
// messages. Every other type ends the connection, and so does a client
// sending beyond connectionLimits.
func (s *liveSession) serveRequests(requests map[string]request) []live.Loop {
	lanes := make(map[string]*live.Lane, len(requests))
	loops := make([]live.Loop, 0, len(requests)+1)
	for messageType, handling := range requests {
		limits := handling.limits
		limits.Excess = s.answer(limits.Excess)
		lane := live.NewLane(s.answer(handling.handle), limits)
		lanes[messageType] = lane
		loops = append(loops, lane.Run)
	}
	return append(loops, live.Dispatch(s.conn, lanes, connectionLimits))
}

// answer runs handle while holding the connection open for its answer; nil
// stays nil.
func (s *liveSession) answer(handle live.Handler) live.Handler {
	if handle == nil {
		return nil
	}
	return func(ctx context.Context, data []byte) (live.End, bool) {
		s.answering.RLock()
		defer s.answering.RUnlock()
		return handle(ctx, data)
	}
}

// closeRateLimited ends the connection of a client that sent a message more
// often than its type allows.
func (s *liveSession) closeRateLimited(context.Context, []byte) (live.End, bool) {
	return live.CloseRateLimited(s.conn), true
}

// sessionStream is what a connection receives from the Runtime, in the order
// it must be sent: lobby updates, then the start, then the views. sessionUUID
// is the Session the connection joined, which its lobby updates and start
// carry so that a client that joined by join code learns it.
type sessionStream[S, V any] struct {
	sessionUUID     string
	nextLobbyUpdate func(ctx context.Context) (runtime.LobbyUpdate, bool)
	started         func(ctx context.Context) (S, bool)
	startMessage    func(sessionUUID string, start S) any
	nextView        func(ctx context.Context) (V, bool)
	viewMessage     func(view V) any
}

// sendSession returns a loop that sends stream in order: no lobby update
// follows the start and no view precedes it.
func sendSession[S, V any](s *liveSession, stream sessionStream[S, V]) live.Loop {
	// Counted when the loop is built, so closeOnRuntimeEnd can never miss it.
	s.streams.Add(1)
	return func(ctx context.Context) (live.End, bool) {
		defer s.streams.Done()
		end, ended, more := sendAll(ctx, s, stream.nextLobbyUpdate, func(update runtime.LobbyUpdate) (any, bool) {
			return lobbyMessage(stream.sessionUUID, update), false
		})
		if ended || !more {
			return end, ended
		}
		start, ok := stream.started(ctx)
		if !ok {
			return live.End{}, false
		}
		end, ended = live.WriteMessage(ctx, s.conn, stream.startMessage(stream.sessionUUID, start))
		if ended || ctx.Err() != nil {
			return end, ended
		}
		s.started.Store(true)
		end, ended, _ = sendAll(ctx, s, stream.nextView, func(view V) (any, bool) {
			return stream.viewMessage(view), false
		})
		return end, ended
	}
}

// sendAll sends, as message, every item next produces until next reports none
// or message marks one as the last. more is false when ctx or a write stopped
// it.
func sendAll[T any](ctx context.Context, s *liveSession, next func(ctx context.Context) (T, bool), message func(item T) (wire any, last bool)) (end live.End, ended bool, more bool) {
	for {
		item, ok := next(ctx)
		if !ok {
			return live.End{}, false, ctx.Err() == nil
		}
		wire, last := message(item)
		if end, ended := live.WriteMessage(ctx, s.conn, wire); ended || ctx.Err() != nil {
			return end, ended, false
		}
		if last {
			return live.End{}, false, true
		}
	}
}

// lobbyMessage is a lobby update of the Session sessionUUID on the wire.
func lobbyMessage(sessionUUID string, update runtime.LobbyUpdate) any {
	members := make([]lobbyMemberMessage, 0, len(update.Members))
	for _, member := range update.Members {
		members = append(members, lobbyMemberMessage{UserUUID: member.UserUUID})
	}
	return lobbyUpdateMessage{Type: liveLobbyUpdate, SessionUUID: sessionUUID, Members: members}
}

// closeOnRuntimeEnd closes the connection once the Runtime ends it, after every
// stream and any answer in progress were delivered, telling the client how
// the Session ended when it did.
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

		if end.SessionStatus != "" {
			message := sessionEndMessage{Type: liveLobbyClosed, Status: end.SessionStatus, EndCause: end.EndCause}
			if s.started.Load() {
				message.Type = liveSessionEnded
			}
			if end, ended := live.WriteMessage(ctx, s.conn, message); ended {
				return end, ended
			}
		}
		status := websocket.StatusInternalError
		switch end.Reason {
		case runtime.EndSessionEnded, runtime.EndDisconnected, runtime.EndLeft, runtime.EndReplaced:
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
