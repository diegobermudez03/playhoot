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

// questionAskGroupProgram builds a minimal hand-assembled engine.Program
// declaring one QuestionSlot and one AskGroupSlot on its "Main" workflow,
// for testing captureInteractions' kind resolution without a real compile.
func questionAskGroupProgram() engine.Program {
	return engine.Program{
		Workflows: map[string]engine.Workflow{
			"Main": {
				Name:          "Main",
				QuestionSlots: []engine.QuestionSlot{{Name: "Q", Question: "PickNumber"}},
				AskGroupSlots: []engine.AskGroupSlot{{Name: "AG", Question: "PickNumber"}},
			},
		},
	}
}

// createInteractionCall/closeActiveInteractionCall record one
// fakeInteractionCaptureRepo call, for assertions.
type createInteractionCall struct {
	sessionID, sessionActorID, openedByTurnID uint
	kind, engineSlot                          string
	enginePath, interactionPayload            []byte
}

type closeActiveInteractionCall struct {
	sessionID, sessionActorID, closedByTurnID uint
	enginePath                                []byte
	engineSlot                                string
}

// fakeInteractionCaptureRepo is a narrow, hand-rolled interactionCaptureRepoAPI
// recording every call it receives, so captureInteractions can be tested as
// a pure function without a real database connection.
type fakeInteractionCaptureRepo struct {
	creates []createInteractionCall
	closes  []closeActiveInteractionCall
}

func (f *fakeInteractionCaptureRepo) CreateInteraction(ctx context.Context, tx *gorm.DB, sessionID uint, sessionActorID uint, kind string, enginePath []byte, engineSlot string, interactionPayload []byte, openedByTurnID uint) (uint, error) {
	f.creates = append(f.creates, createInteractionCall{sessionID, sessionActorID, openedByTurnID, kind, engineSlot, enginePath, interactionPayload})
	return uint(len(f.creates)), nil
}

func (f *fakeInteractionCaptureRepo) CloseActiveInteraction(ctx context.Context, tx *gorm.DB, sessionID uint, enginePath []byte, engineSlot string, sessionActorID uint, closedByTurnID uint) error {
	f.closes = append(f.closes, closeActiveInteractionCall{sessionID, sessionActorID, closedByTurnID, enginePath, engineSlot})
	return nil
}

func TestCaptureInteractions(t *testing.T) {
	t.Run("open_question_output_creates_an_active_question_interaction", func(t *testing.T) {
		repo := &fakeInteractionCaptureRepo{}
		steps := []runtimeturn.StepTrace{
			{Workflow: "Main", Outputs: []engine.Output{
				engine.OpenQuestionOutput{Slot: "Q", Recipient: engine.UserID("7"), Question: "PickNumber"},
			}},
		}

		err := captureInteractions(context.Background(), nil, repo, questionAskGroupProgram(), 1, 10, steps)
		require.NoError(t, err)
		require.Empty(t, repo.closes)
		require.Len(t, repo.creates, 1)
		require.Equal(t, session.InteractionKindQuestion, repo.creates[0].kind)
		require.Equal(t, uint(1), repo.creates[0].sessionID)
		require.Equal(t, uint(7), repo.creates[0].sessionActorID)
		require.Equal(t, "Q", repo.creates[0].engineSlot)
		require.Equal(t, uint(10), repo.creates[0].openedByTurnID)
	})

	t.Run("open_question_output_on_an_ask_group_slot_creates_an_active_ask_group_interaction", func(t *testing.T) {
		repo := &fakeInteractionCaptureRepo{}
		steps := []runtimeturn.StepTrace{
			{Workflow: "Main", Outputs: []engine.Output{
				engine.OpenQuestionOutput{Slot: "AG", Recipient: engine.UserID("3"), Question: "PickNumber"},
			}},
		}

		err := captureInteractions(context.Background(), nil, repo, questionAskGroupProgram(), 1, 10, steps)
		require.NoError(t, err)
		require.Len(t, repo.creates, 1)
		require.Equal(t, session.InteractionKindAskGroup, repo.creates[0].kind)
	})

	t.Run("close_question_output_closes_the_matching_active_interaction", func(t *testing.T) {
		repo := &fakeInteractionCaptureRepo{}
		steps := []runtimeturn.StepTrace{
			{Workflow: "Main", Outputs: []engine.Output{
				engine.CloseQuestionOutput{Slot: "Q", Recipient: engine.UserID("7")},
			}},
		}

		err := captureInteractions(context.Background(), nil, repo, questionAskGroupProgram(), 1, 11, steps)
		require.NoError(t, err)
		require.Empty(t, repo.creates)
		require.Len(t, repo.closes, 1)
		require.Equal(t, uint(1), repo.closes[0].sessionID)
		require.Equal(t, uint(7), repo.closes[0].sessionActorID)
		require.Equal(t, "Q", repo.closes[0].engineSlot)
		require.Equal(t, uint(11), repo.closes[0].closedByTurnID)
	})

	t.Run("open_then_close_in_one_step_use_the_same_engine_path", func(t *testing.T) {
		repo := &fakeInteractionCaptureRepo{}
		steps := []runtimeturn.StepTrace{
			{Workflow: "Main", Path: []engine.PathStep{{Slot: "Opponent"}}, Outputs: []engine.Output{
				engine.OpenQuestionOutput{Slot: "Q", Recipient: engine.UserID("1"), Question: "PickNumber"},
				engine.CloseQuestionOutput{Slot: "Q", Recipient: engine.UserID("2")},
			}},
		}

		err := captureInteractions(context.Background(), nil, repo, questionAskGroupProgram(), 1, 12, steps)
		require.NoError(t, err)
		require.Len(t, repo.creates, 1)
		require.Len(t, repo.closes, 1)
		require.Equal(t, repo.creates[0].enginePath, repo.closes[0].enginePath, "both outputs came from the same Step, addressing the same instance")
		require.NotEqual(t, string(repo.creates[0].enginePath), "[]", "a non-root Path must not encode as the root's empty array")
	})

	t.Run("steps_with_no_outputs_are_skipped", func(t *testing.T) {
		repo := &fakeInteractionCaptureRepo{}
		steps := []runtimeturn.StepTrace{{Workflow: "Main"}}

		err := captureInteractions(context.Background(), nil, repo, questionAskGroupProgram(), 1, 13, steps)
		require.NoError(t, err)
		require.Empty(t, repo.creates)
		require.Empty(t, repo.closes)
	})

	t.Run("output_on_an_undeclared_slot_fails", func(t *testing.T) {
		repo := &fakeInteractionCaptureRepo{}
		steps := []runtimeturn.StepTrace{
			{Workflow: "Main", Outputs: []engine.Output{
				engine.OpenQuestionOutput{Slot: "Missing", Recipient: engine.UserID("7"), Question: "PickNumber"},
			}},
		}

		err := captureInteractions(context.Background(), nil, repo, questionAskGroupProgram(), 1, 10, steps)
		require.Error(t, err)
		require.Empty(t, repo.creates)
	})

	t.Run("a_non_numeric_recipient_fails", func(t *testing.T) {
		repo := &fakeInteractionCaptureRepo{}
		steps := []runtimeturn.StepTrace{
			{Workflow: "Main", Outputs: []engine.Output{
				engine.OpenQuestionOutput{Slot: "Q", Recipient: engine.UserID("not-a-number"), Question: "PickNumber"},
			}},
		}

		err := captureInteractions(context.Background(), nil, repo, questionAskGroupProgram(), 1, 10, steps)
		require.Error(t, err)
		require.Empty(t, repo.creates)
	})
}
