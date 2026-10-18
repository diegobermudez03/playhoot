package controller

import "github.com/diegobermudez03/playhoot/monitoring/metrics"

var (
	controllersCreated = metrics.NewCounter(
		"play_runtime_controllers_created_total",
		"Session controllers this instance created.",
	)
	controllersRetired = metrics.NewCounter(
		"play_runtime_controllers_retired_total",
		"Session controllers replaced because their ownership epoch was stale.",
	)
)

var playersReplaced = metrics.NewCounter(
	"play_runtime_player_connections_replaced_total",
	"Player connections ended because the same user opened a newer one.",
)

// SAMPLE ingress results, as metric label values.
const (
	sampleTransition = "transition"
	sampleRefresh    = "refresh"
	sampleStale      = "stale"
	sampleInvalid    = "invalid"
	sampleDropped    = "dropped"
)

var sampleResultLabel = metrics.NewLabel("result", sampleTransition, sampleRefresh, sampleStale, sampleInvalid, sampleDropped)

var samplesReceived = metrics.NewCounter(
	"play_runtime_samples_total",
	"SAMPLEs received, by result: a new input state, a refresh of the current one, a stale or repeated sequence, invalid, or dropped because no game runs.",
	sampleResultLabel,
)

// What a view message carried, as metric label values.
const (
	viewFull    = "full"
	viewPatch   = "patch"
	viewAckOnly = "ack_only"
)

var viewKindLabel = metrics.NewLabel("kind", viewFull, viewPatch, viewAckOnly)

var viewsSent = metrics.NewCounter(
	"play_runtime_views_sent_total",
	"View messages sent to players after a boundary, by what they carried.",
	viewKindLabel,
)
