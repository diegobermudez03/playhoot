package timers

import (
	"context"
	"testing"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/engine/engineservice"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// createCall/cancelCall record one fakeCaptureRepo call, for assertions.
type createCall struct {
	sessionID, createdByTurnID uint
	engineSlot                 string
	engineKey                  []byte
	delayMs                    int64
}

type cancelCall struct {
	sessionID, closedByTurnID uint
	engineSlot                string
	engineKey                 []byte
}

// fakeCaptureRepo is a narrow, hand-rolled CaptureRepo recording every call
// it receives, so Capture can be tested as a pure function without a real
// database connection - mirrors internal/interactions' own fakeCaptureRepo.
type fakeCaptureRepo struct {
	creates []createCall
	cancels []cancelCall
}

func (f *fakeCaptureRepo) CreateTimerObligation(ctx context.Context, tx *gorm.DB, sessionID uint, engineSlot string, engineKey []byte, delayMs int64, createdByTurnID uint) (uint, error) {
	f.creates = append(f.creates, createCall{sessionID, createdByTurnID, engineSlot, engineKey, delayMs})
	return uint(len(f.creates)), nil
}

func (f *fakeCaptureRepo) CancelActiveTimerObligation(ctx context.Context, tx *gorm.DB, sessionID uint, engineSlot string, engineKey []byte, closedByTurnID uint) error {
	f.cancels = append(f.cancels, cancelCall{sessionID, closedByTurnID, engineSlot, engineKey})
	return nil
}

func TestCapture(t *testing.T) {
	t.Run("schedule_timer_output_creates_an_active_obligation_with_no_key", func(t *testing.T) {
		repo := &fakeCaptureRepo{}
		outputs := []engine.Output{
			engine.ScheduleTimerOutput{Slot: "T", DelayMilliseconds: 5000},
		}

		err := Capture(context.Background(), nil, repo, 1, 10, outputs)
		require.NoError(t, err)
		require.Empty(t, repo.cancels)
		require.Len(t, repo.creates, 1)
		require.Equal(t, uint(1), repo.creates[0].sessionID)
		require.Equal(t, "T", repo.creates[0].engineSlot)
		require.Nil(t, repo.creates[0].engineKey)
		require.Equal(t, int64(5000), repo.creates[0].delayMs)
		require.Equal(t, uint(10), repo.creates[0].createdByTurnID)
	})

	t.Run("schedule_keyed_timer_output_creates_an_active_obligation_with_its_encoded_key", func(t *testing.T) {
		repo := &fakeCaptureRepo{}
		outputs := []engine.Output{
			engine.ScheduleKeyedTimerOutput{Slot: "KT", Key: engine.StringValue{Value: "P0"}, DelayMilliseconds: 1000},
		}

		err := Capture(context.Background(), nil, repo, 1, 10, outputs)
		require.NoError(t, err)
		require.Len(t, repo.creates, 1)
		require.Equal(t, "KT", repo.creates[0].engineSlot)
		require.NotNil(t, repo.creates[0].engineKey)

		decoded, err := engineservice.DecodeValue(repo.creates[0].engineKey)
		require.NoError(t, err)
		require.Equal(t, engine.StringValue{Value: "P0"}, decoded)
	})

	t.Run("two_keyed_schedules_under_the_same_slot_both_create_independent_obligations", func(t *testing.T) {
		repo := &fakeCaptureRepo{}
		outputs := []engine.Output{
			engine.ScheduleKeyedTimerOutput{Slot: "KT", Key: engine.StringValue{Value: "P0"}, DelayMilliseconds: 1000},
			engine.ScheduleKeyedTimerOutput{Slot: "KT", Key: engine.StringValue{Value: "P1"}, DelayMilliseconds: 1000},
		}

		err := Capture(context.Background(), nil, repo, 1, 10, outputs)
		require.NoError(t, err)
		require.Len(t, repo.creates, 2)
		require.NotEqual(t, repo.creates[0].engineKey, repo.creates[1].engineKey)
	})

	t.Run("cancel_timer_output_cancels_the_matching_obligation_with_no_key", func(t *testing.T) {
		repo := &fakeCaptureRepo{}
		outputs := []engine.Output{
			engine.CancelTimerOutput{Slot: "T"},
		}

		err := Capture(context.Background(), nil, repo, 1, 11, outputs)
		require.NoError(t, err)
		require.Empty(t, repo.creates)
		require.Len(t, repo.cancels, 1)
		require.Equal(t, uint(1), repo.cancels[0].sessionID)
		require.Equal(t, "T", repo.cancels[0].engineSlot)
		require.Nil(t, repo.cancels[0].engineKey)
		require.Equal(t, uint(11), repo.cancels[0].closedByTurnID)
	})

	t.Run("cancel_keyed_timer_output_cancels_the_matching_keyed_obligation", func(t *testing.T) {
		repo := &fakeCaptureRepo{}
		outputs := []engine.Output{
			engine.CancelKeyedTimerOutput{Slot: "KT", Key: engine.StringValue{Value: "P0"}},
		}

		err := Capture(context.Background(), nil, repo, 1, 11, outputs)
		require.NoError(t, err)
		require.Len(t, repo.cancels, 1)
		require.NotNil(t, repo.cancels[0].engineKey)
		decoded, err := engineservice.DecodeValue(repo.cancels[0].engineKey)
		require.NoError(t, err)
		require.Equal(t, engine.StringValue{Value: "P0"}, decoded)
	})

	t.Run("no_outputs_is_a_no_op", func(t *testing.T) {
		repo := &fakeCaptureRepo{}

		err := Capture(context.Background(), nil, repo, 1, 13, nil)
		require.NoError(t, err)
		require.Empty(t, repo.creates)
		require.Empty(t, repo.cancels)
	})

	t.Run("a_non_integer_delay_fails", func(t *testing.T) {
		repo := &fakeCaptureRepo{}
		outputs := []engine.Output{
			engine.ScheduleTimerOutput{Slot: "T", DelayMilliseconds: 500.5},
		}

		err := Capture(context.Background(), nil, repo, 1, 10, outputs)
		require.Error(t, err)
		require.Empty(t, repo.creates)
	})

	t.Run("a_negative_delay_fails", func(t *testing.T) {
		repo := &fakeCaptureRepo{}
		outputs := []engine.Output{
			engine.ScheduleTimerOutput{Slot: "T", DelayMilliseconds: -1},
		}

		err := Capture(context.Background(), nil, repo, 1, 10, outputs)
		require.Error(t, err)
		require.Empty(t, repo.creates)
	})
}
