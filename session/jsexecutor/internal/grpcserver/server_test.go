package grpcserver_test

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/diegobermudez03/playhoot/session/jsexecutor/internal/grpcserver"
	"github.com/diegobermudez03/playhoot/session/jsexecutor/internal/sandbox"
	pb "github.com/diegobermudez03/playhoot/session/jsexecutor/proto"
)

// This test proves the network boundary itself (gRPC request/response,
// status-code mapping, deadline propagation) — jsexecutor/internal/sandbox
// already has its own exhaustive test suite for the sandbox mechanics
// themselves, exercised there as plain Go calls rather than over gRPC.
//
// sandbox.Execute re-execs whatever binary os.Executable() resolves to —
// this test binary, not jsexecutor's own production main — so this test
// binary must dispatch into worker mode itself, exactly as jsexecutor's
// main.go does, or every Execute call below would try to re-exec this test
// binary as an ordinary test run instead of a sandbox worker.
func TestMain(m *testing.M) {
	if sandbox.RunAsWorkerIfRequested() {
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// highConcurrencyCeiling is used by every test that isn't itself exercising
// the concurrency ceiling, so ordinary tests never spuriously hit it.
const highConcurrencyCeiling = 1000

func startTestServer(t *testing.T) pb.ExecutorClient {
	t.Helper()
	return startTestServerWithCeiling(t, highConcurrencyCeiling)
}

func startTestServerWithCeiling(t *testing.T, maxConcurrentExecutions int) pb.ExecutorClient {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	pb.RegisterExecutorServer(srv, grpcserver.New(maxConcurrentExecutions))

	go func() {
		_ = srv.Serve(lis)
	}()
	t.Cleanup(srv.Stop)

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	return pb.NewExecutorClient(conn)
}

func testContext() *pb.ExecutionContext {
	return &pb.ExecutionContext{
		LogicalTime: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano),
		RandomSeed:  "424242",
		ActingActor: "actor-1",
	}
}

const counterScript = `
function execute(previousState, event, context) {
	const counter = (previousState && previousState.counter) || 0;
	return { newState: { counter: counter + event.amount }, requestedCommands: [{ type: "noop" }] };
}
`

func TestExecute_DeterministicRoundTrip(t *testing.T) {
	client := startTestServer(t)

	req := &pb.ExecuteRequest{
		Script:        []byte(counterScript),
		PreviousState: []byte(`{"counter": 5}`),
		Event:         []byte(`{"amount": 3}`),
		Context:       testContext(),
	}

	resp1, err := client.Execute(context.Background(), req)
	require.NoError(t, err)
	resp2, err := client.Execute(context.Background(), req)
	require.NoError(t, err)

	require.NotNil(t, resp1.GetSuccess())
	require.NotNil(t, resp2.GetSuccess())
	require.JSONEq(t, string(resp1.GetSuccess().GetNewState()), string(resp2.GetSuccess().GetNewState()))

	var state struct {
		Counter int `json:"counter"`
	}
	require.NoError(t, json.Unmarshal(resp1.GetSuccess().GetNewState(), &state))
	require.Equal(t, 8, state.Counter)
	require.Len(t, resp1.GetSuccess().GetRequestedCommands(), 1)
}

func TestExecute_ScriptThrows_IsRejectedWithOKStatus(t *testing.T) {
	client := startTestServer(t)

	req := &pb.ExecuteRequest{
		Script:        []byte(`function execute() { throw new Error("boom"); }`),
		PreviousState: []byte(`{}`),
		Event:         []byte(`{}`),
		Context:       testContext(),
	}

	resp, err := client.Execute(context.Background(), req)
	require.NoError(t, err, "a business-level rejection must not be an RPC error")
	require.NotNil(t, resp.GetRejected())
	require.Contains(t, resp.GetRejected().GetReason(), "boom")
}

const capabilityProbeScript = `
function execute(previousState, event, context) {
	let escapedReadHostFile = false;
	try {
		const f = std.open("../go.mod", "r");
		if (f !== null) { escapedReadHostFile = true; f.close(); }
	} catch (e) {}
	return {
		newState: {
			escapedReadHostFile: escapedReadHostFile,
			performanceReachable: typeof performance !== "undefined",
			dateNow: Date.now()
		},
		requestedCommands: []
	};
}
`

func TestExecute_SandboxIsolationHoldsOverGRPC(t *testing.T) {
	client := startTestServer(t)

	execCtx := testContext()
	req := &pb.ExecuteRequest{
		Script:        []byte(capabilityProbeScript),
		PreviousState: []byte(`{}`),
		Event:         []byte(`{}`),
		Context:       execCtx,
	}

	resp, err := client.Execute(context.Background(), req)
	require.NoError(t, err)
	require.NotNil(t, resp.GetSuccess())

	var state struct {
		EscapedReadHostFile  bool  `json:"escapedReadHostFile"`
		PerformanceReachable bool  `json:"performanceReachable"`
		DateNow              int64 `json:"dateNow"`
	}
	require.NoError(t, json.Unmarshal(resp.GetSuccess().GetNewState(), &state))
	require.False(t, state.EscapedReadHostFile)
	require.False(t, state.PerformanceReachable)

	logicalTime, err := time.Parse(time.RFC3339Nano, execCtx.LogicalTime)
	require.NoError(t, err)
	require.Equal(t, logicalTime.UnixMilli(), state.DateNow)
}

func TestExecute_CallerDeadline_ReturnsDeadlineExceeded(t *testing.T) {
	client := startTestServer(t)

	pattern := filepath.Join(os.TempDir(), "playhoot-jsexecutor-*")
	before, err := filepath.Glob(pattern)
	require.NoError(t, err)

	req := &pb.ExecuteRequest{
		Script:        []byte(`function execute() { while (true) {} }`),
		PreviousState: []byte(`{}`),
		Event:         []byte(`{}`),
		Context:       testContext(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = client.Execute(ctx, req)
	elapsed := time.Since(start)

	require.Error(t, err)
	require.Equal(t, codes.DeadlineExceeded, status.Code(err))
	require.Less(t, elapsed, 5*time.Second, "the deadline must actually terminate the runaway worker, not merely time out client-side while it keeps running")

	// Give the killed worker's cleanup a brief moment in case of scheduling
	// jitter, then confirm nothing was left behind.
	require.Eventually(t, func() bool {
		after, globErr := filepath.Glob(pattern)
		return globErr == nil && len(after) == len(before)
	}, 2*time.Second, 50*time.Millisecond, "killing a runaway worker across the gRPC boundary must not leak its scratch directory")
}

// TestExecute_ConcurrencyCeiling_RejectsWithResourceExhausted proves the
// Server's concurrency ceiling: a call arriving while the ceiling's only
// slot is already held is rejected immediately with ResourceExhausted,
// never queued and never left to spawn an additional worker process.
func TestExecute_ConcurrencyCeiling_RejectsWithResourceExhausted(t *testing.T) {
	client := startTestServerWithCeiling(t, 1)

	holdCtx, holdCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer holdCancel()

	started := make(chan struct{})
	held := make(chan error, 1)
	go func() {
		req := &pb.ExecuteRequest{
			Script:        []byte(`function execute() { while (true) {} }`),
			PreviousState: []byte(`{}`),
			Event:         []byte(`{}`),
			Context:       testContext(),
		}
		close(started)
		_, err := client.Execute(holdCtx, req)
		held <- err
	}()
	<-started
	// The semaphore is acquired synchronously as soon as the gRPC call
	// reaches the handler, before it ever spawns the (comparatively slow)
	// sandbox worker process - so a short, fixed settle delay is enough to
	// make this call reliably win the race for the ceiling's only slot,
	// rather than racing it against req2 below (whose own probing calls, if
	// not rejected, are exactly as slow to complete as this one - a race
	// between two similarly-slow contenders that req2 can otherwise keep
	// winning by chance for several rounds).
	time.Sleep(200 * time.Millisecond)

	req2 := &pb.ExecuteRequest{
		Script:        []byte(counterScript),
		PreviousState: []byte(`{}`),
		Event:         []byte(`{"amount": 1}`),
		Context:       testContext(),
	}
	require.Eventually(t, func() bool {
		_, err := client.Execute(context.Background(), req2)
		return err != nil && status.Code(err) == codes.ResourceExhausted
	}, 2*time.Second, 20*time.Millisecond, "a second call must be rejected with ResourceExhausted while the ceiling's only slot is held by the runaway first call")

	require.NoError(t, holdCtx.Err(), "the holder must still be running (its own 3s deadline not yet reached) when rejection was observed")

	holdCancel()
	<-held
}

func TestExecute_MalformedRandomSeed_IsInvalidArgument(t *testing.T) {
	client := startTestServer(t)

	req := &pb.ExecuteRequest{
		Script:        []byte(counterScript),
		PreviousState: []byte(`{}`),
		Event:         []byte(`{"amount": 1}`),
		Context: &pb.ExecutionContext{
			LogicalTime: time.Now().Format(time.RFC3339Nano),
			RandomSeed:  "not-a-number",
			ActingActor: "actor-1",
		},
	}

	_, err := client.Execute(context.Background(), req)
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}
