// Package recordlimithits keeps, for auditing, the platform limits the users
// of live Sessions hit, next to the game faults their clients report.
package recordlimithits

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/recordlimithits/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=recordlimithits -destination=service_mock_test.go . hitStore

// hitStore reads the Sessions limit hits belong to and stores their reports.
type hitStore interface {
	FindTarget(ctx context.Context, tx *gorm.DB, sessionUUID string, userUUID string) (*internalrepo.Target, error)
	CreateReport(ctx context.Context, tx *gorm.DB, report internalrepo.Report) error
}

// maxLimitLength bounds a limit's name.
const maxLimitLength = 64

// serverLimitKind is the kind of every limit hit report.
const serverLimitKind = "SERVER_LIMIT"

// Service is the limit hit recording use case.
type Service struct {
	db   utils.DBServicer
	hits hitStore
	now  func() time.Time
}

// New constructs a Service recording in db.
func New(db *gorm.DB) *Service {
	repo := internalrepo.New(db)
	return &Service{db: repo, hits: repo, now: time.Now}
}

// RecordLimitHits stores hits in one transaction, skipping any naming no
// valid Session or user, no limit or no refusal. It returns how many were
// stored.
func (s *Service) RecordLimitHits(ctx context.Context, hits []sessionpkg.LimitHit) (int, error) {
	valid := make([]sessionpkg.LimitHit, 0, len(hits))
	for _, hit := range hits {
		if validHit(hit) {
			valid = append(valid, hit)
		}
	}
	if len(valid) == 0 {
		return 0, nil
	}
	recordedAt := s.now().UTC().Truncate(time.Microsecond)
	return utils.RunInDBTransaction(ctx, s.db, func(ctx context.Context, tx *gorm.DB) (int, error) {
		return s.recordInTx(ctx, tx, valid, recordedAt)
	})
}

// recordInTx stores one report per hit: the limit, and how many times it
// refused the user since the previous flush. A hit of a Session that does not
// exist is skipped.
func (s *Service) recordInTx(ctx context.Context, tx *gorm.DB, hits []sessionpkg.LimitHit, recordedAt time.Time) (int, error) {
	recorded := 0
	for _, hit := range hits {
		target, err := s.hits.FindTarget(ctx, tx, hit.SessionUUID, hit.UserUUID)
		if err != nil {
			return 0, err
		}
		if target == nil {
			continue
		}
		detail, err := json.Marshal(map[string]string{"limit": hit.Limit})
		if err != nil {
			return 0, fmt.Errorf("encoding limit hit detail: %w", err)
		}
		err = s.hits.CreateReport(ctx, tx, internalrepo.Report{
			SessionID:     target.SessionID,
			GameVersionID: target.GameVersionID,
			UserUUID:      hit.UserUUID,
			ParticipantID: target.ParticipantID,
			Source:        string(sessionpkg.GameFaultFromServer),
			Kind:          serverLimitKind,
			Detail:        string(detail),
			Occurrences:   hit.Count,
			ReportedAt:    recordedAt,
		})
		if err != nil {
			return 0, err
		}
		recorded++
	}
	return recorded, nil
}

func validHit(hit sessionpkg.LimitHit) bool {
	if _, err := uuid.Parse(hit.SessionUUID); err != nil {
		return false
	}
	if _, err := uuid.Parse(hit.UserUUID); err != nil {
		return false
	}
	return hit.Limit != "" && len(hit.Limit) <= maxLimitLength && hit.Count > 0
}
