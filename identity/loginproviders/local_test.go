package loginproviders

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/identity"
)

func TestLocalExchange(t *testing.T) {
	local := NewLocal(LocalConfig{AuthorizeURL: "/authorize", CallbackURL: "/callback"})

	first, err := local.Exchange(context.Background(), "  Ana ", "nonce")
	require.NoError(t, err)
	again, err := local.Exchange(context.Background(), "ana", "other")
	require.NoError(t, err)

	require.Equal(t, ExternalIdentity{Issuer: LocalIssuer, Subject: "local:ana", Email: "ana@local.playhoot.invalid", Name: "Ana"}, first)
	require.Equal(t, first.Subject, again.Subject, "the same name is the same subject")

	_, err = local.Exchange(context.Background(), "   ", "nonce")
	require.ErrorIs(t, err, ErrInvalidAnswer)
}

func TestLocalPage(t *testing.T) {
	local := NewLocal(LocalConfig{AuthorizeURL: "/authorize", CallbackURL: "/v1/callback"})

	require.Equal(t, "/authorize?state=s%2B1", local.AuthorizationURL("s+1", "nonce"))
	page := local.AuthorizePage("s+1")
	require.Contains(t, page, `href="/v1/callback?code=Ana&amp;state=s%2B1"`)
	require.Contains(t, page, `href="/v1/callback?error=access_denied&amp;state=s%2B1"`)
	require.Contains(t, page, `name="state" value="s&#43;1"`)
}

func TestRegistry(t *testing.T) {
	disabled := New(Config{})
	_, ok := disabled.Get(identity.ProviderLocal)
	require.False(t, ok)
	_, ok = disabled.LocalAuthorizePage("s")
	require.False(t, ok)

	enabled := New(Config{Local: &LocalConfig{AuthorizeURL: "/a", CallbackURL: "/c"}})
	_, ok = enabled.Get(identity.ProviderLocal)
	require.True(t, ok)
	page, ok := enabled.LocalAuthorizePage("s")
	require.True(t, ok)
	require.True(t, strings.Contains(page, "Local sign-in"))
}
