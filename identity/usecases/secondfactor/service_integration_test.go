package secondfactor

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/totp"
	"github.com/diegobermudez03/playhoot/identity/internal/testdb"
	"github.com/diegobermudez03/playhoot/identity/usecases/authorize"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

var (
	accessKey = []byte(strings.Repeat("a", 32))
	totpKey   = []byte(strings.Repeat("t", 32))
)

func TestSecondFactor(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenIdentityDB(t)
	newAccount := func(admin bool) string {
		userUUID := uuid.NewString()
		var userID int64
		require.NoError(t, db.Raw(`INSERT INTO identity_users (user_uuid, display_name) VALUES (?, ?) RETURNING id`, userUUID, "Admin").Scan(&userID).Error)
		require.NoError(t, db.Exec(`INSERT INTO identity_accounts (user_id) VALUES (?)`, userID).Error)
		if admin {
			require.NoError(t, db.Exec(`INSERT INTO identity_admin_grants (user_id, granted_at, note) VALUES (?, now(), ?)`, userID, "test").Error)
		}
		return userUUID
	}
	newService := func(t *testing.T, clock *time.Time) *Service {
		service, err := New(db, totpKey, accessKey)
		require.NoError(t, err)
		service.now = func() time.Time { return *clock }
		return service
	}
	codeAt := func(t *testing.T, secret string, at time.Time) string {
		code, err := totp.CodeAt(secret, totp.StepAt(at))
		require.NoError(t, err)
		return code
	}
	wrong := func(t *testing.T, secret string, at time.Time) string {
		code := codeAt(t, secret, at)
		if code == "000000" {
			return "000001"
		}
		return "000000"
	}
	// enrolled returns an administrator with a confirmed credential's secret.
	enrolled := func(t *testing.T, service *Service, clock *time.Time) (string, string) {
		admin := newAccount(true)
		enrollment, err := service.EnrollTotp(ctx, admin, "")
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorAccepted, enrollment.Outcome)
		confirmation, err := service.ConfirmTotp(ctx, admin, codeAt(t, enrollment.Secret, *clock))
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorAccepted, confirmation.Outcome)
		*clock = clock.Add(totp.Step)
		return admin, enrollment.Secret
	}
	authorizer, err := authorize.New(accessKey)
	require.NoError(t, err)

	t.Run("enrollment needs the grant and a confirming code", func(t *testing.T) {
		clock := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
		service := newService(t, &clock)

		refused, err := service.EnrollTotp(ctx, newAccount(false), "")
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorNotAdmin, refused.Outcome)
		require.Empty(t, refused.Secret)

		admin := newAccount(true)
		enrollment, err := service.EnrollTotp(ctx, admin, "")
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorAccepted, enrollment.Outcome)
		require.Contains(t, enrollment.URI, "otpauth://totp/")
		var stored int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM identity_totp_credentials WHERE secret_encrypted LIKE ?`, "%"+enrollment.Secret+"%").Scan(&stored).Error)
		require.Zero(t, stored, "the secret is sealed at rest")

		unconfirmed, err := service.VerifyTotp(ctx, admin, codeAt(t, enrollment.Secret, clock), identity.PurposeAdminToken)
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorNotEnrolled, unconfirmed.Outcome, "a pending credential is unusable")

		bad, err := service.ConfirmTotp(ctx, admin, wrong(t, enrollment.Secret, clock))
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorInvalidCode, bad.Outcome)

		good, err := service.ConfirmTotp(ctx, admin, codeAt(t, enrollment.Secret, clock))
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorAccepted, good.Outcome)

		replayed, err := service.VerifyTotp(ctx, admin, codeAt(t, enrollment.Secret, clock), identity.PurposeAdminToken)
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorInvalidCode, replayed.Outcome, "the confirming code is spent")
	})

	t.Run("a pending enrollment expires", func(t *testing.T) {
		clock := time.Date(2026, 11, 15, 11, 0, 0, 0, time.UTC)
		service := newService(t, &clock)
		admin := newAccount(true)
		enrollment, err := service.EnrollTotp(ctx, admin, "")
		require.NoError(t, err)
		clock = clock.Add(totp.PendingLifetime)
		late, err := service.ConfirmTotp(ctx, admin, codeAt(t, enrollment.Secret, clock))
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorNoPendingEnrollment, late.Outcome)
	})

	t.Run("the admin-mode token needs the grant and a valid code", func(t *testing.T) {
		// The authorizer checks the token against the real clock.
		clock := time.Now().UTC()
		service := newService(t, &clock)
		admin, secret := enrolled(t, service, &clock)

		notAdmin, err := service.VerifyTotp(ctx, newAccount(false), "123456", identity.PurposeAdminToken)
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorNotAdmin, notAdmin.Outcome)
		require.Empty(t, notAdmin.Credentials.AccessToken)

		verified, err := service.VerifyTotp(ctx, admin, codeAt(t, secret, clock), identity.PurposeAdminToken)
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorAccepted, verified.Outcome)
		require.Equal(t, identity.LevelAdmin, verified.Credentials.User.Level)
		require.Empty(t, verified.Credentials.RefreshToken)

		caller, decision, err := authorizer.Authorize(ctx, verified.Credentials.AccessToken, identity.OperationManageQueues)
		require.NoError(t, err)
		require.Equal(t, identity.AccessAllowed, decision)
		require.Equal(t, admin, caller.UUID)

		replay, err := service.VerifyTotp(ctx, admin, codeAt(t, secret, clock), identity.PurposeAdminToken)
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorInvalidCode, replay.Outcome)

		revoke := db.Exec(`UPDATE identity_admin_grants g SET revoked_at = now() FROM identity_users u WHERE g.user_id = u.id AND u.user_uuid = ?`, admin)
		require.NoError(t, revoke.Error)
		clock = clock.Add(totp.Step)
		revoked, err := service.VerifyTotp(ctx, admin, codeAt(t, secret, clock), identity.PurposeAdminToken)
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorNotAdmin, revoked.Outcome)

		var attempts int64
		require.NoError(t, db.Raw(`SELECT COUNT(*) FROM identity_second_factor_checks c JOIN identity_users u ON u.id = c.user_id WHERE u.user_uuid = ?`, admin).Scan(&attempts).Error)
		require.EqualValues(t, 4, attempts, "confirmation, token, replay and refusal are all recorded")
	})

	t.Run("wrong codes lock the account for a while", func(t *testing.T) {
		clock := time.Date(2026, 11, 15, 13, 0, 0, 0, time.UTC)
		service := newService(t, &clock)
		admin, secret := enrolled(t, service, &clock)

		for i := range totp.MaxFailures {
			result, err := service.VerifyTotp(ctx, admin, wrong(t, secret, clock), identity.PurposeStepUp)
			require.NoError(t, err)
			if i < totp.MaxFailures-1 {
				require.Equal(t, identity.SecondFactorInvalidCode, result.Outcome)
			} else {
				require.Equal(t, identity.SecondFactorLocked, result.Outcome)
				require.Equal(t, clock.Add(totp.LockDuration), result.LockedUntil)
			}
		}
		locked, err := service.VerifyTotp(ctx, admin, codeAt(t, secret, clock), identity.PurposeStepUp)
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorLocked, locked.Outcome, "a valid code is not evaluated while locked")

		clock = clock.Add(totp.LockDuration)
		unlocked, err := service.VerifyTotp(ctx, admin, codeAt(t, secret, clock), identity.PurposeStepUp)
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorAccepted, unlocked.Outcome)
	})

	t.Run("a step-up proof is spent once and expires", func(t *testing.T) {
		clock := time.Date(2026, 11, 15, 14, 0, 0, 0, time.UTC)
		service := newService(t, &clock)
		admin, secret := enrolled(t, service, &clock)

		first, err := service.VerifyTotp(ctx, admin, codeAt(t, secret, clock), identity.PurposeStepUp)
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorAccepted, first.Outcome)
		require.Equal(t, clock.Add(totp.ProofLifetime), first.StepUpExpiry)
		require.Empty(t, first.Credentials.AccessToken)

		other, err := service.ConsumeStepUpProof(ctx, newAccount(true), first.StepUpProof, "budget.reopen")
		require.NoError(t, err)
		require.Equal(t, identity.StepUpUnknown, other, "a proof belongs to its User")

		consumed, err := service.ConsumeStepUpProof(ctx, admin, first.StepUpProof, "budget.reopen")
		require.NoError(t, err)
		require.Equal(t, identity.StepUpConsumed, consumed)
		var usedFor string
		require.NoError(t, db.Raw(`SELECT used_for FROM identity_second_factor_checks WHERE proof_sha256 = ?`, totp.HashProof(first.StepUpProof)).Scan(&usedFor).Error)
		require.Equal(t, "budget.reopen", usedFor)

		again, err := service.ConsumeStepUpProof(ctx, admin, first.StepUpProof, "budget.cutoff")
		require.NoError(t, err)
		require.Equal(t, identity.StepUpUsed, again)

		clock = clock.Add(totp.Step)
		second, err := service.VerifyTotp(ctx, admin, codeAt(t, secret, clock), identity.PurposeStepUp)
		require.NoError(t, err)
		clock = clock.Add(totp.ProofLifetime)
		expired, err := service.ConsumeStepUpProof(ctx, admin, second.StepUpProof, "budget.reopen")
		require.NoError(t, err)
		require.Equal(t, identity.StepUpExpired, expired)

		garbage, err := service.ConsumeStepUpProof(ctx, admin, "nope", "budget.reopen")
		require.NoError(t, err)
		require.Equal(t, identity.StepUpUnknown, garbage)
	})

	t.Run("re-enrollment needs the current code and replaces it on confirmation", func(t *testing.T) {
		clock := time.Date(2026, 11, 15, 15, 0, 0, 0, time.UTC)
		service := newService(t, &clock)
		admin, oldSecret := enrolled(t, service, &clock)

		missing, err := service.EnrollTotp(ctx, admin, "")
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorCurrentCodeRequired, missing.Outcome)

		bad, err := service.EnrollTotp(ctx, admin, wrong(t, oldSecret, clock))
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorInvalidCode, bad.Outcome)

		renewal, err := service.EnrollTotp(ctx, admin, codeAt(t, oldSecret, clock))
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorAccepted, renewal.Outcome)

		clock = clock.Add(totp.Step)
		stillOld, err := service.VerifyTotp(ctx, admin, codeAt(t, oldSecret, clock), identity.PurposeStepUp)
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorAccepted, stillOld.Outcome, "the old credential works until the new one is confirmed")

		clock = clock.Add(totp.Step)
		confirmed, err := service.ConfirmTotp(ctx, admin, codeAt(t, renewal.Secret, clock))
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorAccepted, confirmed.Outcome)

		clock = clock.Add(totp.Step)
		oldNow, err := service.VerifyTotp(ctx, admin, codeAt(t, oldSecret, clock), identity.PurposeStepUp)
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorInvalidCode, oldNow.Outcome)
		newNow, err := service.VerifyTotp(ctx, admin, codeAt(t, renewal.Secret, clock), identity.PurposeStepUp)
		require.NoError(t, err)
		require.Equal(t, identity.SecondFactorAccepted, newNow.Outcome)
	})

	t.Run("an unknown user is an error", func(t *testing.T) {
		clock := time.Now()
		service := newService(t, &clock)
		_, err := service.VerifyTotp(ctx, uuid.NewString(), "123456", identity.PurposeStepUp)
		require.ErrorIs(t, err, identity.ErrUserNotFound)
	})
}
