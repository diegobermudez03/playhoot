package timers

import (
	"context"
	"errors"
	"testing"

	"github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/platform"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// fakeRepoCall records one repoAPI invocation, in call order, so a test can
// assert both which calls happened and in what order (the cancel-before-
// create replace sequence specifically depends on ordering).
type fakeRepoCall struct {
	method string
	timer  string
	data   []byte
	delay  int64
}

type fakeRepo struct {
	calls     []fakeRepoCall
	createErr error
	cancelErr error
}

func (f *fakeRepo) CreateTimerObligation(_ context.Context, _ *gorm.DB, _ uint, timer string, data []byte, delayMs int64, _ uint) (uint, error) {
	f.calls = append(f.calls, fakeRepoCall{method: "create", timer: timer, data: data, delay: delayMs})
	return 1, f.createErr
}

func (f *fakeRepo) CancelActiveTimerObligation(_ context.Context, _ *gorm.DB, _ uint, timer string, _ uint) error {
	f.calls = append(f.calls, fakeRepoCall{method: "cancel", timer: timer})
	return f.cancelErr
}

func TestApply(t *testing.T) {
	t.Run("schedule_timer_creates_an_obligation", func(t *testing.T) {
		repo := &fakeRepo{}
		commands := []platform.Command{
			&platform.ScheduleTimer{Timer: "round_timer", DelayMS: 5000, Data: []byte(`{"round":1}`)},
		}
		err := Apply(context.Background(), nil, repo, 1, 10, commands)
		require.NoError(t, err)
		require.Equal(t, []fakeRepoCall{
			{method: "cancel", timer: "round_timer"},
			{method: "create", timer: "round_timer", data: []byte(`{"round":1}`), delay: 5000},
		}, repo.calls)
	})

	t.Run("cancel_timer_cancels_the_matching_obligation", func(t *testing.T) {
		repo := &fakeRepo{}
		commands := []platform.Command{&platform.CancelTimer{Timer: "round_timer"}}
		err := Apply(context.Background(), nil, repo, 1, 10, commands)
		require.NoError(t, err)
		require.Equal(t, []fakeRepoCall{{method: "cancel", timer: "round_timer"}}, repo.calls)
	})

	t.Run("cancel_timer_with_no_matching_obligation_is_still_just_a_cancel_call", func(t *testing.T) {
		// Apply itself does not know whether a match exists - the underlying
		// UPDATE ... WHERE state = 'ACTIVE' is a no-op against zero matching
		// rows, which is exactly CancelTimer's own documented contract. This
		// test only confirms Apply issues the call; the no-op behavior itself
		// is a repository-level guarantee, not this package's own.
		repo := &fakeRepo{}
		commands := []platform.Command{&platform.CancelTimer{Timer: "never_scheduled"}}
		err := Apply(context.Background(), nil, repo, 1, 10, commands)
		require.NoError(t, err)
		require.Len(t, repo.calls, 1)
	})

	t.Run("rescheduling_the_same_timer_cancels_before_creating", func(t *testing.T) {
		repo := &fakeRepo{}
		commands := []platform.Command{
			&platform.ScheduleTimer{Timer: "buzzer", DelayMS: 1000},
			&platform.ScheduleTimer{Timer: "buzzer", DelayMS: 2000},
		}
		err := Apply(context.Background(), nil, repo, 1, 10, commands)
		require.NoError(t, err)
		require.Equal(t, []fakeRepoCall{
			{method: "cancel", timer: "buzzer"},
			{method: "create", timer: "buzzer", delay: 1000},
			{method: "cancel", timer: "buzzer"},
			{method: "create", timer: "buzzer", delay: 2000},
		}, repo.calls)
	})

	t.Run("non_timer_commands_are_ignored", func(t *testing.T) {
		repo := &fakeRepo{}
		commands := []platform.Command{
			&platform.SendEvent{Recipients: []platform.ActorRef{"1"}, Name: "celebrate"},
			&platform.SessionComplete{},
		}
		err := Apply(context.Background(), nil, repo, 1, 10, commands)
		require.NoError(t, err)
		require.Empty(t, repo.calls)
	})

	t.Run("distinct_timer_identifiers_stay_independently_active", func(t *testing.T) {
		repo := &fakeRepo{}
		commands := []platform.Command{
			&platform.ScheduleTimer{Timer: "disconnect_timeout:P1", DelayMS: 30000},
			&platform.ScheduleTimer{Timer: "disconnect_timeout:P2", DelayMS: 30000},
		}
		err := Apply(context.Background(), nil, repo, 1, 10, commands)
		require.NoError(t, err)
		require.Equal(t, []fakeRepoCall{
			{method: "cancel", timer: "disconnect_timeout:P1"},
			{method: "create", timer: "disconnect_timeout:P1", delay: 30000},
			{method: "cancel", timer: "disconnect_timeout:P2"},
			{method: "create", timer: "disconnect_timeout:P2", delay: 30000},
		}, repo.calls)
	})

	t.Run("a_cancel_error_stops_processing_and_is_returned", func(t *testing.T) {
		wantErr := errors.New("db down")
		repo := &fakeRepo{cancelErr: wantErr}
		commands := []platform.Command{
			&platform.ScheduleTimer{Timer: "round_timer", DelayMS: 1000},
			&platform.ScheduleTimer{Timer: "other_timer", DelayMS: 1000},
		}
		err := Apply(context.Background(), nil, repo, 1, 10, commands)
		require.ErrorIs(t, err, wantErr)
		require.Len(t, repo.calls, 1, "must not attempt the create after cancel failed, nor process later commands")
	})

	t.Run("a_create_error_stops_processing_and_is_returned", func(t *testing.T) {
		wantErr := errors.New("constraint violation")
		repo := &fakeRepo{createErr: wantErr}
		commands := []platform.Command{&platform.ScheduleTimer{Timer: "round_timer", DelayMS: 1000}}
		err := Apply(context.Background(), nil, repo, 1, 10, commands)
		require.ErrorIs(t, err, wantErr)
	})
}
