package refreshcredentials

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/refreshtoken"
	"github.com/diegobermudez03/playhoot/identity/internal/testdb"
)

var testKey = []byte("0123456789abcdef0123456789abcdef")

// seedGuestSession stores a guest User with one active session row issued at
// issuedAt, and returns its refresh token.
func seedGuestSession(t *testing.T, db *gorm.DB, issuedAt time.Time) string {
	t.Helper()
	var userID int64
	require.NoError(t, db.Raw(`INSERT INTO identity_users (user_uuid, display_name) VALUES (?, 'Ana') RETURNING id`, uuid.NewString()).Scan(&userID).Error)
	token := "phr_" + uuid.NewString()
	require.NoError(t, db.Exec(`
		INSERT INTO identity_sessions (session_uuid, user_id, refresh_token, issued_at, started_at)
		VALUES (?, ?, ?, ?, ?)`, uuid.NewString(), userID, token, issuedAt, issuedAt).Error)
	return token
}

func activeRows(t *testing.T, db *gorm.DB, token string) int64 {
	t.Helper()
	var count int64
	require.NoError(t, db.Raw(`
		SELECT count(*) FROM identity_sessions
		WHERE deactivated_at IS NULL
			AND session_uuid = (SELECT session_uuid FROM identity_sessions WHERE refresh_token = ?)`, token).Scan(&count).Error)
	return count
}

func TestRefreshWithPostgres(t *testing.T) {
	db := testdb.OpenIdentityDB(t)
	service, err := New(db, testKey)
	require.NoError(t, err)
	ctx := context.Background()

	t.Run("a_recent_token_renews_without_writing", func(t *testing.T) {
		token := seedGuestSession(t, db, time.Now().Add(-time.Hour))

		result, err := service.Refresh(ctx, token)

		require.NoError(t, err)
		require.Equal(t, identity.RefreshRenewed, result.Outcome)
		require.Equal(t, token, result.Credentials.RefreshToken)
		require.Equal(t, identity.LevelGuest, result.Credentials.User.Level)
		require.NotEmpty(t, result.Credentials.AccessToken)
	})

	t.Run("an_old_token_rotates_and_still_works_briefly", func(t *testing.T) {
		old := seedGuestSession(t, db, time.Now().Add(-refreshtoken.RotationAge-time.Minute))

		rotated, err := service.Refresh(ctx, old)
		require.NoError(t, err)
		require.Equal(t, identity.RefreshRenewed, rotated.Outcome)
		require.NotEqual(t, old, rotated.Credentials.RefreshToken)
		require.EqualValues(t, 1, activeRows(t, db, old), "one active row per session")

		again, err := service.Refresh(ctx, old)
		require.NoError(t, err)
		require.Equal(t, rotated.Credentials.RefreshToken, again.Credentials.RefreshToken, "the token just replaced gets the active one")
	})

	t.Run("concurrent_renewals_of_an_old_token_rotate_once", func(t *testing.T) {
		old := seedGuestSession(t, db, time.Now().Add(-refreshtoken.RotationAge-time.Minute))

		var wg sync.WaitGroup
		results := make([]identity.RefreshResult, 4)
		errs := make([]error, 4)
		for i := range results {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				results[i], errs[i] = service.Refresh(ctx, old)
			}(i)
		}
		wg.Wait()

		for i := range results {
			require.NoError(t, errs[i])
			require.Equal(t, identity.RefreshRenewed, results[i].Outcome)
			require.Equal(t, results[0].Credentials.RefreshToken, results[i].Credentials.RefreshToken)
		}
		var rows int64
		require.NoError(t, db.Raw(`
			SELECT count(*) FROM identity_sessions
			WHERE session_uuid = (SELECT session_uuid FROM identity_sessions WHERE refresh_token = ?)`, old).Scan(&rows).Error)
		require.EqualValues(t, 2, rows, "the old row and one successor")
	})

	t.Run("a_signed_out_or_unknown_token_is_invalid", func(t *testing.T) {
		token := seedGuestSession(t, db, time.Now().Add(-time.Hour))
		require.NoError(t, db.Exec(`UPDATE identity_sessions SET deactivated_at = now() WHERE refresh_token = ?`, token).Error)

		signedOut, err := service.Refresh(ctx, token)
		require.NoError(t, err)
		require.Equal(t, identity.RefreshInvalid, signedOut.Outcome)

		unknown, err := service.Refresh(ctx, "phr_unknown")
		require.NoError(t, err)
		require.Equal(t, identity.RefreshInvalid, unknown.Outcome)
	})
}
