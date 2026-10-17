package live

import "github.com/diegobermudez03/playhoot/monitoring/metrics"

var (
	causeLabel = metrics.NewLabel("close_cause",
		string(CauseClientClosed),
		string(CauseBinaryFrame),
		string(CauseMalformedMessage),
		string(CauseUnknownMessageType),
		string(CauseEndedByOwner),
		string(CauseServerShutdown),
		string(CauseServerFailure),
		string(CauseWriteFailed),
	)

	connectionsActive = metrics.NewGauge(
		"api_live_connections_active",
		"Live connections this instance is serving.",
	)
	connectionsClosed = metrics.NewCounter(
		"api_live_connections_closed_total",
		"Live connections that ended, by close cause.",
		causeLabel,
	)
	connectionDuration = metrics.NewHistogram(
		"api_live_connection_duration_seconds",
		"How long live connections stayed open.",
		// Connections last from seconds to hours, far beyond latency buckets.
		[]float64{1, 5, 15, 30, 60, 300, 900, 1800, 3600, 7200, 14400},
	)
)
