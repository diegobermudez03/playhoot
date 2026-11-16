package readprofile

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/testdb"
	"github.com/diegobermudez03/playhoot/identity/usecases/signinmethods"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestReadProfile(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenIdentityDB(t)
	service := New(db, signinmethods.New(db))

	newUser := func(account bool, mode string, language string, emails ...string) string {
		userUUID := uuid.NewString()
		var userID int64
		require.NoError(t, db.Raw(`INSERT INTO identity_users (user_uuid, display_name, interface_language) VALUES (?, ?, ?) RETURNING id`,
			userUUID, "Ana", language).Scan(&userID).Error)
		if account {
			require.NoError(t, db.Exec(`INSERT INTO identity_accounts (user_id, mode) VALUES (?, ?)`, userID, mode).Error)
		}
		for _, email := range emails {
			require.NoError(t, db.Exec(`INSERT INTO identity_external_identities (issuer, subject, user_id, email, name) VALUES (?, ?, ?, ?, ?)`,
				"local", uuid.NewString(), userID, email, "Ana").Error)
		}
		return userUUID
	}

	tests := map[string]func(t *testing.T){
		"an account carries its mode, language and ways in": func(t *testing.T) {
			account := newUser(true, "teacher", "pt", "ana@x.com")

			profile, err := service.ReadProfile(ctx, account)
			require.NoError(t, err)
			require.Equal(t, identity.LevelAccount, profile.Level)
			require.Equal(t, identity.ModeTeacher, profile.Mode)
			require.Equal(t, "pt", profile.Language)
			require.Equal(t, "Ana", profile.DisplayName)
			require.Len(t, profile.Methods, 1)
			require.Equal(t, "ana@x.com", profile.Methods[0].Address)
			require.False(t, profile.Methods[0].AddedAt.After(time.Now().UTC().Add(time.Minute)))
		},
		"an account that never switched is normal": func(t *testing.T) {
			account := newUser(true, "normal", "")

			profile, err := service.ReadProfile(ctx, account)
			require.NoError(t, err)
			require.Equal(t, identity.ModeNormal, profile.Mode)
			require.Empty(t, profile.Language)
			require.Empty(t, profile.Methods)
		},
		"a guest is normal, has no language of its own and no methods": func(t *testing.T) {
			guest := newUser(false, "", "")

			profile, err := service.ReadProfile(ctx, guest)
			require.NoError(t, err)
			require.Equal(t, identity.LevelGuest, profile.Level)
			require.Equal(t, identity.ModeNormal, profile.Mode)
			require.Empty(t, profile.Language)
			require.Empty(t, profile.Methods)
		},
		"an unknown or malformed user is not found": func(t *testing.T) {
			_, err := service.ReadProfile(ctx, uuid.NewString())
			require.ErrorIs(t, err, identity.ErrUserNotFound)
			_, err = service.ReadProfile(ctx, "not-a-uuid")
			require.ErrorIs(t, err, identity.ErrUserNotFound)
		},
	}

	for name, run := range tests {
		t.Run(name, run)
	}
}
