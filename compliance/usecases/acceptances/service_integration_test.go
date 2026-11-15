package acceptances

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/compliance"
	"github.com/diegobermudez03/playhoot/compliance/internal/testdb"
	compliancemigration "github.com/diegobermudez03/playhoot/compliance/migration"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestAcceptances(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenComplianceDB(t)
	require.NoError(t, compliancemigration.Seed(db))
	service := New(db)
	accept := func(t *testing.T, person string, kind compliance.TextKind, version int, language string) compliance.AcceptanceOutcome {
		outcome, err := service.RecordAcceptance(ctx, compliance.AcceptanceRequest{UserUUID: person, Kind: kind, Version: version, Language: language, Action: compliance.ActionAccepted, Context: "registration"})
		require.NoError(t, err)
		return outcome
	}
	withdraw := func(t *testing.T, person string, kind compliance.TextKind) compliance.AcceptanceOutcome {
		outcome, err := service.RecordAcceptance(ctx, compliance.AcceptanceRequest{UserUUID: person, Kind: kind, Action: compliance.ActionWithdrawn, Context: "settings"})
		require.NoError(t, err)
		return outcome
	}
	accepted := func(t *testing.T, person string, kind compliance.TextKind) bool {
		text, err := service.GetAcceptanceText(ctx, kind, "es", person)
		require.NoError(t, err)
		return text.Accepted
	}

	// Draft texts exist in every kind and language before any test publishes more.
	for _, kind := range compliance.TextKinds {
		for _, language := range compliance.Languages {
			text, err := service.GetAcceptanceText(ctx, kind, language, "")
			require.NoError(t, err)
			require.Equal(t, 1, text.Version)
			require.Contains(t, text.Content, "DRAFT")
		}
	}
	_, err := service.GetAcceptanceText(ctx, compliance.TextTerms, "fr", "")
	require.True(t, errors.Is(err, compliance.ErrTextNotFound))

	tests := map[string]func(t *testing.T){
		"an acceptance points at the exact version and repeating it changes nothing": func(t *testing.T) {
			person := uuid.NewString()
			require.False(t, accepted(t, person, compliance.TextTerms))
			require.Equal(t, compliance.AcceptanceRecorded, accept(t, person, compliance.TextTerms, 1, "en"))
			require.Equal(t, compliance.AcceptanceUnchanged, accept(t, person, compliance.TextTerms, 1, "en"))
			require.True(t, accepted(t, person, compliance.TextTerms))
			require.Equal(t, compliance.AcceptanceUnknownText, accept(t, person, compliance.TextTerms, 7, "en"))
		},
		"a new version needs a new acceptance": func(t *testing.T) {
			person := uuid.NewString()
			require.Equal(t, compliance.AcceptanceRecorded, accept(t, person, compliance.TextOwnData, 1, "es"))
			now := time.Now()
			require.NoError(t, db.Exec(`INSERT INTO compliance_acceptance_texts (kind, version, language, content, published_at, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
				string(compliance.TextOwnData), 2, "es", "v2", now, now).Error)
			require.False(t, accepted(t, person, compliance.TextOwnData))
			text, err := service.GetAcceptanceText(ctx, compliance.TextOwnData, "es", person)
			require.NoError(t, err)
			require.Equal(t, 2, text.Version)
			require.Equal(t, compliance.AcceptanceRecorded, accept(t, person, compliance.TextOwnData, 2, "es"))
			require.True(t, accepted(t, person, compliance.TextOwnData))
		},
		"marketing can be withdrawn and accepted again; necessary kinds cannot be withdrawn": func(t *testing.T) {
			person := uuid.NewString()
			require.Equal(t, compliance.AcceptanceUnchanged, withdraw(t, person, compliance.TextMarketing))
			require.Equal(t, compliance.AcceptanceRecorded, accept(t, person, compliance.TextMarketing, 1, "pt"))
			require.True(t, accepted(t, person, compliance.TextMarketing))
			require.Equal(t, compliance.AcceptanceRecorded, withdraw(t, person, compliance.TextMarketing))
			require.Equal(t, compliance.AcceptanceUnchanged, withdraw(t, person, compliance.TextMarketing))
			require.False(t, accepted(t, person, compliance.TextMarketing))
			require.Equal(t, compliance.AcceptanceRecorded, accept(t, person, compliance.TextMarketing, 1, "pt"))
			require.True(t, accepted(t, person, compliance.TextMarketing))

			require.Equal(t, compliance.AcceptanceRecorded, accept(t, person, compliance.TextTerms, 1, "pt"))
			require.Equal(t, compliance.AcceptanceNotWithdrawable, withdraw(t, person, compliance.TextTerms))
			require.True(t, accepted(t, person, compliance.TextTerms))
		},
		"malformed requests are refused": func(t *testing.T) {
			person := uuid.NewString()
			require.Equal(t, compliance.AcceptanceInvalid, accept(t, person, "cookies", 1, "es"))
			require.Equal(t, compliance.AcceptanceInvalid, accept(t, person, compliance.TextTerms, 1, "fr"))
			outcome, err := service.RecordAcceptance(ctx, compliance.AcceptanceRequest{UserUUID: person, Kind: compliance.TextTerms, Version: 1, Language: "es", Action: compliance.ActionAccepted})
			require.NoError(t, err)
			require.Equal(t, compliance.AcceptanceInvalid, outcome)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
