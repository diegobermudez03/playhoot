// Package checkplayability answers whether a Game can currently be played by a
// given user.
//
// Placeholder: it does not look the Game up.
package checkplayability

import "context"

// Service is Game Management's playability-check use case.
type Service struct{}

// New constructs a Service.
func New() *Service {
	return &Service{}
}

// IsPlayable reports whether userUUID can currently play gameUUID; a Game that
// does not exist is simply not playable.
//
// Placeholder: answers true for everything.
func (s *Service) IsPlayable(_ context.Context, _ string, _ string) (bool, error) {
	return true, nil
}
