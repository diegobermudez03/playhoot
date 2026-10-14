package api

import (
	"context"
	"net/http"
	"strconv"

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

// restMetrics counts finished REST requests by the route they matched and
// the status code they answered with.
type restMetrics struct {
	requests    *metrics.Counter
	routes      map[string]metrics.LabelValue
	statuses    map[int]metrics.LabelValue
	otherStatus metrics.LabelValue
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
		routes:      make(map[string]metrics.LabelValue, len(patterns)),
		statuses:    make(map[int]metrics.LabelValue, len(trackedStatusCodes)),
		otherStatus: statusLabel.Value(otherStatus),
	}
	for _, pattern := range patterns {
		m.routes[pattern] = routeLabel.Value(pattern)
	}
	for _, code := range trackedStatusCodes {
		m.statuses[code] = statusLabel.Value(strconv.Itoa(code))
	}
	return m
}

// record counts one finished request. pattern must be one of the patterns
// the metric was declared with.
func (m *restMetrics) record(ctx context.Context, pattern string, status int) {
	statusValue, tracked := m.statuses[status]
	if !tracked {
		statusValue = m.otherStatus
	}
	m.requests.Inc(ctx, m.routes[pattern], statusValue)
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
