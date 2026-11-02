// Package recordgamefault keeps the faults of the game frontend of a running
// Session: those players' clients report, and those the platform sees in what
// a frontend sends, so a database audit can tell games that misbehave from
// clients that lie.
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
	"github.com/diegobermudez03/playhoot/play/session/internal/business/sessionstatus"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/recordgamefault/internal/repo"
)

//go:generate mockgen -package=recordgamefault -destination=service_mock_test.go . faultStore

// faultStore reads who reports and stores game fault reports.
type faultStore interface {
	FindReporter(ctx context.Context, sessionUUID string, userUUID string) (*internalrepo.Reporter, error)
	CreateReport(ctx context.Context, report internalrepo.Report) error
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
// participant plays it, each report on its own. A fault of a kind its source does
// not record, or with an invalid detail, is an error: the caller should have
// refused it.
func (s *Service) RecordGameFault(ctx context.Context, sessionUUID string, fault sessionpkg.GameFault) (sessionpkg.RecordGameFaultOutcome, error) {
	if _, err := uuid.Parse(sessionUUID); err != nil {
		return "", sessionpkg.ErrSessionNotFound
	}
	if _, err := uuid.Parse(fault.UserUUID); err != nil || fault.ParticipantID == "" {
		return "", errors.New("game fault without a valid reporter")
	}
	if !gamefault.Known(fault.Source, fault.Kind) {
		return "", fmt.Errorf("unknown game fault kind %q from %q", fault.Kind, fault.Source)
	}
	if !gamefault.ValidDetail(fault.Detail) {
		return "", fmt.Errorf("game fault detail is not a JSON object within %d bytes", sessionpkg.MaxGameFaultDetailBytes)
	}
	reporter, err := s.faults.FindReporter(ctx, sessionUUID, fault.UserUUID)
	if err != nil {
		return "", err
	}
	// Only a running Session's participant, under their own participant id.
	if reporter == nil || reporter.Status != string(sessionstatus.Running) ||
		reporter.ParticipantID == nil || *reporter.ParticipantID != fault.ParticipantID {
		return sessionpkg.GameFaultNotRecorded, nil
	}
	err = s.faults.CreateReport(ctx, internalrepo.Report{
		SessionID:     reporter.SessionID,
		GameVersionID: reporter.GameVersionID,
		UserUUID:      fault.UserUUID,
		ParticipantID: reporter.ParticipantID,
		Source:        string(fault.Source),
		Kind:          string(fault.Kind),
		Detail:        string(fault.Detail),
		Occurrences:   1,
		ReportedAt:    s.now().UTC().Truncate(time.Microsecond),
	})
	if err != nil {
		return "", err
	}
	return sessionpkg.GameFaultRecorded, nil
}
