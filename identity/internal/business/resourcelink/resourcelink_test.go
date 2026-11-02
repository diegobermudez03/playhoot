package resourcelink

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTokens(t *testing.T) {
	first, err := NewToken()
	require.NoError(t, err)
	second, err := NewToken()
	require.NoError(t, err)
	require.NotEqual(t, first, second)
	require.True(t, strings.HasPrefix(first, TokenPrefix))
	require.True(t, WellFormed(first))
	require.NotEqual(t, first, Hash(first), "only the hash is kept")
	require.Len(t, Hash(first), 64)

	for _, malformed := range []string{"", "phl_", "phl_short", strings.TrimPrefix(first, TokenPrefix), first + "x"} {
		require.False(t, WellFormed(malformed), malformed)
	}
}

func TestValidResource(t *testing.T) {
	require.True(t, ValidResource("game", "5c1f..."))
	require.False(t, ValidResource("Game", "x"))
	require.False(t, ValidResource("game", ""))
	require.False(t, ValidResource("game", strings.Repeat("x", 129)))
}
