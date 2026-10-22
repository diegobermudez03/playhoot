package liveconnection

import "github.com/diegobermudez03/playhoot/monitoring/metrics"

var (
	controllersCreated = metrics.NewCounter(
		"play_runtime_controllers_created_total",
		"Session controllers this instance created.",
	)
	lanesActive = metrics.NewGauge(
		"play_runtime_game_lanes_active",
		"Running games whose lane this instance runs.",
	)
	controllersRetired = metrics.NewCounter(
		"play_runtime_controllers_retired_total",
		"Session controllers replaced because their ownership epoch was stale.",
	)
	controllersActive = metrics.NewGauge(
		"play_runtime_controllers_active",
		"Session controllers alive on this instance; one ends with its Session or when the instance shuts down.",
	)
)

// Limits a live Session's requests can exceed, as metric label values.
const (
	limitSampleRate          = "sample_rate"
	limitTimelineParticipant = "timeline_participant"
	limitTimelineSession     = "timeline_session"
	limitCommandPending      = "command_pending"
	limitCommandParticipant  = "command_participant_rate"
	limitCommandSession      = "command_session_rate"
	limitAdminConnections    = "admin_connections"
	limitLiveSessions        = "live_sessions"
	// limitViewsCollapsed is no refusal but is audited like one: a
	// connection taking views too slowly lost them to one FULL view.
	limitViewsCollapsed = "views_collapsed"
	// runtimeLimitPrefix names the Runtime's limits in the audit records.
	runtimeLimitPrefix = "runtime."
)

var limitLabel = metrics.NewLabel("limit",
	limitSampleRate, limitTimelineParticipant, limitTimelineSession,
	limitCommandPending, limitCommandParticipant, limitCommandSession,
	limitAdminConnections, limitLiveSessions,
)

var (
	limitRejections = metrics.NewCounter(
		"play_runtime_limit_rejections_total",
		"Requests a live Session refused for exceeding a platform limit, by limit.",
		limitLabel,
	)
	boundariesDeferred = metrics.NewCounter(
		"play_runtime_boundaries_deferred_total",
		"Boundaries asked for (by COMMANDs, leaves, rejoins) beyond the Session's boundary rate, left to the next tick.",
	)
	boundariesSplit = metrics.NewCounter(
		"play_runtime_boundaries_split_total",
		"Boundaries that took only part of the timeline because it held more operations than one boundary executes.",
	)
	viewsCollapsed = metrics.NewCounter(
		"play_runtime_views_collapsed_total",
		"Times a connection left so many views untaken that they were replaced by one FULL view.",
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
	sampleLimited    = "rate_limited"
)

var sampleResultLabel = metrics.NewLabel("result", sampleTransition, sampleRefresh, sampleStale, sampleInvalid, sampleDropped, sampleLimited)

var samplesReceived = metrics.NewCounter(
	"play_runtime_samples_total",
	"SAMPLEs received, by result: a new input state, a refresh of the current one, a stale or repeated sequence, invalid, dropped because no game runs or a newer connection replaced the sender, or refused by a limit.",
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
