// Package gamefault decides which game faults Session Runtime keeps: the
// kinds each source may record, with a detail small enough to store.
package gamefault

import (
	"encoding/json"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// kinds are the kinds each source records. SERVER_LIMIT, also a SERVER kind,
// is recorded apart, by recordlimithits.
var kinds = map[sessionpkg.GameFaultSource]map[sessionpkg.GameFaultKind]bool{
	sessionpkg.GameFaultFromClient: {
		sessionpkg.GameFaultArtifactUnreachable:      true,
		sessionpkg.GameFaultArtifactHashMismatch:     true,
		sessionpkg.GameFaultReadyTimeout:             true,
		sessionpkg.GameFaultUnsupportedBridgeVersion: true,
		sessionpkg.GameFaultContainerViolation:       true,
		sessionpkg.GameFaultBridgeAbuse:              true,
	},
	sessionpkg.GameFaultFromServer: {
		sessionpkg.GameFaultInvalidSample:  true,
		sessionpkg.GameFaultInvalidCommand: true,
	},
}

// Known reports whether kind is one Session Runtime records from source.
func Known(source sessionpkg.GameFaultSource, kind sessionpkg.GameFaultKind) bool {
	return kinds[source][kind]
}

// ValidDetail reports whether detail is a JSON object within
// sessionpkg.MaxGameFaultDetailBytes.
func ValidDetail(detail []byte) bool {
	if len(detail) == 0 || len(detail) > sessionpkg.MaxGameFaultDetailBytes {
		return false
	}
	var object map[string]json.RawMessage
	return json.Unmarshal(detail, &object) == nil && object != nil
}
