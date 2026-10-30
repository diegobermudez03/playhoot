package login

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/require"
)

func challengeOf(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func TestVerifierMatches(t *testing.T) {
	verifier := "a-verifier-long-enough-to-be-random-0123456789"
	challenge := challengeOf(verifier)

	require.True(t, ValidChallenge(challenge))
	require.True(t, VerifierMatches(challenge, verifier))
	require.False(t, VerifierMatches(challenge, verifier+"x"))
	require.False(t, VerifierMatches(challenge, ""))
}

func TestValidChallenge(t *testing.T) {
	tests := map[string]struct {
		challenge string
		expected  bool
	}{
		"s256_hash":    {challenge: challengeOf("v"), expected: true},
		"plain_value":  {challenge: "plain", expected: false},
		"padded":       {challenge: challengeOf("v") + "=", expected: false},
		"empty":        {challenge: "", expected: false},
		"wrong_length": {challenge: base64.RawURLEncoding.EncodeToString([]byte("short")), expected: false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.expected, ValidChallenge(tc.challenge))
		})
	}
}

func TestRandomValuesDiffer(t *testing.T) {
	first, err := NewCode()
	require.NoError(t, err)
	second, err := NewCode()
	require.NoError(t, err)
	require.NotEqual(t, first, second)
	require.Contains(t, first, "phl_")
}
