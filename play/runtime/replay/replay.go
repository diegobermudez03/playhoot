// Package replay is the contract between a running game and the recorder of
// its approximate replay history: what the game offers, never what the
// recorder stores. Recording is best effort and never slows gameplay down.
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
