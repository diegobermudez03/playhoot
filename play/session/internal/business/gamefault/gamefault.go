// Package gamefault decides which game fault reports Session Runtime keeps:
// the kinds it knows, with a detail small enough to store.
package gamefault

import (
	"encoding/json"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

var kinds = map[sessionpkg.GameFaultKind]bool{
	sessionpkg.GameFaultArtifactUnreachable:      true,
	sessionpkg.GameFaultArtifactHashMismatch:     true,
	sessionpkg.GameFaultReadyTimeout:             true,
	sessionpkg.GameFaultUnsupportedBridgeVersion: true,
	sessionpkg.GameFaultContainerViolation:       true,
	sessionpkg.GameFaultBridgeAbuse:              true,
}

// Known reports whether kind is one Session Runtime records.
func Known(kind sessionpkg.GameFaultKind) bool {
	return kinds[kind]
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
