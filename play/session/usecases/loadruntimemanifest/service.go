// Package loadruntimemanifest reads the runtime manifest of the game version a
// Session is pinned to.
package loadruntimemanifest

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/monitoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/manifest"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/loadruntimemanifest/internal/repo"
)

//go:generate mockgen -package=loadruntimemanifest -destination=service_mock_test.go . manifestFinder

// manifestFinder reads a Session's raw manifest.
type manifestFinder interface {
	FindSessionManifest(ctx context.Context, sessionUUID string) ([]byte, bool, error)
}

var errBrokenManifest = errors.New("session's game version has no valid manifest")

// Service is the runtime manifest use case.
type Service struct {
	manifests manifestFinder
}

// New constructs a Service reading from db.
func New(db *gorm.DB) *Service {
	return &Service{manifests: internalrepo.New(db)}
}

// LoadRuntimeManifest returns the manifest of the version sessionUUID is pinned
// to, or sessionpkg.ErrSessionNotFound when there is no such Session.
func (s *Service) LoadRuntimeManifest(ctx context.Context, sessionUUID string) (sessionpkg.RuntimeManifest, error) {
	if _, err := uuid.Parse(sessionUUID); err != nil {
		return sessionpkg.RuntimeManifest{}, sessionpkg.ErrSessionNotFound
	}
	raw, found, err := s.manifests.FindSessionManifest(ctx, sessionUUID)
	if err != nil {
		return sessionpkg.RuntimeManifest{}, err
	}
	if !found {
		return sessionpkg.RuntimeManifest{}, sessionpkg.ErrSessionNotFound
	}
	parsed, err := manifest.Parse(raw)
	if err != nil {
		// Only an EXECUTABLE version, whose manifest was valid when frozen,
		// can be pinned: anything else is broken data.
		monitoring.Alert(ctx, "play session is pinned to a game version without a valid runtime manifest")
		return sessionpkg.RuntimeManifest{}, fmt.Errorf("%w: %w", errBrokenManifest, err)
	}
	var declared []sessionpkg.RuntimeInput
	for _, input := range parsed.Inputs {
		declared = append(declared, sessionpkg.RuntimeInput{
			Name: input.Name, Type: string(input.Type), Nullable: input.Nullable,
			Min: input.Min, Max: input.Max, Values: input.Values, Default: input.Default,
		})
	}
	return sessionpkg.RuntimeManifest{MinPlayers: parsed.MinPlayers, MaxPlayers: parsed.MaxPlayers, Inputs: declared}, nil
}
