// Package manualladder is the administrator's hand on the restriction
// ladder: the immediate cutoff and the gradual reopening, the only way out
// of shutdown. Both are reached only through flows that spent a step-up
// proof.
package manualladder

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/budget/internal/business/controls"
	"github.com/diegobermudez03/playhoot/budget/internal/ladder"
	internalrepo "github.com/diegobermudez03/playhoot/budget/internal/repo"
	"github.com/diegobermudez03/playhoot/budget/internal/snapshot"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/utils"
)

// Service is the manual cutoff and reopening.
type Service struct {
	repo  *internalrepo.Repo
	queue ladder.Enqueuer
	now   func() time.Time
}

// New constructs a Service on db that leaves notify tasks in queue.
func New(db *gorm.DB, queue ladder.Enqueuer) *Service {
	return &Service{repo: internalrepo.New(db), queue: queue, now: time.Now}
}

// AdminCutoff shuts everything down at once with adminUUID as actor.
// Cutting off while already shut down appends nothing.
func (s *Service) AdminCutoff(ctx context.Context, adminUUID string, reason string) (budget.CutoffOutcome, error) {
	defer logging.Step(ctx, "budget.admin_cutoff").Close()
	if _, err := uuid.Parse(adminUUID); err != nil {
		return "", fmt.Errorf("invalid admin uuid: %w", err)
	}
	if reason == "" || len(reason) > controls.MaxReasonLen {
		return budget.CutoffInvalid, nil
	}
	outcome, err := locked(ctx, s, func(ctx context.Context, tx *gorm.DB, current budget.LadderState, now time.Time) (budget.CutoffOutcome, error) {
		if current.Level == budget.LevelShutdown {
			return budget.CutoffAlreadyApplied, nil
		}
		next := budget.LadderState{Level: budget.LevelShutdown, Reason: reason, Actor: adminUUID}
		if err := ladder.Append(ctx, s.repo, s.queue, tx, next, now); err != nil {
			return "", err
		}
		return budget.CutoffApplied, nil
	})
	logging.LogFields(ctx, logging.Field("budget_cutoff", string(outcome)))
	return outcome, err
}

// Reopen moves out of shutdown gradually. From shutdown it reopens into
// restricted with every category still closed (categories must be empty);
// during a reopening it opens the named categories, each one still closed.
// Anywhere else it answers ReopenNotShutDown.
func (s *Service) Reopen(ctx context.Context, adminUUID string, categories []budget.Category, reason string) (budget.ReopenOutcome, error) {
	defer logging.Step(ctx, "budget.reopen").Close()
	if _, err := uuid.Parse(adminUUID); err != nil {
		return "", fmt.Errorf("invalid admin uuid: %w", err)
	}
	if reason == "" || len(reason) > controls.MaxReasonLen {
		return budget.ReopenInvalid, nil
	}
	for _, c := range categories {
		if !budget.ValidCategory(c) {
			return budget.ReopenInvalid, nil
		}
	}
	outcome, err := locked(ctx, s, func(ctx context.Context, tx *gorm.DB, current budget.LadderState, now time.Time) (budget.ReopenOutcome, error) {
		var next budget.LadderState
		outcome := budget.ReopenOpened
		switch {
		case current.Level == budget.LevelShutdown:
			if len(categories) > 0 {
				return budget.ReopenInvalid, nil
			}
			next = budget.LadderState{Level: budget.LevelRestricted, ReopenClosed: append([]budget.Category{}, budget.Categories...)}
			outcome = budget.ReopenStarted
		case len(current.ReopenClosed) > 0:
			if len(categories) == 0 {
				return budget.ReopenInvalid, nil
			}
			closed := map[budget.Category]bool{}
			for _, c := range current.ReopenClosed {
				closed[c] = true
			}
			for _, c := range categories {
				if !closed[c] {
					return budget.ReopenAlreadyOpen, nil
				}
				delete(closed, c)
			}
			next = budget.LadderState{Level: current.Level, PausedCategories: current.PausedCategories, ReopenClosed: []budget.Category{}}
			for _, c := range budget.Categories {
				if closed[c] {
					next.ReopenClosed = append(next.ReopenClosed, c)
				}
			}
		default:
			return budget.ReopenNotShutDown, nil
		}
		next.Reason, next.Actor = reason, adminUUID
		if err := ladder.Append(ctx, s.repo, s.queue, tx, next, now); err != nil {
			return "", err
		}
		return outcome, nil
	})
	logging.LogFields(ctx, logging.Field("budget_reopen", string(outcome)))
	return outcome, err
}

// locked runs decide under the budget lock with the current ladder state.
func locked[T any](ctx context.Context, s *Service, decide func(ctx context.Context, tx *gorm.DB, current budget.LadderState, now time.Time) (T, error)) (T, error) {
	return utils.RunInDBTransaction(ctx, s.repo, func(ctx context.Context, tx *gorm.DB) (T, error) {
		var zero T
		if err := s.repo.Lock(ctx, tx); err != nil {
			return zero, err
		}
		current, _, err := snapshot.LoadLadder(ctx, s.repo, tx)
		if err != nil {
			return zero, err
		}
		return decide(ctx, tx, current, s.now().UTC())
	})
}
