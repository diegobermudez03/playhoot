package keepsimulationresults

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestKeepSimulationResult(t *testing.T) {
	const (
		versionUUID = "0f8fad5b-d9cb-469f-a165-70867728950e"
		resultUUID  = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	)
	ctx := context.Background()
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	expires := now.Add(30 * time.Minute)
	result := json.RawMessage(`{"Outcome":"ran"}`)

	t.Run("records_the_result_after_clearing_expired_ones", func(t *testing.T) {
		repo := NewMockresultsRepo(gomock.NewController(t))
		gomock.InOrder(
			repo.EXPECT().FindVersionID(ctx, versionUUID).Return(uint(9), true, nil),
			repo.EXPECT().ClearExpired(ctx, now, now.Add(-RecordRetention)).Return(nil),
			repo.EXPECT().Insert(ctx, resultUUID, uint(9), result, now, expires).Return(nil),
		)
		service := &Service{results: repo, now: func() time.Time { return now }}

		require.NoError(t, service.KeepSimulationResult(ctx, versionUUID, resultUUID, result, expires))
	})

	t.Run("an_unknown_version", func(t *testing.T) {
		repo := NewMockresultsRepo(gomock.NewController(t))
		repo.EXPECT().FindVersionID(ctx, versionUUID).Return(uint(0), false, nil)
		service := &Service{results: repo, now: func() time.Time { return now }}

		err := service.KeepSimulationResult(ctx, versionUUID, resultUUID, result, expires)

		require.ErrorIs(t, err, sessionpkg.ErrDraftNotFound)
		require.ErrorIs(t, service.KeepSimulationResult(ctx, "nope", resultUUID, result, expires), sessionpkg.ErrDraftNotFound)
	})

	t.Run("a_failure_keeps_nothing", func(t *testing.T) {
		repo := NewMockresultsRepo(gomock.NewController(t))
		repo.EXPECT().FindVersionID(ctx, versionUUID).Return(uint(9), true, nil)
		repo.EXPECT().ClearExpired(ctx, now, now.Add(-RecordRetention)).Return(errors.New("down"))
		service := &Service{results: repo, now: func() time.Time { return now }}

		require.Error(t, service.KeepSimulationResult(ctx, versionUUID, resultUUID, result, expires))
	})
}

func TestFindAndCountSimulations(t *testing.T) {
	const (
		versionUUID = "0f8fad5b-d9cb-469f-a165-70867728950e"
		resultUUID  = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	)
	ctx := context.Background()
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	repo := NewMockresultsRepo(gomock.NewController(t))
	service := &Service{results: repo, now: func() time.Time { return now }}

	repo.EXPECT().Find(ctx, resultUUID, versionUUID, now).Return(json.RawMessage(`{}`), true, nil)
	found, ok, err := service.FindSimulationResult(ctx, resultUUID, versionUUID)
	require.NoError(t, err)
	require.True(t, ok)
	require.JSONEq(t, `{}`, string(found))

	_, ok, err = service.FindSimulationResult(ctx, "not-a-uuid", versionUUID)
	require.NoError(t, err)
	require.False(t, ok, "an id that is no UUID names nothing")

	repo.EXPECT().CountSince(ctx, versionUUID, now.Add(-time.Hour)).Return(4, nil)
	count, err := service.CountSimulations(ctx, versionUUID, now.Add(-time.Hour))
	require.NoError(t, err)
	require.Equal(t, 4, count)
}
