package gameversion

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStatusAllowsSessions(t *testing.T) {
	type test struct {
		status   Status
		expected bool
	}

	tests := map[string]func(t *testing.T) test{
		"executable":     func(t *testing.T) test { return test{status: Executable, expected: true} },
		"draft":          func(t *testing.T) test { return test{status: Draft} },
		"unknown_status": func(t *testing.T) test { return test{status: Status("SOMETHING_ELSE")} },
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			require.Equal(t, tc.expected, tc.status.AllowsSessions())
		})
	}
}
