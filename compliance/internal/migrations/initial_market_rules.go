package migrations

import (
	"time"

	"github.com/diegobermudez03/playhoot/compliance"
	"github.com/diegobermudez03/playhoot/compliance/internal/business/marketaccess"
)

// marketRuleRow is a compliance_market_rules row as the seed writes it.
type marketRuleRow struct {
	ID             int64
	Country        string
	Subdivision    string
	Function       string
	Enabled        bool
	MinAge         int
	ConditionsNote string
	EffectiveAt    time.Time
	Reason         string
	CreatedAt      time.Time
}

func (marketRuleRow) TableName() string { return "compliance_market_rules" }

const initialReason = "Initial phase 1 coverage."

// usAgeExceptions are the states requiring other than 18.
var usAgeExceptions = map[string]int{"AL": 19, "NE": 19, "MS": 21}

var usSubdivisions = []string{
	"AL", "AK", "AZ", "AR", "CA", "CO", "CT", "DE", "DC", "FL", "GA", "HI", "ID", "IL", "IN", "IA", "KS",
	"KY", "LA", "ME", "MD", "MA", "MI", "MN", "MS", "MO", "MT", "NE", "NV", "NH", "NJ", "NM", "NY", "NC",
	"ND", "OH", "OK", "OR", "PA", "RI", "SC", "SD", "TN", "TX", "UT", "VT", "VA", "WA", "WV", "WI", "WY",
}

// Canada without Quebec: 18 in these provinces, 19 in the rest.
var caSubdivisions = map[string]int{
	"AB": 18, "MB": 18, "ON": 18, "PE": 18, "SK": 18,
	"BC": 19, "NB": 19, "NL": 19, "NS": 19, "NT": 19, "NU": 19, "YT": 19,
}

// initialMarketRules is the initial matrix: Colombia and Mexico enabled;
// Argentina, Chile and Peru present but disabled until their operator
// conditions are met; the US and Canada adults only, per state or province.
// Every other market has no row, so nothing is available there.
func initialMarketRules(at time.Time) []marketRuleRow {
	var rows []marketRuleRow
	add := func(country, subdivision string, minAge int, minorsEnabled, adultsEnabled bool, note string) {
		for _, function := range compliance.Functions {
			row := marketRuleRow{
				Country: country, Subdivision: subdivision, Function: string(function),
				ConditionsNote: note, EffectiveAt: at, Reason: initialReason, CreatedAt: at,
			}
			if marketaccess.AdultFunctions[function] {
				row.Enabled, row.MinAge = adultsEnabled, minAge
			} else {
				row.Enabled = minorsEnabled
			}
			rows = append(rows, row)
		}
	}
	add("CO", "", 18, true, true, "")
	add("MX", "", 18, true, true, "")
	add("AR", "", 18, false, false, "Pending: foreign controller registration with AAIP and international transfer safeguards.")
	add("CL", "", 18, false, false, "Pending: readiness for Law 21.719 (in force 2026-12-01) and international transfer safeguards.")
	add("PE", "", 18, false, false, "Pending: representation in or for Peru required by its regulation.")
	for _, state := range usSubdivisions {
		age := 18
		if exception, ok := usAgeExceptions[state]; ok {
			age = exception
		}
		add("US", state, age, false, true, "")
	}
	for province, age := range caSubdivisions {
		add("CA", province, age, false, true, "")
	}
	return rows
}
