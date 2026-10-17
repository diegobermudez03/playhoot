package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/diegobermudez03/playhoot/monitoring/metrics"
)

// trackedStatusCodes are the HTTP status codes this API distinguishes in its
// REST request metric. Any other code is counted as "other", so a handler
// writing an unexpected code can never introduce a new series.
var trackedStatusCodes = []int{
	200, 201, 204,
	301, 302, 304,
	400, 401, 403, 404, 405, 409, 413, 415, 422, 429,
	500, 501, 502, 503, 504,
}

// handshakeStatusCodes are the codes a WebSocket handshake can end with: 101
// when it was upgraded, otherwise why it was refused before the upgrade.
var handshakeStatusCodes = []int{101, 400, 401, 403, 404, 405, 429, 500, 501, 503}

const otherStatus = "other"

// requestMetricNames names one transport's request metrics.
type requestMetricNames struct {
	requests, requestsHelp string
	duration, durationHelp string
}

// requestMetrics measures finished HTTP requests the same way for every
// route: how many ended with each status code, and how long they took. It
// only describes the HTTP exchange; what an operation means for its own
// domain is that domain's to count.
type requestMetrics struct {
	requests    *metrics.Counter
	duration    *metrics.Histogram
	routes      map[string]metrics.LabelValue
	statuses    map[int]metrics.LabelValue
	otherStatus metrics.LabelValue
	now         func() time.Time
}

// newRESTMetrics declares the REST request metrics for the given route
// patterns, which become the closed set of the route label; that is why it is
// declared once every route is known.
func newRESTMetrics(patterns []string) *requestMetrics {
	return newRequestMetrics(requestMetricNames{
		requests:     "api_http_requests_total",
		requestsHelp: "Finished REST requests, by matched route and response status code.",
		duration:     "api_http_request_duration_seconds",
		durationHelp: "Time taken to serve REST requests, by matched route.",
	}, patterns, trackedStatusCodes)
}

// newHandshakeMetrics declares the WebSocket handshake metrics for the given
// route patterns.
func newHandshakeMetrics(patterns []string) *requestMetrics {
	return newRequestMetrics(requestMetricNames{
		requests:     "api_websocket_handshakes_total",
		requestsHelp: "Finished WebSocket handshakes, by matched route and status code (101 when upgraded).",
		duration:     "api_websocket_handshake_duration_seconds",
		durationHelp: "Time taken to answer WebSocket handshakes, by matched route.",
	}, patterns, handshakeStatusCodes)
}

func newRequestMetrics(names requestMetricNames, patterns []string, statusCodes []int) *requestMetrics {
	routeLabel := metrics.NewLabel("route", patterns...)

	statusValues := make([]string, 0, len(statusCodes)+1)
	for _, code := range statusCodes {
		statusValues = append(statusValues, strconv.Itoa(code))
	}
	statusValues = append(statusValues, otherStatus)
	statusLabel := metrics.NewLabel("status", statusValues...)

	m := &requestMetrics{
		requests:    metrics.NewCounter(names.requests, names.requestsHelp, routeLabel, statusLabel),
		duration:    metrics.NewHistogram(names.duration, names.durationHelp, metrics.LatencyBuckets(), routeLabel),
		routes:      make(map[string]metrics.LabelValue, len(patterns)),
		statuses:    make(map[int]metrics.LabelValue, len(statusCodes)),
		otherStatus: statusLabel.Value(otherStatus),
		now:         time.Now,
	}
	for _, pattern := range patterns {
		m.routes[pattern] = routeLabel.Value(pattern)
	}
	for _, code := range statusCodes {
		m.statuses[code] = statusLabel.Value(strconv.Itoa(code))
	}
	return m
}

// record measures one finished request that took elapsed and answered with
// status. pattern must be one of the patterns the metrics were declared with.
func (m *requestMetrics) record(ctx context.Context, pattern string, status int, elapsed time.Duration) {
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
