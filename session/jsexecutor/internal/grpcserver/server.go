// Package grpcserver implements the Executor gRPC service, translating
// between the wire contract (jsexecutor/proto) and the sandboxed execution
// boundary (jsexecutor/internal/sandbox).
package grpcserver

import (
	"context"
	"errors"
	"strconv"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/diegobermudez03/playhoot/session/jsexecutor/internal/sandbox"
	pb "github.com/diegobermudez03/playhoot/session/jsexecutor/proto"
)

// Server implements pb.ExecutorServer.
type Server struct {
	pb.UnimplementedExecutorServer
}

// New constructs a Server ready to be registered against a grpc.Server.
func New() *Server {
	return &Server{}
}

// Execute runs one authored-script execution. A business-level rejection
// (the script threw, or returned a malformed shape) is returned as response
// data with an OK status; every infrastructure-level failure (a crashed
// worker, an exceeded deadline) is returned as a non-OK gRPC status instead
// of response data, so a caller can distinguish the two the same way gRPC
// already distinguishes "your request was invalid/declined" from "the RPC
// itself failed."
func (s *Server) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	logicalTime, err := time.Parse(time.RFC3339Nano, req.GetContext().GetLogicalTime())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid logical_time: %v", err)
	}
	randomSeed, err := strconv.ParseUint(req.GetContext().GetRandomSeed(), 10, 64)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid random_seed: %v", err)
	}

	in := sandbox.ExecutionInput{
		Script:        sandbox.ResolvedScript{Source: string(req.GetScript())},
		PreviousState: req.GetPreviousState(),
		Event:         req.GetEvent(),
		Context: sandbox.ExecutionContext{
			LogicalTime: logicalTime,
			RandomSeed:  randomSeed,
			ActingActor: req.GetContext().GetActingActor(),
		},
	}

	out, err := sandbox.Execute(ctx, in)
	if err != nil {
		var rejected *sandbox.ScriptRejectedError
		if errors.As(err, &rejected) {
			// A business-level outcome, not an RPC failure: the call itself
			// succeeded, the script simply declined the input. Returned as
			// response data with a nil (OK) error, so a caller can never
			// confuse "the script rejected this" with "the Executor failed."
			return &pb.ExecuteResponse{
				Outcome: &pb.ExecuteResponse_Rejected{
					Rejected: &pb.Rejected{Reason: rejected.Reason},
				},
			}, nil
		}

		var workerErr *sandbox.WorkerExecutionError
		if errors.As(err, &workerErr) {
			if ctx.Err() == context.DeadlineExceeded {
				return nil, status.Error(codes.DeadlineExceeded, workerErr.Error())
			}
			return nil, status.Error(codes.Internal, workerErr.Error())
		}

		return nil, status.Errorf(codes.Internal, "unexpected execution error: %v", err)
	}

	commands := make([][]byte, len(out.RequestedCommands))
	for i, c := range out.RequestedCommands {
		commands[i] = c
	}

	return &pb.ExecuteResponse{
		Outcome: &pb.ExecuteResponse_Success{
			Success: &pb.Success{
				NewState:          out.NewState,
				RequestedCommands: commands,
			},
		},
	}, nil
}
