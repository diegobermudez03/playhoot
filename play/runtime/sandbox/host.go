// Package sandbox runs a game's authored JavaScript: one execution context per
// running Session, each in its own worker process (a re-exec of this binary)
// hosting a QuickJS-on-WebAssembly runtime, so authored code cannot read the
// host's files, network, environment or memory, and a runaway context can be
// killed alone.
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
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ErrUnknownContext is returned for a context ID the Host does not hold.
var ErrUnknownContext = errors.New("unknown sandbox context")

// errBudgetExceeded means a call outlived the execution budget; the authored
// code is to blame, so it is reported as a script failure.
var errBudgetExceeded = errors.New("execution budget exceeded")

// Host owns the execution contexts of this instance. An error from its
// methods is an infrastructure failure; the authored code failing is
// reported in the results, and leaves the context to be discarded.
type Host struct {
	// workerArg is the argument workers are started with; tests replace it.
	workerArg string
	budget    time.Duration

	mu       sync.Mutex
	contexts map[string]*contextProcess
}

// NewHost constructs a Host with no contexts.
func NewHost() *Host {
	return &Host{workerArg: workerModeArg, budget: defaultExecutionBudget, contexts: make(map[string]*contextProcess)}
}

// CreateContext starts a worker for spec's game and loads its script. A
// script that fails to load is reported by the first execution, not here.
func (h *Host) CreateContext(ctx context.Context, spec ContextSpec) (string, error) {
	process, err := h.spawn()
	if err != nil {
		return "", err
	}
	resp, err := process.call(ctx, workerRequest{Operation: workerOperationLoad, Script: string(spec.Script), ScratchDir: process.scratchDir}, h.budget)
	switch {
	case errors.Is(err, errBudgetExceeded):
		process.loadError = "loading script: " + err.Error()
	case err != nil:
		return "", err
	case resp.ScriptError != nil:
		process.loadError = *resp.ScriptError
	}
	id := uuid.NewString()
	h.mu.Lock()
	h.contexts[id] = process
	h.mu.Unlock()
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
	resp, err := process.call(ctx, workerRequest{Operation: workerOperationStart, Participants: input.Participants, RandomSeed: input.RandomSeed}, h.budget)
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
	_, err = process.call(ctx, workerRequest{
		Operation:       workerOperationRestore,
		State:           snapshot.State,
		PRNGState:       snapshot.PRNGState,
		ContextRevision: snapshot.ContextRevision,
	}, h.budget)
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
	resp, err := process.call(ctx, workerRequest{Operation: workerOperationProject, Participant: participantID}, h.budget)
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
func (h *Host) DiscardContext(_ context.Context, contextID string) error {
	h.mu.Lock()
	process, found := h.contexts[contextID]
	delete(h.contexts, contextID)
	h.mu.Unlock()
	if found {
		process.stop()
	}
	return nil
}

// Close stops every context's worker.
func (h *Host) Close() error {
	h.mu.Lock()
	processes := h.contexts
	h.contexts = make(map[string]*contextProcess)
	h.mu.Unlock()
	for _, process := range processes {
		process.stop()
	}
	return nil
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

// spawn starts a worker process with its own scratch directory.
func (h *Host) spawn() (*contextProcess, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolving own executable path: %w", err)
	}
	scratchDir, err := os.MkdirTemp("", "playhoot-jsexecutor-*")
	if err != nil {
		return nil, fmt.Errorf("creating sandbox scratch directory: %w", err)
	}
	cmd := exec.Command(executable, h.workerArg)
	// A child inherits the parent's environment by default, which would hand
	// authored code every secret this process holds.
	cmd.Env = []string{}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = os.RemoveAll(scratchDir)
		return nil, fmt.Errorf("opening worker stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = os.RemoveAll(scratchDir)
		return nil, fmt.Errorf("opening worker stdout: %w", err)
	}
	process := &contextProcess{cmd: cmd, stdin: stdin, stdout: bufio.NewReader(stdout), scratchDir: scratchDir, exited: make(chan struct{})}
	cmd.Stderr = &process.stderr
	if err := cmd.Start(); err != nil {
		_ = os.RemoveAll(scratchDir)
		return nil, fmt.Errorf("starting worker: %w", err)
	}
	go process.wait()
	return process, nil
}

// contextProcess is one context's worker process. Calls are serialized: the
// worker answers one request at a time over its pipes.
type contextProcess struct {
	cmd        *exec.Cmd
	stdin      io.WriteCloser
	stdout     *bufio.Reader
	stderr     boundedBuffer
	scratchDir string
	// loadError is the script's load failure, reported by executions.
	loadError string

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
		p.stopLocked()
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
			p.stopLocked()
			return workerResponse{}, p.failure("reading worker response", result.err)
		}
		var resp workerResponse
		if err := json.Unmarshal(result.line, &resp); err != nil {
			p.stopLocked()
			return workerResponse{}, p.failure("decoding worker response", err)
		}
		if resp.Fatal != nil {
			p.stopLocked()
			return workerResponse{}, fmt.Errorf("sandbox context worker failed: %s", *resp.Fatal)
		}
		return resp, nil
	case <-timer.C:
		p.stopLocked()
		return workerResponse{}, errBudgetExceeded
	case <-ctx.Done():
		p.stopLocked()
		return workerResponse{}, fmt.Errorf("sandbox call abandoned: %w", ctx.Err())
	}
}

func (p *contextProcess) failure(what string, err error) error {
	<-p.exited
	return fmt.Errorf("%s: %w (stderr: %q)", what, err, strings.TrimSpace(p.stderr.String()))
}

func (p *contextProcess) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopLocked()
}

// stopLocked kills the worker and waits for it to exit, so its scratch
// directory is removed with nothing still using it.
func (p *contextProcess) stopLocked() {
	p.stopOnce.Do(func() {
		p.stopped = true
		_ = p.stdin.Close()
		_ = p.cmd.Process.Kill()
		<-p.exited
		_ = os.RemoveAll(p.scratchDir)
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
