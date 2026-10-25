// Package throttle is the API edge's request rate limiting: it answers a
// caller's requests to a route beyond the route's limits with 429, keyed by
// the authorized user or, without one, by client address. It knows no domain.
package throttle

import (
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

// DefaultLimits apply to every route that declares none. A client address is
// allowed more than a user because many people may share one (a classroom
// behind one router).
var DefaultLimits = routing.RequestLimits{
	PerUser:   []ratelimit.Limit{{Count: 5, Per: time.Second}, {Count: 30, Per: time.Minute}},
	PerClient: []ratelimit.Limit{{Count: 20, Per: time.Second}, {Count: 300, Per: time.Minute}},
}

var (
	keyLabel = metrics.NewLabel("key", "user", "client")

	keyUser   = keyLabel.Value("user")
	keyClient = keyLabel.Value("client")

	sweptLabel = metrics.NewLabel("swept", "true", "false")

	rateLimited = metrics.NewCounter(
		"api_http_rate_limited_total",
		"Requests refused for exceeding their route's limits, by what the caller was limited as.",
		keyLabel,
	)
	checkDuration = metrics.NewHistogram(
		"api_http_rate_limit_check_duration_seconds",
		"Time a request spent in its route's rate limit check, by what the caller was limited as and whether the check also forgot the keys whose room came back.",
		// A check takes microseconds; one that sweeps grows with the keys
		// remembered.
		[]float64{0.00001, 0.00005, 0.0001, 0.0005, 0.001, 0.005, 0.01, 0.05, 0.1},
		keyLabel, sweptLabel,
	)
)

// ClientAddress finds a request's client address behind TrustedProxies
// proxies, each of which appends the address it received the request from to
// X-Forwarded-For; with none, it is the connection's own address.
type ClientAddress struct {
	TrustedProxies int
}

// Of returns r's client address. Entries a client wrote itself are never
// trusted: only the ones the trusted proxies appended count.
func (a ClientAddress) Of(r *http.Request) string {
	if a.TrustedProxies > 0 {
		var forwarded []string
		for _, header := range r.Header.Values("X-Forwarded-For") {
			for _, entry := range strings.Split(header, ",") {
				if entry = strings.TrimSpace(entry); entry != "" {
					forwarded = append(forwarded, entry)
				}
			}
		}
		if len(forwarded) > 0 {
			return forwarded[max(0, len(forwarded)-a.TrustedProxies)]
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Requests refuses a caller's requests beyond limits through refuse, or with
// 429 and a Retry-After when refuse is nil or does not answer. A caller the
// route authorized is limited by UserUUID, any other by client address.
func Requests(limits routing.RequestLimits, clients ClientAddress, now func() time.Time, refuse routing.LimitRefuser) routing.Middleware {
	perUser := keyedOrNil(limits.PerUser)
	perClient := keyedOrNil(limits.PerClient)
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			limiter, key, kind := perClient, "", keyClient
			if userUUID, ok := auth.UserUUID(ctx); ok {
				limiter, key, kind = perUser, userUUID, keyUser
			} else {
				key = clients.Of(r)
			}
			if limiter != nil {
				began := time.Now()
				allowed, retryAfter, swept := limiter.Allow(key, now())
				checkDuration.ObserveDuration(ctx, time.Since(began), kind, sweptLabel.Value(strconv.FormatBool(swept)))
				if !allowed {
					rateLimited.Inc(ctx, kind)
					logging.LogFields(ctx, logging.Field("rate_limited", true))
					routing.NoteLimitRefusal(ctx, routing.RefusedRateLimited)
					if refuse != nil && refuse(w, r, routing.LimitRefusal{Reason: routing.RefusedRateLimited, RetryAfter: retryAfter}) {
						return
					}
					w.Header().Set("Retry-After", strconv.Itoa(RetryAfterSeconds(retryAfter)))
					httpx.WriteCodedError(w, http.StatusTooManyRequests, routing.RefusedRateLimited, "too many requests, try again later")
					return
				}
			}
			next(w, r)
		}
	}
}

// RetryAfterSeconds is retryAfter in whole seconds, rounded up and at least
// one, as clients are told to wait.
func RetryAfterSeconds(retryAfter time.Duration) int {
	return max(1, int(math.Ceil(retryAfter.Seconds())))
}

// keyedOrNil is a limiter for limits, or nil when there are none.
func keyedOrNil(limits []ratelimit.Limit) *ratelimit.Keyed {
	if len(limits) == 0 {
		return nil
	}
	return ratelimit.NewKeyed(limits...)
}
