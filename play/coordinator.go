package play

import (
	"context"
	"log"
	"sync"

	"github.com/diegobermudez03/playhoot/logging"
)

// Conn is a bound live connection's outbound delivery capability, satisfied
// by the transport adapter (api) that owns the actual socket. play never
// knows the underlying transport (WebSocket, or anything else).
//
// Deliver is called synchronously from whichever goroutine drove the
// committing call (Start/AnswerInteraction); an implementation must not
// block on a slow/unresponsive client - buffer internally and return
// promptly. Deliver's error return is informational only: a failed
// delivery is always best-effort and never retried, queued, or fed back
// into Session Runtime.
type Conn interface {
	Deliver(Event) error
}

// Coordinator is the Live Session Coordinator: an in-process, per-Session
// registry of bound live connections (a plain mutex-guarded map - no
// external routing/coordination service), translating decoded client
// commands into calls against SessionRuntime and fanning out the Events a
// committed call produces to every currently-bound recipient connection.
//
// Coordinator owns no authoritative Session truth: every mutation happens
// through rt, and rt's own call already durably commits before Coordinator
// ever sees an Event to fan out - Coordinator itself does nothing to
// enforce this, it is simply never handed an Event before rt's call
// returns.
type Coordinator struct {
	rt SessionRuntime

	mu       sync.Mutex
	sessions map[SessionUUID]map[UserUUID]Conn
}

// NewCoordinator constructs a Coordinator dispatching against rt.
func NewCoordinator(rt SessionRuntime) *Coordinator {
	return &Coordinator{rt: rt, sessions: make(map[SessionUUID]map[UserUUID]Conn)}
}

// Create forwards directly to rt.Create. Create happens before any live
// connection exists, so it never touches the connection registry.
func (c *Coordinator) Create(ctx context.Context, gameUUID, hostUserUUID, idempotencyKey string) (CreatedSession, error) {
	defer logging.Step(ctx, "Coordinator.Create").Close()
	logging.LogFields(ctx,
		logging.Field("game_uuid", gameUUID),
		logging.Field("host_user_uuid", hostUserUUID),
	)
	return c.rt.Create(ctx, gameUUID, hostUserUUID, idempotencyKey)
}

// Join forwards directly to rt.Join, for the same reason Create does.
func (c *Coordinator) Join(ctx context.Context, joinCode uint, userUUID, displayName, idempotencyKey string) (JoinResult, error) {
	defer logging.Step(ctx, "Coordinator.Join").Close()
	logging.LogFields(ctx,
		logging.Field("join_code", joinCode),
		logging.Field("user_uuid", userUUID),
	)
	return c.rt.Join(ctx, joinCode, userUUID, displayName, idempotencyKey)
}

// Bind registers conn as sessionUUID/userUUID's live connection, replacing
// any previously bound connection for that same key outright - there is no
// disconnect grace period, debounce, or reconnect handling.
//
// The returned unbind func must be called exactly once, when conn's own
// transport-level connection closes, so the registry never retains a dead
// connection past its transport's actual lifetime. Unbind is a no-op if
// conn has since been replaced by a later Bind for the same key.
func (c *Coordinator) Bind(sessionUUID SessionUUID, userUUID UserUUID, conn Conn) (unbind func()) {
	c.mu.Lock()
	defer c.mu.Unlock()

	conns, ok := c.sessions[sessionUUID]
	if !ok {
		conns = make(map[UserUUID]Conn)
		c.sessions[sessionUUID] = conns
	}
	conns[userUUID] = conn

	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		if conns, ok := c.sessions[sessionUUID]; ok {
			if conns[userUUID] == conn {
				delete(conns, userUUID)
			}
			if len(conns) == 0 {
				delete(c.sessions, sessionUUID)
			}
		}
	}
}

// Start calls rt.Start and returns its resulting Events alongside the
// outcome, instead of fanning them out itself: the caller (api) owns
// sequencing its own direct reply to the command against Deliver, which
// matters when the caller's own connection is also one of Events'
// recipients - Coordinator has no way to know api wants its own reply
// enqueued first, so it leaves that ordering to api rather than guessing.
// Coordinator still owns the fan-out mechanism itself (Deliver).
func (c *Coordinator) Start(ctx context.Context, sessionUUID SessionUUID, userUUID UserUUID, idempotencyKey string) (StartOutcome, []Event, error) {
	defer logging.Step(ctx, "Coordinator.Start").Close()
	logging.LogFields(ctx,
		logging.Field("session_uuid", string(sessionUUID)),
		logging.Field("user_uuid", string(userUUID)),
	)
	result, err := c.rt.Start(ctx, string(sessionUUID), string(userUUID), idempotencyKey)
	if err != nil {
		return "", nil, err
	}
	return result.Outcome, result.Events, nil
}

// AnswerInteraction calls rt.AnswerInteraction and returns its resulting
// Events alongside the outcome, for the same reason Start does.
// sessionUUID is supplied by the caller (the connection's own bound
// Session, per Bind) rather than derived from interactionUUID, since
// Coordinator's registry is keyed by Session, not by interaction.
func (c *Coordinator) AnswerInteraction(ctx context.Context, sessionUUID SessionUUID, interactionUUID InteractionUUID, userUUID UserUUID, answer []byte) (AnswerOutcome, []Event, error) {
	defer logging.Step(ctx, "Coordinator.AnswerInteraction").Close()
	logging.LogFields(ctx,
		logging.Field("session_uuid", string(sessionUUID)),
		logging.Field("interaction_uuid", string(interactionUUID)),
		logging.Field("user_uuid", string(userUUID)),
	)
	result, err := c.rt.AnswerInteraction(ctx, string(interactionUUID), string(userUUID), answer)
	if err != nil {
		return "", nil, err
	}
	return result.Outcome, result.Events, nil
}

// Deliver fans events out to whichever recipients currently have a bound
// connection for sessionUUID. A recipient with no bound connection simply
// receives nothing - there is no queueing, resync, or replay of a missed
// Event. A Deliver failure on one connection is logged and never affects
// any other connection, and never reopens/retries/reinterprets the
// already-committed call that produced events.
func (c *Coordinator) Deliver(sessionUUID SessionUUID, events []Event) {
	if len(events) == 0 {
		return
	}

	c.mu.Lock()
	conns := c.sessions[sessionUUID]
	targets := make(map[UserUUID]Conn, len(conns))
	for userUUID, conn := range conns {
		targets[userUUID] = conn
	}
	c.mu.Unlock()

	for _, event := range events {
		conn, ok := targets[event.Recipient]
		if !ok {
			continue
		}
		if err := conn.Deliver(event); err != nil {
			log.Printf("play: best-effort delivery failed: session=%s recipient=%s kind=%s: %v", sessionUUID, event.Recipient, event.Kind, err)
		}
	}
}
