// Package replay is the contract between a running game and the recorder of
// its approximate replay history: what the game offers, never what the
// recorder stores. Recording is best effort and never slows gameplay down,
// except for a finished game's final capture, which is stored durably before
// the Session is FINISHED.
package replay

// Capture is a consistent copy of a running game at one committed boundary.
// State is the authoritative global state, which must never reach a client.
type Capture struct {
	SessionUUID     string
	GameVersionUUID string
	SessionTimeMs   int64
	ContextRevision int64
	State           []byte

	// Participants are the Session's ParticipantIDs, in order.
	Participants []string

	// CommandResponses are the COMMAND responses the boundary produced, in
	// timeline order.
	CommandResponses []CommandResponse

	// Errors are the logical errors the boundary returned, in timeline order.
	Errors []GameError

	// Events are the logical events the boundary emitted, in timeline order.
	Events []GameEvent
}

// GameEvent is a logical event the game emitted, placed like a GameError. A
// replay shows it only from the perspective of its Participants, or from
// every perspective when there are none.
type GameEvent struct {
	Participants  []string
	SessionTimeMs int64
	Order         uint64

	// Event is the game's value, as JSON.
	Event []byte
}

// GameError is a logical error the game returned to one participant. Only
// ParticipantID may ever see it: a replay must not show it from another
// perspective.
type GameError struct {
	ParticipantID string
	SessionTimeMs int64
	// Order places it among the timeline operations of the same time: the
	// last one the call that returned it ran. Errors of update(), which runs
	// after every operation of its Capture, have none (0).
	Order uint64

	// Error is the game's value, as JSON.
	Error []byte
}

// CommandResponse is what a COMMAND answered. Only ParticipantID, who sent
// it, may ever see it: a replay must not show it from another perspective.
type CommandResponse struct {
	ParticipantID string
	SessionTimeMs int64
	// Order places it among the timeline operations of the same time.
	Order uint64

	// Response is the game's answer, as JSON.
	Response []byte
}
