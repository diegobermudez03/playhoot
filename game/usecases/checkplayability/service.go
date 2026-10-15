// Package checkplayability is Game Management's capability for answering
// whether a Game can currently be played by a given user.
//
// This is a placeholder: it does not look the Game up at all. It is here so
// the callers of this capability can be built and wired before Game
// Management's real behavior exists.
package checkplayability

import "context"

// Service is Game Management's playability-check use case.
type Service struct{}

// New constructs a Service.
func New() *Service {
	return &Service{}
}

// IsPlayable reports whether gameUUID identifies a Game that userUUID can
// currently play. A Game that does not exist and one that is not available
// to the user are both simply not playable.
//
// For now it ignores both arguments and answers true for everything.
//
// Eventually it resolves the Game and answers from its current state: it
// must exist, have a published version a Session can be created from, and
// be visible to userUUID (public or hidden for anyone, private only for its
// owner).
func (s *Service) IsPlayable(_ context.Context, _ string, _ string) (bool, error) {
	return true, nil
}
