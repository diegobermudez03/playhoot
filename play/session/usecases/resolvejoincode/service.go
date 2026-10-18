// Package resolvejoincode finds the Session a join code names.
package resolvejoincode

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/resolvejoincode/internal/repo"
)

//go:generate mockgen -package=resolvejoincode -destination=service_mock_test.go . joinCodeFinder

// joinCodeFinder reads active join codes.
type joinCodeFinder interface {
	FindActiveJoinCode(ctx context.Context, code string) (*internalrepo.JoinCode, error)
}

// Service is the join code resolution use case.
type Service struct {
	joinCodes joinCodeFinder
	now       func() time.Time
}

// New constructs a Service reading from db.
func New(db *gorm.DB) *Service {
	return &Service{joinCodes: internalrepo.New(db), now: time.Now}
}

// ResolveJoinCode returns the Session joinCode names while the code is active
// and unexpired. Codes are matched regardless of letter case.
func (s *Service) ResolveJoinCode(ctx context.Context, joinCode string) (sessionpkg.JoinCodeResolution, error) {
	found, err := s.joinCodes.FindActiveJoinCode(ctx, strings.ToUpper(joinCode))
	if err != nil {
		return sessionpkg.JoinCodeResolution{}, err
	}
	// An overdue code has expired even if nothing released it yet.
	if found == nil || !s.now().Before(found.ExpiresAt) {
		return sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeNotFound}, nil
	}
	return sessionpkg.JoinCodeResolution{Outcome: sessionpkg.JoinCodeFound, SessionUUID: found.SessionUUID}, nil
}
