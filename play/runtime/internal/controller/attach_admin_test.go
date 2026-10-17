package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

func TestAttachAdmin(t *testing.T) {
	controller, _ := NewRegistry().ControllerFor(context.Background(), "session-1", 1)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	first := controller.AttachAdmin("creator")
	second := controller.AttachAdmin("creator")
	require.Equal(t, 2, controller.AdminCount())

	_, err := first.SelectPerspective(ctx, "P1")
	require.ErrorIs(t, err, ErrNotImplemented)
	_, err = first.StartSession(ctx)
	require.ErrorIs(t, err, ErrNotImplemented)
	_, err = first.CancelSession(ctx)
	require.ErrorIs(t, err, ErrNotImplemented)

	first.Disconnect()
	first.Disconnect()
	require.Equal(t, 1, controller.AdminCount())
	end, ended := first.Ended(ctx)
	require.True(t, ended)
	require.Equal(t, runtime.ConnectionEnd{Reason: runtime.EndDisconnected}, end)
	_, more := first.NextView(ctx)
	require.False(t, more)
	_, more = first.NextLobbyUpdate(ctx)
	require.False(t, more)

	waiting, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer stop()
	_, ended = second.Ended(waiting)
	require.False(t, ended, "the other admin connection must stay attached")
}
