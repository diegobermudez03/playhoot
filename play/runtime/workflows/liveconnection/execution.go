package liveconnection

import (
	"context"
	"fmt"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox"
)

//go:generate mockgen -package=liveconnection -destination=execution_mock_test.go . gameSandbox

// gameSandbox runs a game's scripts in execution contexts, one per running
// Session. An error is an infrastructure failure, never the script's.
type gameSandbox interface {
	CreateContext(ctx context.Context, spec sandbox.ContextSpec) (string, error)
	Start(ctx context.Context, contextID string, input sandbox.StartInput) (sandbox.StartResult, error)
	Restore(ctx context.Context, contextID string, snapshot sandbox.Snapshot) (sandbox.RestoreResult, error)
	Execute(ctx context.Context, contextID string, batch sandbox.Batch) (sandbox.ExecutionResult, error)
	Project(ctx context.Context, contextID string, participantID string) (sandbox.ProjectResult, error)
	DiscardContext(ctx context.Context, contextID string) error
}

// errScript means the authored code failed; detail is its error.
type errScript struct{ detail string }

func (e errScript) Error() string { return "script failed: " + e.detail }

// createContext gives the game a new sandbox context for spec.
func (c *sessionController) createContext(ctx context.Context, spec sandbox.ContextSpec) error {
	contextID, err := c.deps.sandbox.CreateContext(ctx, spec)
	if err != nil {
		return fmt.Errorf("creating sandbox context: %w", err)
	}
	c.contextID = contextID
	return nil
}

// restoreContext gives the game a new sandbox context at game's confirmed
// state; none is left when it fails.
func (c *sessionController) restoreContext(ctx context.Context, game Game) error {
	if err := c.createContext(ctx, sandbox.ContextSpec{GameVersionUUID: game.GameVersionUUID, Script: game.Script}); err != nil {
		return err
	}
	restored, err := c.deps.sandbox.Restore(ctx, c.contextID, sandbox.Snapshot{
		State:           game.State,
		PRNGState:       game.PRNGState,
		ContextRevision: game.ContextRevision,
		SessionTimeMs:   game.ExecutedThroughSessionTimeMs,
	})
	if err != nil {
		c.discardContext(ctx)
		return fmt.Errorf("restoring sandbox context: %w", err)
	}
	if restored.Outcome != sandbox.ScriptSucceeded {
		c.discardContext(ctx)
		return errScript{detail: restored.ScriptError}
	}
	return nil
}

// projectAll returns every participant's view from the game's context, keyed
// by ParticipantID.
func (c *sessionController) projectAll(ctx context.Context, participantIDs []string) (map[string][]byte, error) {
	views := make(map[string][]byte, len(participantIDs))
	for _, participantID := range participantIDs {
		projection, err := c.deps.sandbox.Project(ctx, c.contextID, participantID)
		if err != nil {
			return nil, fmt.Errorf("projecting for %s: %w", participantID, err)
		}
		switch projection.Outcome {
		case sandbox.ScriptSucceeded:
			views[participantID] = projection.View
		case sandbox.ScriptFailed:
			return nil, errScript{detail: projection.ScriptError}
		default:
			return nil, fmt.Errorf("unknown project outcome %q", projection.Outcome)
		}
	}
	return views, nil
}

// discardContext releases the game's sandbox context, if any. Failing only
// leaks it in the sandbox, so it is logged, not returned.
func (c *sessionController) discardContext(ctx context.Context) {
	if c.contextID == "" {
		return
	}
	if err := c.deps.sandbox.DiscardContext(ctx, c.contextID); err != nil {
		logging.LogError(ctx, fmt.Errorf("discarding sandbox context: %w", err))
	}
	c.contextID = ""
}
