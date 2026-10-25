package gamefault

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestKnown(t *testing.T) {
	type test struct {
		source sessionpkg.GameFaultSource
		kind   sessionpkg.GameFaultKind

		expected bool
	}

	client, server := sessionpkg.GameFaultFromClient, sessionpkg.GameFaultFromServer
	tests := map[string]func(t *testing.T) test{
		"bridge_abuse_from_the_client": func(t *testing.T) test {
			return test{source: client, kind: sessionpkg.GameFaultBridgeAbuse, expected: true}
		},
		"container_violation_from_the_client": func(t *testing.T) test {
			return test{source: client, kind: sessionpkg.GameFaultContainerViolation, expected: true}
		},
		"artifact_unreachable_from_the_client": func(t *testing.T) test {
			return test{source: client, kind: sessionpkg.GameFaultArtifactUnreachable, expected: true}
		},
		"invalid_sample_from_the_server": func(t *testing.T) test {
			return test{source: server, kind: sessionpkg.GameFaultInvalidSample, expected: true}
		},
		"invalid_command_from_the_server": func(t *testing.T) test {
			return test{source: server, kind: sessionpkg.GameFaultInvalidCommand, expected: true}
		},
		"a_server_kind_from_the_client": func(t *testing.T) test {
			return test{source: client, kind: sessionpkg.GameFaultInvalidSample}
		},
		"a_client_kind_from_the_server": func(t *testing.T) test {
			return test{source: server, kind: sessionpkg.GameFaultBridgeAbuse}
		},
		"server_limit_is_not_recorded_here": func(t *testing.T) test {
			return test{source: server, kind: sessionpkg.GameFaultKind("SERVER_LIMIT")}
		},
		"unknown_source": func(t *testing.T) test {
			return test{source: sessionpkg.GameFaultSource("ADMIN"), kind: sessionpkg.GameFaultBridgeAbuse}
		},
		"unknown":              func(t *testing.T) test { return test{source: client, kind: sessionpkg.GameFaultKind("CHEATING")} },
		"lowercase_wire_value": func(t *testing.T) test { return test{source: client, kind: sessionpkg.GameFaultKind("bridge_abuse")} },
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			require.Equal(t, tc.expected, Known(tc.source, tc.kind))
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
