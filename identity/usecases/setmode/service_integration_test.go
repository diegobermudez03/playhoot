package setmode

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/accesstoken"
	"github.com/diegobermudez03/playhoot/identity/internal/business/setting"
	"github.com/diegobermudez03/playhoot/identity/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

var testAccessKey = []byte("0123456789abcdef0123456789abcdef")

func TestSetMode(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenIdentityDB(t)
	service, err := New(db, testAccessKey)
	require.NoError(t, err)
	signer, err := accesstoken.NewSigner(testAccessKey)
	require.NoError(t, err)

	newUser := func(account bool) string {
		userUUID := uuid.NewString()
		var userID int64
		require.NoError(t, db.Raw(`INSERT INTO identity_users (user_uuid, display_name) VALUES (?, ?) RETURNING id`, userUUID, "Ana").Scan(&userID).Error)
		if account {
			require.NoError(t, db.Exec(`INSERT INTO identity_accounts (user_id) VALUES (?)`, userID).Error)
		}
		return userUUID
	}

	tests := map[string]func(t *testing.T){
		"switching answers a token carrying the new mode and records the change": func(t *testing.T) {
			account := newUser(true)

			result, err := service.SetMode(ctx, account, string(identity.ModeTeacher))
			require.NoError(t, err)
			require.Equal(t, identity.ModeSet, result.Outcome)
			require.Empty(t, result.Credentials.RefreshToken, "the client keeps the refresh token it has")

			carried, err := signer.Verify(result.Credentials.AccessToken, result.Credentials.AccessIssuedAt)
			require.NoError(t, err)
			require.Equal(t, identity.ModeTeacher, carried.Mode)
			require.Equal(t, account, carried.UUID)

			var stored string
			require.NoError(t, db.Raw(`SELECT a.mode FROM identity_accounts a JOIN identity_users u ON u.id = a.user_id
				WHERE u.user_uuid = ?`, account).Scan(&stored).Error)
			require.Equal(t, string(identity.ModeTeacher), stored)

			var changes []struct {
				FromValue string
				ToValue   string
			}
			require.NoError(t, db.Raw(`SELECT c.from_value, c.to_value FROM identity_setting_changes c
				JOIN identity_users u ON u.id = c.user_id WHERE u.user_uuid = ? AND c.setting = ?`, account, setting.Mode).
				Scan(&changes).Error)
			require.Len(t, changes, 1)
			require.Equal(t, "normal", changes[0].FromValue)
			require.Equal(t, "teacher", changes[0].ToValue)
		},
		"switching back is one more change, never a new account": func(t *testing.T) {
			account := newUser(true)

			_, err := service.SetMode(ctx, account, string(identity.ModeTeacher))
			require.NoError(t, err)
			result, err := service.SetMode(ctx, account, string(identity.ModeNormal))
			require.NoError(t, err)
			require.Equal(t, identity.ModeSet, result.Outcome)

			carried, err := signer.Verify(result.Credentials.AccessToken, result.Credentials.AccessIssuedAt)
			require.NoError(t, err)
			require.Equal(t, identity.ModeNormal, carried.Mode)

			var accounts int
			require.NoError(t, db.Raw(`SELECT COUNT(*) FROM identity_accounts a JOIN identity_users u ON u.id = a.user_id
				WHERE u.user_uuid = ?`, account).Scan(&accounts).Error)
			require.Equal(t, 1, accounts)
		},
		"a guest is refused and nothing is recorded": func(t *testing.T) {
			guest := newUser(false)

			result, err := service.SetMode(ctx, guest, string(identity.ModeTeacher))
			require.NoError(t, err)
			require.Equal(t, identity.ModeNotAccount, result.Outcome)
			require.Empty(t, result.Credentials.AccessToken)

			var changes int
			require.NoError(t, db.Raw(`SELECT COUNT(*) FROM identity_setting_changes c
				JOIN identity_users u ON u.id = c.user_id WHERE u.user_uuid = ?`, guest).Scan(&changes).Error)
			require.Zero(t, changes)
		},
		"an unknown user is not found": func(t *testing.T) {
			_, err := service.SetMode(ctx, uuid.NewString(), string(identity.ModeTeacher))
			require.ErrorIs(t, err, identity.ErrUserNotFound)
		},
	}

	for name, run := range tests {
		t.Run(name, run)
	}
}
