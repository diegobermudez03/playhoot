package emailcode

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalize(t *testing.T) {
	tests := map[string]struct {
		address   string
		expected  string
		recognized bool
	}{
		"trimmed_and_lowered": {address: "  Ana.Ruiz@Example.COM ", expected: "ana.ruiz@example.com", recognized: true},
		"subaddressed":        {address: "ana+playhoot@example.com", expected: "ana+playhoot@example.com", recognized: true},
		"no_at":               {address: "ana.example.com"},
		"two_ats":             {address: "ana@example@com"},
		"no_local_part":       {address: "@example.com"},
		"no_domain":           {address: "ana@"},
		"domain_without_dot":  {address: "ana@example"},
		"domain_ends_in_dot":  {address: "ana@example."},
		"inner_space":         {address: "an a@example.com"},
		"empty":               {address: "   "},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			normalized, ok := Normalize(tc.address)
			require.Equal(t, tc.recognized, ok)
			require.Equal(t, tc.expected, normalized)
		})
	}
}

func TestCodes(t *testing.T) {
	tests := map[string]func(t *testing.T){
		"a code is six digits": func(t *testing.T) {
			code, err := NewCode()
			require.NoError(t, err)
			require.Len(t, code, 6)
			for _, digit := range code {
				require.True(t, digit >= '0' && digit <= '9')
			}
		},
		"codes differ": func(t *testing.T) {
			seen := map[string]bool{}
			for range 50 {
				code, err := NewCode()
				require.NoError(t, err)
				seen[code] = true
			}
			require.Greater(t, len(seen), 40)
		},
		"only the right code matches its hash": func(t *testing.T) {
			hash := Hash("123456")
			require.NotContains(t, hash, "123456")
			require.True(t, Matches(hash, "123456"))
			require.False(t, Matches(hash, "123457"))
			require.False(t, Matches(hash, ""))
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
