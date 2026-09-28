package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/diegobermudez03/playhoot/game/session/jsexecutor/proto"
)

// unavailableRetries bounds how many additional attempts follow a
// connection-level (Unavailable) failure. Execution is a pure function with
// no side effects, so a request that never reached a worker is always safe
// to retry; no other failure is retried by this client.
const unavailableRetries = 2

const retryBackoff = 50 * time.Millisecond

// GRPCClient is the production Executor implementation, reaching a
// separately deployed Executor service over gRPC.
type GRPCClient struct {
	conn   *grpc.ClientConn
	client pb.ExecutorClient
}

// NewGRPCClient dials target and returns a client ready for concurrent use.
// Callers choose their own transport credentials/dial options; this
// constructor does not default to an insecure connection.
func NewGRPCClient(target string, opts ...grpc.DialOption) (*GRPCClient, error) {
	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, fmt.Errorf("executor: dialing %q: %w", target, err)
	}
	return &GRPCClient{conn: conn, client: pb.NewExecutorClient(conn)}, nil
}

// Close releases the underlying connection.
func (c *GRPCClient) Close() error {
	return c.conn.Close()
}

func (c *GRPCClient) Execute(ctx context.Context, in ExecutionInput) (ExecutionOutput, error) {
	req := toProtoRequest(in)

	var lastErr error
	for attempt := 0; attempt <= unavailableRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ExecutionOutput{}, &ExecutorError{Reason: "context done before retry", Cause: ctx.Err()}
			case <-time.After(retryBackoff):
			}
		}

		resp, err := c.client.Execute(ctx, req)
		if err == nil {
			return fromProtoResponse(resp)
		}

		if status.Code(err) != codes.Unavailable {
			return ExecutionOutput{}, &ExecutorError{Reason: "executor request failed", Cause: err}
		}
		lastErr = err
	}

	return ExecutionOutput{}, &ExecutorError{Reason: "executor unavailable after retries", Cause: lastErr}
}

func toProtoRequest(in ExecutionInput) *pb.ExecuteRequest {
	return &pb.ExecuteRequest{
		Script:        []byte(in.Script.Source),
		PreviousState: in.PreviousState,
		Event:         in.Event,
		Context: &pb.ExecutionContext{
			LogicalTime: in.Context.LogicalTime.UTC().Format(time.RFC3339Nano),
			RandomSeed:  strconv.FormatUint(in.Context.RandomSeed, 10),
			ActingActor: in.Context.ActingActor,
		},
	}
}

func fromProtoResponse(resp *pb.ExecuteResponse) (ExecutionOutput, error) {
	if rejected := resp.GetRejected(); rejected != nil {
		return ExecutionOutput{}, &ScriptRejectedError{Reason: rejected.GetReason()}
	}

	success := resp.GetSuccess()
	if success == nil {
		return ExecutionOutput{}, &ExecutorError{Reason: "executor returned neither a success nor a rejected outcome"}
	}

	rawCommands := success.GetRequestedCommands()
	commands := make([]json.RawMessage, len(rawCommands))
	for i, c := range rawCommands {
		commands[i] = c
	}

	return ExecutionOutput{
		NewState:          success.GetNewState(),
		RequestedCommands: commands,
	}, nil
}
