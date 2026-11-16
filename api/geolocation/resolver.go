package geolocation

import "context"

type clientAddressContextKey struct{}

// WithClientAddress returns ctx carrying the address the request came from,
// as the edge resolved it behind its trusted proxies.
func WithClientAddress(ctx context.Context, address string) context.Context {
	return context.WithValue(ctx, clientAddressContextKey{}, address)
}

// ClientAddress returns the address the request came from, empty when the
// edge did not note one.
func ClientAddress(ctx context.Context) string {
	address, _ := ctx.Value(clientAddressContextKey{}).(string)
	return address
}

// Resolver answers the approximate country of the request being served. A
// zero Resolver, and a locator that cannot tell, answer unknown: a country
// is a signal, never a requirement.
type Resolver struct {
	Locator Locator
}

// Country returns the ISO country the caller's address suggests, or "".
func (r Resolver) Country(ctx context.Context) string {
	if r.Locator == nil {
		return ""
	}
	return ApproximateCountry(ctx, r.Locator, ClientAddress(ctx))
}
