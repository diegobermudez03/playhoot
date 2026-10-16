// Package runtime is the live Session Runtime's public contract. It imports
// only the standard library; the message catalog grows as messages are
// needed.
package runtime

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

// Connection is a client's live connection as two channels of Runtime
// messages. The transport side writes Inbound and closes it when the client
// leaves; the Runtime writes Outbound and ends the connection by sending a
// Close and then closing Outbound. The Runtime must never block writing to
// Outbound: a client too slow to keep up is the Runtime's to disconnect.
type Connection struct {
	Inbound  chan<- ClientMessage
	Outbound <-chan ServerMessage
}

// ClientMessage is a message a client sends to the Runtime.
type ClientMessage interface {
	isClientMessage()
}

// ServerMessage is a message the Runtime sends to a client.
type ServerMessage interface {
	isServerMessage()
}

// Rejected tells the client it was not admitted; a Close follows it.
type Rejected struct {
	Reason RejectReason
}

func (Rejected) isServerMessage() {}

// RejectReason is why a connection was not admitted.
type RejectReason string

const (
	RejectSessionUnavailable RejectReason = "session_unavailable"
	RejectNotCreator         RejectReason = "not_creator"

	// RejectOwnerChanged means the connection was routed to a former owner;
	// the client should reconnect.
	RejectOwnerChanged RejectReason = "owner_changed"
)

// Close ends the connection with Reason. It is the last message on a
// connection and is not delivered as a message.
type Close struct {
	Reason CloseReason
}

func (Close) isServerMessage() {}

// CloseReason is why the Runtime ends a connection.
type CloseReason string

const (
	CloseAdmissionRejected CloseReason = "admission_rejected"
)

// AttachOutcome is whether a connection was admitted into a Session.
type AttachOutcome string

const (
	AttachAccepted AttachOutcome = "accepted"
	AttachRejected AttachOutcome = "rejected"
)

// AttachAdminResult is the outcome of attaching an admin connection. Its
// Connection is served either way; a rejected one only delivers the
// rejection and closes.
type AttachAdminResult struct {
	Outcome    AttachOutcome
	Connection Connection
}
