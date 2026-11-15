package marketaccess

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/compliance"
)

func TestDecide(t *testing.T) {
	adult := &Declared{Country: "CO", MeetsAge: true, ThresholdAge: 18}
	enabled18 := &Rule{Enabled: true, MinAge: 18}
	tests := map[string]struct {
		facts   Facts
		outcome compliance.AccessOutcome
		reason  compliance.BlockReason
	}{
		"nothing known asks for a declaration": {
			facts: Facts{Function: compliance.FunctionCreateGames}, outcome: compliance.AccessNeedsDeclaration,
		},
		"an adult in an enabled market is allowed": {
			facts: Facts{Function: compliance.FunctionCreateGames, Declaration: adult, Rule: enabled18}, outcome: compliance.AccessAllowed,
		},
		"a market without a rule is blocked": {
			facts:   Facts{Function: compliance.FunctionCreateGames, Declaration: &Declared{Country: "BR", MeetsAge: true, ThresholdAge: 18}},
			outcome: compliance.AccessBlocked, reason: compliance.BlockMarketUnavailable,
		},
		"a disabled rule is blocked": {
			facts:   Facts{Function: compliance.FunctionCreateGames, Declaration: adult, Rule: &Rule{Enabled: false, MinAge: 18}},
			outcome: compliance.AccessBlocked, reason: compliance.BlockMarketUnavailable,
		},
		"an adult function without a declaration asks for one": {
			facts: Facts{Function: compliance.FunctionCreateGames, ApproximateCountry: "CO", Rule: enabled18}, outcome: compliance.AccessNeedsDeclaration,
		},
		"a location in a per-state country asks for a declaration": {
			facts: Facts{Function: compliance.FunctionCreateGames, ApproximateCountry: "US", CountryCovered: true}, outcome: compliance.AccessNeedsDeclaration,
		},
		"a location in an uncovered country is blocked": {
			facts:   Facts{Function: compliance.FunctionInteractiveUse, ApproximateCountry: "BR"},
			outcome: compliance.AccessBlocked, reason: compliance.BlockMarketUnavailable,
		},
		"a declaration below the age is blocked": {
			facts:   Facts{Function: compliance.FunctionCreateGames, Declaration: &Declared{Country: "CO", MeetsAge: false, ThresholdAge: 18}, Rule: enabled18},
			outcome: compliance.AccessBlocked, reason: compliance.BlockAgeRequirement,
		},
		"known minority binds over an adult declaration": {
			facts:   Facts{Function: compliance.FunctionCreateGames, Declaration: adult, Rule: enabled18, Minority: true},
			outcome: compliance.AccessBlocked, reason: compliance.BlockAgeRequirement,
		},
		"a higher age than declared for asks again": {
			facts: Facts{Function: compliance.FunctionCreateGames, Declaration: adult, Rule: &Rule{Enabled: true, MinAge: 19}}, outcome: compliance.AccessNeedsDeclaration,
		},
		"a minor function needs no age": {
			facts: Facts{Function: compliance.FunctionMinorClassParticipation, ApproximateCountry: "CO", Rule: &Rule{Enabled: true}, Minority: true}, outcome: compliance.AccessAllowed,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			decision := Decide(tc.facts)
			require.Equal(t, tc.outcome, decision.Outcome)
			require.Equal(t, tc.reason, decision.Reason)
		})
	}
}

func TestDecideLocationDiscrepancy(t *testing.T) {
	tests := map[string]func(t *testing.T){
		"a different location is reported and does not block": func(t *testing.T) {
			decision := Decide(Facts{Function: compliance.FunctionCreateGames, Declaration: &Declared{Country: "CO", MeetsAge: true, ThresholdAge: 18}, ApproximateCountry: "BR", Rule: &Rule{Enabled: true, MinAge: 18}})
			require.Equal(t, compliance.AccessAllowed, decision.Outcome)
			require.True(t, decision.LocationDiscrepancy)
			require.Equal(t, "CO", decision.Country)
		},
		"an unknown location is no discrepancy": func(t *testing.T) {
			decision := Decide(Facts{Function: compliance.FunctionCreateGames, Declaration: &Declared{Country: "CO", MeetsAge: true, ThresholdAge: 18}, Rule: &Rule{Enabled: true, MinAge: 18}})
			require.False(t, decision.LocationDiscrepancy)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}

func TestCompare(t *testing.T) {
	co := Declared{Country: "CO", MeetsAge: true, ThresholdAge: 18}
	tests := map[string]struct {
		next     Declared
		expected compliance.DeclarationOutcome
	}{
		"the same answer":             {next: co, expected: compliance.DeclarationUnchanged},
		"another country":             {next: Declared{Country: "MX", MeetsAge: true, ThresholdAge: 18}, expected: compliance.DeclarationConflict},
		"another age answer":          {next: Declared{Country: "CO", MeetsAge: false, ThresholdAge: 18}, expected: compliance.DeclarationConflict},
		"a higher age asked anew":     {next: Declared{Country: "CO", MeetsAge: false, ThresholdAge: 19}, expected: compliance.DeclarationRecorded},
		"a lower age changes nothing": {next: Declared{Country: "CO", MeetsAge: true, ThresholdAge: 16}, expected: compliance.DeclarationUnchanged},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.expected, Compare(co, tc.next))
		})
	}
}
