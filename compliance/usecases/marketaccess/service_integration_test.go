package marketaccess

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/compliance"
	"github.com/diegobermudez03/playhoot/compliance/internal/testdb"
)

func TestMain(m *testing.M) {
	os.Exit(testdb.Run(m))
}

func TestMarketAccess(t *testing.T) {
	ctx := context.Background()
	db := testdb.OpenComplianceDB(t)
	service := New(db)
	declare := func(t *testing.T, userUUID, country, subdivision string, meetsAge bool, origin compliance.DeclarationOrigin) compliance.DeclarationResult {
		result, err := service.RecordDeclaration(ctx, compliance.Declaration{UserUUID: userUUID, Country: country, Subdivision: subdivision, MeetsAge: meetsAge, Origin: origin})
		require.NoError(t, err)
		return result
	}
	check := func(t *testing.T, userUUID string, function compliance.Function, approximate string) compliance.AccessDecision {
		decision, err := service.CheckMarketAccess(ctx, compliance.AccessRequest{UserUUID: userUUID, Function: function, ApproximateCountry: approximate})
		require.NoError(t, err)
		return decision
	}

	tests := map[string]func(t *testing.T){
		"an adult declared in Colombia may create games": func(t *testing.T) {
			person := uuid.NewString()
			result := declare(t, person, "CO", "", true, compliance.DeclaredByUser)
			require.Equal(t, compliance.DeclarationRecorded, result.Outcome)
			require.Equal(t, 18, result.ThresholdAge)
			require.Equal(t, compliance.AccessAllowed, check(t, person, compliance.FunctionCreateGames, "").Outcome)
		},
		"markets outside the coverage or still disabled are blocked": func(t *testing.T) {
			for _, country := range []string{"BR", "AR", "CL", "PE"} {
				person := uuid.NewString()
				require.Equal(t, compliance.DeclarationRecorded, declare(t, person, country, "", true, compliance.DeclaredByUser).Outcome)
				decision := check(t, person, compliance.FunctionInteractiveUse, "")
				require.Equal(t, compliance.AccessBlocked, decision.Outcome, country)
				require.Equal(t, compliance.BlockMarketUnavailable, decision.Reason, country)
			}
		},
		"the US is adults only, per state": func(t *testing.T) {
			person := uuid.NewString()
			require.Equal(t, compliance.DeclarationInvalid, declare(t, person, "US", "", true, compliance.DeclaredByUser).Outcome)
			result := declare(t, person, "US", "MS", true, compliance.DeclaredByUser)
			require.Equal(t, 21, result.ThresholdAge)
			require.Equal(t, compliance.AccessAllowed, check(t, person, compliance.FunctionTeach, "").Outcome)
			require.Equal(t, compliance.AccessBlocked, check(t, person, compliance.FunctionMinorClassParticipation, "").Outcome)
			require.Equal(t, 19, declare(t, uuid.NewString(), "US", "AL", true, compliance.DeclaredByUser).ThresholdAge)
			require.Equal(t, 18, declare(t, uuid.NewString(), "US", "TX", true, compliance.DeclaredByUser).ThresholdAge)
		},
		"Canada has its provinces' ages and no Quebec": func(t *testing.T) {
			require.Equal(t, 19, declare(t, uuid.NewString(), "CA", "BC", true, compliance.DeclaredByUser).ThresholdAge)
			require.Equal(t, 18, declare(t, uuid.NewString(), "CA", "ON", true, compliance.DeclaredByUser).ThresholdAge)
			quebec := uuid.NewString()
			declare(t, quebec, "CA", "QC", true, compliance.DeclaredByUser)
			require.Equal(t, compliance.BlockMarketUnavailable, check(t, quebec, compliance.FunctionCreateGames, "").Reason)
		},
		"without a declaration an adult function asks for one": func(t *testing.T) {
			require.Equal(t, compliance.AccessNeedsDeclaration, check(t, uuid.NewString(), compliance.FunctionCreateGames, "").Outcome)
			require.Equal(t, compliance.AccessNeedsDeclaration, check(t, uuid.NewString(), compliance.FunctionCreateGames, "CO").Outcome)
			require.Equal(t, compliance.AccessNeedsDeclaration, check(t, "", compliance.FunctionCreateGames, "US").Outcome)
			require.Equal(t, compliance.AccessBlocked, check(t, "", compliance.FunctionCreateGames, "BR").Outcome)
		},
		"travel or a VPN is reported, not blocked": func(t *testing.T) {
			person := uuid.NewString()
			declare(t, person, "MX", "", true, compliance.DeclaredByUser)
			decision := check(t, person, compliance.FunctionCreateGames, "BR")
			require.Equal(t, compliance.AccessAllowed, decision.Outcome)
			require.True(t, decision.LocationDiscrepancy)
			require.False(t, check(t, person, compliance.FunctionCreateGames, "").LocationDiscrepancy)
		},
		"a declaration is recorded once and a different answer is refused": func(t *testing.T) {
			person := uuid.NewString()
			require.Equal(t, compliance.DeclarationRecorded, declare(t, person, "CO", "", true, compliance.DeclaredByUser).Outcome)
			require.Equal(t, compliance.DeclarationUnchanged, declare(t, person, "CO", "", true, compliance.DeclaredByUser).Outcome)
			require.Equal(t, compliance.DeclarationConflict, declare(t, person, "MX", "", true, compliance.DeclaredByUser).Outcome)
			require.Equal(t, compliance.DeclarationConflict, declare(t, person, "CO", "", false, compliance.DeclaredByUser).Outcome)
			var count int64
			require.NoError(t, db.Raw(`SELECT COUNT(*) FROM compliance_declarations WHERE user_uuid = ?`, person).Scan(&count).Error)
			require.EqualValues(t, 1, count)
		},
		"support corrects a declaration": func(t *testing.T) {
			person := uuid.NewString()
			declare(t, person, "CO", "", true, compliance.DeclaredByUser)
			require.Equal(t, compliance.DeclarationRecorded, declare(t, person, "PE", "", true, compliance.DeclaredBySupport).Outcome)
			require.Equal(t, "PE", check(t, person, compliance.FunctionCreateGames, "").Country)
			require.Equal(t, compliance.DeclarationUnchanged, declare(t, person, "PE", "", true, compliance.DeclaredByUser).Outcome)
		},
		"known minority binds even after an adult correction": func(t *testing.T) {
			person := uuid.NewString()
			declare(t, person, "CO", "", false, compliance.DeclaredByUser)
			require.Equal(t, compliance.BlockAgeRequirement, check(t, person, compliance.FunctionSubscribe, "").Reason)
			require.Equal(t, compliance.DeclarationRecorded, declare(t, person, "CO", "", true, compliance.DeclaredBySupport).Outcome)
			require.Equal(t, compliance.BlockAgeRequirement, check(t, person, compliance.FunctionSubscribe, "").Reason)
			require.Equal(t, compliance.AccessAllowed, check(t, person, compliance.FunctionMinorClassParticipation, "").Outcome)
		},
		"a rule not yet effective is ignored": func(t *testing.T) {
			now := time.Now()
			require.NoError(t, db.Exec(`INSERT INTO compliance_market_rules (country, subdivision, function, enabled, min_age, conditions_note, effective_at, reason, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, "ZY", "", string(compliance.FunctionCreateGames), true, 18, "", now.Add(time.Hour), "test", now).Error)
			person := uuid.NewString()
			declare(t, person, "ZY", "", true, compliance.DeclaredByUser)
			require.Equal(t, compliance.BlockMarketUnavailable, check(t, person, compliance.FunctionCreateGames, "").Reason)
			service.now = func() time.Time { return now.Add(2 * time.Hour) }
			defer func() { service.now = time.Now }()
			require.Equal(t, compliance.AccessAllowed, check(t, person, compliance.FunctionCreateGames, "").Outcome)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
