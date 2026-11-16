package admintexts

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/compliance"
	"github.com/diegobermudez03/playhoot/compliance/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestAdminTexts(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenComplianceDB(t)
	service := New(db)

	versionsOf := func(t *testing.T, kind compliance.TextKind, language string) []compliance.TextVersion {
		t.Helper()
		catalog, err := service.AdminListTexts(ctx)
		require.NoError(t, err)
		for _, entry := range catalog {
			if entry.Kind == kind && entry.Language == language {
				return entry.Versions
			}
		}
		t.Fatalf("the catalog has no %s in %s", kind, language)
		return nil
	}

	t.Run("the catalog lists every kind and language, missing pairs included", func(t *testing.T) {
		catalog, err := service.AdminListTexts(ctx)
		require.NoError(t, err)
		require.Len(t, catalog, len(compliance.TextKinds)*len(compliance.Languages))
		require.Empty(t, versionsOf(t, compliance.TextClassDataAgreement, "pt"))
	})

	t.Run("publishing bumps the version of that kind and language only", func(t *testing.T) {
		first, err := service.AdminPublishText(ctx, compliance.TextTerms, "es", "Primera version")
		require.NoError(t, err)
		require.Equal(t, compliance.TextPublished, first.Outcome)
		require.Equal(t, 1, first.Version)

		second, err := service.AdminPublishText(ctx, compliance.TextTerms, "es", "Segunda version")
		require.NoError(t, err)
		require.Equal(t, 2, second.Version)

		other, err := service.AdminPublishText(ctx, compliance.TextTerms, "en", "First version")
		require.NoError(t, err)
		require.Equal(t, 1, other.Version, "publishing one language never moves another")

		versions := versionsOf(t, compliance.TextTerms, "es")
		require.Len(t, versions, 2)
		require.Equal(t, 2, versions[0].Version, "newest first")
	})

	t.Run("a published version is never edited", func(t *testing.T) {
		_, err := service.AdminPublishText(ctx, compliance.TextOwnData, "es", "Texto uno")
		require.NoError(t, err)
		_, err = service.AdminPublishText(ctx, compliance.TextOwnData, "es", "Texto dos")
		require.NoError(t, err)

		var contents []string
		require.NoError(t, db.Raw(`SELECT content FROM compliance_acceptance_texts
			WHERE kind = ? AND language = ? ORDER BY version`, string(compliance.TextOwnData), "es").Scan(&contents).Error)
		require.Equal(t, []string{"Texto uno", "Texto dos"}, contents)
	})

	t.Run("an unknown kind, language or empty content is refused", func(t *testing.T) {
		for _, refused := range []struct {
			kind     compliance.TextKind
			language string
			content  string
		}{
			{kind: "nonsense", language: "es", content: "x"},
			{kind: compliance.TextTerms, language: "fr", content: "x"},
			{kind: compliance.TextTerms, language: "es", content: "   "},
			{kind: compliance.TextTerms, language: "es", content: strings.Repeat("a", MaxContentBytes+1)},
		} {
			published, err := service.AdminPublishText(ctx, refused.kind, refused.language, refused.content)
			require.NoError(t, err)
			require.Equal(t, compliance.TextInvalid, published.Outcome)
		}
	})
}
