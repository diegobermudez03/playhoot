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

func eventsOf(t *testing.T, db *gorm.DB, attemptID int64) []string {
	t.Helper()
	var events []string
	require.NoError(t, db.Raw(`SELECT event FROM identity_login_attempt_events WHERE attempt_id = ? ORDER BY id`, attemptID).Scan(&events).Error)
	return events
}

func startedAttempt(t *testing.T, repo *Repo, db *gorm.DB, startedAt time.Time) string {
	t.Helper()
	state := uuid.NewString()
	require.NoError(t, repo.CreateAttempt(context.Background(), db, NewAttempt{
		State: state, Provider: "local", CodeChallenge: "challenge", Nonce: "nonce", StartedAt: startedAt,
	}))
	return state
}

func TestAttemptLifecycle(t *testing.T) {
	db := testdb.OpenIdentityDB(t)
	repo := New(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	userID, _, err := repo.CreateAccount(ctx, db, uuid.NewString(), ExternalIdentity{Issuer: "issuer", Subject: uuid.NewString()}, now)
	require.NoError(t, err)

	state := startedAttempt(t, repo, db, now)
	answered, err := repo.AnswerAttempt(ctx, db, "local", state, now, now.Add(-5*time.Minute))
	require.NoError(t, err)
	require.NotNil(t, answered)
	require.Equal(t, "nonce", answered.Nonce)
	again, err := repo.AnswerAttempt(ctx, db, "local", state, now, now.Add(-5*time.Minute))
	require.NoError(t, err)
	require.Nil(t, again, "an attempt is answered once")

	code := "phl_" + state
	require.NoError(t, repo.IssueCode(ctx, db, answered.ID, code, now.Add(time.Minute), AttemptEvent{
		UserID: userID, ObservedEmail: "ana@example.com", ObservedName: "Ana", OccurredAt: now,
	}))
	locked, err := repo.LockAttemptByCode(ctx, db, code)
	require.NoError(t, err)
	require.Equal(t, userID, locked.UserID)
	require.Empty(t, locked.DisplayName, "an account starts without a display name")
	require.Nil(t, locked.CompletedAt)

	require.NoError(t, repo.CloseAttempt(ctx, db, answered.ID, AttemptEvent{Event: EventCompleted, UserID: userID, OccurredAt: now}))
	closed, err := repo.LockAttemptByCode(ctx, db, code)
	require.NoError(t, err)
	require.NotNil(t, closed.CompletedAt)
	require.NoError(t, repo.CloseAttempt(ctx, db, answered.ID, AttemptEvent{Event: EventRejected, Reason: "used", OccurredAt: now.Add(time.Second)}))
	reclosed, err := repo.LockAttemptByCode(ctx, db, code)
	require.NoError(t, err)
	require.Equal(t, closed.CompletedAt.UTC(), reclosed.CompletedAt.UTC(), "closing again keeps the first close")

	require.Equal(t, []string{"started", "answered", "code_issued", "completed", "rejected"}, eventsOf(t, db, answered.ID))
	var observed struct {
		ObservedEmail string
		ObservedName  string
		UserID        int64
	}
	require.NoError(t, db.Raw(`SELECT observed_email, observed_name, user_id FROM identity_login_attempt_events WHERE attempt_id = ? AND event = 'code_issued'`, answered.ID).Scan(&observed).Error)
	require.Equal(t, "ana@example.com", observed.ObservedEmail)
	require.Equal(t, "Ana", observed.ObservedName)
	require.Equal(t, userID, observed.UserID)

	unknown, err := repo.LockAttemptByCode(ctx, db, "phl_unknown")
	require.NoError(t, err)
	require.Nil(t, unknown)
}

func TestAnswerAttemptRefusesExpiredOrAnotherProvider(t *testing.T) {
	db := testdb.OpenIdentityDB(t)
	repo := New(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	old := startedAttempt(t, repo, db, now.Add(-10*time.Minute))
	expired, err := repo.AnswerAttempt(ctx, db, "local", old, now, now.Add(-5*time.Minute))
	require.NoError(t, err)
	require.Nil(t, expired)

	fresh := startedAttempt(t, repo, db, now)
	other, err := repo.AnswerAttempt(ctx, db, "google", fresh, now, now.Add(-5*time.Minute))
	require.NoError(t, err)
	require.Nil(t, other)
}

func TestCreateAccountAndFindBoundUser(t *testing.T) {
	db := testdb.OpenIdentityDB(t)
	repo := New(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	identity := ExternalIdentity{Issuer: "issuer", Subject: "sub-" + uuid.NewString(), Email: "ana@example.com", Name: "Ana"}

	unbound, err := repo.FindBoundUser(ctx, db, identity.Issuer, identity.Subject)
	require.NoError(t, err)
	require.Zero(t, unbound)

	userID, bound, err := repo.CreateAccount(ctx, db, uuid.NewString(), identity, now)
	require.NoError(t, err)
	require.True(t, bound)
	var account struct {
		Accounts    int64
		DisplayName *string
	}
	require.NoError(t, db.Raw(`
		SELECT (SELECT count(*) FROM identity_accounts WHERE user_id = ?) AS accounts, display_name
		FROM identity_users WHERE id = ?`, userID, userID).Scan(&account).Error)
	require.EqualValues(t, 1, account.Accounts)
	require.Nil(t, account.DisplayName)

	found, err := repo.FindBoundUser(ctx, db, identity.Issuer, identity.Subject)
	require.NoError(t, err)
	require.Equal(t, userID, found)

	tx := db.Begin()
	defer tx.Rollback()
	_, boundAgain, err := repo.CreateAccount(ctx, tx, uuid.NewString(), identity, now)
	require.NoError(t, err)
	require.False(t, boundAgain, "an identity binds to one User")
}

func TestOpenSessionAndEndSessionOf(t *testing.T) {
	db := testdb.OpenIdentityDB(t)
	repo := New(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	userID, _, err := repo.CreateAccount(ctx, db, uuid.NewString(), ExternalIdentity{Issuer: "issuer", Subject: uuid.NewString()}, now)
	require.NoError(t, err)
	expiresAt := now.Add(24 * time.Hour)
	account := NewSession{SessionUUID: uuid.NewString(), UserID: userID, RefreshToken: "phr_" + uuid.NewString(), IssuedAt: now, ExpiresAt: &expiresAt}
	guest := NewSession{SessionUUID: uuid.NewString(), UserID: userID, RefreshToken: "phr_" + uuid.NewString(), IssuedAt: now}

	require.NoError(t, repo.OpenSession(ctx, db, account))
	require.NoError(t, repo.OpenSession(ctx, db, guest))
	require.NoError(t, repo.EndSessionOf(ctx, db, guest.RefreshToken, now))
	require.NoError(t, repo.EndSessionOf(ctx, db, "phr_unknown", now), "an unknown token ends nothing")

	activeOf := func(token string) int64 {
		var active int64
		require.NoError(t, db.Raw(`SELECT count(*) FROM identity_sessions WHERE refresh_token = ? AND deactivated_at IS NULL`, token).Scan(&active).Error)
		return active
	}
	require.EqualValues(t, 1, activeOf(account.RefreshToken))
	require.Zero(t, activeOf(guest.RefreshToken))
}
