package emaillogin

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/emailcode"
	"github.com/diegobermudez03/playhoot/identity/internal/testdb"
	"github.com/diegobermudez03/playhoot/identity/usecases/createguest"
)

var accessKey = []byte("an-access-token-key-of-32-bytes!")

// sentCode is one code left in the queue, as the notify task carries it.
type sentCode struct {
	recipient string
	code      string
	taskType  string
}

// outbox stands for Identity's queue: it keeps the notify tasks instead of
// running them, which is where a test reads the code from.
type outbox struct {
	sent []sentCode
}

func (o *outbox) Enqueue(_ *gorm.DB, taskType string, payload any, _ time.Time, _ string) error {
	task, ok := payload.(notification)
	if !ok {
		return nil
	}
	code, _ := task.Skeleton["code"].(string)
	o.sent = append(o.sent, sentCode{recipient: task.Recipient, code: code, taskType: taskType})
	return nil
}

// last is the code sent most recently.
func (o *outbox) last(t *testing.T) sentCode {
	t.Helper()
	require.NotEmpty(t, o.sent, "no code was sent")
	return o.sent[len(o.sent)-1]
}

type flow struct {
	db      *gorm.DB
	manager *Manager
	queue   *outbox
}

func newFlow(t *testing.T) *flow {
	db := testdb.OpenIdentityDB(t)
	queue := &outbox{}
	manager, err := New(db, queue, accessKey)
	require.NoError(t, err)
	return &flow{db: db, manager: manager, queue: queue}
}

// at pins the workflow's clock, so lifetimes are tested without waiting.
func (f *flow) at(moment time.Time) {
	f.manager.now = func() time.Time { return moment }
}

// address is a fresh address, so tests never share one attempt's limits.
func address() string {
	return uuid.NewString() + "@example.com"
}

// signIn asks for a code and submits it, as a person does.
func (f *flow) signIn(t *testing.T, email string) identity.CompleteEmailLoginResult {
	t.Helper()
	return f.signInAs(t, email, identity.Credentials{})
}

func (f *flow) signInAs(t *testing.T, email string, guest identity.Credentials) identity.CompleteEmailLoginResult {
	t.Helper()
	ctx := context.Background()
	started, err := f.manager.StartEmailLogin(ctx, email, string(identity.LanguageSpanish), guest.User.UUID)
	require.NoError(t, err)
	require.Equal(t, identity.EmailLoginAcknowledged, started.Outcome)
	completed, err := f.manager.CompleteEmailLogin(ctx, email, f.queue.last(t).code, guest.RefreshToken)
	require.NoError(t, err)
	return completed
}

// guest creates a guest with its own credentials.
func (f *flow) guest(t *testing.T, name string) identity.Credentials {
	t.Helper()
	guests, err := createguest.New(f.db, accessKey)
	require.NoError(t, err)
	created, err := guests.CreateGuest(context.Background(), name, uuid.NewString())
	require.NoError(t, err)
	require.Equal(t, identity.GuestCreated, created.Outcome)
	return created.Credentials
}

// historyOf is the attempt's events for an address, oldest first.
func (f *flow) historyOf(t *testing.T, email string) []string {
	t.Helper()
	var events []string
	require.NoError(t, f.db.Raw(`
		SELECT event.event FROM identity_email_code_events event
		JOIN identity_email_codes code ON code.id = event.code_id
		WHERE code.email = ? ORDER BY event.id`, email).Scan(&events).Error)
	return events
}

func TestEmailCodeSignIn(t *testing.T) {
	ctx := context.Background()
	tests := map[string]func(t *testing.T){
		"an unknown address registers an account": func(t *testing.T) {
			f := newFlow(t)
			email := address()
			completed := f.signIn(t, email)
			require.Equal(t, identity.EmailLoginCompleted, completed.Outcome)
			require.Equal(t, identity.LevelAccount, completed.Credentials.User.Level)
			require.NotEmpty(t, completed.Credentials.RefreshToken)
			require.Equal(t, []string{"sent", "completed"}, f.historyOf(t, email))
		},
		"signing in again is the same account": func(t *testing.T) {
			f := newFlow(t)
			email := address()
			first := f.signIn(t, email)
			second := f.signIn(t, email)
			require.Equal(t, identity.EmailLoginCompleted, second.Outcome)
			require.Equal(t, first.Credentials.User.UUID, second.Credentials.User.UUID)
		},
		"case and spaces around an address do not make another account": func(t *testing.T) {
			f := newFlow(t)
			email := address()
			first := f.signIn(t, email)
			second := f.signIn(t, "  "+strings.ToUpper(email)+" ")
			require.Equal(t, identity.EmailLoginCompleted, second.Outcome)
			require.Equal(t, first.Credentials.User.UUID, second.Credentials.User.UUID)
		},
		"a guest upgrade keeps the uuid and the name": func(t *testing.T) {
			f := newFlow(t)
			guest := f.guest(t, "Ana")
			completed := f.signInAs(t, address(), guest)
			require.Equal(t, identity.EmailLoginCompleted, completed.Outcome)
			require.Equal(t, guest.User.UUID, completed.Credentials.User.UUID)
			require.Equal(t, "Ana", completed.Credentials.User.DisplayName)
			require.Equal(t, identity.LevelAccount, completed.Credentials.User.Level)
			var active int64
			require.NoError(t, f.db.Raw(`
				SELECT count(*) FROM identity_sessions s JOIN identity_users u ON u.id = s.user_id
				WHERE u.user_uuid = ? AND s.deactivated_at IS NULL`, guest.User.UUID).Scan(&active).Error)
			require.Equal(t, int64(1), active, "the guest's own session ended with the upgrade")
		},
		"a guest signing in to an existing account keeps the histories apart": func(t *testing.T) {
			f := newFlow(t)
			email := address()
			owner := f.signIn(t, email)
			guest := f.guest(t, "Ana")
			completed := f.signInAs(t, email, guest)
			require.Equal(t, identity.EmailLoginCompleted, completed.Outcome)
			require.Equal(t, owner.Credentials.User.UUID, completed.Credentials.User.UUID)
			require.NotEqual(t, guest.User.UUID, completed.Credentials.User.UUID)
		},
		"a wrong code is refused and the attempt stands": func(t *testing.T) {
			f := newFlow(t)
			email := address()
			_, err := f.manager.StartEmailLogin(ctx, email, "", "")
			require.NoError(t, err)
			wrong, err := f.manager.CompleteEmailLogin(ctx, email, "000000", "")
			require.NoError(t, err)
			require.Equal(t, identity.EmailLoginInvalidCode, wrong.Outcome)
			right, err := f.manager.CompleteEmailLogin(ctx, email, f.queue.last(t).code, "")
			require.NoError(t, err)
			require.Equal(t, identity.EmailLoginCompleted, right.Outcome)
		},
		"five wrong codes exhaust the attempt": func(t *testing.T) {
			f := newFlow(t)
			email := address()
			_, err := f.manager.StartEmailLogin(ctx, email, "", "")
			require.NoError(t, err)
			code := f.queue.last(t).code
			for try := range emailcode.MaxWrongCodes {
				refused, err := f.manager.CompleteEmailLogin(ctx, email, "000000", "")
				require.NoError(t, err)
				if try+1 < emailcode.MaxWrongCodes {
					require.Equal(t, identity.EmailLoginInvalidCode, refused.Outcome)
				} else {
					require.Equal(t, identity.EmailLoginAttemptGone, refused.Outcome)
				}
			}
			// Even the right code no longer works: a new one must be asked for.
			spent, err := f.manager.CompleteEmailLogin(ctx, email, code, "")
			require.NoError(t, err)
			require.Equal(t, identity.EmailLoginAttemptGone, spent.Outcome)
			require.Contains(t, f.historyOf(t, email), "exhausted")
		},
		"a new code supersedes the one before it": func(t *testing.T) {
			f := newFlow(t)
			email := address()
			_, err := f.manager.StartEmailLogin(ctx, email, "", "")
			require.NoError(t, err)
			first := f.queue.last(t).code
			_, err = f.manager.StartEmailLogin(ctx, email, "", "")
			require.NoError(t, err)
			second := f.queue.last(t).code
			require.NotEqual(t, first, second)
			stale, err := f.manager.CompleteEmailLogin(ctx, email, first, "")
			require.NoError(t, err)
			require.Equal(t, identity.EmailLoginInvalidCode, stale.Outcome)
			completed, err := f.manager.CompleteEmailLogin(ctx, email, second, "")
			require.NoError(t, err)
			require.Equal(t, identity.EmailLoginCompleted, completed.Outcome)
			require.Contains(t, f.historyOf(t, email), "superseded")
		},
		"a code outlives nothing past its ten minutes": func(t *testing.T) {
			f := newFlow(t)
			email := address()
			start := time.Now().UTC()
			f.at(start)
			_, err := f.manager.StartEmailLogin(ctx, email, "", "")
			require.NoError(t, err)
			code := f.queue.last(t).code
			f.at(start.Add(emailcode.Lifetime + time.Second))
			expired, err := f.manager.CompleteEmailLogin(ctx, email, code, "")
			require.NoError(t, err)
			require.Equal(t, identity.EmailLoginAttemptGone, expired.Outcome)
		},
		"nothing reveals whether an address has an account": func(t *testing.T) {
			f := newFlow(t)
			known := address()
			f.signIn(t, known)
			forKnown, err := f.manager.StartEmailLogin(ctx, known, "", "")
			require.NoError(t, err)
			forUnknown, err := f.manager.StartEmailLogin(ctx, address(), "", "")
			require.NoError(t, err)
			require.Equal(t, forUnknown, forKnown)
			require.Equal(t, identity.EmailLoginAcknowledged, forKnown.Outcome)
		},
		"a refused rate answers like every other request": func(t *testing.T) {
			f := newFlow(t)
			email := address()
			for range emailcode.BurstSends {
				acknowledged, err := f.manager.StartEmailLogin(ctx, email, "", "")
				require.NoError(t, err)
				require.Equal(t, identity.EmailLoginAcknowledged, acknowledged.Outcome)
			}
			sent := len(f.queue.sent)
			refused, err := f.manager.StartEmailLogin(ctx, email, "", "")
			require.NoError(t, err)
			require.Equal(t, identity.EmailLoginAcknowledged, refused.Outcome)
			require.Len(t, f.queue.sent, sent, "no further code is sent")
		},
		"a value that is not an address is refused outright": func(t *testing.T) {
			f := newFlow(t)
			refused, err := f.manager.StartEmailLogin(ctx, "not-an-address", "", "")
			require.NoError(t, err)
			require.Equal(t, identity.EmailLoginInvalidAddress, refused.Outcome)
			require.Empty(t, f.queue.sent)
		},
		"the code travels only in the email": func(t *testing.T) {
			f := newFlow(t)
			email := address()
			_, err := f.manager.StartEmailLogin(ctx, email, string(identity.LanguageEnglish), "")
			require.NoError(t, err)
			sent := f.queue.last(t)
			var stored []string
			require.NoError(t, f.db.Raw(`SELECT code_sha256 FROM identity_email_codes WHERE email = ?`, email).Scan(&stored).Error)
			require.Len(t, stored, 1)
			require.NotEqual(t, sent.code, stored[0])
			require.Equal(t, emailcode.Hash(sent.code), stored[0])
			// The task names the attempt, never the address.
			var attemptUUID string
			require.NoError(t, f.db.Raw(`SELECT attempt_uuid FROM identity_email_codes WHERE email = ?`, email).Scan(&attemptUUID).Error)
			require.Equal(t, attemptUUID, sent.recipient)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}

func TestEmailMethodLinking(t *testing.T) {
	ctx := context.Background()
	tests := map[string]func(t *testing.T){
		"an account adds another address": func(t *testing.T) {
			f := newFlow(t)
			owner := f.signIn(t, address()).Credentials.User
			second := address()
			started, err := f.manager.StartLinkEmail(ctx, owner.UUID, second, string(identity.LanguageSpanish))
			require.NoError(t, err)
			require.Equal(t, identity.EmailLoginAcknowledged, started.Outcome)
			linked, err := f.manager.CompleteLinkEmail(ctx, owner.UUID, second, f.queue.last(t).code)
			require.NoError(t, err)
			require.Equal(t, identity.MethodLinked, linked.Outcome)
			// The added address now signs that same account in.
			back := f.signIn(t, second)
			require.Equal(t, owner.UUID, back.Credentials.User.UUID)
		},
		"an address on another account is never merged": func(t *testing.T) {
			f := newFlow(t)
			theirs := address()
			f.signIn(t, theirs)
			owner := f.signIn(t, address()).Credentials.User
			_, err := f.manager.StartLinkEmail(ctx, owner.UUID, theirs, "")
			require.NoError(t, err)
			refused, err := f.manager.CompleteLinkEmail(ctx, owner.UUID, theirs, f.queue.last(t).code)
			require.NoError(t, err)
			require.Equal(t, identity.MethodInUse, refused.Outcome)
			require.Contains(t, f.historyOf(t, theirs), "refused_in_use")
		},
		"linking an address the account already has changes nothing": func(t *testing.T) {
			f := newFlow(t)
			email := address()
			owner := f.signIn(t, email).Credentials.User
			_, err := f.manager.StartLinkEmail(ctx, owner.UUID, email, "")
			require.NoError(t, err)
			linked, err := f.manager.CompleteLinkEmail(ctx, owner.UUID, email, f.queue.last(t).code)
			require.NoError(t, err)
			require.Equal(t, identity.MethodLinked, linked.Outcome)
			var methods int64
			require.NoError(t, f.db.Raw(`
				SELECT count(*) FROM identity_external_identities e JOIN identity_users u ON u.id = e.user_id
				WHERE u.user_uuid = ? AND e.removed_at IS NULL`, owner.UUID).Scan(&methods).Error)
			require.Equal(t, int64(1), methods)
		},
		"a link code never opens a session": func(t *testing.T) {
			f := newFlow(t)
			owner := f.signIn(t, address()).Credentials.User
			second := address()
			_, err := f.manager.StartLinkEmail(ctx, owner.UUID, second, "")
			require.NoError(t, err)
			refused, err := f.manager.CompleteEmailLogin(ctx, second, f.queue.last(t).code, "")
			require.NoError(t, err)
			require.Equal(t, identity.EmailLoginAttemptGone, refused.Outcome)
		},
		"another account cannot finish somebody else's link": func(t *testing.T) {
			f := newFlow(t)
			owner := f.signIn(t, address()).Credentials.User
			other := f.signIn(t, address()).Credentials.User
			second := address()
			_, err := f.manager.StartLinkEmail(ctx, owner.UUID, second, "")
			require.NoError(t, err)
			refused, err := f.manager.CompleteLinkEmail(ctx, other.UUID, second, f.queue.last(t).code)
			require.NoError(t, err)
			require.Equal(t, identity.MethodAttemptGone, refused.Outcome)
		},
		"a wrong code refuses the link": func(t *testing.T) {
			f := newFlow(t)
			owner := f.signIn(t, address()).Credentials.User
			second := address()
			_, err := f.manager.StartLinkEmail(ctx, owner.UUID, second, "")
			require.NoError(t, err)
			refused, err := f.manager.CompleteLinkEmail(ctx, owner.UUID, second, "000000")
			require.NoError(t, err)
			require.Equal(t, identity.MethodInvalidCode, refused.Outcome)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
