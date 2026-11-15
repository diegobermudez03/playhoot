package geolocation

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
)

type fixedLocator struct {
	country string
	err     error
	asked   []netip.Addr
}

func (l *fixedLocator) Country(_ context.Context, address netip.Addr) (string, error) {
	l.asked = append(l.asked, address)
	return l.country, l.err
}

func TestApproximateCountry(t *testing.T) {
	ctx := context.Background()
	tests := map[string]func(t *testing.T){
		"the local locator never knows": func(t *testing.T) {
			require.Equal(t, "", ApproximateCountry(ctx, Unknown{}, "190.25.1.1"))
		},
		"a public address takes the locator's country, normalized": func(t *testing.T) {
			locator := &fixedLocator{country: " co "}
			require.Equal(t, "CO", ApproximateCountry(ctx, locator, "190.25.1.1"))
		},
		"private, loopback and malformed addresses are never looked up": func(t *testing.T) {
			locator := &fixedLocator{country: "CO"}
			for _, address := range []string{"10.0.0.1", "127.0.0.1", "::1", "not-an-ip", ""} {
				require.Equal(t, "", ApproximateCountry(ctx, locator, address), address)
			}
			require.Empty(t, locator.asked)
		},
		"a failure or an answer that is not a country is unknown": func(t *testing.T) {
			require.Equal(t, "", ApproximateCountry(ctx, &fixedLocator{err: errors.New("down")}, "190.25.1.1"))
			require.Equal(t, "", ApproximateCountry(ctx, &fixedLocator{country: "Colombia"}, "190.25.1.1"))
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}
