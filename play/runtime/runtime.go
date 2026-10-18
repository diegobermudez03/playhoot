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

	// RejectSessionStarted means the Session left its lobby; only its
	// participants may come back, by rejoining.
	RejectSessionStarted RejectReason = "session_started"

	// RejectLobbyFull means the lobby holds as many players as the game
	// allows.
	RejectLobbyFull RejectReason = "lobby_full"

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

	// EndSessionAborted means the Session could not go on because of a
	// failure, such as its game failing to start.
	EndSessionAborted EndReason = "session_aborted"

	// EndLeft means the player left the Session, or its lobby.
	EndLeft EndReason = "left"

	// EndReplaced means the same user opened a newer player connection,
	// which took this one's place; the client should not reconnect.
	EndReplaced EndReason = "replaced"

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
// connection. The caller must run NextLobbyUpdate, Started, NextView and
// Ended until they report the end, and call Disconnect once when the client
// is gone.
type AdminConnection struct {
	// SelectPerspective switches the participant whose view this connection
	// receives; a FULL view of them follows through NextView.
	SelectPerspective func(ctx context.Context, participantID string) (SelectPerspectiveResult, error)

	// StartSession starts the game with the players in the lobby.
	StartSession func(ctx context.Context) (StartSessionResult, error)

	// CancelSession ends the Session as cancelled by its creator.
	CancelSession func(ctx context.Context) (CancelSessionResult, error)

	// NextLobbyUpdate blocks until the lobby changes; see LobbyUpdate. False
	// means no update follows: the Session started or the connection ended.
	NextLobbyUpdate func(ctx context.Context) (LobbyUpdate, bool)

	// Started blocks until the Session starts; false means the connection
	// ended, or ctx was done, first.
	Started func(ctx context.Context) (SessionStart, bool)

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

	// PerspectiveCreatorPlaying means the creator also plays, so watching
	// another participant is denied.
	PerspectiveCreatorPlaying SelectPerspectiveOutcome = "creator_is_playing"
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

// StartSessionOutcome is the decision a StartSession call reached.
type StartSessionOutcome string

const (
	SessionStarted StartSessionOutcome = "started"

	// StartNotInLobby means the Session already started, ended, or its lobby
	// expired.
	StartNotInLobby StartSessionOutcome = "not_in_lobby"

	// StartInvalidPlayerCount means the lobby holds fewer or more players
	// than the game allows.
	StartInvalidPlayerCount StartSessionOutcome = "invalid_player_count"

	// StartGameUnavailable means the Session's game version can no longer be
	// played.
	StartGameUnavailable StartSessionOutcome = "game_unavailable"

	// StartFailed means the game's script failed to start, so the Session was
	// aborted and its connections end as EndSessionAborted.
	StartFailed StartSessionOutcome = "start_failed"
)

// StartSessionResult is the outcome of starting a Session.
type StartSessionResult struct {
	Outcome StartSessionOutcome
}

// LobbyUpdate is the lobby's members after a change. Closed means the lobby
// closed without the Session starting: no update follows it and the
// connection ends.
type LobbyUpdate struct {
	Members []LobbyMember
	Closed  bool
}

// LobbyMember is a player waiting in the lobby.
type LobbyMember struct {
	UserUUID string
}

// SessionStart is who plays a started Session, in ParticipantID order.
type SessionStart struct {
	Participants []Participant
}

// Participant is a player of a started Session. The game knows them only by
// ParticipantID, which never changes for the Session.
type Participant struct {
	ParticipantID string
	UserUUID      string
}

// PlayerStart is a Session's start as one of its players sees it.
type PlayerStart struct {
	SessionStart

	// ParticipantID is the player's own.
	ParticipantID string
}

// AttachPlayerResult is the outcome of attaching a player connection.
// Rejection is set only when rejected, Connection only when accepted.
type AttachPlayerResult struct {
	Outcome    AttachOutcome
	Rejection  RejectReason
	Connection PlayerConnection
}

// PlayerConnection is everything a caller can do with an admitted player
// connection. The caller must run NextLobbyUpdate, Started, NextView and
// Ended until they report the end, and call Disconnect once when the client
// is gone.
type PlayerConnection struct {
	// Leave removes the player from the Session for good; the connection
	// then ends as EndLeft.
	Leave func(ctx context.Context) (LeaveResult, error)

	// Sample delivers the player's current input state. It is not answered;
	// how far SAMPLEs were consumed arrives with the views.
	Sample func(ctx context.Context, sample Sample) error

	// Command runs a game command and returns its correlated result.
	Command func(ctx context.Context, command Command) (CommandResult, error)

	// Resync asks for the player's current state again; a FULL view follows
	// through NextView.
	Resync func(ctx context.Context) error

	// NextLobbyUpdate blocks until the lobby changes; see LobbyUpdate. False
	// means no update follows: the Session started or the connection ended.
	NextLobbyUpdate func(ctx context.Context) (LobbyUpdate, bool)

	// Started blocks until the Session starts; false means the connection
	// ended, or ctx was done, first.
	Started func(ctx context.Context) (PlayerStart, bool)

	// NextView blocks until the player's next view. Every view produced
	// before the connection ended is returned before false.
	NextView func(ctx context.Context) (OwnView, bool)

	// Ended blocks until the connection ends and reports why; false means
	// ctx was done first.
	Ended func(ctx context.Context) (ConnectionEnd, bool)

	// Disconnect tells the Runtime the client is gone. Calling it again has
	// no effect.
	Disconnect func()
}

// OwnView is a player's view of the game together with the highest SAMPLE
// sequence the game consumed, which the client reconciles its prediction with.
type OwnView struct {
	PlayerView
	ConsumedThroughSampleSequence int64
}

// Sample is a player's full input state. Sequence increases per player;
// a stale or repeated one is ignored.
type Sample struct {
	Sequence int64

	// Inputs is a JSON object of the inputs the game declares; omitted ones
	// take their defaults.
	Inputs []byte
}

// Command is a request to the game. Token makes it idempotent within the
// player's Session.
type Command struct {
	Token string
	Name  string

	// Payload is JSON whose shape the game declares for Name; it may be nil.
	Payload []byte
}

// CommandOutcome is how the Runtime handled a Command.
type CommandOutcome string

const (
	// CommandExecuted means the game ran the command; Response holds its
	// answer, which may be a rejection by the game's own rules.
	CommandExecuted CommandOutcome = "executed"

	// CommandAlreadyProcessed means the token was already executed; its
	// original response is not kept.
	CommandAlreadyProcessed CommandOutcome = "already_processed"

	// CommandInvalid means the command is not declared by the game or its
	// payload does not match the declaration.
	CommandInvalid CommandOutcome = "invalid"

	// CommandNotRunning means the Session is not running a game.
	CommandNotRunning CommandOutcome = "session_not_running"
)

// CommandResult is the outcome of a Command. Response is set only when
// executed, as JSON whose shape only the game defines.
type CommandResult struct {
	Token    string
	Outcome  CommandOutcome
	Response []byte
}

// LeaveOutcome is the decision a Leave call reached.
type LeaveOutcome string

const (
	PlayerLeft LeaveOutcome = "left"
)

// LeaveResult is the outcome of leaving a Session.
type LeaveResult struct {
	Outcome LeaveOutcome
}
