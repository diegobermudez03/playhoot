package me

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/identity"
)

func TestDelete(t *testing.T) {
	t.Run("an anonymous caller is sent to authenticate", func(t *testing.T) {
		handler := NewHandler(stubAuthorizer{level: identity.LevelGuest}, Dependencies{})

		recorder := serve(t, handler, http.MethodPost, "/v1/identity/me/delete", "", "")

		require.Equal(t, http.StatusUnauthorized, recorder.Code)
	})

	t.Run("a guest deletes themself", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		deletion := NewMockidentityDeleter(ctrl)
		deletion.EXPECT().
			DeleteUser(gomock.Any(), testUserUUID, identity.DeleteRequested).
			Return(identity.UserDeleted, nil)
		handler := NewHandler(stubAuthorizer{level: identity.LevelGuest}, Dependencies{Deletion: deletion})

		recorder := serve(t, handler, http.MethodPost, "/v1/identity/me/delete", testToken, "")

		require.Equal(t, http.StatusOK, recorder.Code)
		require.JSONEq(t, `{"deleted":true}`, recorder.Body.String())
		require.Equal(t, "no-store", recorder.Header().Get("Cache-Control"))
	})

	t.Run("deleting again answers the same", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		deletion := NewMockidentityDeleter(ctrl)
		deletion.EXPECT().DeleteUser(gomock.Any(), testUserUUID, identity.DeleteRequested).
			Return(identity.UserAlreadyDeleted, nil)
		handler := NewHandler(stubAuthorizer{level: identity.LevelAccount}, Dependencies{Deletion: deletion})

		recorder := serve(t, handler, http.MethodPost, "/v1/identity/me/delete", testToken, "")

		require.Equal(t, http.StatusOK, recorder.Code)
		require.JSONEq(t, `{"deleted":true}`, recorder.Body.String())
	})

	t.Run("a credential naming nobody has nothing left to delete", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		deletion := NewMockidentityDeleter(ctrl)
		deletion.EXPECT().DeleteUser(gomock.Any(), testUserUUID, identity.DeleteRequested).
			Return(identity.DeleteUserOutcome(""), identity.ErrUserNotFound)
		handler := NewHandler(stubAuthorizer{level: identity.LevelGuest}, Dependencies{Deletion: deletion})

		recorder := serve(t, handler, http.MethodPost, "/v1/identity/me/delete", testToken, "")

		require.Equal(t, http.StatusOK, recorder.Code)
		require.JSONEq(t, `{"deleted":true}`, recorder.Body.String())
	})

	t.Run("a deletion that cannot run now is answered as unavailable", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		deletion := NewMockidentityDeleter(ctrl)
		deletion.EXPECT().DeleteUser(gomock.Any(), testUserUUID, identity.DeleteRequested).
			Return(identity.DeleteUserOutcome(""), errors.New("database unreachable"))
		handler := NewHandler(stubAuthorizer{level: identity.LevelGuest}, Dependencies{Deletion: deletion})

		recorder := serve(t, handler, http.MethodPost, "/v1/identity/me/delete", testToken, "")

		require.Equal(t, http.StatusServiceUnavailable, recorder.Code)
	})
}
