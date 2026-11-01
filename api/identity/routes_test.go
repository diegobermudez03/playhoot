package identity

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/api/internal/throttle"
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

func TestGuestCreationIsCappedPerAddressPerHour(t *testing.T) {
	for _, route := range NewHandler(nil, nil, nil, nil, nil, nil, Logins{}).RESTRoutes() {
		if route.Pattern != "POST /v1/guests" {
			continue
		}
		require.NotNil(t, route.Limits)
		require.Contains(t, route.Limits.PerClient, ratelimit.Limit{Count: 200, Per: time.Hour})
		for _, limit := range throttle.DefaultLimits.PerClient {
			require.Contains(t, route.Limits.PerClient, limit, "the defaults still apply")
		}
		return
	}
	t.Fatal("POST /v1/guests is not declared")
}
