package leavesession

import (
	"context"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
)

//go:generate mockgen -package=leavesession -destination=repo_mock_test.go . repoAPI

// Input is LeaveSession's public/application-facing command. UserUUID is an
// already-authenticated caller identity (GAME-ADR-0005).
type Input struct {
	SessionUUID    string
	UserUUID       string
	IdempotencyKey string
}

// Result is LeaveSession's logical outcome, also the shape persisted as the
// idempotency record's replayable response payload.
type Result struct {
	SessionUUID string `json:"session_uuid"`
}

type UseCase struct {
	repo repoAPI
}

func New(db *gorm.DB) *UseCase {
	return &UseCase{
		repo: newRepo(db),
	}
}

// LeaveSession deactivates the caller's Participant, releasing their lobby
// slot while keeping the underlying SessionActor durable and host authority
// unaffected. See WORK-0001's Session/Actor/Participant Operation Behavior -
// LeaveSession.
func (c *UseCase) LeaveSession(ctx context.Context, input Input) (Result, error) {
	defer logging.Step(ctx, "LeaveSession").Close()
	logging.LogFields(ctx,
		logging.Field("session_uuid", input.SessionUUID),
		logging.Field("user_uuid", input.UserUUID),
	)

	return c.repo.leaveSession(ctx, leaveParams{
		SessionUUID:    input.SessionUUID,
		UserUUID:       input.UserUUID,
		IdempotencyKey: input.IdempotencyKey,
	})
}
