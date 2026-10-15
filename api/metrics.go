package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/diegobermudez03/playhoot/monitoring/metrics"
)

// trackedStatusCodes are the HTTP status codes this API distinguishes in its
// request metric. Any other code is counted as "other", so a handler writing
// an unexpected code can never introduce a new series.
var trackedStatusCodes = []int{
	200, 201, 204,
	301, 302, 304,
	400, 401, 403, 404, 405, 409, 413, 415, 422, 429,
	500, 501, 502, 503, 504,
}

const otherStatus = "other"

// restMetrics measures finished REST requests the same way for every route:
// how many ended with each HTTP status code, and how long they took. It only
// describes the HTTP exchange; what an operation means for its own domain
// (for example how many Sessions were created) is that domain's to count.
type restMetrics struct {
	requests    *metrics.Counter
	duration    *metrics.Histogram
	routes      map[string]metrics.LabelValue
	statuses    map[int]metrics.LabelValue
	otherStatus metrics.LabelValue
	now         func() time.Time
}

// newRESTMetrics declares the request metric for the given route patterns.
// The patterns become the closed set of values of the route label, which is
// why the metric is declared here, once every route is known, instead of
// where the routes are declared.
func newRESTMetrics(patterns []string) *restMetrics {
	routeLabel := metrics.NewLabel("route", patterns...)

	statusValues := make([]string, 0, len(trackedStatusCodes)+1)
	for _, code := range trackedStatusCodes {
		statusValues = append(statusValues, strconv.Itoa(code))
	}
	statusValues = append(statusValues, otherStatus)
	statusLabel := metrics.NewLabel("status", statusValues...)

	m := &restMetrics{
		requests: metrics.NewCounter(
			"api_http_requests_total",
			"Finished REST requests, by matched route and response status code.",
			routeLabel, statusLabel,
		),
		duration: metrics.NewHistogram(
			"api_http_request_duration_seconds",
			"Time taken to serve REST requests, by matched route.",
			metrics.LatencyBuckets(),
			routeLabel,
		),
		routes:      make(map[string]metrics.LabelValue, len(patterns)),
		statuses:    make(map[int]metrics.LabelValue, len(trackedStatusCodes)),
		otherStatus: statusLabel.Value(otherStatus),
		now:         time.Now,
	}
	for _, pattern := range patterns {
		m.routes[pattern] = routeLabel.Value(pattern)
	}
	for _, code := range trackedStatusCodes {
		m.statuses[code] = statusLabel.Value(strconv.Itoa(code))
	}
	return m
}

// record measures one finished request that took elapsed and answered with
// status. pattern must be one of the patterns the metrics were declared with.
func (m *restMetrics) record(ctx context.Context, pattern string, status int, elapsed time.Duration) {
	statusValue, tracked := m.statuses[status]
	if !tracked {
		statusValue = m.otherStatus
	}
	route := m.routes[pattern]
	m.requests.Inc(ctx, route, statusValue)
	m.duration.ObserveDuration(ctx, elapsed, route)
}

// statusRecorder remembers the status code a handler wrote, which
// http.ResponseWriter does not expose.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func newStatusRecorder(w http.ResponseWriter) *statusRecorder {
	return &statusRecorder{ResponseWriter: w, status: http.StatusOK}
}

func (s *statusRecorder) WriteHeader(status int) {
	s.status = status
	s.ResponseWriter.WriteHeader(status)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}
