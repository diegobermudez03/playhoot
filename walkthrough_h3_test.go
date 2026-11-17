package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/compliance"
	"github.com/diegobermudez03/playhoot/game/usecases/deleteusergames"
	"github.com/diegobermudez03/playhoot/game/usecases/findtemplate"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/usecases/authorize"
	"github.com/diegobermudez03/playhoot/identity/usecases/createguest"
	"github.com/diegobermudez03/playhoot/identity/usecases/deleteuser"
	"github.com/diegobermudez03/playhoot/identity/usecases/expireguests"
	"github.com/diegobermudez03/playhoot/identity/usecases/refreshcredentials"
	"github.com/diegobermudez03/playhoot/identity/usecases/setdisplayname"
	"github.com/diegobermudez03/playhoot/identity/usecases/setmode"
	"github.com/diegobermudez03/playhoot/identity/usecases/signinmethods"
	"github.com/diegobermudez03/playhoot/identity/workflows/emaillogin"
	"github.com/diegobermudez03/playhoot/notifications/emailproviders"
	"github.com/diegobermudez03/playhoot/notifications/usecases/deleteuserentries"
	"github.com/diegobermudez03/playhoot/orchestrator/accountcleanup"
	"github.com/diegobermudez03/playhoot/orchestrator/gameauthoring"
	"github.com/diegobermudez03/playhoot/orchestrator/marketgate"
	"github.com/diegobermudez03/playhoot/play/session/buildsignal"
	"github.com/diegobermudez03/playhoot/play/session/storage"
	"github.com/diegobermudez03/playhoot/play/session/usecases/anonymizeuser"
	"github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

// codeTap is Identity's queue as the email code workflow sees it: every task
// still goes to the real queue, and the code a notify task carries is noted,
// since outside a test it exists only in the email.
type codeTap struct {
	queue *txqueue.Queue
	codes map[string]string
}

func (c *codeTap) Enqueue(tx *gorm.DB, taskType string, payload any, runAt time.Time, key string) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var task struct {
		Recipient string         `json:"recipient"`
		Skeleton  map[string]any `json:"skeleton"`
	}
	if err := json.Unmarshal(raw, &task); err != nil {
		return err
	}
	if code, ok := task.Skeleton["code"].(string); ok {
		c.codes[task.Recipient] = code
	}
	return c.queue.Enqueue(tx, taskType, payload, runAt, key)
}

// latest is the code sent most recently, whichever attempt it was for.
func (c *codeTap) latest(t *testing.T, before map[string]string) string {
	t.Helper()
	for recipient, code := range c.codes {
		if before[recipient] != code {
			return code
		}
	}
	t.Fatal("no new code was sent")
	return ""
}

func (c *codeTap) snapshot() map[string]string {
	copied := make(map[string]string, len(c.codes))
	for recipient, code := range c.codes {
		copied[recipient] = code
	}
	return copied
}

// TestH3Walkthrough demonstrates identity and access end to end on the local
// database, in order: each step builds on the state the previous left. Every
// step name starts with the rule it shows.
func TestH3Walkthrough(t *testing.T) {
	ctx := context.Background()
	db := walkthroughDB.Open(t)
	s := wireH1(t, db, emailproviders.NewResend(walkthroughWebhook), emailproviders.NewSES(walkthroughWebhook))

	tap := &codeTap{queue: s.identityQ, codes: map[string]string{}}
	emails, err := emaillogin.New(db, tap, walkthroughAccessKey)
	require.NoError(t, err)
	guests, err := createguest.New(db, walkthroughAccessKey)
	require.NoError(t, err)
	modes, err := setmode.New(db, walkthroughAccessKey)
	require.NoError(t, err)
	authorizer, err := authorize.New(walkthroughAccessKey)
	require.NoError(t, err)
	refreshes, err := refreshcredentials.New(db, walkthroughAccessKey)
	require.NoError(t, err)
	methods, names := signinmethods.New(db), setdisplayname.New(db)
	deletion := deleteuser.New(db, s.identityQ)
	accountcleanup.New(deleteusergames.New(db), anonymizeuser.New(db), deleteuserentries.New(db)).Register(s.identityQ)
	expiry := expireguests.New(db, deletion)

	gate := marketgate.New(s.declarations)
	store, _, err := storage.Open(ctx, "", "")
	require.NoError(t, err)
	drafts := versiondrafting.New(db, store, buildsignal.New())
	authoring := gameauthoring.New(gate, s.games, nil, drafts, nil, nil, s.games, drafts, nil, findtemplate.New(db))

	// signInByCode asks for a code and submits it, as a person does.
	signInByCode := func(t *testing.T, address string, guest identity.Credentials) identity.Credentials {
		t.Helper()
		before := tap.snapshot()
		started, err := emails.StartEmailLogin(ctx, address, string(identity.LanguageSpanish), guest.User.UUID)
		require.NoError(t, err)
		require.Equal(t, identity.EmailLoginAcknowledged, started.Outcome)
		completed, err := emails.CompleteEmailLogin(ctx, address, tap.latest(t, before), guest.RefreshToken)
		require.NoError(t, err)
		require.Equal(t, identity.EmailLoginCompleted, completed.Outcome)
		return completed.Credentials
	}
	newGuest := func(t *testing.T, name string) identity.Credentials {
		t.Helper()
		created, err := guests.CreateGuest(ctx, name, uuid.NewString())
		require.NoError(t, err)
		require.Equal(t, identity.GuestCreated, created.Outcome)
		return created.Credentials
	}
	createGame := func(userUUID string, country string) error {
		_, err := authoring.CreateGame(ctx, userUUID, "Walkthrough game", uuid.NewString(), "", country)
		return err
	}
	declare := func(t *testing.T, userUUID string, country string, meetsAge bool) compliance.DeclarationOutcome {
		t.Helper()
		result, err := s.declarations.RecordDeclaration(ctx, compliance.Declaration{
			UserUUID: userUUID, Country: country, MeetsAge: meetsAge, Origin: compliance.DeclaredByUser,
		})
		require.NoError(t, err)
		return result.Outcome
	}

	anaAddress, guestAddress, extraAddress := uuid.NewString()+"@example.com", uuid.NewString()+"@example.com", uuid.NewString()+"@example.com"
	var ana, upgraded, minor identity.Credentials

	steps := []struct {
		name string
		run  func(t *testing.T)
	}{
		{"P03§10·D-033 an email address and its code alone register an account", func(t *testing.T) {
			ana = signInByCode(t, anaAddress, identity.Credentials{})
			require.Equal(t, identity.LevelAccount, ana.User.Level)
			require.Equal(t, identity.ModeNormal, ana.User.Mode)
			s.drain(t)
			require.Equal(t, int64(1), countOf(t, db, `SELECT COUNT(*) FROM identity_email_codes WHERE email = ?`, anaAddress))
		}},
		{"P03§10 signing in again with the same address enters the same account", func(t *testing.T) {
			again := signInByCode(t, anaAddress, identity.Credentials{})
			require.Equal(t, ana.User.UUID, again.User.UUID)
		}},
		{"V-09 a wrong code is refused and leaves the attempt live", func(t *testing.T) {
			before := tap.snapshot()
			_, err := emails.StartEmailLogin(ctx, anaAddress, string(identity.LanguageSpanish), "")
			require.NoError(t, err)
			code := tap.latest(t, before)
			wrong := "000000"
			if code == wrong {
				wrong = "111111"
			}
			refused, err := emails.CompleteEmailLogin(ctx, anaAddress, wrong, "")
			require.NoError(t, err)
			require.Equal(t, identity.EmailLoginInvalidCode, refused.Outcome)
			accepted, err := emails.CompleteEmailLogin(ctx, anaAddress, code, "")
			require.NoError(t, err)
			require.Equal(t, identity.EmailLoginCompleted, accepted.Outcome)
			spent, err := emails.CompleteEmailLogin(ctx, anaAddress, code, "")
			require.NoError(t, err)
			require.Equal(t, identity.EmailLoginAttemptGone, spent.Outcome, "a code signs in once")
		}},
		{"V-09 a fourth code within fifteen minutes is acknowledged the same way and not sent", func(t *testing.T) {
			before := tap.snapshot()
			started, err := emails.StartEmailLogin(ctx, anaAddress, string(identity.LanguageSpanish), "")
			require.NoError(t, err)
			require.Equal(t, identity.EmailLoginAcknowledged, started.Outcome)
			require.Equal(t, before, tap.snapshot(), "no code left for the address")
		}},
		{"P02§9·P03§10 a guest upgrades by code and keeps its identity and name", func(t *testing.T) {
			guest := newGuest(t, "Gus")
			upgraded = signInByCode(t, guestAddress, guest)
			require.Equal(t, guest.User.UUID, upgraded.User.UUID)
			require.Equal(t, identity.LevelAccount, upgraded.User.Level)
			require.Equal(t, "Gus", upgraded.User.DisplayName)
		}},
		{"P03§12·D-035 a second way in is linked by proving it, and signs into the same account", func(t *testing.T) {
			before := tap.snapshot()
			_, err := emails.StartLinkEmail(ctx, ana.User.UUID, extraAddress, string(identity.LanguageSpanish))
			require.NoError(t, err)
			linked, err := emails.CompleteLinkEmail(ctx, ana.User.UUID, extraAddress, tap.latest(t, before))
			require.NoError(t, err)
			require.Equal(t, identity.MethodLinked, linked.Outcome)
			require.Equal(t, ana.User.UUID, signInByCode(t, extraAddress, identity.Credentials{}).User.UUID)
		}},
		{"D-035 a way in that belongs to another account is refused with method_in_use, never merged", func(t *testing.T) {
			before := tap.snapshot()
			_, err := emails.StartLinkEmail(ctx, upgraded.User.UUID, extraAddress, string(identity.LanguageSpanish))
			require.NoError(t, err)
			refused, err := emails.CompleteLinkEmail(ctx, upgraded.User.UUID, extraAddress, tap.latest(t, before))
			require.NoError(t, err)
			require.Equal(t, identity.MethodInUse, refused.Outcome)
		}},
		{"P03§12 a way in can be removed, never the last one", func(t *testing.T) {
			listed, err := methods.ListMethods(ctx, ana.User.UUID)
			require.NoError(t, err)
			require.Len(t, listed, 2)
			removed, err := methods.RemoveMethod(ctx, ana.User.UUID, listed[0].MethodID)
			require.NoError(t, err)
			require.Equal(t, identity.MethodRemoved, removed)
			last, err := methods.RemoveMethod(ctx, ana.User.UUID, listed[1].MethodID)
			require.NoError(t, err)
			require.Equal(t, identity.MethodLastOne, last)
		}},
		{"P19§1·D-034 switching mode answers a token that carries it, and renewals keep it", func(t *testing.T) {
			switched, err := modes.SetMode(ctx, ana.User.UUID, string(identity.ModeTeacher))
			require.NoError(t, err)
			require.Equal(t, identity.ModeSet, switched.Outcome)
			require.Empty(t, switched.Credentials.RefreshToken, "the client keeps its refresh token")
			user, decision, err := authorizer.Authorize(ctx, switched.Credentials.AccessToken, identity.OperationSetOwnMode)
			require.NoError(t, err)
			require.Equal(t, identity.AccessAllowed, decision)
			require.Equal(t, identity.ModeTeacher, user.Mode)
			renewed, err := refreshes.Refresh(ctx, ana.RefreshToken)
			require.NoError(t, err)
			require.Equal(t, identity.ModeTeacher, renewed.Credentials.User.Mode)

			back, err := modes.SetMode(ctx, ana.User.UUID, string(identity.ModeNormal))
			require.NoError(t, err)
			require.Equal(t, identity.ModeSet, back.Outcome)
		}},
		{"P19§1 a guest has no mode to switch", func(t *testing.T) {
			refused, err := modes.SetMode(ctx, newGuest(t, "Gina").User.UUID, string(identity.ModeTeacher))
			require.NoError(t, err)
			require.Equal(t, identity.ModeNotAccount, refused.Outcome)
		}},
		{"V-10 a display name changes three times in thirty days, then the refusal names the next date", func(t *testing.T) {
			for i, name := range []string{"Ana B", "Ana C", "Ana D"} {
				set, err := names.SetDisplayName(ctx, ana.User.UUID, name)
				require.NoError(t, err)
				require.Equal(t, identity.DisplayNameSet, set.Outcome, "change %d", i+1)
			}
			same, err := names.SetDisplayName(ctx, ana.User.UUID, "Ana D")
			require.NoError(t, err)
			require.Equal(t, identity.DisplayNameSet, same.Outcome, "the name in force is not a change")
			refused, err := names.SetDisplayName(ctx, ana.User.UUID, "Ana E")
			require.NoError(t, err)
			require.Equal(t, identity.DisplayNameLimitReached, refused.Outcome)
			require.True(t, refused.NextChangeAt.After(time.Now()))
		}},
		{"P21§3·D-038 creating a game asks for a declaration, and goes ahead once declared", func(t *testing.T) {
			var owed *marketgate.DeclarationRequiredError
			require.ErrorAs(t, createGame(ana.User.UUID, ""), &owed)
			require.Equal(t, compliance.FunctionCreateGames, owed.Function)
			require.Equal(t, compliance.DeclarationRecorded, declare(t, ana.User.UUID, "CO", true))
			require.NoError(t, createGame(ana.User.UUID, ""))
			require.Equal(t, int64(1), countOf(t, db, `SELECT COUNT(*) FROM games WHERE owner_uuid = ? AND deleted_at IS NULL`, ana.User.UUID))
		}},
		{"P21§6 a declaration below the age binds: the adult function stays closed and the answer cannot be changed", func(t *testing.T) {
			minor = signInByCode(t, uuid.NewString()+"@example.com", identity.Credentials{})
			require.Equal(t, compliance.DeclarationRecorded, declare(t, minor.User.UUID, "CO", false))
			var restricted *marketgate.RestrictedError
			require.ErrorAs(t, createGame(minor.User.UUID, ""), &restricted)
			require.Equal(t, compliance.BlockAgeRequirement, restricted.Reason)
			require.Equal(t, compliance.DeclarationConflict, declare(t, minor.User.UUID, "CO", true), "only support corrects it")
			require.ErrorAs(t, createGame(minor.User.UUID, ""), &restricted)
			require.Equal(t, int64(1), countOf(t, db, `SELECT COUNT(*) FROM compliance_minority_signals WHERE user_uuid = ?`, minor.User.UUID))
		}},
		{"C-01·D-038 joining asks nothing: open where unknown or open, refused only where restricted", func(t *testing.T) {
			nobody := newGuest(t, "Nico").User.UUID
			require.NoError(t, gate.Admit(ctx, nobody, compliance.FunctionInteractiveUse, ""), "an unknown place asks nothing")
			require.NoError(t, gate.Admit(ctx, nobody, compliance.FunctionInteractiveUse, "MX"), "no age or country is asked at join")
			var restricted *marketgate.RestrictedError
			require.ErrorAs(t, gate.Admit(ctx, nobody, compliance.FunctionInteractiveUse, "AR"), &restricted)
			require.Equal(t, compliance.BlockMarketUnavailable, restricted.Reason)
			require.ErrorAs(t, gate.Admit(ctx, minor.User.UUID, compliance.FunctionInteractiveUse, "CO"), &restricted,
				"known minority closes open play; a minor's way in is H8's authorized participation")
			require.Equal(t, compliance.BlockAgeRequirement, restricted.Reason)
		}},
		{"P03§8·D-036 deleting an account ends its access, deletes its games, and keeps the compliance evidence", func(t *testing.T) {
			outcome, err := deletion.DeleteUser(ctx, ana.User.UUID, identity.DeleteRequested)
			require.NoError(t, err)
			require.Equal(t, identity.UserDeleted, outcome)
			renewed, err := refreshes.Refresh(ctx, ana.RefreshToken)
			require.NoError(t, err)
			require.Equal(t, identity.RefreshInvalid, renewed.Outcome)
			left, err := methods.ListMethods(ctx, ana.User.UUID)
			require.NoError(t, err)
			require.Empty(t, left, "no way in leads to the deleted account")
			s.drain(t)

			require.Zero(t, countOf(t, db, `SELECT COUNT(*) FROM games WHERE owner_uuid = ? AND deleted_at IS NULL`, ana.User.UUID))
			require.Zero(t, countOf(t, db, `SELECT COUNT(*) FROM identity_sessions session JOIN identity_users users ON users.id = session.user_id
				WHERE users.user_uuid = ? AND session.deactivated_at IS NULL`, ana.User.UUID))
			require.Equal(t, int64(1), countOf(t, db, `SELECT COUNT(*) FROM compliance_declarations WHERE user_uuid = ?`, ana.User.UUID),
				"the declaration is evidence and stays")
			require.Zero(t, countOf(t, db, `SELECT COUNT(*) FROM identity_queue_tasks WHERE type = ?`, txqueue.TypeAccountCleanup))
		}},
		{"P02§9·D-036 a guest unused for ninety days expires through the same deletion; an upgraded guest never does", func(t *testing.T) {
			stale := newGuest(t, "Olga")
			longAgo := time.Now().Add(-91 * 24 * time.Hour)
			for _, who := range []string{stale.User.UUID, upgraded.User.UUID} {
				require.NoError(t, db.Exec(`UPDATE identity_users SET created_at = ? WHERE user_uuid = ?`, longAgo, who).Error)
				require.NoError(t, db.Exec(`UPDATE identity_sessions SET last_used_at = ? WHERE user_id = (SELECT id FROM identity_users WHERE user_uuid = ?)`,
					longAgo, who).Error)
			}
			expired, err := expiry.Expire(ctx)
			require.NoError(t, err)
			require.GreaterOrEqual(t, expired, int64(1))
			s.drain(t)

			require.Equal(t, int64(1), countOf(t, db, `SELECT COUNT(*) FROM identity_users WHERE user_uuid = ? AND deleted_origin = ?`,
				stale.User.UUID, identity.DeleteExpiry))
			renewed, err := refreshes.Refresh(ctx, stale.RefreshToken)
			require.NoError(t, err)
			require.Equal(t, identity.RefreshInvalid, renewed.Outcome)
			require.Zero(t, countOf(t, db, `SELECT COUNT(*) FROM identity_users WHERE user_uuid = ? AND deleted_at IS NOT NULL`, upgraded.User.UUID))
		}},
	}
	for _, step := range steps {
		if !t.Run(step.name, step.run) {
			t.FailNow()
		}
	}
}
