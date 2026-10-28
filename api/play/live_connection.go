package play

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/live"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/api/internal/throttle"
	"github.com/diegobermudez03/playhoot/api/internal/wsx"
	"github.com/diegobermudez03/playhoot/identity"
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

// refusalLinger bounds how long a connection upgraded only to be rejected
// stays open, whatever its client does: enough to send the rejection and the
// close, never enough for a flood of refused handshakes to pile up.
var refusalLinger = 2 * time.Second

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

	// Requirements is set only when the reason is device_not_supported.
	Requirements *inputRequirementsMessage `json:"requirements,omitempty"`

	// RetryAfterSeconds is set only when the reason is rate_limited.
	RetryAfterSeconds int `json:"retry_after_seconds,omitempty"`

	// Status and EndCause are how the Session ended, set only when the
	// reason is session_ended.
	Status   string `json:"status,omitempty"`
	EndCause string `json:"end_cause,omitempty"`
}

// inputRequirementsMessage is what a game needs of a player's device: the
// device classes that can play it and the interactions it uses.
type inputRequirementsMessage struct {
	Devices          string `json:"devices"`
	UsesKeyboard     bool   `json:"uses_keyboard"`
	UsesPointerHover bool   `json:"uses_pointer_hover"`
	UsesPointerPress bool   `json:"uses_pointer_press"`
}

type lobbyUpdateMessage struct {
	Type        string               `json:"type"`
	SessionUUID string               `json:"session_uuid"`
	Members     []lobbyMemberMessage `json:"members"`
	// MinPlayers and MaxPlayers bound how many players the game can start
	// with, so a host knows when starting is possible.
	MinPlayers int `json:"min_players"`
	MaxPlayers int `json:"max_players"`
}

type lobbyMemberMessage struct {
	UserUUID    string `json:"user_uuid"`
	DisplayName string `json:"display_name"`
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
	DisplayName   string `json:"display_name"`
}

type frontendMessage struct {
	URL       string          `json:"url"`
	SHA256    string          `json:"sha256"`
	ExpiresAt time.Time       `json:"expires_at"`
	Viewport  viewportMessage `json:"viewport"`
	Assets    []assetMessage  `json:"assets"`
}

type assetMessage struct {
	Key      string `json:"key"`
	Type     string `json:"type"`
	MIMEType string `json:"mime_type"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	URL      string `json:"url"`
}

type viewportMessage struct {
	AspectRatio string `json:"aspect_ratio"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
}

// frontendJSON is a started Session's game frontend, on the wire.
func frontendJSON(start runtime.SessionStart) frontendMessage {
	viewport := start.Frontend.Viewport
	assets := make([]assetMessage, len(start.Frontend.Assets))
	for i, asset := range start.Frontend.Assets {
		assets[i] = assetMessage{Key: asset.Key, Type: asset.Type, MIMEType: asset.MIMEType, SHA256: asset.SHA256, Size: asset.Size, URL: asset.URL}
	}
	return frontendMessage{
		URL:       start.Frontend.URL,
		SHA256:    start.Frontend.SHA256,
		ExpiresAt: start.Frontend.ExpiresAt,
		Viewport:  viewportMessage{AspectRatio: viewport.AspectRatio, Width: viewport.Width, Height: viewport.Height},
		Assets:    assets,
	}
}

// participantsJSON is who plays a started Session, on the wire.
func participantsJSON(start runtime.SessionStart) []participantMessage {
	participants := make([]participantMessage, 0, len(start.Participants))
	for _, participant := range start.Participants {
		participants = append(participants, participantJSON(participant))
	}
	return participants
}

// participantJSON is a participant of a started Session, on the wire.
func participantJSON(participant runtime.Participant) participantMessage {
	return participantMessage{ParticipantID: participant.ParticipantID, UserUUID: participant.UserUUID, DisplayName: participant.DisplayName}
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
	caller      identity.User
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

	caller, callerOK := auth.Caller(ctx)
	sessionUUID, ownerEpoch, routedOK := routedSessionFrom(ctx)
	if !callerOK || !routedOK {
		// Only possible if the route was registered without its middlewares.
		logging.LogFields(ctx, logging.Field("wiring_error", "live join reached without an authorized caller or a routed session"))
		httpx.WriteError(w, http.StatusInternalServerError, "joining session failed")
		return routedConnection{}, false
	}

	conn, release, ok := h.upgrade(w, r, caller.UUID)
	if !ok {
		return routedConnection{}, false
	}
	return routedConnection{
		ctx:         context.WithoutCancel(ctx),
		conn:        conn,
		caller:      caller,
		sessionUUID: sessionUUID,
		ownerEpoch:  ownerEpoch,
		release:     release,
	}, true
}

// refuseUpgraded tells an authorized caller who cannot join why, as
// rejection on an upgraded connection it then closes, since browsers cannot
// read why an upgrade was refused. The connection holds no room afterwards.
func (h *Handler) refuseUpgraded(w http.ResponseWriter, r *http.Request, rejection rejectedMessage) {
	ctx := r.Context()

	caller, ok := auth.Caller(ctx)
	if !ok {
		// Only possible if the route was registered without its authorization.
		logging.LogFields(ctx, logging.Field("wiring_error", "live join refused without an authorized caller"))
		httpx.WriteError(w, http.StatusInternalServerError, "joining session failed")
		return
	}
	conn, release, ok := h.upgrade(w, r, caller.UUID)
	if !ok {
		return
	}
	defer release()
	reject(ctx, conn, rejection)
}

// upgrade upgrades r once this instance and callerUUID have room for one more
// live connection; release gives that room back. On false the request was
// answered.
func (h *Handler) upgrade(w http.ResponseWriter, r *http.Request, callerUUID string) (*websocket.Conn, func(), bool) {
	ctx := r.Context()

	release, err := h.live.Reserve(ctx, callerUUID)
	if err != nil {
		logging.LogFields(ctx, logging.Field("live_connection_refused", err.Error()))
		status, reason, message := http.StatusServiceUnavailable, routing.RefusedServerBusy, "server busy, try again later"
		if errors.Is(err, live.ErrCallerFull) {
			status, reason, message = http.StatusTooManyRequests, routing.RefusedTooManyConnections, "too many live connections open"
		}
		routing.NoteLimitRefusal(ctx, reason)
		if !h.refuseLimit(w, r, routing.LimitRefusal{Reason: reason}) {
			httpx.WriteCodedError(w, status, reason, message)
		}
		return nil, nil, false
	}

	conn, err := wsx.Accept(w, r, h.webSocketOriginPatterns)
	if err != nil {
		release()
		// Accept already answered the request.
		logging.LogError(ctx, err)
		return nil, nil, false
	}
	return conn, release, true
}

// refuseLimit tells an authorized live caller a limit refused it, as a
// rejection on an upgraded connection it then closes, since browsers cannot
// read why an upgrade was refused. It needs none of the room Reserve holds;
// it reports false, answering nothing, when there is no room to refuse.
func (h *Handler) refuseLimit(w http.ResponseWriter, r *http.Request, refusal routing.LimitRefusal) bool {
	ctx := r.Context()

	caller, ok := auth.Caller(ctx)
	if !ok {
		return false
	}
	release, ok := h.live.ReserveRefusal(ctx, caller.UUID)
	if !ok {
		logging.LogFields(ctx, logging.Field("readable_refusal", "no_room"))
		return false
	}
	defer release()
	conn, err := wsx.Accept(w, r, h.webSocketOriginPatterns)
	if err != nil {
		// Accept already answered the request.
		logging.LogError(ctx, err)
		return true
	}
	rejection := rejectedMessage{Reason: refusal.Reason}
	if refusal.RetryAfter > 0 {
		rejection.RetryAfterSeconds = throttle.RetryAfterSeconds(refusal.RetryAfter)
	}
	reject(ctx, conn, rejection)
	return true
}

// admitted reports whether the Runtime accepted the connection. Otherwise it
// sent the rejection, or failed the connection, and closed it.
func admitted(ctx context.Context, conn *websocket.Conn, outcome runtime.AttachOutcome, rejection rejectedMessage) bool {
	logging.LogFields(ctx, logging.Field("attach_outcome", string(outcome)))
	switch outcome {
	case runtime.AttachAccepted:
		return true
	case runtime.AttachRejected:
		reject(ctx, conn, rejection)
	default:
		logging.LogFields(ctx, logging.Field("attach_error", "unknown admission outcome"))
		wsx.Abort(conn)
	}
	return false
}

// reject tells the client why it may not join, as rejection, and closes the
// connection, within refusalLinger whether or not the client takes it.
func reject(ctx context.Context, conn *websocket.Conn, rejection rejectedMessage) {
	logging.LogFields(ctx, logging.Field("reject_reason", rejection.Reason))
	rejection.Type = liveRejected
	lingering, cancel := context.WithTimeout(ctx, refusalLinger)
	defer cancel()
	// Reading in the background until the deadline answers the client's
	// close at once, and drops the socket at the deadline when the client
	// never answers, which the close handshake alone would wait seconds for.
	conn.CloseRead(lingering)
	_, _ = live.WriteMessage(lingering, conn, rejection)
	_ = conn.Close(websocket.StatusPolicyViolation, "admission rejected")
}

// liveSession is what every served live connection shares: its socket, the
// lock held while answering, and the Runtime streams still delivering.
type liveSession struct {
	conn *websocket.Conn

	// answering is read-held by each lane while it answers and write-held to
	// close, so lanes never wait for each other and an answer is never cut.
	answering sync.RWMutex
	streams   sync.WaitGroup
	// started is set, and startSent closed, once the client was sent the
	// Session's start; sessionSent is closed once the session stream stopped.
	started     atomic.Bool
	startSent   chan struct{}
	sessionSent chan struct{}
}

func newLiveSession(conn *websocket.Conn) *liveSession {
	return &liveSession{conn: conn, startSent: make(chan struct{}), sessionSent: make(chan struct{})}
}

// request is how a connection handles one type of client message, and how
// often and how many of it it takes. Types naming the same lane share it,
// with its first type's limits, so they are handled in the order they came.
type request struct {
	handle live.Handler
	limits live.LaneLimits
	lane   string
}

// limitAuditor records, for the Session's audit, a limit that refused what
// the connection's user sent.
type limitAuditor func(ctx context.Context, limit string)

// auditedLimits are limits telling audit which one refused, named
// api.<name>.<limit>.
func auditedLimits(limits live.LaneLimits, name string, audit limitAuditor) live.LaneLimits {
	limits.Refused = func(ctx context.Context, limit string) { audit(ctx, "api."+name+"."+limit) }
	return limits
}

// serveRequests returns the loops handling the client's messages: one reader
// and one lane per message type, or per shared lane, so each type only waits
// for its own lane's earlier messages. Every other type ends the connection,
// and so does a client sending beyond connectionLimits. Every refusal is
// audited through audit.
func (s *liveSession) serveRequests(requests map[string]request, audit limitAuditor) []live.Loop {
	lanes := make(map[string]*live.Lane, len(requests))
	loops := make([]live.Loop, 0, len(requests)+1)
	shared := make(map[string]map[string]live.Handler)
	for messageType, handling := range requests {
		if handling.lane == "" {
			continue
		}
		if shared[handling.lane] == nil {
			shared[handling.lane] = make(map[string]live.Handler)
		}
		shared[handling.lane][messageType] = handling.handle
	}
	byLane := make(map[string]*live.Lane, len(shared))
	for _, messageType := range sortedTypes(requests) {
		handling := requests[messageType]
		name, handle := messageType, handling.handle
		if handling.lane != "" {
			if lane, found := byLane[handling.lane]; found {
				lanes[messageType] = lane
				continue
			}
			name, handle = handling.lane, byType(shared[handling.lane])
		}
		limits := auditedLimits(handling.limits, name, audit)
		limits.Excess = s.answer(limits.Excess)
		lane := live.NewLane(s.answer(handle), limits)
		lanes[messageType] = lane
		if handling.lane != "" {
			byLane[handling.lane] = lane
		}
		loops = append(loops, lane.Run)
	}
	limits := connectionLimits
	limits.Refused = func(ctx context.Context, limit string) { audit(ctx, "api.connection."+limit) }
	return append(loops, live.Dispatch(s.conn, lanes, limits))
}

// sortedTypes is requests' message types in a fixed order, so a shared lane
// always takes the same type's limits.
func sortedTypes(requests map[string]request) []string {
	types := make([]string, 0, len(requests))
	for messageType := range requests {
		types = append(types, messageType)
	}
	sort.Strings(types)
	return types
}

// byType handles a shared lane's messages with their own type's handler.
func byType(handlers map[string]live.Handler) live.Handler {
	return func(ctx context.Context, data []byte) (live.End, bool) {
		var message struct {
			Type string `json:"type"`
		}
		// Dispatch already read the type, so this cannot fail.
		_ = json.Unmarshal(data, &message)
		return handlers[message.Type](ctx, data)
	}
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
		defer close(s.sessionSent)
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
		close(s.startSent)
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
		members = append(members, lobbyMemberMessage{UserUUID: member.UserUUID, DisplayName: member.DisplayName})
	}
	return lobbyUpdateMessage{Type: liveLobbyUpdate, SessionUUID: sessionUUID, Members: members, MinPlayers: update.MinPlayers, MaxPlayers: update.MaxPlayers}
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
