package gamefault

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestKnown(t *testing.T) {
	type test struct {
		kind sessionpkg.GameFaultKind

		expected bool
	}

	tests := map[string]func(t *testing.T) test{
		"bridge_abuse":         func(t *testing.T) test { return test{kind: sessionpkg.GameFaultBridgeAbuse, expected: true} },
		"container_violation":  func(t *testing.T) test { return test{kind: sessionpkg.GameFaultContainerViolation, expected: true} },
		"artifact_unreachable": func(t *testing.T) test { return test{kind: sessionpkg.GameFaultArtifactUnreachable, expected: true} },
		"unknown":              func(t *testing.T) test { return test{kind: sessionpkg.GameFaultKind("CHEATING")} },
		"lowercase_wire_value": func(t *testing.T) test { return test{kind: sessionpkg.GameFaultKind("bridge_abuse")} },
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			require.Equal(t, tc.expected, Known(tc.kind))
		})
	}
}

func TestValidDetail(t *testing.T) {
	type test struct {
		detail []byte

		expected bool
	}

	tests := map[string]func(t *testing.T) test{
		"object":       func(t *testing.T) test { return test{detail: []byte(`{"refused":{"sample":900}}`), expected: true} },
		"empty_object": func(t *testing.T) test { return test{detail: []byte(`{}`), expected: true} },
		"array":        func(t *testing.T) test { return test{detail: []byte(`[1]`)} },
		"null":         func(t *testing.T) test { return test{detail: []byte(`null`)} },
		"missing":      func(t *testing.T) test { return test{} },
		"not_json":     func(t *testing.T) test { return test{detail: []byte(`{oops`)} },
		"too_large": func(t *testing.T) test {
			return test{detail: []byte(`{"x":"` + strings.Repeat("a", sessionpkg.MaxGameFaultDetailBytes) + `"}`)}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			require.Equal(t, tc.expected, ValidDetail(tc.detail))
		})
	}
}
