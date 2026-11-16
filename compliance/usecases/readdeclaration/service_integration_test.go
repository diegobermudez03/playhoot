package readdeclaration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/compliance/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestReadDeclaration(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenComplianceDB(t)
	service := New(db)

	declare := func(t *testing.T, userUUID, country, subdivision string, meetsAge bool, at time.Time) {
		t.Helper()
		require.NoError(t, db.Exec(`INSERT INTO compliance_declarations
			(user_uuid, country, subdivision, meets_age, threshold_age, origin, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, userUUID, country, subdivision, meetsAge, 18, "user", at).Error)
	}

	t.Run("the latest declaration is the one in force", func(t *testing.T) {
		person := uuid.NewString()
		earlier := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
		declare(t, person, "CO", "", true, earlier)
		declare(t, person, "US", "CA", true, earlier.Add(24*time.Hour))

		declaration, err := service.ReadDeclaration(ctx, person)
		require.NoError(t, err)
		require.True(t, declaration.Found)
		require.Equal(t, "US", declaration.Country)
		require.Equal(t, "CA", declaration.Subdivision)
		require.True(t, declaration.MeetsAge)
		require.Equal(t, 18, declaration.ThresholdAge)
		require.Equal(t, earlier.Add(24*time.Hour), declaration.DeclaredAt.UTC())
	})

	t.Run("someone who declared nothing has none", func(t *testing.T) {
		declaration, err := service.ReadDeclaration(ctx, uuid.NewString())
		require.NoError(t, err)
		require.False(t, declaration.Found)
	})

	t.Run("a malformed user is an error, never an empty answer", func(t *testing.T) {
		_, err := service.ReadDeclaration(ctx, "not-a-uuid")
		require.Error(t, err)
	})
}
