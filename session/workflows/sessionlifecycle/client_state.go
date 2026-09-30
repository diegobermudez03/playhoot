package sessionlifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/session"
	"github.com/diegobermudez03/playhoot/session/internal/executor"
	internalrepo "github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/repo"
	"github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/visibility"
	"gorm.io/gorm"
)

// getClientStateRepoAPI is GetClientState's own narrow persistence
// contract - every method here is a plain, unlocked read (no row lock, no
// mutation): GetClientState is a pure computation, never a state
// transition, so it never needs the FOR UPDATE serialization every LOBBY/
// RUNNING-phase mutation's own LockSessionByUUID provides.
type getClientStateRepoAPI interface {
	ResolveSessionForClientState(ctx context.Context, sessionUUID string) (*internalrepo.Session, error)
	FindActor(ctx context.Context, tx *gorm.DB, sessionID uint, userUUID string) (*internalrepo.Actor, error)
	GetRuntimeTurn(ctx context.Context, tx *gorm.DB, turnID uint) (*internalrepo.RuntimeTurn, error)
	ResolveGameVersionArtifact(ctx context.Context, definitionUUID string) (*internalrepo.GameVersionArtifact, error)
}

// GetClientState computes viewer userUUID's own current, privacy-safe
// ClientState directly from sessionUUID's persisted authoritative state -
// no replay of prior turns/events. It is a pure read: it never mutates
// Session state and never locks the Session row. It is exposed for a live
// connection layer to call as its own projection step on initial load or
// reconnect - it does not itself decide when that should happen, and grows
// no delivery/connection-handling mechanism of its own.
//
// The privacy boundary is primarily capability-based: the pinned artifact's
// own declared visibility schema drives a filtering step that constructs
// viewer's own projection input *before* the authored project() function
// ever runs, so the untrusted script never receives another viewer's
// private/server-only data at all - it is never trusted with everything
// and checked afterward. A defense-in-depth check runs after project()
// returns, as a safety net against a filtering-step bug or a misconfigured
// schema, never as the primary guarantee.
func (m *Manager) GetClientState(ctx context.Context, sessionUUID session.SessionUUID, userUUID session.UserUUID) (session.GetClientStateResult, error) {
	defer logging.Step(ctx, "SessionLifecycle.GetClientState").Close()
	logging.LogFields(ctx,
		logging.Field("session_uuid", string(sessionUUID)),
		logging.Field("user_uuid", string(userUUID)),
	)

	found, err := m.getClientStateRepo.ResolveSessionForClientState(ctx, string(sessionUUID))
	if err != nil {
		return session.GetClientStateResult{}, err
	}
	if found == nil {
		return session.GetClientStateResult{}, session.ErrSessionNotFound
	}

	// A missing current_turn_id means no RuntimeTurn has ever committed -
	// the Session is still LOBBY. Checked directly rather than via Phase,
	// since it is the exact fact GetClientState actually depends on: a
	// TERMINAL Session still has a valid current_turn_id (its own final
	// state) and a caller may legitimately want a last ClientState for it.
	if found.CurrentTurnID == nil {
		return session.GetClientStateResult{Outcome: session.GetClientStateOutcomeNotRunning}, nil
	}

	db := m.dbServicer.GetDB()

	actor, err := m.getClientStateRepo.FindActor(ctx, db, found.ID, string(userUUID))
	if err != nil {
		return session.GetClientStateResult{}, err
	}
	if actor == nil {
		return session.GetClientStateResult{Outcome: session.GetClientStateOutcomeNotAParticipant}, nil
	}

	currentTurn, err := m.getClientStateRepo.GetRuntimeTurn(ctx, db, *found.CurrentTurnID)
	if err != nil {
		return session.GetClientStateResult{}, err
	}
	if currentTurn == nil {
		monitoring.Alert(ctx, "session current_turn_id does not resolve to a runtime turn")
		return session.GetClientStateResult{}, fmt.Errorf("session %d current_turn_id %d does not resolve to a runtime turn", found.ID, *found.CurrentTurnID)
	}

	artifact, err := m.getClientStateRepo.ResolveGameVersionArtifact(ctx, found.GameDefinitionUUID)
	if err != nil {
		return session.GetClientStateResult{}, err
	}
	if artifact == nil {
		monitoring.Alert(ctx, "session pinned game version artifact is missing")
		return session.GetClientStateResult{}, session.ErrPinnedDefinitionMissing
	}

	schema, err := visibility.ParseSchema(artifact.ProjectionVisibility)
	if err != nil {
		return session.GetClientStateResult{}, fmt.Errorf("decoding projection visibility schema: %s", err)
	}

	viewer := string(actorRefForActorID(actor.ID))

	projectionInput, err := visibility.Filter(currentTurn.NewState, viewer, schema)
	if err != nil {
		return session.GetClientStateResult{}, fmt.Errorf("filtering projection input: %s", err)
	}

	out, err := m.executor.Project(ctx, executor.ProjectionInput{
		Script:  executor.ResolvedScript{Source: artifact.BackendScript},
		State:   projectionInput,
		Viewer:  viewer,
		Context: executor.ExecutionContext{LogicalTime: time.Now().UTC(), RandomSeed: drawSeed()},
	})
	if err != nil {
		var rejected *executor.ScriptRejectedError
		if errors.As(err, &rejected) {
			return session.GetClientStateResult{Outcome: session.GetClientStateOutcomeProjectionRejected}, nil
		}
		return session.GetClientStateResult{}, err
	}

	// Defense-in-depth only (see this method's own doc comment): Filter
	// already constructed projectionInput so the script never received
	// excluded data. A finding here means Filter itself, or the artifact's
	// own declared schema, has a bug - alerted, not silently swallowed,
	// but also not treated as this call's own infrastructure failure
	// (LeakCheck's own documented limitations mean a finding is a signal
	// to investigate, not certain proof).
	if findings, checkErr := visibility.LeakCheck(currentTurn.NewState, viewer, out.ClientState, schema); checkErr != nil {
		monitoring.Alert(ctx, "projection leak check itself failed to run")
	} else if len(findings) > 0 {
		monitoring.Alert(ctx, "projection leak check flagged a possible privacy boundary violation")
	}

	return session.GetClientStateResult{
		Outcome:     session.GetClientStateOutcomeComputed,
		SessionUUID: sessionUUID,
		ClientState: out.ClientState,
	}, nil
}
