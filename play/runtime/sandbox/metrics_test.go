package sandbox

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/logging"
)

type measurement struct {
	Metric string            `json:"metric"`
	Value  float64           `json:"value"`
	Labels map[string]string `json:"labels"`
}

// measurements returns what ctx's request log recorded, by metric name.
func measurements(t *testing.T, ctx context.Context) map[string][]measurement {
	t.Helper()
	var output bytes.Buffer
	logging.FinishRequestLog(ctx, slog.New(slog.NewJSONHandler(&output, nil)), "test")
	var entry struct {
		Metrics []measurement `json:"metrics"`
	}
	require.NoError(t, json.Unmarshal(output.Bytes(), &entry))
	byName := make(map[string][]measurement)
	for _, m := range entry.Metrics {
		byName[m.Metric] = append(byName[m.Metric], m)
	}
	return byName
}

func TestHostMetrics(t *testing.T) {
	ctx := logging.Start(context.Background())
	host := NewHost(HostLimits{MaxContexts: 1, MaxConcurrentExecutions: 2})
	t.Cleanup(func() { _ = host.Close() })

	runaway, err := host.CreateContext(ctx, ContextSpec{Script: []byte(`function start() { while (true) {} }`)})
	require.NoError(t, err)
	// Set once loaded: loading compiles the runtime, which takes longer.
	host.budget = 300 * time.Millisecond
	_, err = host.CreateContext(ctx, ContextSpec{Script: []byte(`function start() { return { state: {} }; }`)})
	require.ErrorIs(t, err, ErrNoCapacity)
	result, err := host.Start(ctx, runaway, StartInput{Participants: []string{"P1"}})
	require.NoError(t, err)
	require.Equal(t, ScriptFailed, result.Outcome)
	require.NoError(t, host.DiscardContext(ctx, runaway))

	recorded := measurements(t, ctx)
	require.Len(t, recorded["play_runtime_sandbox_capacity_rejections_total"], 1)
	require.Len(t, recorded["play_runtime_sandbox_context_creation_duration_seconds"], 1)
	require.Contains(t, recorded["play_runtime_sandbox_contexts_limit"], measurement{Metric: "play_runtime_sandbox_contexts_limit", Value: 1, Labels: map[string]string{}})
	active := recorded["play_runtime_sandbox_contexts_active"]
	require.Equal(t, float64(0), active[len(active)-1].Value, "the discarded context no longer counts")
	require.Contains(t, recorded["play_runtime_sandbox_executions_limit"], measurement{Metric: "play_runtime_sandbox_executions_limit", Value: 2, Labels: map[string]string{}})
	durations := map[string]bool{}
	for _, m := range recorded["play_runtime_sandbox_execution_duration_seconds"] {
		durations[m.Labels["operation"]+"/"+m.Labels["outcome"]] = true
	}
	require.Equal(t, map[string]bool{"load/succeeded": true, "start/budget_exceeded": true}, durations)
	require.Equal(t, []measurement{{Metric: "play_runtime_sandbox_workers_stopped_total", Value: 1, Labels: map[string]string{"reason": "budget_exceeded"}}},
		recorded["play_runtime_sandbox_workers_stopped_total"], "a worker is counted once, by the reason it first stopped")
	require.NotEmpty(t, recorded["play_runtime_sandbox_worker_memory_bytes"])
	require.Greater(t, recorded["play_runtime_sandbox_worker_memory_bytes"][0].Value, float64(0), "workers report their memory")
}
