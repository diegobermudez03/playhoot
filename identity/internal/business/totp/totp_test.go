package totp

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// rfcSecret is RFC 6238's SHA-1 test key, "12345678901234567890".
var rfcSecret = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))

func TestCodeAtMatchesTheRFCVectors(t *testing.T) {
	tests := map[string]func(t *testing.T) (int64, string){
		"59":         func(t *testing.T) (int64, string) { return 59, "287082" },
		"1111111109": func(t *testing.T) (int64, string) { return 1111111109, "081804" },
		"1234567890": func(t *testing.T) (int64, string) { return 1234567890, "005924" },
		"2000000000": func(t *testing.T) (int64, string) { return 2000000000, "279037" },
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			unix, expected := setup(t)
			code, err := CodeAt(rfcSecret, StepAt(time.Unix(unix, 0)))
			require.NoError(t, err)
			require.Equal(t, expected, code)
		})
	}
}

func TestMatch(t *testing.T) {
	now := time.Unix(1234567890, 0)
	current := StepAt(now)
	codeAt := func(step int64) string {
		code, err := CodeAt(rfcSecret, step)
		require.NoError(t, err)
		return code
	}
	type test struct {
		code         string
		expectedStep int64
		expectedOK   bool
	}
	tests := map[string]func(t *testing.T) test{
		"current": func(t *testing.T) test { return test{code: codeAt(current), expectedStep: current, expectedOK: true} },
		"one_step_early": func(t *testing.T) test {
			return test{code: codeAt(current - 1), expectedStep: current - 1, expectedOK: true}
		},
		"one_step_late": func(t *testing.T) test {
			return test{code: codeAt(current + 1), expectedStep: current + 1, expectedOK: true}
		},
		"two_steps_early": func(t *testing.T) test { return test{code: codeAt(current - 2)} },
		"malformed":       func(t *testing.T) test { return test{code: "12a456"} },
		"too_short":       func(t *testing.T) test { return test{code: "12345"} },
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			step, ok, err := Match(rfcSecret, tc.code, now)
			require.NoError(t, err)
			require.Equal(t, tc.expectedOK, ok)
			require.Equal(t, tc.expectedStep, step)
		})
	}
}

func TestLockedUntil(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	ago := func(minutes ...int) []time.Time {
		times := make([]time.Time, len(minutes))
		for i, m := range minutes {
			times[i] = now.Add(-time.Duration(m) * time.Minute)
		}
		return times
	}
	type test struct {
		failures       []time.Time
		expectedLocked bool
		expectedUntil  time.Time
	}
	tests := map[string]func(t *testing.T) test{
		"four_failures_do_not_lock": func(t *testing.T) test { return test{failures: ago(1, 2, 3, 4)} },
		"five_within_the_window_lock": func(t *testing.T) test {
			return test{failures: ago(1, 2, 3, 4, 5), expectedLocked: true, expectedUntil: now.Add(14 * time.Minute)}
		},
		"five_spread_beyond_the_window_do_not": func(t *testing.T) test { return test{failures: ago(1, 2, 3, 4, 17)} },
		"the_lock_ends": func(t *testing.T) test {
			return test{failures: ago(15, 16, 17, 18, 19), expectedUntil: now}
		},
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			until, locked := LockedUntil(tc.failures, now)
			require.Equal(t, tc.expectedLocked, locked)
			require.Equal(t, tc.expectedUntil, until)
		})
	}
}

func TestSealer(t *testing.T) {
	sealer, err := NewSealer([]byte(strings.Repeat("k", KeyBytes)))
	require.NoError(t, err)
	sealed, err := sealer.Seal(rfcSecret)
	require.NoError(t, err)
	require.NotContains(t, sealed, rfcSecret)
	opened, err := sealer.Open(sealed)
	require.NoError(t, err)
	require.Equal(t, rfcSecret, opened)

	other, err := NewSealer([]byte(strings.Repeat("o", KeyBytes)))
	require.NoError(t, err)
	_, err = other.Open(sealed)
	require.ErrorIs(t, err, ErrUnsealable)

	_, err = NewSealer([]byte("short"))
	require.Error(t, err)
}

func TestProofs(t *testing.T) {
	proof, err := NewProof()
	require.NoError(t, err)
	require.True(t, WellFormedProof(proof))
	require.False(t, WellFormedProof("not-a-proof"))
	require.Len(t, HashProof(proof), 64)
	require.True(t, ValidAction("budget.reopen"))
	require.False(t, ValidAction(""))
	require.False(t, ValidAction(strings.Repeat("a", MaxActionLength+1)))
}
