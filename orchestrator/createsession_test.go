package orchestrator

import (
	"context"
	"errors"
	"testing"

	"github.com/diegobermudez03/playhoot/session"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCreateSession_Visible(t *testing.T) {
	ctrl := gomock.NewController(t)
	visibility := NewMockvisibilityChecker(ctrl)
	sessions := NewMocksessionCreator(ctrl)

	gameUUID := session.GameUUID("game-1")
	hostUserUUID := session.UserUUID("user-1")
	idempotencyKey := session.IdempotencyKey("key-1")
	expected := session.CreatedSession{SessionUUID: "session-1", JoinCode: 1234}

	visibility.EXPECT().IsVisible(gomock.Any(), "game-1").Return(true, true, nil)
	sessions.EXPECT().Create(gomock.Any(), gameUUID, hostUserUUID, idempotencyKey).Return(expected, nil)

	o := New(visibility, sessions)
	result, err := o.CreateSession(context.Background(), gameUUID, hostUserUUID, idempotencyKey)
	require.NoError(t, err)
	require.Equal(t, expected, result)
}

func TestCreateSession_NotVisible(t *testing.T) {
	ctrl := gomock.NewController(t)
	visibility := NewMockvisibilityChecker(ctrl)
	sessions := NewMocksessionCreator(ctrl)

	visibility.EXPECT().IsVisible(gomock.Any(), "game-1").Return(false, true, nil)
	sessions.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	o := New(visibility, sessions)
	_, err := o.CreateSession(context.Background(), "game-1", "user-1", "key-1")
	require.ErrorIs(t, err, session.ErrGameNotFound)
}

func TestCreateSession_NotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	visibility := NewMockvisibilityChecker(ctrl)
	sessions := NewMocksessionCreator(ctrl)

	visibility.EXPECT().IsVisible(gomock.Any(), "missing").Return(false, false, nil)
	sessions.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	o := New(visibility, sessions)
	_, err := o.CreateSession(context.Background(), "missing", "user-1", "key-1")
	require.ErrorIs(t, err, session.ErrGameNotFound)
}

func TestCreateSession_VisibilityCheckError(t *testing.T) {
	ctrl := gomock.NewController(t)
	visibility := NewMockvisibilityChecker(ctrl)
	sessions := NewMocksessionCreator(ctrl)

	visibilityErr := errors.New("game management unavailable")
	visibility.EXPECT().IsVisible(gomock.Any(), "game-1").Return(false, false, visibilityErr)
	sessions.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Times(0)

	o := New(visibility, sessions)
	_, err := o.CreateSession(context.Background(), "game-1", "user-1", "key-1")
	require.ErrorIs(t, err, visibilityErr)
}

func TestCreateSession_DownstreamCreateError(t *testing.T) {
	ctrl := gomock.NewController(t)
	visibility := NewMockvisibilityChecker(ctrl)
	sessions := NewMocksessionCreator(ctrl)

	createErr := errors.New("session runtime unavailable")
	visibility.EXPECT().IsVisible(gomock.Any(), "game-1").Return(true, true, nil)
	sessions.EXPECT().Create(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(session.CreatedSession{}, createErr)

	o := New(visibility, sessions)
	_, err := o.CreateSession(context.Background(), "game-1", "user-1", "key-1")
	require.ErrorIs(t, err, createErr)
}
