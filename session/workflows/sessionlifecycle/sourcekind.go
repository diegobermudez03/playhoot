package sessionlifecycle

// session_runtime_turns.source_kind labels, persisted on every committed
// RuntimeTurn as its own durable-cause-log discriminator, retained for
// reconstruction/audit purposes only, never the live-correctness path.
const (
	// sessionStartSourceKind is Start's own RuntimeTurn.
	sessionStartSourceKind = "SESSION_START"

	// timerExpiredSourceKind is the RuntimeTurn a timer obligation's
	// expiration causes (Manager.ExpireTimer).
	timerExpiredSourceKind = "TIMER_EXPIRED"

	// playerEventSourceKind is the RuntimeTurn a submitted player event
	// causes (Manager.SubmitPlayerEvent). Its durable content lives in
	// session_cause_events (cause_kind "PLAYER_EVENT"), referenced by the
	// Turn's source_cause_event_id.
	playerEventSourceKind = "PLAYER_EVENT"

	// sessionCancelledSourceKind is the RuntimeTurn a host's manual
	// cancellation causes when the authored script itself reacts to
	// SESSION_CANCELLED (Manager.CancelSession). Its durable content lives
	// in session_cause_events (cause_kind "SESSION_CANCELLED"), referenced
	// by the Turn's source_cause_event_id. A cancellation the script
	// rejects outright never reaches this - no RuntimeTurn is ever created
	// for it.
	sessionCancelledSourceKind = "SESSION_CANCELLED"
)
