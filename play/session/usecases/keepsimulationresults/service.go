// Package keepsimulationresults records the simulations the Runtime plays on
// a version: each one's whole result, kept until it expires for its states
// to be read on demand, and a record of it afterwards, for counting.
package keepsimulationresults

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/keepsimulationresults/internal/repo"
)

// RecordRetention is how long a simulation's record stays after it was
// played (its result is cleared once it expires), so simulations can be
// counted over that window.
const RecordRetention = 7 * 24 * time.Hour

//go:generate mockgen -package=keepsimulationresults -destination=service_mock_test.go . resultsRepo

// resultsRepo is the use case's persistence contract.
type resultsRepo interface {
	FindVersionID(ctx context.Context, versionUUID string) (uint, bool, error)
	Insert(ctx context.Context, resultUUID string, versionID uint, result json.RawMessage, createdAt time.Time, expiresAt time.Time) error
	ClearExpired(ctx context.Context, now time.Time, forgetBefore time.Time) error
	Find(ctx context.Context, resultUUID string, versionUUID string, now time.Time) (json.RawMessage, bool, error)
	CountSince(ctx context.Context, versionUUID string, since time.Time) (int, error)
}

// Service is the simulation results use case.
type Service struct {
	results resultsRepo
	now     func() time.Time
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{results: internalrepo.New(db), now: time.Now}
}

// KeepSimulationResult records result as resultUUID of versionUUID, readable
// until expiresAt. A version Play does not have is sessionpkg.ErrDraftNotFound.
// Expired results are cleared and old records forgotten on the way.
func (s *Service) KeepSimulationResult(ctx context.Context, versionUUID string, resultUUID string, result json.RawMessage, expiresAt time.Time) error {
	defer logging.Step(ctx, "keepsimulationresults.keep").Close()

	if _, err := uuid.Parse(resultUUID); err != nil {
		return fmt.Errorf("result uuid is not a valid UUID")
	}
	if _, err := uuid.Parse(versionUUID); err != nil {
		return fmt.Errorf("version %q: %w", versionUUID, sessionpkg.ErrDraftNotFound)
	}
	versionID, found, err := s.results.FindVersionID(ctx, versionUUID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("version %s: %w", versionUUID, sessionpkg.ErrDraftNotFound)
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	if err := s.results.ClearExpired(ctx, now, now.Add(-RecordRetention)); err != nil {
		return err
	}
	return s.results.Insert(ctx, resultUUID, versionID, result, now, expiresAt.UTC().Truncate(time.Microsecond))
}

// FindSimulationResult is the result resultUUID names when it is
// versionUUID's and has not expired, or false.
func (s *Service) FindSimulationResult(ctx context.Context, resultUUID string, versionUUID string) (json.RawMessage, bool, error) {
	if _, err := uuid.Parse(resultUUID); err != nil {
		return nil, false, nil
	}
	if _, err := uuid.Parse(versionUUID); err != nil {
		return nil, false, nil
	}
	return s.results.Find(ctx, resultUUID, versionUUID, s.now().UTC())
}

// CountSimulations counts versionUUID's simulations played after since
// (within RecordRetention): what a cap on simulations per version counts.
func (s *Service) CountSimulations(ctx context.Context, versionUUID string, since time.Time) (int, error) {
	if _, err := uuid.Parse(versionUUID); err != nil {
		return 0, nil
	}
	return s.results.CountSince(ctx, versionUUID, since.UTC())
}
