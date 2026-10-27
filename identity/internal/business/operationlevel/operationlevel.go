// Package operationlevel is the catalog of the minimum authentication level
// each operation requires: the rule Authorize applies and the list clients
// read to know where to send a person to authenticate.
package operationlevel

import (
	"sort"

	"github.com/diegobermudez03/playhoot/identity"
)

// required is the catalog. Every identity.Operation must be in it.
var required = map[identity.Operation]identity.Level{
	identity.OperationCreateSession:         identity.LevelAccount,
	identity.OperationJoinSessionAsAdmin:    identity.LevelAccount,
	identity.OperationJoinSessionAsPlayer:   identity.LevelGuest,
	identity.OperationReportClientTelemetry: identity.LevelGuest,
	identity.OperationViewOwnIdentity:       identity.LevelGuest,
}

// rank orders the levels; a level missing from it satisfies nothing.
var rank = map[identity.Level]int{
	identity.LevelAnonymous: 0,
	identity.LevelGuest:     1,
	identity.LevelAccount:   2,
}

// Required returns the minimum level operation requires, or false when the
// catalog does not know it.
func Required(operation identity.Operation) (identity.Level, bool) {
	level, ok := required[operation]
	return level, ok
}

// Satisfies reports whether a caller at level may perform an operation that
// requires the level required.
func Satisfies(level identity.Level, required identity.Level) bool {
	have, ok := rank[level]
	if !ok {
		return false
	}
	need, ok := rank[required]
	if !ok {
		return false
	}
	return have >= need
}

// Catalog returns every operation with its required level, ordered by
// operation.
func Catalog() []identity.OperationLevel {
	catalog := make([]identity.OperationLevel, 0, len(required))
	for operation, level := range required {
		catalog = append(catalog, identity.OperationLevel{Operation: operation, RequiredLevel: level})
	}
	sort.Slice(catalog, func(i, j int) bool { return catalog[i].Operation < catalog[j].Operation })
	return catalog
}
