package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

func TestAttachAdmin(t *testing.T) {
	controller := newTestController(t, 8)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, _, ok := controller.AdmitPlayer(ctx, "player-1")
	require.True(t, ok)

	start := func(context.Context) (runtime.StartSessionResult, error) {
		return runtime.StartSessionResult{Outcome: runtime.StartInvalidPlayerCount}, nil
	}
	first := controller.AttachAdmin("creator", start)
	second := controller.AttachAdmin("creator", start)
	require.Equal(t, 2, controller.AdminCount())

	update, more := first.NextLobbyUpdate(ctx)
	require.True(t, more)
	require.Equal(t, runtime.LobbyUpdate{Members: []runtime.LobbyMember{{UserUUID: "player-1"}}}, update, "the first update is the lobby as it is")

	_, err := first.SelectPerspective(ctx, "P1")
	require.ErrorIs(t, err, ErrNotImplemented)
	started, err := first.StartSession(ctx)
	require.NoError(t, err)
	require.Equal(t, runtime.StartSessionResult{Outcome: runtime.StartInvalidPlayerCount}, started, "starting is the given start")
	_, err = first.CancelSession(ctx)
	require.ErrorIs(t, err, ErrNotImplemented)

	first.Disconnect()
	first.Disconnect()
	require.Equal(t, 1, controller.AdminCount())
	end, ended := first.Ended(ctx)
	require.True(t, ended)
	require.Equal(t, runtime.ConnectionEnd{Reason: runtime.EndDisconnected}, end)
	_, more = first.NextView(ctx)
	require.False(t, more)
	_, more = first.NextLobbyUpdate(ctx)
	require.False(t, more)
	_, more = first.Started(ctx)
	require.False(t, more, "an ended connection never sees a start")

	waiting, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer stop()
	_, ended = second.Ended(waiting)
	require.False(t, ended, "the other admin connection must stay attached")
}
