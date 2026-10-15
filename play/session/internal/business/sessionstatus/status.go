// Package sessionstatus defines the lifecycle states a Session goes through.
package sessionstatus

// Status is where a Session is in its lifecycle.
type Status string

const (
	// Lobby means the Session was created and players can join; it has not
	// started.
	Lobby Status = "LOBBY"

	// Running means the Session started and is being played.
	Running Status = "RUNNING"

	// Finished means the game itself ended the Session.
	Finished Status = "FINISHED"

	// Cancelled means the platform ended the Session at someone's request
	// (for example the host, or every player leaving).
	Cancelled Status = "CANCELLED"

	// Expired means the Session ran out of time (lobby expiry, inactivity,
	// maximum lifetime).
	Expired Status = "EXPIRED"

	// Aborted means the Session could not go on because of a failure.
	Aborted Status = "ABORTED"
)
