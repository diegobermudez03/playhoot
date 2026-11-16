// Package accountcleanup carries the account-cleanup task Identity leaves
// when a person is deleted (flow delete-account, steps 2 to 4): each domain
// clears what it holds that names them, while the shared history they took
// part in stays readable.
package accountcleanup

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/utils/txqueue"
)

// gameDeleter is Game's deletion of an owner's games.
type gameDeleter interface {
	DeleteUserGames(ctx context.Context, ownerUUID string) (int64, error)
}

// rosterAnonymizer is Play's clearing of a person's roster name snapshots.
type rosterAnonymizer interface {
	AnonymizeUser(ctx context.Context, userUUID string) (int64, error)
}

// entryDeleter is Notifications' deletion of a person's entries and
// preferences.
type entryDeleter interface {
	DeleteUserEntries(ctx context.Context, userUUID string) (int64, error)
}

// Payload is the account-cleanup task as Identity enqueues it.
type Payload struct {
	UserUUID string `json:"user_uuid"`
}

// Cleanup is the cross-domain cleanup of a deleted person.
type Cleanup struct {
	games    gameDeleter
	rosters  rosterAnonymizer
	notifier entryDeleter
}

// New constructs a Cleanup over the domains that hold user data today. H5,
// H6 and H8 add their own steps when theirs starts to.
func New(games gameDeleter, rosters rosterAnonymizer, notifier entryDeleter) *Cleanup {
	return &Cleanup{games: games, rosters: rosters, notifier: notifier}
}

// Register handles account-cleanup tasks on Identity's queue, the one queue
// a deletion is enqueued on.
func (c *Cleanup) Register(queue *txqueue.Queue) {
	queue.HandleDelivery(txqueue.TypeAccountCleanup, func(ctx context.Context, task txqueue.Task) error {
		return c.Clean(ctx, task.Payload)
	})
}

// Clean runs the steps in order. Every step is idempotent, so a retry after
// a failure in the middle repeats the earlier ones harmlessly; a payload
// that cannot be read turns the task dead at once, which is an alert, never
// a silent loss.
func (c *Cleanup) Clean(ctx context.Context, raw json.RawMessage) error {
	var payload Payload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return txqueue.Permanent(fmt.Errorf("reading an account cleanup: %w", err))
	}
	if payload.UserUUID == "" {
		return txqueue.Permanent(fmt.Errorf("an account cleanup names no user"))
	}
	games, err := c.games.DeleteUserGames(ctx, payload.UserUUID)
	if err != nil {
		return err
	}
	rosters, err := c.rosters.AnonymizeUser(ctx, payload.UserUUID)
	if err != nil {
		return err
	}
	entries, err := c.notifier.DeleteUserEntries(ctx, payload.UserUUID)
	if err != nil {
		return err
	}
	logging.LogFields(ctx,
		logging.Field("cleanup_games_deleted", games),
		logging.Field("cleanup_rosters_cleared", rosters),
		logging.Field("cleanup_entries_deleted", entries),
	)
	return nil
}
