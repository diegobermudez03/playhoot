// Package deleteuser ends an identity, guest or account: access is withdrawn
// before the call returns and the cross-domain cleanup is left queued. The
// User keeps resolving — to nobody in particular — so the games other people
// played with them stay readable.
package deleteuser

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	internalrepo "github.com/diegobermudez03/playhoot/identity/usecases/deleteuser/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

//go:generate mockgen -package=deleteuser -destination=service_mock_test.go . userStore,cleanupQueue

// userStore reads and deletes a User inside a transaction.
type userStore interface {
	LockUser(ctx context.Context, tx *gorm.DB, userUUID string) (*internalrepo.User, error)
	EndAccess(ctx context.Context, tx *gorm.DB, userID int64, at time.Time) error
	ClearGuestRequest(ctx context.Context, tx *gorm.DB, userUUID string) error
	ClearIdentifyingData(ctx context.Context, tx *gorm.DB, user internalrepo.User, at time.Time) error
	ResetMode(ctx context.Context, tx *gorm.DB, user internalrepo.User, mode string, at time.Time) error
	MarkDeleted(ctx context.Context, tx *gorm.DB, userID int64, origin string, at time.Time) error
}

// cleanupQueue carries the cross-domain cleanup, enqueued in the same
// transaction as the deletion.
type cleanupQueue interface {
	Enqueue(tx *gorm.DB, taskType string, payload any, runAt time.Time, idempotencyKey string) error
}

// CleanupPayload is the account-cleanup task: the User every domain clears
// what it holds about.
type CleanupPayload struct {
	UserUUID string `json:"user_uuid"`
}

var (
	originLabel  = metrics.NewLabel("origin", string(identity.DeleteRequested), string(identity.DeleteExpiry))
	outcomeLabel = metrics.NewLabel("outcome", string(identity.UserDeleted), string(identity.UserAlreadyDeleted))

	deletionsTotal = metrics.NewCounter(
		"identity_user_deletions_total",
		"Identities deleted, by what caused the deletion and whether it was already done.",
		originLabel, outcomeLabel,
	)
)

// Service is Identity's deletion use case.
type Service struct {
	db    utils.DBServicer
	users userStore
	queue cleanupQueue
	now   func() time.Time
}

// New constructs a Service on db, leaving the cleanup task on queue.
func New(db *gorm.DB, queue cleanupQueue) *Service {
	repo := internalrepo.New(db)
	return &Service{db: repo, users: repo, queue: queue, now: time.Now}
}

// DeleteUser deletes userUUID. identity.ErrUserNotFound when no such User
// exists; deleting a User already deleted changes nothing and is not an
// error.
func (s *Service) DeleteUser(ctx context.Context, userUUID string, origin identity.DeleteOrigin) (identity.DeleteUserOutcome, error) {
	defer logging.Step(ctx, "identity.delete_user").Close()

	now := s.now().UTC().Truncate(time.Microsecond)
	outcome, err := utils.RunInDBTransaction(ctx, s.db, func(ctx context.Context, tx *gorm.DB) (identity.DeleteUserOutcome, error) {
		return s.deleteInTx(ctx, tx, userUUID, origin, now)
	})
	if err != nil {
		return "", err
	}
	deletionsTotal.Inc(ctx, originLabel.Value(string(origin)), outcomeLabel.Value(string(outcome)))
	logging.LogFields(ctx, logging.Field("delete_user", string(outcome)), logging.Field("delete_origin", string(origin)))
	return outcome, nil
}

// deleteInTx does the whole deletion at once: either access ends, the data
// that names the person goes and the cleanup is queued, or nothing happens.
func (s *Service) deleteInTx(ctx context.Context, tx *gorm.DB, userUUID string, origin identity.DeleteOrigin, now time.Time) (identity.DeleteUserOutcome, error) {
	user, err := s.users.LockUser(ctx, tx, userUUID)
	if err != nil {
		return "", err
	}
	if user == nil {
		return "", identity.ErrUserNotFound
	}
	if user.DeletedAt != nil {
		return identity.UserAlreadyDeleted, nil
	}
	if err := s.users.EndAccess(ctx, tx, user.UserID, now); err != nil {
		return "", err
	}
	if err := s.users.ClearGuestRequest(ctx, tx, userUUID); err != nil {
		return "", err
	}
	if err := s.users.ClearIdentifyingData(ctx, tx, *user, now); err != nil {
		return "", err
	}
	if err := s.users.ResetMode(ctx, tx, *user, string(identity.ModeNormal), now); err != nil {
		return "", err
	}
	if err := s.users.MarkDeleted(ctx, tx, user.UserID, string(origin), now); err != nil {
		return "", err
	}
	// Keyed by the User: a deletion and an expiry of the same person, or a
	// retry of either, leave one cleanup.
	err = s.queue.Enqueue(tx, txqueue.TypeAccountCleanup, CleanupPayload{UserUUID: userUUID}, time.Time{}, userUUID)
	if err != nil {
		return "", err
	}
	return identity.UserDeleted, nil
}
