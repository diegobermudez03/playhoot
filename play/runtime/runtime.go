// Package runtime is the live Session Runtime's public contract. It imports
// only the standard library; each kind of live connection declares its own
// operations and messages as they are needed.
package runtime

import (
	"context"
	"errors"
	"time"
)

// OwnerOutcome is where the live owner of a Session was found.
type OwnerOutcome string

const (
	// OwnerHere means this instance owns the Session, already or by claiming
	// it now.
	OwnerHere OwnerOutcome = "owned_here"

	// OwnerElsewhere means another instance owns the Session.
	OwnerElsewhere OwnerOutcome = "owned_elsewhere"

	// OwnerSessionNotFound means no Session has that UUID, so none was
	// claimed.
	OwnerSessionNotFound OwnerOutcome = "session_not_found"

	// OwnerSessionEnded means the Session ended, so it was not claimed;
	// Ended says how.
	OwnerSessionEnded OwnerOutcome = "session_ended"

	// OwnerSessionUnavailable means the Session exists and has not ended but
	// cannot be joined live, such as a lobby past its expiry, so it was not
	// claimed.
	OwnerSessionUnavailable OwnerOutcome = "session_unavailable"
)

// OwnerPlacement is the live owner of a Session. Address and Epoch are set
// only when the Session has an owner, Ended only when it ended.
type OwnerPlacement struct {
	Outcome OwnerOutcome

	// Address is the owner's internal address; it must never reach a client.
	Address string

	// Epoch identifies this period of ownership; the owner rejects live
	// operations routed under an older one.
	Epoch int64

	Ended EndedSession
}

// EndedSession is how a Session ended: its terminal status and end cause, as
// the durable Session records them.
type EndedSession struct {
	Status   string
	EndCause string
}

// Player is who opens a player connection, as the caller resolved them.
type Player struct {
	UserUUID string

	// DisplayName is how the other players see them.
	DisplayName string

	// Device is the class the client says it is; any value other than
	// DeviceDesktop or DeviceMobile, none included, is a device no game
	// supports.
	Device Device
}

// Device is the class a player's client classifies itself as.
type Device string

const (
	DeviceDesktop Device = "desktop"
	DeviceMobile  Device = "mobile"
)

// InputRequirements are the interactions a game version uses and the device
// classes that can play it: Devices is desktop, mobile or both.
type InputRequirements struct {
	Devices          string
	UsesKeyboard     bool
	UsesPointerHover bool
	UsesPointerPress bool
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

	// RejectParticipantDisconnected means the player was disconnected, by
	// leaving or being away past the transport grace, and the game does not
	// support rejoining.
	RejectParticipantDisconnected RejectReason = "participant_disconnected"

	// RejectRejoinRejected means the player was disconnected and the game
	// did not take them back.
	RejectRejoinRejected RejectReason = "rejoin_rejected"

	// RejectOwnerChanged means the connection was routed to a former owner;
	// the client should reconnect.
	RejectOwnerChanged RejectReason = "owner_changed"

	// RejectTooManyAdmins means the Session has as many admin connections as
	// it allows.
	RejectTooManyAdmins RejectReason = "too_many_admins"

	// RejectServerBusy means this instance runs as many live Sessions as it
	// may; the client may try again later.
	RejectServerBusy RejectReason = "server_busy"

	// RejectDeviceNotSupported means the player's device cannot play the
	// Session's game, checked on every player connection; the rejection
	// carries the game's InputRequirements.
	RejectDeviceNotSupported RejectReason = "device_not_supported"
)

// EndReason is why the Runtime ended a live connection.
type EndReason string

const (
	// EndDisconnected means the connection's Disconnect was called.
	EndDisconnected EndReason = "disconnected"

	// EndSessionEnded means the Session reached a terminal state; the
	// ConnectionEnd tells which and why when the Runtime knows it.
	EndSessionEnded EndReason = "session_ended"

	// EndLeft means the player left the Session, or its lobby.
	EndLeft EndReason = "left"

	// EndReplaced means the same user opened a newer player connection,
	// which took this one's place; the client should not reconnect.
	EndReplaced EndReason = "replaced"

	// EndOwnerChanged means this instance no longer serves the Session under
	// the connection's ownership; the client should reconnect.
	EndOwnerChanged EndReason = "owner_changed"
)

// ConnectionEnd is how a live connection ended on the Runtime's side. When
// the Session ended, SessionStatus and EndCause are the terminal status and
// cause it reached (FINISHED, CANCELLED, EXPIRED or ABORTED), for the client
// to present; they are empty otherwise.
type ConnectionEnd struct {
	Reason        EndReason
	SessionStatus string
	EndCause      string
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

	// Resync asks for the selected participant's current view again; a FULL
	// view follows through NextView.
	Resync func(ctx context.Context) error

	// LimitHit records, for the Session's audit, that a limit of the entry
	// point refused what this admin sent. It never blocks.
	LimitHit func(ctx context.Context, limit string)

	// StartSession starts the game with the players in the lobby.
	StartSession func(ctx context.Context) (StartSessionResult, error)

	// CancelSession ends the Session, in its lobby or running, as cancelled
	// by its creator; every connection then ends as EndSessionEnded.
	CancelSession func(ctx context.Context) (CancelSessionResult, error)

	// NextLobbyUpdate blocks until the lobby changes; see LobbyUpdate. False
	// means no update follows: the Session started or the connection ended.
	NextLobbyUpdate func(ctx context.Context) (LobbyUpdate, bool)

	// Started blocks until the Session starts; false means the connection
	// ended, or ctx was done, first.
	Started func(ctx context.Context) (SessionStart, bool)

	// NextView blocks until the next view of the selected participant. Every
	// view produced before the connection ended is returned before false.
	NextView func(ctx context.Context) (WatchedView, bool)

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

	// Revision numbers the views of one connection: each FULL view or patch
	// is the previous one plus one, and a view that changes nothing repeats
	// it. A patch that does not follow the client's revision means one was
	// lost: the client asks for a resync.
	Revision int64

	// State is the projection the game's script returned, as JSON whose shape
	// only the game defines.
	State []byte
}

// WatchedView is a participant's view as an admin watching them receives it:
// their logical events, but not their acknowledgement nor their errors.
type WatchedView struct {
	PlayerView

	// Events are delivered like OwnView's.
	Events [][]byte
}

// SelectPerspectiveOutcome is the decision a SelectPerspective call reached.
type SelectPerspectiveOutcome string

const (
	PerspectiveSelected           SelectPerspectiveOutcome = "selected"
	PerspectiveUnknownParticipant SelectPerspectiveOutcome = "unknown_participant"

	// PerspectiveCreatorPlaying means the creator had a player connection in
	// this Session, so watching any participant is denied for good.
	PerspectiveCreatorPlaying SelectPerspectiveOutcome = "creator_is_playing"

	// PerspectiveNotRunning means the Session is not running a game; nothing
	// is selected.
	PerspectiveNotRunning SelectPerspectiveOutcome = "session_not_running"
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

// TerminateSessionOutcome is the decision a TerminateSession call reached.
type TerminateSessionOutcome string

const (
	// SessionTerminated means the platform ended the Session now.
	SessionTerminated TerminateSessionOutcome = "terminated"

	// TerminateAlreadyEnded means the Session had already ended.
	TerminateAlreadyEnded TerminateSessionOutcome = "already_ended"
)

// TerminateSessionResult is the outcome of the platform ending a Session.
type TerminateSessionResult struct {
	Outcome TerminateSessionOutcome
}

// StartSessionOutcome is the decision a StartSession call reached.
type StartSessionOutcome string

const (
	SessionStarted StartSessionOutcome = "started"

	// StartAlreadyStarted means the Session is already running.
	StartAlreadyStarted StartSessionOutcome = "already_started"

	// StartInProgress means another Start attempt is running; its outcome
	// reaches every connection.
	StartInProgress StartSessionOutcome = "start_in_progress"

	// StartNotInLobby means the Session ended or its lobby expired.
	StartNotInLobby StartSessionOutcome = "not_in_lobby"

	// StartInvalidPlayerCount means the lobby holds fewer or more players
	// than the game allows.
	StartInvalidPlayerCount StartSessionOutcome = "invalid_player_count"

	// StartGameUnavailable means the Session's game version can no longer be
	// played.
	StartGameUnavailable StartSessionOutcome = "game_unavailable"

	// StartFailed means the game's script failed to start, so the Session was
	// aborted and its connections end as EndSessionEnded.
	StartFailed StartSessionOutcome = "start_failed"

	// StartSandboxUnavailable means this instance cannot run another game
	// now; the Session stays in its lobby and starting again may succeed.
	StartSandboxUnavailable StartSessionOutcome = "sandbox_unavailable"
)

// StartSessionResult is the outcome of starting a Session.
type StartSessionResult struct {
	Outcome StartSessionOutcome
}

// LobbyUpdate is the lobby's members after a change, with how many players
// the game can start with. A lobby that closes without starting ends its
// connections, telling them how the Session ended.
type LobbyUpdate struct {
	Members    []LobbyMember
	MinPlayers int
	MaxPlayers int
}

// LobbyMember is a player waiting in the lobby.
type LobbyMember struct {
	UserUUID    string
	DisplayName string
}

// SessionStart is a started Session: who plays, in ParticipantID order, and
// the game frontend that renders their views.
type SessionStart struct {
	Participants []Participant
	Frontend     GameFrontend
}

// GameFrontend is signed access to the frontend package and assets of the
// Session's game version, all expiring at ExpiresAt, after the Session ends;
// SHA256 lets the client verify what it downloads.
type GameFrontend struct {
	URL       string
	SHA256    string
	ExpiresAt time.Time
	Viewport  GameViewport

	// Assets are the version's assets the frontend may load; empty when it
	// has none.
	Assets []GameAsset
}

// GameAsset is signed access to one asset of the game version, by the logical
// key the game's frontend knows it as. Type is image, audio or video; SHA256
// and Size (bytes) let the client verify what it downloads.
type GameAsset struct {
	Key      string
	Type     string
	MIMEType string
	SHA256   string
	Size     int64
	URL      string
}

// GameViewport is the fixed shape the game's frontend is shown in: one of the
// aspect ratios Playhoot offers and its logical resolution, the size the
// frontend draws in and receives input in.
type GameViewport struct {
	AspectRatio string
	Width       int
	Height      int
}

// Participant is a player of a started Session. The game knows them only by
// ParticipantID, which never changes for the Session.
type Participant struct {
	ParticipantID string
	UserUUID      string
	DisplayName   string
}

// PresenceStatus is whether a running game's participant is playing.
type PresenceStatus string

const (
	// PresenceActive is a participant playing with a connection.
	PresenceActive PresenceStatus = "active"

	// PresenceReconnecting is a participant who lost their connection, or
	// came back by rejoining, and still plays for a short grace until a
	// connection attaches.
	PresenceReconnecting PresenceStatus = "reconnecting"

	// PresenceDisconnected is a participant the game was told disconnected
	// because their connection did not come back in time.
	PresenceDisconnected PresenceStatus = "disconnected"

	// PresenceLeft is a participant the game was told disconnected because
	// they left.
	PresenceLeft PresenceStatus = "left"
)

// RosterMember is a participant of a running game with their presence.
type RosterMember struct {
	Participant
	Status PresenceStatus
}

// RosterUpdate is every participant of a running game, in ParticipantID
// order, after a change of presence.
type RosterUpdate struct {
	Members []RosterMember
}

// PlayerStart is a Session's start as one of its players sees it.
type PlayerStart struct {
	SessionStart

	// ParticipantID is the player's own.
	ParticipantID string
}

// AttachPlayerResult is the outcome of attaching a player connection.
// Rejection is set only when rejected, Connection only when accepted, and
// Requirements only when rejected as RejectDeviceNotSupported.
type AttachPlayerResult struct {
	Outcome      AttachOutcome
	Rejection    RejectReason
	Connection   PlayerConnection
	Requirements InputRequirements
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
	// how far SAMPLEs were consumed arrives with the views. Inputs the game
	// does not declare are ErrInvalidSample.
	Sample func(ctx context.Context, sample Sample) error

	// Command runs a game command and returns its correlated result. A
	// command the game does not declare, or whose payload does not match its
	// declaration, is ErrInvalidCommand.
	Command func(ctx context.Context, command Command) (CommandResult, error)

	// Resync asks for the player's current state again; a FULL view follows
	// through NextView.
	Resync func(ctx context.Context) error

	// LimitHit records, for the Session's audit, that a limit of the entry
	// point refused what this player sent; limit names it, such as
	// api.command.rate. It never blocks.
	LimitHit func(ctx context.Context, limit string)

	// ReportGameFault records that the player's client saw the running
	// game's frontend fail or misbehave. It is not answered and never changes
	// the game: a report from a connection that does not play the running
	// game, or that cannot be stored, is dropped.
	ReportGameFault func(ctx context.Context, fault GameFault) error

	// NextLobbyUpdate blocks until the lobby changes; see LobbyUpdate. False
	// means no update follows: the Session started or the connection ended.
	NextLobbyUpdate func(ctx context.Context) (LobbyUpdate, bool)

	// Started blocks until the Session starts; false means the connection
	// ended, or ctx was done, first.
	Started func(ctx context.Context) (PlayerStart, bool)

	// NextView blocks until the player's next view. Every view produced
	// before the connection ended is returned before false.
	NextView func(ctx context.Context) (OwnView, bool)

	// NextRosterUpdate blocks until the running game's roster changes, the
	// first one right after the start; see RosterUpdate. Only the latest
	// not yet returned is kept. False means the connection ended.
	NextRosterUpdate func(ctx context.Context) (RosterUpdate, bool)

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

	// Errors are the logical errors the game returned to the player while
	// producing this view, in timeline order, each as JSON only the game
	// defines. They are sent once and never again, not even on a resync.
	Errors [][]byte

	// Events are the logical events the game emitted for the player while
	// producing this view, in timeline order, delivered like Errors.
	Events [][]byte
}

// Sample is a player's full input state. Sequence increases per player;
// a stale or repeated one is ignored.
type Sample struct {
	Sequence int64

	// Inputs is a JSON object of the inputs the game declares; omitted ones
	// take their defaults.
	Inputs []byte
}

// GameFaultKind is how a player's client saw the game frontend fail or
// misbehave.
type GameFaultKind string

const (
	GameFaultArtifactUnreachable      GameFaultKind = "artifact_unreachable"
	GameFaultArtifactHashMismatch     GameFaultKind = "artifact_hash_mismatch"
	GameFaultReadyTimeout             GameFaultKind = "ready_timeout"
	GameFaultUnsupportedBridgeVersion GameFaultKind = "unsupported_bridge_version"
	GameFaultContainerViolation       GameFaultKind = "container_violation"
	GameFaultBridgeAbuse              GameFaultKind = "bridge_abuse"
)

// GameFault is a player's client reporting a fault of the game frontend it
// ran: the client's claim, kept as audit evidence, never trusted. Detail is a
// JSON object the client chose.
type GameFault struct {
	Kind   GameFaultKind
	Detail []byte
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

	// CommandNotRunning means the Session is not running a game.
	CommandNotRunning CommandOutcome = "session_not_running"

	// CommandRateLimited means the player, or the Session, sent more commands
	// than allowed; the token was not used, so it may be retried later.
	CommandRateLimited CommandOutcome = "rate_limited"
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

// ErrInvalidSample is returned by Sample for inputs the game does not
// declare, or values they cannot hold: the game's frontend sent data the game
// cannot take. The SAMPLE never reaches the game, it was recorded as a fault
// of the game before returning (a failure to record it is only logged), and
// the caller ends the player's connection.
var ErrInvalidSample = errors.New("invalid sample")

// ErrInvalidCommand is returned by Command for a command the game does not
// declare, or a payload that does not match its declaration, as
// ErrInvalidSample is for a SAMPLE. The command never runs and its token stays
// unused. A command the game ran and answered with a rejection of its own is
// CommandExecuted, never this.
var ErrInvalidCommand = errors.New("invalid command")
