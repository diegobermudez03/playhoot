// Package reconcilesessions is the background job that ends the Sessions past
// a deadline that nothing live ended: lobbies past their expiry, and running
// Sessions past their lifetime or their owner lease. It runs after the
// lifecycle, not as part of it.
package reconcilesessions

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/batches/reconcilesessions/internal/repo"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/endcause"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/reconciliation"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/sessionstatus"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=reconcilesessions -destination=service_mock_test.go . sessionRepo

// sessionRepo is what the job reads, locks and ends Sessions with.
type sessionRepo interface {
	FindDue(ctx context.Context, lobbyStatus string, runningStatus string, now time.Time, limit int) ([]string, error)
	LockSession(ctx context.Context, tx *gorm.DB, sessionUUID string, lockTimeout time.Duration) (*repo.Session, error)
	EndSession(ctx context.Context, tx *gorm.DB, sessionID uint, fromStatus string, toStatus string, endCause string, at time.Time) error
}

// defaultInterval is how often each instance runs a pass, and defaultBatch
// how many due Sessions a pass reads at a time.
const (
	defaultInterval = time.Minute * 5
	defaultBatch    = 100
)

// defaultLockTimeout is how long judging a Session waits for a transaction
// holding it, all of them short (heartbeat, checkpoint, End Session), before
// it fails and is left for the next pass.
const defaultLockTimeout = 5 * time.Second

var (
	reconciledCauseLabel = metrics.NewLabel("cause",
		string(sessionpkg.EndLobbyExpired),
		string(sessionpkg.EndGameFinished),
		string(sessionpkg.EndMaxLifetime),
		string(sessionpkg.EndOwnerLost),
	)

	reconciledSessions = metrics.NewCounter(
		"play_session_reconciled_total",
		"Sessions reconciliation ended because nothing live would, by end cause.",
		reconciledCauseLabel,
	)
	passDuration = metrics.NewHistogram(
		"play_session_reconciliation_duration_seconds",
		"How long one reconciliation pass took.",
		metrics.LatencyBuckets(),
	)
	failures = metrics.NewCounter(
		"play_session_reconciliation_failures_total",
		"Sessions a reconciliation pass could not judge or end; judged again by the next pass.",
	)
)

// Service is the session reconciliation job.
type Service struct {
	sessions sessionRepo
	// db is the handle transactions are opened on.
	db          utils.DBServicer
	now         func() time.Time
	interval    time.Duration
	batch       int
	lockTimeout time.Duration
	logger      *slog.Logger
}

// New constructs a Service reading and ending Sessions in db.
func New(db *gorm.DB) *Service {
	sessions := repo.New(db)
	return &Service{
		sessions:    sessions,
		db:          sessions,
		now:         time.Now,
		interval:    defaultInterval,
		batch:       defaultBatch,
		lockTimeout: defaultLockTimeout,
		logger:      slog.Default(),
	}
}

// Run runs a pass every interval until ctx is done; each pass works until
// nothing due is left, then waits for the next interval. Every instance runs
// it: each Session is judged under its row lock, so instances never end the
// same Session twice.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			passCtx := logging.Start(ctx)
			ended, err := s.ReconcileSessions(passCtx)
			if err != nil {
				logging.LogError(passCtx, err)
			}
			if ended > 0 || err != nil {
				logging.LogFields(passCtx, logging.Field("sessions_reconciled", ended))
				logging.FinishRequestLog(passCtx, s.logger, "session reconciliation")
			}
		}
	}
}

// ReconcileSessions ends the Sessions past a deadline when the pass began,
// batch at a time until a page ends nothing, and returns how many it ended.
// A Session it fails on is tried once per pass and reported in the error.
func (s *Service) ReconcileSessions(ctx context.Context) (int, error) {
	defer logging.Step(ctx, "reconcilesessions.reconcile_sessions").Close()
	began := time.Now()
	defer func() { passDuration.ObserveDuration(ctx, time.Since(began)) }()

	now := s.now().UTC()
	ended := 0
	failed := make(map[string]bool)
	var errs []error
	for {
		due, err := s.sessions.FindDue(ctx, string(sessionstatus.Lobby), string(sessionstatus.Running), now, s.batch)
		if err != nil {
			return ended, errors.Join(append(errs, err)...)
		}
		endedNow := 0
		for _, sessionUUID := range due {
			if failed[sessionUUID] {
				continue
			}
			cause, err := utils.RunInDBTransaction(ctx, s.db, func(ctx context.Context, tx *gorm.DB) (endcause.Cause, error) {
				return s.reconcileSessionInTx(ctx, tx, sessionUUID)
			})
			if err != nil {
				failed[sessionUUID] = true
				failures.Inc(ctx)
				errs = append(errs, fmt.Errorf("reconciling session %s: %w", sessionUUID, err))
				continue
			}
			if cause != "" {
				endedNow++
				reconciledSessions.Inc(ctx, reconciledCauseLabel.Value(string(cause)))
			}
		}
		ended += endedNow
		if len(due) < s.batch || endedNow == 0 {
			return ended, errors.Join(errs...)
		}
	}
}

// reconcileSessionInTx judges sessionUUID again under its row lock, since it
// may have changed since it was read, and ends it if it still must end,
// returning the cause; empty when it was left as it is.
func (s *Service) reconcileSessionInTx(ctx context.Context, tx *gorm.DB, sessionUUID string) (endcause.Cause, error) {
	session, err := s.sessions.LockSession(ctx, tx, sessionUUID, s.lockTimeout)
	if err != nil || session == nil {
		return "", err
	}
	judged := reconciliation.Session{
		Status:          sessionstatus.Status(session.Status),
		LobbyExpiresAt:  session.LobbyExpiresAt,
		ExpiresAt:       session.ExpiresAt,
		ReplayFinalized: session.ReplayFinalized,
	}
	if session.OwnerLeaseExpiresAt != nil {
		judged.OwnerLeaseExpiresAt = *session.OwnerLeaseExpiresAt
	}
	now := s.now().UTC()
	cause, mustEnd := reconciliation.CauseFor(judged, now)
	if !mustEnd {
		return "", nil
	}
	ending, _ := endcause.EndingOf(cause)
	if err := s.sessions.EndSession(ctx, tx, session.ID, session.Status, string(ending.Status), string(cause), now.Truncate(time.Microsecond)); err != nil {
		return "", err
	}
	return cause, nil
}
