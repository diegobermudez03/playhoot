package sessionlifecycle

import (
	"context"
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/game/session/workflows/sessionlifecycle/internal/runtimeturn"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// createInteractionCall/closeActiveInteractionCall record one
// fakeInteractionCaptureRepo call, for assertions.
type createInteractionCall struct {
	sessionID, sessionActorID, openedByTurnID uint
	kind                                      string
	engineInteractionID                       uint64
	interactionPayload                        []byte
}

type closeActiveInteractionCall struct {
	sessionID, sessionActorID, closedByTurnID uint
	engineInteractionID                       uint64
}

// fakeInteractionCaptureRepo is a narrow, hand-rolled interactionCaptureRepoAPI
// recording every call it receives, so captureInteractions can be tested as
// a pure function without a real database connection.
type fakeInteractionCaptureRepo struct {
	creates []createInteractionCall
	closes  []closeActiveInteractionCall
}

func (f *fakeInteractionCaptureRepo) CreateInteraction(ctx context.Context, tx *gorm.DB, sessionID uint, sessionActorID uint, kind string, engineInteractionID uint64, interactionPayload []byte, openedByTurnID uint) (uint, error) {
	f.creates = append(f.creates, createInteractionCall{sessionID, sessionActorID, openedByTurnID, kind, engineInteractionID, interactionPayload})
	return uint(len(f.creates)), nil
}

func (f *fakeInteractionCaptureRepo) CloseActiveInteraction(ctx context.Context, tx *gorm.DB, sessionID uint, engineInteractionID uint64, sessionActorID uint, closedByTurnID uint) error {
	f.closes = append(f.closes, closeActiveInteractionCall{sessionID, sessionActorID, closedByTurnID, engineInteractionID})
	return nil
}

func TestCaptureInteractions(t *testing.T) {
	t.Run("open_question_output_creates_an_active_question_interaction", func(t *testing.T) {
		repo := &fakeInteractionCaptureRepo{}
		steps := []runtimeturn.StepTrace{
			{Outputs: []engine.Output{
				engine.OpenQuestionOutput{Slot: "Q", Recipient: engine.UserID("7"), Question: "PickNumber", InteractionID: 42, Kind: engine.InteractionKindQuestion},
			}},
		}

		err := captureInteractions(context.Background(), nil, repo, 1, 10, steps)
		require.NoError(t, err)
		require.Empty(t, repo.closes)
		require.Len(t, repo.creates, 1)
		require.Equal(t, session.InteractionKindQuestion, repo.creates[0].kind)
		require.Equal(t, uint(1), repo.creates[0].sessionID)
		require.Equal(t, uint(7), repo.creates[0].sessionActorID)
		require.Equal(t, uint64(42), repo.creates[0].engineInteractionID)
		require.Equal(t, uint(10), repo.creates[0].openedByTurnID)
	})

	t.Run("open_question_output_with_ask_group_kind_creates_an_active_ask_group_interaction", func(t *testing.T) {
		repo := &fakeInteractionCaptureRepo{}
		steps := []runtimeturn.StepTrace{
			{Outputs: []engine.Output{
				engine.OpenQuestionOutput{Slot: "AG", Recipient: engine.UserID("3"), Question: "PickNumber", InteractionID: 43, Kind: engine.InteractionKindAskGroup},
			}},
		}

		err := captureInteractions(context.Background(), nil, repo, 1, 10, steps)
		require.NoError(t, err)
		require.Len(t, repo.creates, 1)
		require.Equal(t, session.InteractionKindAskGroup, repo.creates[0].kind)
	})

	t.Run("close_question_output_closes_the_matching_active_interaction", func(t *testing.T) {
		repo := &fakeInteractionCaptureRepo{}
		steps := []runtimeturn.StepTrace{
			{Outputs: []engine.Output{
				engine.CloseQuestionOutput{Slot: "Q", Recipient: engine.UserID("7"), InteractionID: 42},
			}},
		}

		err := captureInteractions(context.Background(), nil, repo, 1, 11, steps)
		require.NoError(t, err)
		require.Empty(t, repo.creates)
		require.Len(t, repo.closes, 1)
		require.Equal(t, uint(1), repo.closes[0].sessionID)
		require.Equal(t, uint(7), repo.closes[0].sessionActorID)
		require.Equal(t, uint64(42), repo.closes[0].engineInteractionID)
		require.Equal(t, uint(11), repo.closes[0].closedByTurnID)
	})

	t.Run("open_then_close_in_one_step_use_the_same_interaction_id", func(t *testing.T) {
		repo := &fakeInteractionCaptureRepo{}
		steps := []runtimeturn.StepTrace{
			{Outputs: []engine.Output{
				engine.OpenQuestionOutput{Slot: "Q", Recipient: engine.UserID("1"), Question: "PickNumber", InteractionID: 99, Kind: engine.InteractionKindQuestion},
				engine.CloseQuestionOutput{Slot: "Q", Recipient: engine.UserID("2"), InteractionID: 99},
			}},
		}

		err := captureInteractions(context.Background(), nil, repo, 1, 12, steps)
		require.NoError(t, err)
		require.Len(t, repo.creates, 1)
		require.Len(t, repo.closes, 1)
		require.Equal(t, repo.creates[0].engineInteractionID, repo.closes[0].engineInteractionID, "both outputs came from the same Step, addressing the same occurrence")
	})

	t.Run("steps_with_no_outputs_are_skipped", func(t *testing.T) {
		repo := &fakeInteractionCaptureRepo{}
		steps := []runtimeturn.StepTrace{{}}

		err := captureInteractions(context.Background(), nil, repo, 1, 13, steps)
		require.NoError(t, err)
		require.Empty(t, repo.creates)
		require.Empty(t, repo.closes)
	})

	t.Run("an_unknown_interaction_kind_fails", func(t *testing.T) {
		repo := &fakeInteractionCaptureRepo{}
		steps := []runtimeturn.StepTrace{
			{Outputs: []engine.Output{
				engine.OpenQuestionOutput{Slot: "Q", Recipient: engine.UserID("7"), Question: "PickNumber", InteractionID: 1, Kind: engine.InteractionKind(99)},
			}},
		}

		err := captureInteractions(context.Background(), nil, repo, 1, 10, steps)
		require.Error(t, err)
		require.Empty(t, repo.creates)
	})

	t.Run("a_non_numeric_recipient_fails", func(t *testing.T) {
		repo := &fakeInteractionCaptureRepo{}
		steps := []runtimeturn.StepTrace{
			{Outputs: []engine.Output{
				engine.OpenQuestionOutput{Slot: "Q", Recipient: engine.UserID("not-a-number"), Question: "PickNumber", InteractionID: 1, Kind: engine.InteractionKindQuestion},
			}},
		}

		err := captureInteractions(context.Background(), nil, repo, 1, 10, steps)
		require.Error(t, err)
		require.Empty(t, repo.creates)
	})
}
