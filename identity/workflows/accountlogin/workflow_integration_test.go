package accountlogin

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/testdb"
	"github.com/diegobermudez03/playhoot/identity/loginproviders"
	"github.com/diegobermudez03/playhoot/identity/usecases/createguest"
)

var accessKey = []byte("an-access-token-key-of-32-bytes!")

func challengeOf(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

type flow struct {
	db      *gorm.DB
	manager *Manager
}

func newFlow(t *testing.T) flow {
	db := testdb.OpenIdentityDB(t)
	registry := loginproviders.New(loginproviders.Config{Local: &loginproviders.LocalConfig{AuthorizeURL: "/authorize", CallbackURL: "/callback"}})
	manager, err := New(db, registry, accessKey)
	require.NoError(t, err)
	return flow{db: db, manager: manager}
}

// started starts a local login, returning its state and verifier.
func (f flow) started(t *testing.T) (string, string) {
	t.Helper()
	return f.startedBy(t, "")
}

// startedBy starts a local login by guestUserUUID ("" for nobody).
func (f flow) startedBy(t *testing.T, guestUserUUID string) (string, string) {
	t.Helper()
	verifier := uuid.NewString() + uuid.NewString()
	started, err := f.manager.StartLogin(context.Background(), "local", challengeOf(verifier), guestUserUUID)
	require.NoError(t, err)
	require.Equal(t, identity.LoginStarted, started.Outcome)
	authorization, err := url.Parse(started.AuthorizationURL)
	require.NoError(t, err)
	return authorization.Query().Get("state"), verifier
}

func (f flow) signIn(t *testing.T, name string, replaces string) identity.Credentials {
	t.Helper()
	return f.signInAs(t, name, identity.Credentials{RefreshToken: replaces})
}

// signInAs signs in as name from guest (its User and refresh token; empty for
// nobody), as a client holding that guest does.
func (f flow) signInAs(t *testing.T, name string, guest identity.Credentials) identity.Credentials {
	t.Helper()
	ctx := context.Background()
	replaces := guest.RefreshToken
	state, verifier := f.startedBy(t, guest.User.UUID)
	answer, err := f.manager.HandleCallback(ctx, "local", state, name, "")
	require.NoError(t, err)
	require.Equal(t, identity.LoginCodeIssued, answer.Outcome)
	completed, err := f.manager.CompleteLogin(ctx, answer.LoginCode, verifier, replaces)
	require.NoError(t, err)
	require.Equal(t, identity.LoginCompleted, completed.Outcome)
	return completed.Credentials
}

// historyOf is the attempt's events, oldest first, as "event" or
// "event:reason".
func (f flow) historyOf(t *testing.T, state string) []string {
	t.Helper()
	var rows []struct {
		Event  string
		Reason *string
	}
	require.NoError(t, f.db.Raw(`
		SELECT event.event, event.reason FROM identity_login_attempt_events event
		JOIN identity_login_attempts attempt ON attempt.id = event.attempt_id
		WHERE attempt.state = ? ORDER BY event.id`, state).Scan(&rows).Error)
	history := make([]string, 0, len(rows))
	for _, row := range rows {
		entry := row.Event
		if row.Reason != nil {
			entry += ":" + *row.Reason
		}
		history = append(history, entry)
	}
	return history
}

func TestSignInWithTheLocalProvider(t *testing.T) {
	f := newFlow(t)
	name := "Ana " + uuid.NewString()[:8]

	first := f.signIn(t, name, "")
	again := f.signIn(t, name, "")

	require.Equal(t, identity.LevelAccount, first.User.Level)
	require.Empty(t, first.User.DisplayName, "the person chooses the display name after signing in")
	require.Equal(t, first.User.UUID, again.User.UUID, "signing in again resolves the same User")
	require.NotEqual(t, first.RefreshToken, again.RefreshToken, "each sign-in is its own session")
}

func TestAttemptHistory(t *testing.T) {
	f := newFlow(t)
	ctx := context.Background()

	state, verifier := f.started(t)
	answer, err := f.manager.HandleCallback(ctx, "local", state, "Beto "+uuid.NewString()[:8], "")
	require.NoError(t, err)
	_, err = f.manager.CompleteLogin(ctx, answer.LoginCode, "not-the-verifier", "")
	require.NoError(t, err)
	retried, err := f.manager.CompleteLogin(ctx, answer.LoginCode, verifier, "")
	require.NoError(t, err)
	require.Equal(t, identity.LoginCodeInvalid, retried.Outcome, "a wrong verifier used the code up")
	require.Equal(t, []string{"started", "answered", "code_issued:created", "rejected:verifier_mismatch", "rejected:used"}, f.historyOf(t, state))

	cancelled, _ := f.started(t)
	_, err = f.manager.HandleCallback(ctx, "local", cancelled, "", "access_denied")
	require.NoError(t, err)
	require.Equal(t, []string{"started", "answered", "cancelled"}, f.historyOf(t, cancelled))

	invalid, _ := f.started(t)
	_, err = f.manager.HandleCallback(ctx, "local", invalid, "   ", "")
	require.NoError(t, err)
	require.Equal(t, []string{"started", "answered", "failed:invalid_answer"}, f.historyOf(t, invalid))
}

func TestObservedDataIsKeptPerLogin(t *testing.T) {
	f := newFlow(t)
	name := "Carla " + uuid.NewString()[:8]
	f.signIn(t, name, "")
	f.signIn(t, name, "")

	var observed []string
	require.NoError(t, f.db.Raw(`
		SELECT observed_name FROM identity_login_attempt_events
		WHERE event = 'code_issued' AND observed_name = ?`, name).Scan(&observed).Error)
	require.Len(t, observed, 2, "each login keeps what the provider said")
}

func TestSignInEndsTheGuestSessionItReplaces(t *testing.T) {
	f := newFlow(t)
	guests, err := createguest.New(f.db, accessKey)
	require.NoError(t, err)
	guest, err := guests.CreateGuest(context.Background(), "Dani", uuid.NewString())
	require.NoError(t, err)

	account := f.signIn(t, "Dani "+uuid.NewString()[:8], guest.Credentials.RefreshToken)

	require.NotEqual(t, guest.Credentials.User.UUID, account.User.UUID, "a switch, not a conversion")
	var active int64
	require.NoError(t, f.db.Raw(`SELECT count(*) FROM identity_sessions WHERE refresh_token = ? AND deactivated_at IS NULL`, guest.Credentials.RefreshToken).Scan(&active).Error)
	require.Zero(t, active)
}

func TestConcurrentFirstSignInsCreateOneUser(t *testing.T) {
	f := newFlow(t)
	ctx := context.Background()
	name := "Eva " + uuid.NewString()[:8]
	const logins = 4
	states := make([]string, logins)
	verifiers := make([]string, logins)
	for i := range logins {
		states[i], verifiers[i] = f.started(t)
	}

	codes := make([]string, logins)
	errs := make([]error, logins)
	var wait sync.WaitGroup
	for i := range logins {
		wait.Add(1)
		go func() {
			defer wait.Done()
			answer, err := f.manager.HandleCallback(ctx, "local", states[i], name, "")
			codes[i], errs[i] = answer.LoginCode, err
		}()
	}
	wait.Wait()

	users := map[string]bool{}
	for i := range logins {
		require.NoError(t, errs[i])
		completed, err := f.manager.CompleteLogin(ctx, codes[i], verifiers[i], "")
		require.NoError(t, err)
		users[completed.Credentials.User.UUID] = true
	}
	require.Len(t, users, 1)
	var bindings int64
	require.NoError(t, f.db.Raw(`SELECT count(*) FROM identity_external_identities WHERE name = ?`, name).Scan(&bindings).Error)
	require.EqualValues(t, 1, bindings, "no account left behind by the sign-ins that lost")
}

func TestRepeatedCallbackFails(t *testing.T) {
	f := newFlow(t)
	ctx := context.Background()
	state, _ := f.started(t)

	first, err := f.manager.HandleCallback(ctx, "local", state, "Fede", "")
	require.NoError(t, err)
	repeated, err := f.manager.HandleCallback(ctx, "local", state, "Fede", "")
	require.NoError(t, err)

	require.Equal(t, identity.LoginCodeIssued, first.Outcome)
	require.Equal(t, identity.LoginCallbackResult{Outcome: identity.LoginFailed}, repeated)
}

func (f flow) newGuest(t *testing.T, name string) identity.Credentials {
	t.Helper()
	guests, err := createguest.New(f.db, accessKey)
	require.NoError(t, err)
	created, err := guests.CreateGuest(context.Background(), name, uuid.NewString())
	require.NoError(t, err)
	return created.Credentials
}

func TestAGuestSigningUpKeepsTheirUser(t *testing.T) {
	f := newFlow(t)
	guest := f.newGuest(t, "Gus")

	account := f.signInAs(t, "Gus "+uuid.NewString()[:8], guest)

	require.Equal(t, guest.User.UUID, account.User.UUID, "the guest became the account")
	require.Equal(t, identity.LevelAccount, account.User.Level)
	require.Equal(t, "Gus", account.User.DisplayName, "the guest's name is kept")
	var active int64
	require.NoError(t, f.db.Raw(`SELECT count(*) FROM identity_sessions WHERE refresh_token = ? AND deactivated_at IS NULL`, guest.RefreshToken).Scan(&active).Error)
	require.Zero(t, active, "the guest's session, which never expires, ended")
	var reason string
	require.NoError(t, f.db.Raw(`
		SELECT event.reason FROM identity_login_attempt_events event
		WHERE event.event = 'code_issued' AND event.user_id = (SELECT id FROM identity_users WHERE user_uuid = ?)`, guest.User.UUID).Scan(&reason).Error)
	require.Equal(t, "linked_guest", reason)
}

func TestAGuestSigningInToAnExistingAccountSwitches(t *testing.T) {
	f := newFlow(t)
	name := "Hugo " + uuid.NewString()[:8]
	existing := f.signIn(t, name, "")
	guest := f.newGuest(t, "Hugo")

	account := f.signInAs(t, name, guest)

	require.Equal(t, existing.User.UUID, account.User.UUID, "the existing account wins")
	require.NotEqual(t, guest.User.UUID, account.User.UUID)
	var guestAccounts int64
	require.NoError(t, f.db.Raw(`
		SELECT count(*) FROM identity_accounts WHERE user_id = (SELECT id FROM identity_users WHERE user_uuid = ?)`, guest.User.UUID).Scan(&guestAccounts).Error)
	require.Zero(t, guestAccounts, "the guest stays a guest")
}
