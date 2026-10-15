// Package playlifecycle groups the cross-domain write operations that take a
// Game from being chosen to being played as a Session: today creating a
// Session, later starting or cancelling one, and so on. It is organized by
// that behavior, not by the domains involved, so it can call as many domains
// as the behavior needs.
package playlifecycle

// Manager coordinates the play-lifecycle operations. Each operation lives in
// its own file and declares its own narrow interfaces for the capabilities it
// calls. It owns no business entity.
type Manager struct {
	games    gamePlayabilityChecker
	sessions sessionCreator
}

// New constructs a Manager. games is Game Management's side of the
// operations that need to know whether a Game can be played, and sessions is
// the Session domain's durable creation.
func New(games gamePlayabilityChecker, sessions sessionCreator) *Manager {
	return &Manager{games: games, sessions: sessions}
}
