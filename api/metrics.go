package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/routing"
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

// noLimitRefusal is the limit_refusal of a handshake no edge limit refused.
const noLimitRefusal = "none"

// handshakeMetrics are the request metrics of WebSocket handshakes, which
// also tell which of the edge's limits refused one, since such a refusal may
// be told on an upgraded connection and so be counted as 101.
type handshakeMetrics struct {
	*requestMetrics
	limitRefusals  map[string]metrics.LabelValue
	noLimitRefusal metrics.LabelValue
}

// newHandshakeMetrics declares the WebSocket handshake metrics for the given
// route patterns.
func newHandshakeMetrics(patterns []string) *handshakeMetrics {
	reasons := []string{routing.RefusedRateLimited, routing.RefusedTooManyConnections, routing.RefusedServerBusy}
	refusalLabel := metrics.NewLabel("limit_refusal", append([]string{noLimitRefusal}, reasons...)...)
	m := &handshakeMetrics{
		requestMetrics: newRequestMetrics(requestMetricNames{
			requests:     "api_websocket_handshakes_total",
			requestsHelp: "Finished WebSocket handshakes, by matched route, status code (101 when upgraded) and which edge limit refused it, if one did, whether told before or after the upgrade.",
			duration:     "api_websocket_handshake_duration_seconds",
			durationHelp: "Time taken to answer WebSocket handshakes, by matched route.",
		}, patterns, handshakeStatusCodes, refusalLabel),
		limitRefusals:  make(map[string]metrics.LabelValue, len(reasons)),
		noLimitRefusal: refusalLabel.Value(noLimitRefusal),
	}
	for _, reason := range reasons {
		m.limitRefusals[reason] = refusalLabel.Value(reason)
	}
	return m
}

// record measures one finished handshake that took elapsed, answered with
// status, refused by the edge limit limitRefusal (empty when none did).
func (m *handshakeMetrics) record(ctx context.Context, pattern string, status int, limitRefusal string, elapsed time.Duration) {
	refusal, known := m.limitRefusals[limitRefusal]
	if !known {
		refusal = m.noLimitRefusal
	}
	m.requestMetrics.record(ctx, pattern, status, elapsed, refusal)
}

// newRequestMetrics declares request metrics whose count is labeled by route,
// status and, after them, extraLabels.
func newRequestMetrics(names requestMetricNames, patterns []string, statusCodes []int, extraLabels ...metrics.Label) *requestMetrics {
	routeLabel := metrics.NewLabel("route", patterns...)

	statusValues := make([]string, 0, len(statusCodes)+1)
	for _, code := range statusCodes {
		statusValues = append(statusValues, strconv.Itoa(code))
	}
	statusValues = append(statusValues, otherStatus)
	statusLabel := metrics.NewLabel("status", statusValues...)

	m := &requestMetrics{
		requests:    metrics.NewCounter(names.requests, names.requestsHelp, append([]metrics.Label{routeLabel, statusLabel}, extraLabels...)...),
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
// status, with the values of the extra labels the metrics were declared with.
// pattern must be one of the patterns the metrics were declared with.
func (m *requestMetrics) record(ctx context.Context, pattern string, status int, elapsed time.Duration, extra ...metrics.LabelValue) {
	statusValue, tracked := m.statuses[status]
	if !tracked {
		statusValue = m.otherStatus
	}
	route := m.routes[pattern]
	m.requests.Inc(ctx, append([]metrics.LabelValue{route, statusValue}, extra...)...)
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
