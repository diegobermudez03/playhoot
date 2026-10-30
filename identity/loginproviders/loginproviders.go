// Package loginproviders holds the external identity providers accounts sign
// in with, behind one port: where to send the person, and what identity the
// provider's answer proves.
package loginproviders

import (
	"context"
	"errors"

	"github.com/diegobermudez03/playhoot/identity"
)

// ExternalIdentity is who a provider vouches for: Issuer and Subject identify
// them; Email and Name are only observed and never identify anyone.
type ExternalIdentity struct {
	Issuer  string
	Subject string
	Email   string
	Name    string
}

// ErrInvalidAnswer is returned when the provider's answer proves nothing: a
// code it does not recognize, a token that fails verification.
var ErrInvalidAnswer = errors.New("invalid provider answer")

// Provider is one external identity provider.
type Provider interface {
	// AuthorizationURL is where the person proves their identity; the
	// provider answers the callback with a code, state and nonce bound to it.
	AuthorizationURL(state string, nonce string) string
	// Exchange turns the provider's code into the identity it proves, or
	// ErrInvalidAnswer; any other error means the provider is unreachable.
	Exchange(ctx context.Context, code string, nonce string) (ExternalIdentity, error)
}

// Config chooses the enabled providers.
type Config struct {
	// Local enables the fictitious local provider; never in production.
	Local *LocalConfig
}

// Registry is the set of enabled providers.
type Registry struct {
	providers map[identity.Provider]Provider
	local     *Local
}

// New returns the providers config enables.
func New(config Config) *Registry {
	registry := &Registry{providers: map[identity.Provider]Provider{}}
	if config.Local != nil {
		registry.local = NewLocal(*config.Local)
		registry.providers[identity.ProviderLocal] = registry.local
	}
	return registry
}

// Get returns the provider named name, if enabled.
func (r *Registry) Get(name identity.Provider) (Provider, bool) {
	provider, ok := r.providers[name]
	return provider, ok
}

// LocalAuthorizePage returns the local provider's sign-in page for an attempt,
// or false when the local provider is not enabled.
func (r *Registry) LocalAuthorizePage(state string) (string, bool) {
	if r.local == nil {
		return "", false
	}
	return r.local.AuthorizePage(state), true
}
