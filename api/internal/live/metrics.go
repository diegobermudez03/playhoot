package live

import "github.com/diegobermudez03/playhoot/monitoring/metrics"

// Limits a client message can exceed, as metric label values.
const (
	limitConnectionMessages = "connection_messages"
	limitConnectionBytes    = "connection_bytes"
	limitTypeRate           = "type_rate"
	limitTypeQueue          = "type_queue"
)

// Reasons a live connection is refused before its upgrade, as metric label
// values.
const (
	refusedInstanceFull = "instance_full"
	refusedCallerFull   = "caller_full"
)

var (
	causeLabel = metrics.NewLabel("close_cause",
		string(CauseClientClosed),
		string(CauseBinaryFrame),
		string(CauseMalformedMessage),
		string(CauseUnknownMessageType),
		string(CauseRateLimited),
		string(CauseEndedByOwner),
		string(CauseServerShutdown),
		string(CauseServerFailure),
		string(CauseWriteFailed),
		string(CauseSlowConsumer),
		string(CauseUnresponsive),
	)
	limitLabel   = metrics.NewLabel("limit", limitConnectionMessages, limitConnectionBytes, limitTypeRate, limitTypeQueue)
	refusedLabel = metrics.NewLabel("reason", refusedInstanceFull, refusedCallerFull)

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
	messagesRefused = metrics.NewCounter(
		"api_live_messages_refused_total",
		"Client messages refused for exceeding a limit, by limit: the connection's message or byte rate (which ends it), or its type's rate or queue.",
		limitLabel,
	)
	connectionsRefused = metrics.NewCounter(
		"api_live_connections_refused_total",
		"Live connections refused before their upgrade, by reason: the instance or the caller had as many as allowed.",
		refusedLabel,
	)
)
