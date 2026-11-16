package unsubscribe

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSealer(t *testing.T) {
	sealer, err := New(bytes.Repeat([]byte{7}, 32))
	require.NoError(t, err)
	tests := map[string]func(t *testing.T){
		"a sealed address opens again and is not readable in the token": func(t *testing.T) {
			token, err := sealer.Seal("ana@example.com")
			require.NoError(t, err)
			require.NotContains(t, token, "ana")
			email, err := sealer.Open(token)
			require.NoError(t, err)
			require.Equal(t, "ana@example.com", email)
		},
		"a token from another key or tampered is refused": func(t *testing.T) {
			other, err := New(bytes.Repeat([]byte{8}, 32))
			require.NoError(t, err)
			token, err := other.Seal("ana@example.com")
			require.NoError(t, err)
			_, err = sealer.Open(token)
			require.ErrorIs(t, err, ErrInvalid)
			_, err = sealer.Open(strings.Repeat("A", 60))
			require.ErrorIs(t, err, ErrInvalid)
		},
		"a short key is refused": func(t *testing.T) {
			_, err := New([]byte("short"))
			require.Error(t, err)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
