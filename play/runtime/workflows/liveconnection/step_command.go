package liveconnection

import (
	"context"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/play/runtime"
)

var (
	commandOutcomeLabel = metrics.NewLabel("outcome",
		string(runtime.CommandExecuted),
		string(runtime.CommandAlreadyProcessed),
		string(runtime.CommandInvalid),
		string(runtime.CommandNotRunning),
	)

	commandsReceived = metrics.NewCounter(
		"play_runtime_commands_total",
		"COMMANDs received, by outcome: executed, token already executed, invalid against the manifest, or no game running.",
		commandOutcomeLabel,
	)
	commandDuration = metrics.NewHistogram(
		"play_runtime_command_duration_seconds",
		"From receiving a COMMAND to answering it, by outcome.",
		metrics.LatencyBuckets(),
		commandOutcomeLabel,
	)
)

// participantCommands is one participant's COMMAND tokens: those waiting in
// the timeline, and every one executed in the Session, kept for its lifetime.
type participantCommands struct {
	pending   map[string]*pendingCommand
	processed map[string]bool
}

// newParticipantCommands starts a participant's tokens with processed, the
// ones already executed.
func newParticipantCommands(processed []string) *participantCommands {
	tokens := &participantCommands{pending: make(map[string]*pendingCommand), processed: make(map[string]bool, len(processed))}
	for _, token := range processed {
		tokens.processed[token] = true
	}
	return tokens
}

// executed moves a pending command's token to processed once it executed.
func (t *participantCommands) executed(command *pendingCommand, response []byte) {
	delete(t.pending, command.token)
	t.processed[command.token] = true
	command.resolve(runtime.CommandExecuted, response)
}

// abandonPending answers every pending command as no longer running; their
// tokens were never executed.
func (t *participantCommands) abandonPending() {
	for token, command := range t.pending {
		command.resolve(runtime.CommandNotRunning, nil)
		delete(t.pending, token)
	}
}

// pendingCommand is a COMMAND accepted into the timeline. done is closed once
// it executed or can no longer execute, after outcome and response are set.
type pendingCommand struct {
	token string
	name  string
	// payload is the full payload, validated against the manifest.
	payload []byte

	done     chan struct{}
	outcome  runtime.CommandOutcome
	response []byte
}

// resolve answers the command. The caller holds the controller's lock.
func (p *pendingCommand) resolve(outcome runtime.CommandOutcome, response []byte) {
	p.outcome = outcome
	p.response = response
	close(p.done)
}

// acceptCommand runs a player's COMMAND and waits for its answer. It is
// checked against the manifest before any JavaScript, executed once per
// token at its place in the timeline, and asks for a boundary at once.
func (c *sessionController) acceptCommand(ctx context.Context, player *connection, command runtime.Command) (runtime.CommandResult, error) {
	received := time.Now()
	result, err := c.runCommand(ctx, player, command)
	if err != nil {
		return runtime.CommandResult{}, err
	}
	outcome := commandOutcomeLabel.Value(string(result.Outcome))
	commandsReceived.Inc(ctx, outcome)
	commandDuration.ObserveDuration(ctx, time.Since(received), outcome)
	return result, nil
}

func (c *sessionController) runCommand(ctx context.Context, player *connection, command runtime.Command) (runtime.CommandResult, error) {
	answer := func(outcome runtime.CommandOutcome) runtime.CommandResult {
		return runtime.CommandResult{Token: command.Token, Outcome: outcome}
	}
	s := c.lock()
	if s.phase != phaseRunning || player.participantID == "" || s.players[player.userUUID] != player {
		// A replaced connection no longer plays; it is ending anyway.
		c.unlock()
		return answer(runtime.CommandNotRunning), nil
	}
	payload, err := s.manifest.Commands.Normalize(command.Name, command.Payload)
	if err != nil {
		c.unlock()
		return answer(runtime.CommandInvalid), nil
	}
	tokens := s.commands[player.participantID]
	if tokens.processed[command.Token] {
		c.unlock()
		return answer(runtime.CommandAlreadyProcessed), nil
	}
	pending, duplicate := tokens.pending[command.Token]
	if !duplicate {
		pending = &pendingCommand{token: command.Token, name: command.Name, payload: payload, done: make(chan struct{})}
		tokens.pending[command.Token] = pending
		s.enqueue(Operation{ParticipantID: player.participantID, Command: pending})
	}
	c.unlock()
	if !duplicate {
		c.requestFlush()
	}

	select {
	case <-pending.done:
	case <-ctx.Done():
		// It stays in the timeline; a retry of its token tells how it went.
		return runtime.CommandResult{}, fmt.Errorf("waiting for command %s: %w", command.Name, ctx.Err())
	}
	// Resolved before done was closed and never again.
	if duplicate && pending.outcome == runtime.CommandExecuted {
		// Only the original request gets the response.
		return answer(runtime.CommandAlreadyProcessed), nil
	}
	return runtime.CommandResult{Token: command.Token, Outcome: pending.outcome, Response: pending.response}, nil
}
