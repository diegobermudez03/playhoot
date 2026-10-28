package displayname

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalize(t *testing.T) {
	type test struct {
		name string

		expected      string
		expectedValid bool
	}

	tests := map[string]func(t *testing.T) test{
		"trimmed": func(t *testing.T) test {
			return test{name: "  Ana  ", expected: "Ana", expectedValid: true}
		},
		"longest_allowed_in_characters_not_bytes": func(t *testing.T) test {
			return test{name: strings.Repeat("ñ", MaxLength), expected: strings.Repeat("ñ", MaxLength), expectedValid: true}
		},
		"too_long": func(t *testing.T) test {
			return test{name: strings.Repeat("a", MaxLength+1)}
		},
		"only_spaces": func(t *testing.T) test {
			return test{name: "   "}
		},
		"control_character": func(t *testing.T) test {
			return test{name: "An\na"}
		},
		"invalid_utf8": func(t *testing.T) test {
			return test{name: "An\xffa"}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			got, valid := Normalize(tc.name)

			require.Equal(t, tc.expectedValid, valid)
			require.Equal(t, tc.expected, got)
		})
	}
}
