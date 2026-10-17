// Package runtime is the live Session Runtime's public contract. It imports
// only the standard library; each kind of live connection declares its own
// operations and messages as they are needed.
package runtime

import "context"

// OwnerOutcome is where the live owner of a Session was found.
type OwnerOutcome string

const (
	// OwnerHere means this instance owns the Session, already or by claiming
	// it now.
	OwnerHere OwnerOutcome = "owned_here"

	// OwnerElsewhere means another instance owns the Session.
	OwnerElsewhere OwnerOutcome = "owned_elsewhere"

	// OwnerSessionUnavailable means the Session does not exist or cannot be
	// joined live, so it was not claimed.
	OwnerSessionUnavailable OwnerOutcome = "session_unavailable"
)

// OwnerPlacement is the live owner of a Session. Address and Epoch are set
// only when the Session has an owner.
type OwnerPlacement struct {
	Outcome OwnerOutcome

	// Address is the owner's internal address; it must never reach a client.
	Address string

	// Epoch identifies this period of ownership; the owner rejects live
	// operations routed under an older one.
	Epoch int64
}

// AttachOutcome is whether a connection was admitted into a Session.
type AttachOutcome string

const (
	AttachAccepted AttachOutcome = "accepted"
	AttachRejected AttachOutcome = "rejected"
)

// RejectReason is why a connection was not admitted.
type RejectReason string

const (
	RejectSessionUnavailable RejectReason = "session_unavailable"
	RejectNotCreator         RejectReason = "not_creator"

	// RejectOwnerChanged means the connection was routed to a former owner;
	// the client should reconnect.
	RejectOwnerChanged RejectReason = "owner_changed"
)

// EndReason is why the Runtime ended a live connection.
type EndReason string

const (
	// EndDisconnected means the connection's Disconnect was called.
	EndDisconnected EndReason = "disconnected"

	// EndSessionEnded means the Session reached a terminal state.
	EndSessionEnded EndReason = "session_ended"

	// EndOwnerChanged means this instance no longer serves the Session under
	// the connection's ownership; the client should reconnect.
	EndOwnerChanged EndReason = "owner_changed"
)

// ConnectionEnd is how a live connection ended on the Runtime's side.
type ConnectionEnd struct {
	Reason EndReason
}

// AttachAdminResult is the outcome of attaching an admin connection.
// Rejection is set only when rejected, Connection only when accepted.
type AttachAdminResult struct {
	Outcome    AttachOutcome
	Rejection  RejectReason
	Connection AdminConnection
}

// AdminConnection is everything a caller can do with an admitted admin
// connection. The caller must run NextView and Ended until they report the
// end, and call Disconnect once when the client is gone.
type AdminConnection struct {
	// SelectPerspective switches the participant whose view this connection
	// receives; a FULL view of them follows through NextView.
	SelectPerspective func(ctx context.Context, participantID string) (SelectPerspectiveResult, error)

	// CancelSession ends the Session as cancelled by its creator.
	CancelSession func(ctx context.Context) (CancelSessionResult, error)

	// NextView blocks until the next view of the selected participant. Every
	// view produced before the connection ended is returned before false.
	NextView func(ctx context.Context) (PlayerView, bool)

	// Ended blocks until the connection ends and reports why; false means
	// ctx was done first.
	Ended func(ctx context.Context) (ConnectionEnd, bool)

	// Disconnect tells the Runtime the client is gone. Calling it again has
	// no effect.
	Disconnect func()
}

// PlayerView is one participant's view of the game, as their projection.
type PlayerView struct {
	ParticipantID string

	// Full means State replaces the previous view rather than patching it.
	Full bool

	// State is the projection the game's script returned, as JSON whose shape
	// only the game defines.
	State []byte
}

// SelectPerspectiveOutcome is the decision a SelectPerspective call reached.
type SelectPerspectiveOutcome string

const (
	PerspectiveSelected           SelectPerspectiveOutcome = "selected"
	PerspectiveUnknownParticipant SelectPerspectiveOutcome = "unknown_participant"
)

// SelectPerspectiveResult is the outcome of selecting a perspective.
type SelectPerspectiveResult struct {
	Outcome SelectPerspectiveOutcome
}

// CancelSessionOutcome is the decision a CancelSession call reached.
type CancelSessionOutcome string

const (
	SessionCancelled    CancelSessionOutcome = "cancelled"
	SessionAlreadyEnded CancelSessionOutcome = "already_ended"
)

// CancelSessionResult is the outcome of cancelling a Session.
type CancelSessionResult struct {
	Outcome CancelSessionOutcome
}
