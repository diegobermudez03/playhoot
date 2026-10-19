package replayrecording

import "github.com/diegobermudez03/playhoot/monitoring/metrics"

// What happened to an offered capture, as metric label values.
const (
	captureRecorded = "recorded"
	captureDropped  = "dropped"
	captureRejected = "rejected"
)

// What happened to a sealed, non-final chunk, as metric label values.
const (
	chunkStored          = "stored"
	chunkDroppedBacklog  = "dropped_backlog"
	chunkDroppedFailures = "dropped_failures"
	chunkDroppedShutdown = "dropped_shutdown"
	chunkDroppedEncoding = "dropped_encoding"
)

// Outcomes of one storage call, as metric label values.
const (
	storeSucceeded = "stored"
	storeFailed    = "failed"
)

var (
	captureOutcomeLabel = metrics.NewLabel("outcome", captureRecorded, captureDropped, captureRejected)
	chunkOutcomeLabel   = metrics.NewLabel("outcome", chunkStored, chunkDroppedBacklog, chunkDroppedFailures, chunkDroppedShutdown, chunkDroppedEncoding)
	storeOutcomeLabel   = metrics.NewLabel("outcome", storeSucceeded, storeFailed)

	capturesTotal = metrics.NewCounter(
		"play_runtime_replay_captures_total",
		"Committed ticks offered to replay, by what the recorder did with them.",
		captureOutcomeLabel,
	)
	chunksTotal = metrics.NewCounter(
		"play_runtime_replay_chunks_total",
		"Sealed best-effort replay chunks, by whether they were stored or dropped.",
		chunkOutcomeLabel,
	)
	storeDuration = metrics.NewHistogram(
		"play_runtime_replay_chunk_store_duration_seconds",
		"How long one attempt to store a replay chunk took, by outcome.",
		metrics.LatencyBuckets(),
		storeOutcomeLabel,
	)
	chunkBytes = metrics.NewHistogram(
		"play_runtime_replay_chunk_bytes",
		"Compressed size of the replay chunks sealed.",
		[]float64{1 << 10, 16 << 10, 64 << 10, 256 << 10, 1 << 20, 4 << 20, 16 << 20},
	)
	chunkSpan = metrics.NewHistogram(
		"play_runtime_replay_chunk_session_seconds",
		"Effective Session time the replay chunks sealed cover.",
		[]float64{1, 5, 10, 20, 30, 60, 120, 300},
	)
	pendingChunks = metrics.NewGauge(
		"play_runtime_replay_pending_chunks",
		"Sealed replay chunks waiting in memory to be stored.",
	)
	finalsTotal = metrics.NewCounter(
		"play_runtime_replay_finals_total",
		"Attempts to store a finished game's final replay chunk, by outcome.",
		storeOutcomeLabel,
	)
)
