package sandbox

import "github.com/diegobermudez03/playhoot/monitoring/metrics"

// Operations a context call can be, as metric label values.
var operationLabel = metrics.NewLabel("operation",
	string(workerOperationLoad),
	string(workerOperationStart),
	string(workerOperationExecute),
	string(workerOperationRestore),
	string(workerOperationProject),
)

// Call outcomes, as metric label values.
const (
	callSucceeded      = "succeeded"
	callScriptFailed   = "script_failed"
	callBudgetExceeded = "budget_exceeded"
	callError          = "error"
)

var callOutcomeLabel = metrics.NewLabel("outcome", callSucceeded, callScriptFailed, callBudgetExceeded, callError)

// Why a worker process stopped, as metric label values.
const (
	stopDiscarded      = "discarded"
	stopShutdown       = "shutdown"
	stopBudgetExceeded = "budget_exceeded"
	stopAbandoned      = "abandoned"
	stopCrashed        = "crashed"
	stopFailed         = "failed"
)

var stopReasonLabel = metrics.NewLabel("reason", stopDiscarded, stopShutdown, stopBudgetExceeded, stopAbandoned, stopCrashed, stopFailed)

// byteBuckets bound the sizes of states, views and worker memory.
var byteBuckets = []float64{1 << 10, 4 << 10, 16 << 10, 64 << 10, 256 << 10, 1 << 20, 16 << 20, 64 << 20, 160 << 20, 512 << 20}

var (
	contextsActive = metrics.NewGauge(
		"play_runtime_sandbox_contexts_active",
		"Sandbox contexts (worker processes) this instance holds, being created included.",
	)
	contextsLimit = metrics.NewGauge(
		"play_runtime_sandbox_contexts_limit",
		"Most sandbox contexts this instance may hold.",
	)
	capacityRejections = metrics.NewCounter(
		"play_runtime_sandbox_capacity_rejections_total",
		"Sandbox contexts refused because the instance held as many as its limit allows.",
	)
	contextCreationDuration = metrics.NewHistogram(
		"play_runtime_sandbox_context_creation_duration_seconds",
		"How long starting a worker and loading its script took.",
		metrics.LatencyBuckets(),
	)
	workersStopped = metrics.NewCounter(
		"play_runtime_sandbox_workers_stopped_total",
		"Sandbox worker processes stopped, by reason.",
		stopReasonLabel,
	)

	executionsInFlight = metrics.NewGauge(
		"play_runtime_sandbox_executions_in_flight",
		"Sandbox calls running at once across all contexts.",
	)
	executionsLimit = metrics.NewGauge(
		"play_runtime_sandbox_executions_limit",
		"Most sandbox calls this instance runs at once.",
	)
	executionSlotWait = metrics.NewHistogram(
		"play_runtime_sandbox_execution_slot_wait_seconds",
		"How long a sandbox call waited for a free execution slot.",
		metrics.LatencyBuckets(),
	)
	executionSlotAbandoned = metrics.NewCounter(
		"play_runtime_sandbox_execution_slot_abandoned_total",
		"Sandbox calls whose caller gave up while waiting for an execution slot.",
	)
	executionDuration = metrics.NewHistogram(
		"play_runtime_sandbox_execution_duration_seconds",
		"How long a sandbox call ran once it had a slot, by operation and outcome.",
		metrics.LatencyBuckets(),
		operationLabel,
		callOutcomeLabel,
	)

	outputSize = metrics.NewHistogram(
		"play_runtime_sandbox_output_bytes",
		"Size of the state or view a successful sandbox call returned, by operation.",
		byteBuckets,
		operationLabel,
	)
	workerMemory = metrics.NewHistogram(
		"play_runtime_sandbox_worker_memory_bytes",
		"Memory a worker process reported holding after a call.",
		byteBuckets,
	)
	workersMemoryTotal = metrics.NewGauge(
		"play_runtime_sandbox_workers_memory_total_bytes",
		"Memory all of this instance's workers last reported holding, together.",
	)
)
