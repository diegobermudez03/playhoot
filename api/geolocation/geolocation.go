// Package geolocation is the port through which the API edge learns the
// approximate country of a client address. It is a coarse signal: never
// precise location, and unknown whenever it cannot tell.
package geolocation

import (
	"context"
	"log/slog"
	"net/netip"
	"regexp"
	"strings"
)

// Locator finds the country an address most likely belongs to, as an ISO
// 3166-1 alpha-2 code, or "" when it cannot tell.
type Locator interface {
	Country(ctx context.Context, address netip.Addr) (string, error)
}

// Unknown is the local Locator: it never knows the country.
type Unknown struct{}

// Country always answers unknown.
func (Unknown) Country(context.Context, netip.Addr) (string, error) {
	return "", nil
}

var countryPattern = regexp.MustCompile(`^[A-Z]{2}$`)

// ApproximateCountry returns the country locator finds for clientAddress,
// or "" when the address is not routable, the locator fails or answers
// something that is not a country code. A failure never blocks anything.
func ApproximateCountry(ctx context.Context, locator Locator, clientAddress string) string {
	address, err := netip.ParseAddr(clientAddress)
	if err != nil || !address.IsGlobalUnicast() || address.IsPrivate() {
		return ""
	}
	country, err := locator.Country(ctx, address.Unmap())
	if err != nil {
		slog.WarnContext(ctx, "approximate location unavailable", "error", err)
		return ""
	}
	country = strings.ToUpper(strings.TrimSpace(country))
	if !countryPattern.MatchString(country) {
		return ""
	}
	return country
}
