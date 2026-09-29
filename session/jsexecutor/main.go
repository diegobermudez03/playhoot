package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"runtime"
	"strconv"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthgrpc "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/diegobermudez03/playhoot/session/jsexecutor/internal/grpcserver"
	"github.com/diegobermudez03/playhoot/session/jsexecutor/internal/sandbox"
	pb "github.com/diegobermudez03/playhoot/session/jsexecutor/proto"
)

func main() {
	// This process may be a re-exec of itself acting as a sandboxed
	// JavaScript execution worker, not the gRPC server. That dispatch must
	// happen before any other initialization, and this process must exit
	// immediately afterward.
	if sandbox.RunAsWorkerIfRequested() {
		return
	}

	port, err := readGRPCPort()
	if err != nil {
		log.Fatalf("reading configuration: %v", err)
	}
	maxConcurrentExecutions, err := readMaxConcurrentExecutions()
	if err != nil {
		log.Fatalf("reading configuration: %v", err)
	}

	lis, err := net.Listen("tcp", ":"+strconv.Itoa(port))
	if err != nil {
		log.Fatalf("listening on :%d: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterExecutorServer(grpcServer, grpcserver.New(maxConcurrentExecutions))

	healthServer := health.NewServer()
	healthgrpc.RegisterHealthServer(grpcServer, healthServer)
	healthServer.SetServingStatus("", healthgrpc.HealthCheckResponse_SERVING)

	log.Printf("jsexecutor listening on :%d", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("serving gRPC: %v", err)
	}
}

func readGRPCPort() (int, error) {
	raw := strings.TrimSpace(os.Getenv("GRPC_PORT"))
	if raw == "" {
		return 0, fmt.Errorf("GRPC_PORT is required")
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("GRPC_PORT must be a valid TCP port")
	}
	return port, nil
}

// readMaxConcurrentExecutions reads the Executor's own concurrency ceiling:
// a defensive cap on how many sandbox.Execute calls may run at once,
// protecting this service's own host from unbounded worker-process
// spawning under a request burst. MAX_CONCURRENT_EXECUTIONS is optional -
// unset or empty falls back to a conservative, documented engineering
// default (runtime.NumCPU() * 4), not a business/product-reviewed number:
// this ceiling protects the Executor's own host from aggregate overload,
// it does not enforce any per-user/business policy.
func readMaxConcurrentExecutions() (int, error) {
	raw := strings.TrimSpace(os.Getenv("MAX_CONCURRENT_EXECUTIONS"))
	if raw == "" {
		return runtime.NumCPU() * 4, nil
	}
	max, err := strconv.Atoi(raw)
	if err != nil || max < 1 {
		return 0, fmt.Errorf("MAX_CONCURRENT_EXECUTIONS must be a positive integer")
	}
	return max, nil
}
