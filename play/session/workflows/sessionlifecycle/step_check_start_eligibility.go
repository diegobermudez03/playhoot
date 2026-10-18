package sessionlifecycle

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/manifest"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/sessionstatus"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/sessionlifecycle/internal/repo"
)

//go:generate mockgen -package=sessionlifecycle -destination=step_check_start_eligibility_mock_test.go . checkStartRepo,scriptLoader

// checkStartRepo is CheckStartEligibility's own narrow persistence contract.
type checkStartRepo interface {
	FindStartCandidate(ctx context.Context, sessionUUID string) (*internalrepo.StartCandidate, error)
}

// scriptLoader reads verified game version content from object storage.
type scriptLoader interface {
	Load(ctx context.Context, locator objectstore.Locator) ([]byte, error)
}

// CheckStartEligibility reports whether userUUID may start sessionUUID with
// playerCount players and, when eligible, returns the backend script of the
// version it is pinned to.
func (m *Manager) CheckStartEligibility(ctx context.Context, sessionUUID string, userUUID string, playerCount int) (sessionpkg.StartEligibility, error) {
	defer logging.Step(ctx, "sessionlifecycle.check_start_eligibility").Close()

	if _, err := uuid.Parse(sessionUUID); err != nil {
		return sessionpkg.StartEligibility{Outcome: sessionpkg.StartSessionNotFound}, nil
	}
	candidate, err := m.checkStartRepo.FindStartCandidate(ctx, sessionUUID)
	if err != nil {
		return sessionpkg.StartEligibility{}, err
	}
	if candidate == nil {
		return sessionpkg.StartEligibility{Outcome: sessionpkg.StartSessionNotFound}, nil
	}
	if !sameUUID(candidate.CreatedByUserUUID, userUUID) {
		return sessionpkg.StartEligibility{Outcome: sessionpkg.StartNotCreator}, nil
	}
	switch sessionstatus.Status(candidate.Status) {
	case sessionstatus.Lobby:
		// An overdue lobby has expired even if nothing recorded it yet.
		if !m.now().Before(candidate.LobbyExpiresAt) {
			return sessionpkg.StartEligibility{Outcome: sessionpkg.StartNotInLobby}, nil
		}
	case sessionstatus.Running:
		return sessionpkg.StartEligibility{Outcome: sessionpkg.StartAlreadyStarted}, nil
	default:
		return sessionpkg.StartEligibility{Outcome: sessionpkg.StartNotInLobby}, nil
	}

	// Only an EXECUTABLE version, with a valid manifest and its script, can
	// be pinned: anything else is broken data, not an unavailable game.
	if !gameversion.Status(candidate.VersionStatus).AllowsSessions() {
		monitoring.Alert(ctx, "play session is pinned to a game version that is not executable")
		return sessionpkg.StartEligibility{}, fmt.Errorf("game version %s is %s, not executable", candidate.GameVersionUUID, candidate.VersionStatus)
	}
	parsed, err := manifest.Parse(candidate.Manifest)
	if err != nil {
		monitoring.Alert(ctx, "play session is pinned to a game version without a valid runtime manifest")
		return sessionpkg.StartEligibility{}, fmt.Errorf("game version %s: %w", candidate.GameVersionUUID, err)
	}
	if playerCount < parsed.MinPlayers || playerCount > parsed.MaxPlayers {
		return sessionpkg.StartEligibility{Outcome: sessionpkg.StartInvalidPlayerCount}, nil
	}
	if candidate.ScriptObjectKey == nil || candidate.ScriptSHA256 == nil || candidate.ScriptSize == nil {
		monitoring.Alert(ctx, "play session is pinned to a game version without a backend script")
		return sessionpkg.StartEligibility{}, fmt.Errorf("game version %s has no backend script", candidate.GameVersionUUID)
	}
	script, err := m.scripts.Load(ctx, objectstore.Locator{
		ObjectKey: *candidate.ScriptObjectKey,
		SHA256:    *candidate.ScriptSHA256,
		Size:      *candidate.ScriptSize,
	})
	if err != nil {
		return sessionpkg.StartEligibility{}, fmt.Errorf("loading backend script: %w", err)
	}
	return sessionpkg.StartEligibility{
		Outcome:         sessionpkg.StartEligible,
		GameVersionUUID: candidate.GameVersionUUID,
		Script:          script,
	}, nil
}

// sameUUID compares two UUIDs regardless of their textual form.
func sameUUID(a string, b string) bool {
	parsedA, errA := uuid.Parse(a)
	parsedB, errB := uuid.Parse(b)
	return errA == nil && errB == nil && parsedA == parsedB
}
