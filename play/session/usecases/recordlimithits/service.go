// Package recordlimithits keeps, for auditing, the platform limits the users
// of live Sessions hit, next to the game faults their clients report.
package recordlimithits

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/recordlimithits/internal/repo"
)

//go:generate mockgen -package=recordlimithits -destination=service_mock_test.go . hitStore

// hitStore adds limit hits to the Sessions' audit records.
type hitStore interface {
	RecordHits(ctx context.Context, hits []internalrepo.Hit, recordedAt time.Time) (int, error)
}

// maxLimitLength bounds a limit's name, which becomes a key of the record.
const maxLimitLength = 64

// Service is the limit hit recording use case.
type Service struct {
	hits hitStore
	now  func() time.Time
}

// New constructs a Service recording in db.
func New(db *gorm.DB) *Service {
	return &Service{hits: internalrepo.New(db), now: time.Now}
}

// RecordLimitHits stores hits in one transaction, skipping any naming no
// valid Session or user, no limit or no refusal. It returns how many were
// stored.
func (s *Service) RecordLimitHits(ctx context.Context, hits []sessionpkg.LimitHit) (int, error) {
	valid := make([]internalrepo.Hit, 0, len(hits))
	for _, hit := range hits {
		if !validHit(hit) {
			continue
		}
		valid = append(valid, internalrepo.Hit{SessionUUID: hit.SessionUUID, UserUUID: hit.UserUUID, Limit: hit.Limit, Count: hit.Count})
	}
	if len(valid) == 0 {
		return 0, nil
	}
	return s.hits.RecordHits(ctx, valid, s.now().UTC().Truncate(time.Microsecond))
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
