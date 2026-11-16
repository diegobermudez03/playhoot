package listentries

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/notifications"
	"github.com/diegobermudez03/playhoot/notifications/internal/testdb"
	"github.com/diegobermudez03/playhoot/notifications/usecases/acceptentry"
	"github.com/diegobermudez03/playhoot/notifications/usecases/markread"
	"github.com/diegobermudez03/playhoot/notifications/usecases/preferences"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestEntriesAndPreferences(t *testing.T) {
	ctx := context.Background()
	db := testdb.Open(t)
	accept := acceptentry.New(db, txqueue.NewRegistry(db, txqueue.DefaultWorkerConfig).Queue(notifications.QueueDomain))
	list, read, prefs := New(db), markread.New(db), preferences.New(db)
	add := func(user string, source string, typ notifications.Type) string {
		result, err := accept.AcceptEntry(ctx, notifications.Notice{SourceDomain: source, Recipient: user, Type: typ, IdempotencyKey: uuid.NewString()})
		require.NoError(t, err)
		return result.EntryUUID
	}
	tests := map[string]func(t *testing.T){
		"listing filters and marks nothing read": func(t *testing.T) {
			user := uuid.NewString()
			add(user, "budget", notifications.TypeBudgetLevelChanged)
			add(user, "identity", notifications.TypeAccountSecurity)
			add(user, "identity", notifications.TypeAccessCode)
			page, err := list.ListEntries(ctx, user, notifications.EntryFilter{})
			require.NoError(t, err)
			require.Len(t, page.Entries, 2, "an access code is no in-app entry")
			require.Equal(t, 2, page.Unread)
			page, err = list.ListEntries(ctx, user, notifications.EntryFilter{SourceDomain: "identity"})
			require.NoError(t, err)
			require.Len(t, page.Entries, 1)
			require.Equal(t, 2, page.Unread)
		},
		"an entry is read once and only by its recipient": func(t *testing.T) {
			user := uuid.NewString()
			entry := add(user, "budget", notifications.TypeBudgetLevelChanged)
			outcome, err := read.MarkRead(ctx, uuid.NewString(), entry)
			require.NoError(t, err)
			require.Equal(t, notifications.UnknownEntry, outcome)
			outcome, err = read.MarkRead(ctx, user, entry)
			require.NoError(t, err)
			require.Equal(t, notifications.MarkedRead, outcome)
			outcome, err = read.MarkRead(ctx, user, entry)
			require.NoError(t, err)
			require.Equal(t, notifications.AlreadyRead, outcome)
			page, err := list.ListEntries(ctx, user, notifications.EntryFilter{})
			require.NoError(t, err)
			require.Equal(t, 0, page.Unread)
			require.NotNil(t, page.Entries[0].ReadAt)
		},
		"preferences start at the defaults and append on change": func(t *testing.T) {
			user := uuid.NewString()
			current, err := prefs.ReadPreferences(ctx, user)
			require.NoError(t, err)
			require.Equal(t, notifications.DefaultPreferences, current)
			_, err = prefs.SetPreferences(ctx, user, notifications.Preferences{AI: true})
			require.NoError(t, err)
			_, err = prefs.SetPreferences(ctx, user, notifications.Preferences{AI: true})
			require.NoError(t, err)
			var rows int
			require.NoError(t, db.Raw(`SELECT COUNT(*) FROM notifications_preferences WHERE user_uuid = ?`, user).Scan(&rows).Error)
			require.Equal(t, 1, rows)
			current, err = prefs.ReadPreferences(ctx, user)
			require.NoError(t, err)
			require.Equal(t, notifications.Preferences{AI: true}, current)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
