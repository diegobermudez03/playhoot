package setlanguage

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/setting"
	"github.com/diegobermudez03/playhoot/identity/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestSetLanguage(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenIdentityDB(t)
	service := New(db)

	newUser := func(account bool) string {
		userUUID := uuid.NewString()
		var userID int64
		require.NoError(t, db.Raw(`INSERT INTO identity_users (user_uuid, display_name) VALUES (?, ?) RETURNING id`, userUUID, "Ana").Scan(&userID).Error)
		if account {
			require.NoError(t, db.Exec(`INSERT INTO identity_accounts (user_id) VALUES (?)`, userID).Error)
		}
		return userUUID
	}
	language := func(t *testing.T, userUUID string) string {
		var stored string
		require.NoError(t, db.Raw(`SELECT interface_language FROM identity_users WHERE user_uuid = ?`, userUUID).Scan(&stored).Error)
		return stored
	}

	tests := map[string]func(t *testing.T){
		"an account chooses a language and the change is recorded": func(t *testing.T) {
			account := newUser(true)

			outcome, err := service.SetLanguage(ctx, account, "pt")
			require.NoError(t, err)
			require.Equal(t, identity.LanguageSet, outcome)
			require.Equal(t, "pt", language(t, account))

			var changes []struct {
				FromValue string
				ToValue   string
			}
			require.NoError(t, db.Raw(`SELECT c.from_value, c.to_value FROM identity_setting_changes c
				JOIN identity_users u ON u.id = c.user_id WHERE u.user_uuid = ? AND c.setting = ?`, account, setting.Language).
				Scan(&changes).Error)
			require.Len(t, changes, 1)
			require.Equal(t, "", changes[0].FromValue)
			require.Equal(t, "pt", changes[0].ToValue)
		},
		"choosing the same language again changes nothing": func(t *testing.T) {
			account := newUser(true)

			_, err := service.SetLanguage(ctx, account, "es")
			require.NoError(t, err)
			outcome, err := service.SetLanguage(ctx, account, "es")
			require.NoError(t, err)
			require.Equal(t, identity.LanguageSet, outcome)

			var changes int
			require.NoError(t, db.Raw(`SELECT COUNT(*) FROM identity_setting_changes c
				JOIN identity_users u ON u.id = c.user_id WHERE u.user_uuid = ?`, account).Scan(&changes).Error)
			require.Equal(t, 1, changes)
		},
		"a language Playhoot does not speak is refused": func(t *testing.T) {
			account := newUser(true)

			outcome, err := service.SetLanguage(ctx, account, "fr")
			require.NoError(t, err)
			require.Equal(t, identity.LanguageInvalid, outcome)
			require.Empty(t, language(t, account))
		},
		"a guest keeps its preference in the browser": func(t *testing.T) {
			guest := newUser(false)

			outcome, err := service.SetLanguage(ctx, guest, "en")
			require.NoError(t, err)
			require.Equal(t, identity.LanguageNotAccount, outcome)
			require.Empty(t, language(t, guest))
		},
		"an unknown user is not found": func(t *testing.T) {
			_, err := service.SetLanguage(ctx, uuid.NewString(), "en")
			require.ErrorIs(t, err, identity.ErrUserNotFound)
		},
	}

	for name, run := range tests {
		t.Run(name, run)
	}
}
