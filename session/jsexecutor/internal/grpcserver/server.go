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

// Server implements pb.ExecutorServer. sem bounds how many sandbox.Execute
// calls may run concurrently, protecting the Executor's own host from
// spawning an unbounded number of worker processes under a request burst -
// a defensive ceiling on this service's own aggregate resource consumption,
// not a per-user/business rate-limiting policy (that is Session Runtime's
// own, separately owned, concern).
type Server struct {
	pb.UnimplementedExecutorServer
	sem chan struct{}
}

// New constructs a Server ready to be registered against a grpc.Server.
// maxConcurrentExecutions bounds how many Execute calls this Server allows
// in flight at once; a call arriving once that many are already running is
// rejected immediately with codes.ResourceExhausted rather than queued -
// queuing would only move the same unbounded-accumulation risk from too
// many OS processes to too many blocked callers waiting on a channel, so
// this keeps the Server itself simple (accept or reject, no queue/timeout
// state to manage) and leaves the backpressure decision (retry now, retry
// later, surface a failure) to the caller, which has the business context
// to make it.
func New(maxConcurrentExecutions int) *Server {
	return &Server{sem: make(chan struct{}, maxConcurrentExecutions)}
}

// Execute runs one authored-script execution. A business-level rejection
// (the script threw, or returned a malformed shape) is returned as response
// data with an OK status; every infrastructure-level failure (a crashed
// worker, an exceeded deadline) is returned as a non-OK gRPC status instead
// of response data, so a caller can distinguish the two the same way gRPC
// already distinguishes "your request was invalid/declined" from "the RPC
// itself failed." A call exceeding the concurrency ceiling never reaches
// the sandbox at all - it is rejected with codes.ResourceExhausted before
// any worker process would have been spawned.
func (s *Server) Execute(ctx context.Context, req *pb.ExecuteRequest) (*pb.ExecuteResponse, error) {
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	default:
		return nil, status.Error(codes.ResourceExhausted, "executor at capacity: too many concurrent executions")
	}

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

// Project computes one viewer's ClientState. Unlike Execute, req's own
// projection_input is never the full authoritative state - Session
// Runtime's own visibility-filtering step already constructed it before
// this call, so this Server has no privacy logic of its own beyond
// invoking the script's second pure entry point. Concurrency/error-handling
// shape mirrors Execute exactly, including sharing the same concurrency
// ceiling (both operations spawn the same kind of sandbox worker process).
func (s *Server) Project(ctx context.Context, req *pb.ProjectRequest) (*pb.ProjectResponse, error) {
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	default:
		return nil, status.Error(codes.ResourceExhausted, "executor at capacity: too many concurrent executions")
	}

	logicalTime, err := time.Parse(time.RFC3339Nano, req.GetContext().GetLogicalTime())
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid logical_time: %v", err)
	}
	randomSeed, err := strconv.ParseUint(req.GetContext().GetRandomSeed(), 10, 64)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "invalid random_seed: %v", err)
	}

	in := sandbox.ProjectInput{
		Script: sandbox.ResolvedScript{Source: string(req.GetScript())},
		State:  req.GetProjectionInput(),
		Viewer: req.GetViewer(),
		Context: sandbox.ExecutionContext{
			LogicalTime: logicalTime,
			RandomSeed:  randomSeed,
		},
	}

	out, err := sandbox.Project(ctx, in)
	if err != nil {
		var rejected *sandbox.ScriptRejectedError
		if errors.As(err, &rejected) {
			return &pb.ProjectResponse{
				Outcome: &pb.ProjectResponse_Rejected{
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

	return &pb.ProjectResponse{
		Outcome: &pb.ProjectResponse_Success{
			Success: &pb.ProjectSuccess{ClientState: out.ClientState},
		},
	}, nil
}
