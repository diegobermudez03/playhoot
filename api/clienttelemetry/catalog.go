package clienttelemetry

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/diegobermudez03/playhoot/monitoring/metrics"
)

// The catalog of what a client may report. A client is not trusted: values
// are bounded, every label value comes from a closed set, and the metrics are
// read as trends of what players experience, never to decide about a user or
// a game. They are declared here because this route group owns the contract
// of client telemetry; the client owns the behavior they describe.

const (
	// maxDurationMs bounds a reported duration (10 minutes).
	maxDurationMs = 600_000
	// maxCount bounds a reported count, which the client coalesces.
	maxCount = 10_000
)

var (
	closeKinds     = []string{"retryable", "unreachable", "protocol", "replaced", "left", "session_ended", "disconnected", "other"}
	reconnectKinds = []string{"reconnected", "gave_up"}
	refusalReasons = []string{"invalid", "over_limit"}
	cacheLabel     = metrics.NewLabel("cache", "hit", "miss")
	closeLabel     = metrics.NewLabel("close", closeKinds...)
	reconnectLabel = metrics.NewLabel("outcome", reconnectKinds...)
	refusalLabel   = metrics.NewLabel("reason", refusalReasons...)
	originLabel    = metrics.NewLabel("origin", "presentation", "bridge", "transport", "backend_rejection", "artifact")
	kindLabel      = metrics.NewLabel("kind", allKinds()...)
	refusedLabel   = metrics.NewLabel("reason", "unknown_event", "invalid_value")

	artifactLoad = metrics.NewHistogram(
		"client_game_artifact_load_seconds",
		"How long players' clients took to get a game's frontend package, from their cache or by downloading it, as they report it.",
		[]float64{0.05, 0.1, 0.25, 0.5, 1, 2, 4, 8, 15, 30, 60},
		cacheLabel,
	)
	gameReady = metrics.NewHistogram(
		"client_game_ready_seconds",
		"How long a game's frontend took to answer the client's bridge handshake once loaded, as clients report it.",
		[]float64{0.05, 0.1, 0.25, 0.5, 1, 2, 4, 8, 15},
	)
	connectionsLost = metrics.NewCounter(
		"client_connections_lost_total",
		"Live connections players' clients saw close, by how they classified the close.",
		closeLabel,
	)
	reconnects = metrics.NewCounter(
		"client_reconnects_total",
		"Reconnections players' clients tried after losing their live connection, by outcome.",
		reconnectLabel,
	)
	resyncs = metrics.NewCounter(
		"client_resyncs_total",
		"FULL views players' clients asked for because they missed a view.",
	)
	bridgeRefusals = metrics.NewCounter(
		"client_bridge_refusals_total",
		"Messages players' clients refused from the game frontend, by reason: invalid, or over the bridge's limits.",
		refusalLabel,
	)
	clientErrors = metrics.NewCounter(
		"client_errors_total",
		"Errors players' clients saw, classified by origin and kind.",
		originLabel, kindLabel,
	)
	eventsRefused = metrics.NewCounter(
		"client_telemetry_events_refused_total",
		"Reported telemetry events dropped for naming no known event or carrying a value out of the catalog.",
		refusedLabel,
	)
)

// errorKinds are the kinds of error each origin may report.
var errorKinds = map[string][]string{
	"presentation": {"render_failed"},
	"bridge":       {"bridge_abuse", "container_violation"},
	"transport":    {"unreachable", "connection_lost", "protocol_error", "network"},
	"backend_rejection": {
		"lobby_full", "session_started", "session_unavailable", "participant_disconnected", "rejoin_rejected",
		"server_busy", "owner_changed", "not_creator", "too_many_admins", "rejected_unknown", "token_invalid", "rate_limited", "operation_forbidden", "http_error",
	},
	"artifact": {"artifact_unreachable", "artifact_hash_mismatch", "ready_timeout", "unsupported_bridge_version"},
}

func allKinds() []string {
	var kinds []string
	for _, origin := range []string{"presentation", "bridge", "transport", "backend_rejection", "artifact"} {
		kinds = append(kinds, errorKinds[origin]...)
	}
	return kinds
}

// event is one reported event; which fields count depends on its name.
type event struct {
	Name       string `json:"name"`
	DurationMs *int64 `json:"duration_ms"`
	Cached     *bool  `json:"cached"`
	Close      string `json:"close"`
	Outcome    string `json:"outcome"`
	Reason     string `json:"reason"`
	Origin     string `json:"origin"`
	Kind       string `json:"kind"`
	Count      *int64 `json:"count"`
}

// recordEvent records raw as its metric, or counts it refused when it is not
// in the catalog.
func recordEvent(ctx context.Context, raw json.RawMessage) {
	var e event
	if err := json.Unmarshal(raw, &e); err != nil {
		eventsRefused.Inc(ctx, refusedLabel.Value("invalid_value"))
		return
	}
	recorded, known := record(ctx, e)
	switch {
	case !known:
		eventsRefused.Inc(ctx, refusedLabel.Value("unknown_event"))
	case !recorded:
		eventsRefused.Inc(ctx, refusedLabel.Value("invalid_value"))
	}
}

// record records e when its values are in the catalog. known is false when
// its name is not an event of the catalog.
func record(ctx context.Context, e event) (recorded bool, known bool) {
	switch e.Name {
	case "artifact_load":
		seconds, ok := duration(e.DurationMs)
		if !ok || e.Cached == nil {
			return false, true
		}
		cache := "miss"
		if *e.Cached {
			cache = "hit"
		}
		artifactLoad.Observe(ctx, seconds, cacheLabel.Value(cache))
	case "game_ready":
		seconds, ok := duration(e.DurationMs)
		if !ok {
			return false, true
		}
		gameReady.Observe(ctx, seconds)
	case "connection_lost":
		count, ok := counted(e.Count)
		if !ok || !slices.Contains(closeKinds, e.Close) {
			return false, true
		}
		connectionsLost.Add(ctx, count, closeLabel.Value(e.Close))
	case "reconnect":
		count, ok := counted(e.Count)
		if !ok || !slices.Contains(reconnectKinds, e.Outcome) {
			return false, true
		}
		reconnects.Add(ctx, count, reconnectLabel.Value(e.Outcome))
	case "resync":
		count, ok := counted(e.Count)
		if !ok {
			return false, true
		}
		resyncs.Add(ctx, count)
	case "bridge_refusal":
		count, ok := counted(e.Count)
		if !ok || !slices.Contains(refusalReasons, e.Reason) {
			return false, true
		}
		bridgeRefusals.Add(ctx, count, refusalLabel.Value(e.Reason))
	case "error":
		count, ok := counted(e.Count)
		if !ok || !knownError(e.Origin, e.Kind) {
			return false, true
		}
		clientErrors.Add(ctx, count, originLabel.Value(e.Origin), kindLabel.Value(e.Kind))
	default:
		return false, false
	}
	return true, true
}

func duration(ms *int64) (float64, bool) {
	if ms == nil || *ms < 0 || *ms > maxDurationMs {
		return 0, false
	}
	return float64(*ms) / 1000, true
}

// counted is a reported count; a missing one is one.
func counted(count *int64) (float64, bool) {
	if count == nil {
		return 1, true
	}
	if *count < 1 || *count > maxCount {
		return 0, false
	}
	return float64(*count), true
}

func knownError(origin, kind string) bool {
	return slices.Contains(errorKinds[origin], kind)
}
