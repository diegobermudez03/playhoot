// Package sandbox runs a game's authored JavaScript: one execution context per
// running Session, each in its own worker process (a re-exec of this binary)
// hosting a QuickJS-on-WebAssembly runtime with no host capability, so a
// runaway context can be killed alone.
package sandbox

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// ErrUnknownContext is returned for a context ID the Host does not hold.
var ErrUnknownContext = errors.New("unknown sandbox context")

// ErrNoCapacity is returned when this instance already runs as many contexts
// as its limits allow; trying again later may succeed.
var ErrNoCapacity = errors.New("sandbox has no capacity for another context")

// errBudgetExceeded means a call outlived the execution budget; the authored
// code is to blame, so it is reported as a script failure.
var errBudgetExceeded = errors.New("execution budget exceeded")

// Default instance-wide limits, used when HostLimits leaves them at zero.
const defaultMaxContexts = 32

// HostLimits bound what one instance's sandbox may use at once. Zero values
// take defaults: 32 contexts, and as many concurrent executions as CPUs.
type HostLimits struct {
	// MaxContexts bounds the live worker processes; with each context's
	// memory limit it also bounds their aggregate memory.
	MaxContexts int

	// MaxConcurrentExecutions bounds the calls running at once across all
	// contexts; further calls wait for a free slot.
	MaxConcurrentExecutions int
}

// Host owns the execution contexts of this instance. An error from its
// methods is an infrastructure failure; the authored code failing is
// reported in the results, and leaves the context to be discarded.
type Host struct {
	// workerArg is the argument workers are started with; tests replace it.
	workerArg   string
	budget      time.Duration
	maxContexts int
	// executions holds one token per call running in any context.
	executions chan struct{}

	mu       sync.Mutex
	contexts map[string]*contextProcess
	// spawning counts contexts being created, which already hold capacity.
	spawning int
}

// NewHost constructs a Host with no contexts, bounded by limits.
func NewHost(limits HostLimits) *Host {
	if limits.MaxContexts <= 0 {
		limits.MaxContexts = defaultMaxContexts
	}
	if limits.MaxConcurrentExecutions <= 0 {
		limits.MaxConcurrentExecutions = runtime.NumCPU()
	}
	return &Host{
		workerArg:   workerModeArg,
		budget:      defaultExecutionBudget,
		maxContexts: limits.MaxContexts,
		executions:  make(chan struct{}, limits.MaxConcurrentExecutions),
		contexts:    make(map[string]*contextProcess),
	}
}

// CreateContext starts a worker for spec's game and loads its script, or
// returns ErrNoCapacity. A script that fails to load is reported by the
// first execution, not here.
func (h *Host) CreateContext(ctx context.Context, spec ContextSpec) (string, error) {
	h.mu.Lock()
	if len(h.contexts)+h.spawning >= h.maxContexts {
		h.mu.Unlock()
		capacityRejections.Inc(ctx)
		return "", ErrNoCapacity
	}
	h.spawning++
	h.mu.Unlock()
	h.recordContexts(ctx)
	defer func() {
		h.mu.Lock()
		h.spawning--
		h.mu.Unlock()
		h.recordContexts(ctx)
	}()

	began := time.Now()
	process, err := h.spawn()
	if err != nil {
		return "", err
	}
	resp, err := h.call(ctx, process, workerRequest{Operation: workerOperationLoad, Script: string(spec.Script)})
	switch {
	case errors.Is(err, errBudgetExceeded):
		process.loadError = "loading script: " + err.Error()
	case err != nil:
		process.stop(ctx, stopFailed)
		return "", err
	case resp.ScriptError != nil:
		process.loadError = *resp.ScriptError
	}
	id := uuid.NewString()
	h.mu.Lock()
	h.contexts[id] = process
	h.mu.Unlock()
	contextCreationDuration.ObserveDuration(ctx, time.Since(began))
	return id, nil
}

// Start runs the game's start in contextID with input.
func (h *Host) Start(ctx context.Context, contextID string, input StartInput) (StartResult, error) {
	process, err := h.context(contextID)
	if err != nil {
		return StartResult{}, err
	}
	if process.loadError != "" {
		return StartResult{Outcome: ScriptFailed, ScriptError: process.loadError}, nil
	}
	resp, err := h.call(ctx, process, workerRequest{Operation: workerOperationStart, Participants: input.Participants, RandomSeed: input.RandomSeed})
	switch {
	case errors.Is(err, errBudgetExceeded):
		return StartResult{Outcome: ScriptFailed, ScriptError: "start(): " + err.Error()}, nil
	case err != nil:
		return StartResult{}, err
	case resp.ScriptError != nil:
		return StartResult{Outcome: ScriptFailed, ScriptError: *resp.ScriptError}, nil
	}
	return StartResult{Outcome: ScriptSucceeded, State: resp.State, PRNGState: resp.PRNGState, ContextRevision: resp.ContextRevision}, nil
}

// Execute runs batch's operations in contextID, in order, as one atomic
// step.
func (h *Host) Execute(ctx context.Context, contextID string, batch Batch) (ExecutionResult, error) {
	process, err := h.context(contextID)
	if err != nil {
		return ExecutionResult{}, err
	}
	steps := make([]workerStep, len(batch.Operations))
	for i, operation := range batch.Operations {
		steps[i] = workerStep{SessionTime: operation.SessionTimeMs, Update: operation.Update}
		for _, transition := range operation.Transitions {
			steps[i].Transitions = append(steps[i].Transitions, workerTransition{Participant: transition.ParticipantID, Inputs: transition.Inputs, SessionTime: transition.SessionTimeMs})
		}
		if command := operation.Command; command != nil {
			steps[i].Command = &workerCommand{Participant: command.ParticipantID, Name: command.Name, Payload: command.Payload, SessionTime: operation.SessionTimeMs}
		}
	}
	resp, err := h.call(ctx, process, workerRequest{Operation: workerOperationExecute, Steps: steps})
	switch {
	case errors.Is(err, errBudgetExceeded):
		return ExecutionResult{Outcome: ScriptFailed, ScriptError: "execution: " + err.Error()}, nil
	case err != nil:
		return ExecutionResult{}, err
	case resp.ScriptError != nil:
		return ExecutionResult{Outcome: ScriptFailed, ScriptError: *resp.ScriptError}, nil
	}
	result := ExecutionResult{Outcome: ScriptSucceeded, State: resp.State, PRNGState: resp.PRNGState, ContextRevision: resp.ContextRevision}
	for _, response := range resp.CommandResponses {
		result.CommandResponses = append(result.CommandResponses, response)
	}
	for _, gameError := range resp.Errors {
		result.Errors = append(result.Errors, GameError{Operation: gameError.Step, ParticipantID: gameError.Participant, Error: gameError.Error})
	}
	for _, gameEvent := range resp.Events {
		result.Events = append(result.Events, GameEvent{Operation: gameEvent.Step, Participants: gameEvent.Participants, Event: gameEvent.Event})
	}
	return result, nil
}

// Restore sets contextID at snapshot, the confirmed state of a game already
// running, instead of starting it.
func (h *Host) Restore(ctx context.Context, contextID string, snapshot Snapshot) (RestoreResult, error) {
	process, err := h.context(contextID)
	if err != nil {
		return RestoreResult{}, err
	}
	if process.loadError != "" {
		return RestoreResult{Outcome: ScriptFailed, ScriptError: process.loadError}, nil
	}
	_, err = h.call(ctx, process, workerRequest{
		Operation:       workerOperationRestore,
		State:           snapshot.State,
		PRNGState:       snapshot.PRNGState,
		ContextRevision: snapshot.ContextRevision,
		SessionTimeMs:   snapshot.SessionTimeMs,
	})
	if err != nil {
		return RestoreResult{}, err
	}
	return RestoreResult{Outcome: ScriptSucceeded}, nil
}

// Project returns participantID's view of contextID's current state.
func (h *Host) Project(ctx context.Context, contextID string, participantID string) (ProjectResult, error) {
	process, err := h.context(contextID)
	if err != nil {
		return ProjectResult{}, err
	}
	resp, err := h.call(ctx, process, workerRequest{Operation: workerOperationProject, Participant: participantID})
	switch {
	case errors.Is(err, errBudgetExceeded):
		return ProjectResult{Outcome: ScriptFailed, ScriptError: "project(): " + err.Error()}, nil
	case err != nil:
		return ProjectResult{}, err
	case resp.ScriptError != nil:
		return ProjectResult{Outcome: ScriptFailed, ScriptError: *resp.ScriptError}, nil
	}
	return ProjectResult{Outcome: ScriptSucceeded, View: resp.View}, nil
}

// DiscardContext stops contextID's worker and forgets it. Discarding a context
// the Host does not hold does nothing.
func (h *Host) DiscardContext(ctx context.Context, contextID string) error {
	h.mu.Lock()
	process, found := h.contexts[contextID]
	delete(h.contexts, contextID)
	h.mu.Unlock()
	if found {
		process.stop(ctx, stopDiscarded)
		h.recordContexts(ctx)
	}
	return nil
}

// Close stops every context's worker.
func (h *Host) Close() error {
	ctx := context.Background()
	h.mu.Lock()
	processes := h.contexts
	h.contexts = make(map[string]*contextProcess)
	h.mu.Unlock()
	for _, process := range processes {
		process.stop(ctx, stopShutdown)
	}
	h.recordContexts(ctx)
	return nil
}

// recordContexts measures how many contexts this instance holds against its
// limit, and the memory their workers last reported.
func (h *Host) recordContexts(ctx context.Context) {
	h.mu.Lock()
	active := len(h.contexts) + h.spawning
	var memory uint64
	for _, process := range h.contexts {
		memory += process.memoryBytes.Load()
	}
	h.mu.Unlock()
	contextsActive.Set(ctx, float64(active))
	contextsLimit.Set(ctx, float64(h.maxContexts))
	workersMemoryTotal.Set(ctx, float64(memory))
}

func (h *Host) context(contextID string) (*contextProcess, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	process, found := h.contexts[contextID]
	if !found {
		return nil, fmt.Errorf("%w: %s", ErrUnknownContext, contextID)
	}
	return process, nil
}

// call runs req in process once an execution slot is free; the budget only
// counts from then.
func (h *Host) call(ctx context.Context, process *contextProcess, req workerRequest) (workerResponse, error) {
	waited := time.Now()
	select {
	case h.executions <- struct{}{}:
	case <-ctx.Done():
		executionSlotAbandoned.Inc(ctx)
		return workerResponse{}, fmt.Errorf("waiting for a sandbox execution slot: %w", ctx.Err())
	}
	executionSlotWait.ObserveDuration(ctx, time.Since(waited))
	executionsInFlight.Inc(ctx)
	executionsLimit.Set(ctx, float64(cap(h.executions)))
	defer func() {
		<-h.executions
		executionsInFlight.Dec(ctx)
	}()

	began := time.Now()
	resp, err := process.call(ctx, req, h.budget)
	operation := operationLabel.Value(string(req.Operation))
	outcome := callSucceeded
	switch {
	case errors.Is(err, errBudgetExceeded):
		outcome = callBudgetExceeded
	case err != nil:
		outcome = callError
	case resp.ScriptError != nil:
		outcome = callScriptFailed
	}
	executionDuration.ObserveDuration(ctx, time.Since(began), operation, callOutcomeLabel.Value(outcome))
	if err == nil {
		process.memoryBytes.Store(resp.MemoryBytes)
		workerMemory.Observe(ctx, float64(resp.MemoryBytes))
		if outcome == callSucceeded && (req.Operation == workerOperationStart || req.Operation == workerOperationExecute || req.Operation == workerOperationProject) {
			outputSize.Observe(ctx, float64(len(resp.State)+len(resp.View)), operation)
		}
		h.recordContexts(ctx)
	}
	return resp, err
}

// spawn starts a worker process.
func (h *Host) spawn() (*contextProcess, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolving own executable path: %w", err)
	}
	cmd := exec.Command(executable, h.workerArg)
	// Defense in depth: the runtime gives authored code no environment
	// anyway, but the worker process itself inherits none of this process's
	// secrets either.
	cmd.Env = []string{}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("opening worker stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("opening worker stdout: %w", err)
	}
	process := &contextProcess{cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout), exited: make(chan struct{})}
	cmd.Stderr = &process.stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting worker: %w", err)
	}
	go process.wait()
	return process, nil
}

// contextProcess is one context's worker process. Calls are serialized: the
// worker answers one request at a time over its pipes.
type contextProcess struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr boundedBuffer
	// loadError is the script's load failure, reported by executions.
	loadError string
	// memoryBytes is the memory the worker reported after its last call.
	memoryBytes atomic.Uint64

	mu       sync.Mutex
	stopped  bool
	stopOnce sync.Once
	exited   chan struct{}
}

type callResult struct {
	line []byte
	err  error
}

// call sends req and waits for its response, killing the worker when the
// budget or ctx ends first; the context is unusable afterwards.
func (p *contextProcess) call(ctx context.Context, req workerRequest, budget time.Duration) (workerResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return workerResponse{}, errors.New("sandbox context worker is stopped")
	}
	encoded, err := json.Marshal(req)
	if err != nil {
		return workerResponse{}, fmt.Errorf("encoding worker request: %w", err)
	}
	if _, err := p.stdin.Write(append(encoded, '\n')); err != nil {
		p.stopLocked(ctx, stopCrashed)
		return workerResponse{}, p.failure("writing worker request", err)
	}

	answered := make(chan callResult, 1)
	go func() {
		line, err := p.stdout.ReadBytes('\n')
		answered <- callResult{line: line, err: err}
	}()
	timer := time.NewTimer(budget)
	defer timer.Stop()
	select {
	case result := <-answered:
		if result.err != nil {
			p.stopLocked(ctx, stopCrashed)
			return workerResponse{}, p.failure("reading worker response", result.err)
		}
		var resp workerResponse
		if err := json.Unmarshal(result.line, &resp); err != nil {
			p.stopLocked(ctx, stopCrashed)
			return workerResponse{}, p.failure("decoding worker response", err)
		}
		if resp.Fatal != nil {
			p.stopLocked(ctx, stopFailed)
			return workerResponse{}, fmt.Errorf("sandbox context worker failed: %s", *resp.Fatal)
		}
		return resp, nil
	case <-timer.C:
		p.stopLocked(ctx, stopBudgetExceeded)
		return workerResponse{}, errBudgetExceeded
	case <-ctx.Done():
		p.stopLocked(ctx, stopAbandoned)
		return workerResponse{}, fmt.Errorf("sandbox call abandoned: %w", ctx.Err())
	}
}

func (p *contextProcess) failure(what string, err error) error {
	<-p.exited
	return fmt.Errorf("%s: %w (stderr: %q)", what, err, strings.TrimSpace(p.stderr.String()))
}

func (p *contextProcess) stop(ctx context.Context, reason string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked(ctx, reason)
}

// stopLocked kills the worker and waits for it to exit; only the first stop
// is counted, with its reason.
func (p *contextProcess) stopLocked(ctx context.Context, reason string) {
	p.stopOnce.Do(func() {
		p.stopped = true
		_ = p.stdin.Close()
		_ = p.cmd.Process.Kill()
		<-p.exited
		workersStopped.Inc(ctx, stopReasonLabel.Value(reason))
	})
}

func (p *contextProcess) wait() {
	_ = p.cmd.Wait()
	close(p.exited)
}

// boundedBuffer keeps the first bytes a worker writes to stderr, for
// diagnosing its failure.
type boundedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

const maxStderrBytes = 4096

func (b *boundedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room := maxStderrBytes - b.buf.Len(); room > 0 {
		if len(data) > room {
			b.buf.Write(data[:room])
		} else {
			b.buf.Write(data)
		}
	}
	return len(data), nil
}

func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
