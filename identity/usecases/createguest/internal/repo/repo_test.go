package repo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity/internal/testdb"
)

func newGuest(requestedAt time.Time) NewGuest {
	return NewGuest{
		IdempotencyKey: uuid.NewString(),
		UserUUID:       uuid.NewString(),
		DisplayName:    "Ana",
		SessionUUID:    uuid.NewString(),
		RefreshToken:   "phr_" + uuid.NewString(),
		RequestedAt:    requestedAt,
	}
}

func countUsers(t *testing.T, db *gorm.DB, userUUID string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Raw(`SELECT count(*) FROM identity_users WHERE user_uuid = ?`, userUUID).Scan(&count).Error)
	return count
}

// create claims guest's key and, when fresh, creates the guest, as the
// service does in one transaction.
func create(t *testing.T, ctx context.Context, repo *Repo, db *gorm.DB, guest NewGuest) (int64, *GuestRequest) {
	t.Helper()
	tx := db.Begin()
	defer tx.Rollback()
	id, existing, err := repo.ClaimGuestRequest(ctx, tx, guest)
	require.NoError(t, err)
	if existing == nil {
		require.NoError(t, repo.CreateGuestUser(ctx, tx, guest))
	}
	require.NoError(t, tx.Commit().Error)
	return id, existing
}

func TestClaimAndCreateGuest(t *testing.T) {
	db := testdb.OpenIdentityDB(t)
	repo := New(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	t.Run("fresh_key_creates_the_user_and_an_active_never_expiring_session", func(t *testing.T) {
		guest := newGuest(now)

		id, existing := create(t, ctx, repo, db, guest)

		require.NotZero(t, id)
		require.Nil(t, existing)
		require.EqualValues(t, 1, countUsers(t, db, guest.UserUUID))
		var session struct {
			SessionUUID   string
			ExpiresAt     *time.Time
			DeactivatedAt *time.Time
		}
		require.NoError(t, db.Raw(`SELECT session_uuid, expires_at, deactivated_at FROM identity_sessions WHERE refresh_token = ?`, guest.RefreshToken).Scan(&session).Error)
		require.Equal(t, guest.SessionUUID, session.SessionUUID)
		require.Nil(t, session.ExpiresAt)
		require.Nil(t, session.DeactivatedAt)
	})

	t.Run("claimed_key_returns_the_earlier_request_with_its_active_refresh_token", func(t *testing.T) {
		first := newGuest(now)
		create(t, ctx, repo, db, first)
		again := newGuest(now)
		again.IdempotencyKey = first.IdempotencyKey
		again.DisplayName = "Bea"

		id, existing := create(t, ctx, repo, db, again)

		require.Zero(t, id)
		require.NotNil(t, existing)
		existing.RequestedAt = existing.RequestedAt.UTC() // the driver reads it in local time
		require.Equal(t, &GuestRequest{DisplayName: "Ana", UserUUID: first.UserUUID, RequestedAt: now, ActiveRefreshToken: &first.RefreshToken}, existing)
		require.Zero(t, countUsers(t, db, again.UserUUID))
	})

	t.Run("signed_out_session_has_no_active_refresh_token", func(t *testing.T) {
		first := newGuest(now)
		create(t, ctx, repo, db, first)
		require.NoError(t, db.Exec(`UPDATE identity_sessions SET deactivated_at = now() WHERE session_uuid = ?`, first.SessionUUID).Error)
		again := newGuest(now)
		again.IdempotencyKey = first.IdempotencyKey

		_, existing := create(t, ctx, repo, db, again)

		require.NotNil(t, existing)
		require.Nil(t, existing.ActiveRefreshToken)
	})
}
