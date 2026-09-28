package executor_test

import (
	"context"
	"encoding/json"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/diegobermudez03/playhoot/session/internal/executor"
	pb "github.com/diegobermudez03/playhoot/session/jsexecutor/proto"
)

// stubServer is a hand-written, real gRPC service implementing the actual
// generated pb.ExecutorServer interface, so these tests prove wire
// compatibility with the real .proto contract - not merely that this
// package's own Go types behave as expected in isolation.
type stubServer struct {
	pb.UnimplementedExecutorServer
	handle func(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error)
	calls  atomic.Int32
}

func (s *stubServer) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	s.calls.Add(1)
	return s.handle(ctx, req)
}

func startStub(t *testing.T, stub *stubServer) *executor.GRPCClient {
	t.Helper()

	lis := bufconn.Listen(1024 * 1024)
	srv := grpc.NewServer()
	pb.RegisterExecutorServer(srv, stub)
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	client, err := executor.NewGRPCClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	return client
}

func testInput() executor.ExecutionInput {
	return executor.ExecutionInput{
		Script:        executor.ResolvedScript{Source: "function execute(){}"},
		PreviousState: json.RawMessage(`{}`),
		Event:         json.RawMessage(`{}`),
		Context: executor.ExecutionContext{
			LogicalTime: time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC),
			RandomSeed:  42,
			ActingActor: "actor-1",
		},
	}
}

func TestGRPCClient_SuccessPassthrough(t *testing.T) {
	stub := &stubServer{handle: func(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
		return &pb.ExecuteResponse{
			Outcome: &pb.ExecuteResponse_Success{
				Success: &pb.Success{NewState: []byte(`{"ok":true}`)},
			},
		}, nil
	}}
	client := startStub(t, stub)

	out, err := client.Execute(context.Background(), testInput())
	require.NoError(t, err)
	require.JSONEq(t, `{"ok":true}`, string(out.NewState))
	require.EqualValues(t, 1, stub.calls.Load())
}

func TestGRPCClient_RejectedPassthrough(t *testing.T) {
	stub := &stubServer{handle: func(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
		return &pb.ExecuteResponse{
			Outcome: &pb.ExecuteResponse_Rejected{Rejected: &pb.Rejected{Reason: "boom"}},
		}, nil
	}}
	client := startStub(t, stub)

	_, err := client.Execute(context.Background(), testInput())
	require.Error(t, err)
	var rejected *executor.ScriptRejectedError
	require.ErrorAs(t, err, &rejected)
	require.Equal(t, "boom", rejected.Reason)
}

func TestGRPCClient_DeadlineExceeded_DoesNotRetry(t *testing.T) {
	stub := &stubServer{handle: func(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
		<-ctx.Done()
		return nil, status.Error(codes.DeadlineExceeded, "deadline exceeded")
	}}
	client := startStub(t, stub)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := client.Execute(ctx, testInput())
	elapsed := time.Since(start)

	require.Error(t, err)
	var execErr *executor.ExecutorError
	require.ErrorAs(t, err, &execErr)
	require.Less(t, elapsed, 1*time.Second, "must not retry after a deadline-exceeded failure")
	require.EqualValues(t, 1, stub.calls.Load(), "a deadline-exceeded failure must not be retried")
}

func TestGRPCClient_Unavailable_RetriesThenSucceeds(t *testing.T) {
	stub := &stubServer{}
	stub.handle = func(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
		if stub.calls.Load() < 3 {
			return nil, status.Error(codes.Unavailable, "no healthy instance yet")
		}
		return &pb.ExecuteResponse{
			Outcome: &pb.ExecuteResponse_Success{Success: &pb.Success{NewState: []byte(`{}`)}},
		}, nil
	}
	client := startStub(t, stub)

	out, err := client.Execute(context.Background(), testInput())
	require.NoError(t, err)
	require.NotNil(t, out.NewState)
	require.EqualValues(t, 3, stub.calls.Load(), "should retry twice (3 total attempts) before succeeding")
}

func TestGRPCClient_Unavailable_ExhaustsRetries(t *testing.T) {
	stub := &stubServer{handle: func(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
		return nil, status.Error(codes.Unavailable, "still down")
	}}
	client := startStub(t, stub)

	_, err := client.Execute(context.Background(), testInput())
	require.Error(t, err)
	var execErr *executor.ExecutorError
	require.ErrorAs(t, err, &execErr)
	require.Equal(t, codes.Unavailable, status.Code(execErr.Cause))
	require.EqualValues(t, 3, stub.calls.Load(), "1 initial attempt + 2 retries, then give up")
}

func TestGRPCClient_OtherStatus_DoesNotRetry(t *testing.T) {
	stub := &stubServer{handle: func(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
		return nil, status.Error(codes.Internal, "unexpected")
	}}
	client := startStub(t, stub)

	_, err := client.Execute(context.Background(), testInput())
	require.Error(t, err)
	var execErr *executor.ExecutorError
	require.ErrorAs(t, err, &execErr)
	require.EqualValues(t, 1, stub.calls.Load(), "a non-Unavailable, non-DeadlineExceeded failure must not be retried")
}

func TestFake_ReturnsConfiguredOutcome(t *testing.T) {
	fake := &executor.Fake{
		ExecuteFunc: func(ctx context.Context, in executor.ExecutionInput) (executor.ExecutionOutput, error) {
			return executor.ExecutionOutput{NewState: json.RawMessage(`{"fake":true}`)}, nil
		},
	}

	out, err := fake.Execute(context.Background(), testInput())
	require.NoError(t, err)
	require.JSONEq(t, `{"fake":true}`, string(out.NewState))
}
