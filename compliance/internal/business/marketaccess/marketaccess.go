// Package marketaccess holds the pure rules of market access: which place a
// request is judged by, whether a function is available there to a person,
// and how a new residence-and-age declaration relates to the previous one.
package marketaccess

import (
	"regexp"

	"github.com/diegobermudez03/playhoot/compliance"
)

// DefaultThreshold is the age asked for in a place the matrix does not
// cover, where nothing is available anyway.
const DefaultThreshold = 18

var (
	countryPattern     = regexp.MustCompile(`^[A-Z]{2}$`)
	subdivisionPattern = regexp.MustCompile(`^[A-Z0-9]{1,3}$`)
)

// AdultFunctions need the declared age of the place; the minor functions
// need an authorization instead and have no age.
var AdultFunctions = map[compliance.Function]bool{
	compliance.FunctionCreateGames:    true,
	compliance.FunctionSubscribe:      true,
	compliance.FunctionTeach:          true,
	compliance.FunctionInteractiveUse: true,
}

// KnownFunction reports whether function is in the matrix.
func KnownFunction(function compliance.Function) bool {
	for _, known := range compliance.Functions {
		if function == known {
			return true
		}
	}
	return false
}

// ValidCountry reports whether country is an ISO 3166-1 alpha-2 code.
func ValidCountry(country string) bool {
	return countryPattern.MatchString(country)
}

// ValidSubdivision reports whether subdivision is empty or an ISO 3166-2
// subdivision suffix.
func ValidSubdivision(subdivision string) bool {
	return subdivision == "" || subdivisionPattern.MatchString(subdivision)
}

// Declared is a stored declaration as the rules need it.
type Declared struct {
	Country      string
	Subdivision  string
	MeetsAge     bool
	ThresholdAge int
}

// Rule is the matrix rule in force for a place and function.
type Rule struct {
	Enabled bool
	MinAge  int
}

// Facts are what Decide needs about a request. Declaration is nil when the
// person declared nothing; Rule is nil when the matrix has no rule in force
// for the judged place; CountryCovered reports whether the matrix has any
// rule for the judged country.
type Facts struct {
	Function           compliance.Function
	Declaration        *Declared
	ApproximateCountry string
	Rule               *Rule
	CountryCovered     bool
	Minority           bool
}

// Place is where a request is judged: the declared residence, or else the
// approximate location (a country only). ok is false when both are unknown.
func Place(declaration *Declared, approximateCountry string) (country, subdivision string, ok bool) {
	if declaration != nil {
		return declaration.Country, declaration.Subdivision, true
	}
	if approximateCountry != "" {
		return approximateCountry, "", true
	}
	return "", "", false
}

// Decide answers whether the function is available. A known minority binds
// over an adult declaration; an approximate location that differs from the
// declared residence is reported, never blocking on its own.
func Decide(facts Facts) compliance.AccessDecision {
	decision := compliance.AccessDecision{}
	if country, subdivision, ok := Place(facts.Declaration, facts.ApproximateCountry); ok {
		decision.Country, decision.Subdivision = country, subdivision
	} else {
		decision.Outcome = compliance.AccessNeedsDeclaration
		return decision
	}
	decision.LocationDiscrepancy = facts.Declaration != nil && facts.ApproximateCountry != "" && facts.ApproximateCountry != facts.Declaration.Country
	if facts.Rule == nil {
		if facts.Declaration == nil && facts.CountryCovered {
			// The country decides per state or province: only a declaration
			// says which.
			decision.Outcome = compliance.AccessNeedsDeclaration
			return decision
		}
		return blocked(decision, compliance.BlockMarketUnavailable)
	}
	if !facts.Rule.Enabled {
		return blocked(decision, compliance.BlockMarketUnavailable)
	}
	if facts.Rule.MinAge > 0 {
		if facts.Minority {
			return blocked(decision, compliance.BlockAgeRequirement)
		}
		if facts.Declaration == nil {
			decision.Outcome = compliance.AccessNeedsDeclaration
			return decision
		}
		if !facts.Declaration.MeetsAge {
			return blocked(decision, compliance.BlockAgeRequirement)
		}
		if facts.Declaration.ThresholdAge < facts.Rule.MinAge {
			// The age asked for rose since the person declared.
			decision.Outcome = compliance.AccessNeedsDeclaration
			return decision
		}
	}
	decision.Outcome = compliance.AccessAllowed
	return decision
}

func blocked(decision compliance.AccessDecision, reason compliance.BlockReason) compliance.AccessDecision {
	decision.Outcome = compliance.AccessBlocked
	decision.Reason = reason
	return decision
}

// Compare tells how a new answer relates to the previous declaration: the
// same answer changes nothing; a different one is a conflict, unless the age
// asked for the same place rose, which asks the question anew.
func Compare(previous Declared, next Declared) compliance.DeclarationOutcome {
	samePlace := previous.Country == next.Country && previous.Subdivision == next.Subdivision
	if samePlace && previous.MeetsAge == next.MeetsAge && previous.ThresholdAge >= next.ThresholdAge {
		return compliance.DeclarationUnchanged
	}
	if samePlace && previous.ThresholdAge < next.ThresholdAge {
		return compliance.DeclarationRecorded
	}
	return compliance.DeclarationConflict
}
