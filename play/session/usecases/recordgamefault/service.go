// Package recordgamefault keeps the faults players' clients report about the
// game frontend of their running Session, so a database audit can tell games
// that misbehave from clients that lie.
package recordgamefault

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gamefault"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/recordgamefault/internal/repo"
)

//go:generate mockgen -package=recordgamefault -destination=service_mock_test.go . faultStore

// faultStore stores game fault reports.
type faultStore interface {
	RecordFault(ctx context.Context, sessionUUID string, fault internalrepo.Fault, reportedAt time.Time) (bool, error)
}

// Service is the game fault recording use case.
type Service struct {
	faults faultStore
	now    func() time.Time
}

// New constructs a Service recording faults in db.
func New(db *gorm.DB) *Service {
	return &Service{faults: internalrepo.New(db), now: time.Now}
}

// RecordGameFault stores fault for sessionUUID while the Session runs and the
// reporter plays it. The same participant reporting the same kind again in
// the Session is counted on the same record. A fault of an unknown kind or
// with an invalid detail is an error: the caller should have refused it.
func (s *Service) RecordGameFault(ctx context.Context, sessionUUID string, fault sessionpkg.GameFault) (sessionpkg.RecordGameFaultOutcome, error) {
	if _, err := uuid.Parse(sessionUUID); err != nil {
		return "", sessionpkg.ErrSessionNotFound
	}
	if _, err := uuid.Parse(fault.UserUUID); err != nil || fault.ParticipantID == "" {
		return "", errors.New("game fault without a valid reporter")
	}
	if !gamefault.Known(fault.Kind) {
		return "", fmt.Errorf("unknown game fault kind %q", fault.Kind)
	}
	if !gamefault.ValidDetail(fault.Detail) {
		return "", fmt.Errorf("game fault detail is not a JSON object within %d bytes", sessionpkg.MaxGameFaultDetailBytes)
	}
	recorded, err := s.faults.RecordFault(ctx, sessionUUID, internalrepo.Fault{
		UserUUID:      fault.UserUUID,
		ParticipantID: fault.ParticipantID,
		Kind:          string(fault.Kind),
		Detail:        fault.Detail,
	}, s.now().UTC().Truncate(time.Microsecond))
	if err != nil {
		return "", err
	}
	if !recorded {
		return sessionpkg.GameFaultNotRecorded, nil
	}
	return sessionpkg.GameFaultRecorded, nil
}
