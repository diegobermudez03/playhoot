package liveconnection

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestJoiningARunningGameGetsFrontendAccessSignedNow(t *testing.T) {
	type test struct {
		signed    sessionpkg.FrontendAccess
		signedErr error

		expected runtime.GameFrontend
	}

	startedWith := runtime.GameFrontend{URL: "https://storage.example/frontend?sig=start", SHA256: "9f86d081", ExpiresAt: time.Date(2026, 10, 18, 12, 15, 0, 0, time.UTC)}
	fresh := sessionpkg.FrontendAccess{URL: "https://storage.example/frontend?sig=now", SHA256: "9f86d081", ExpiresAt: time.Date(2026, 10, 18, 13, 15, 0, 0, time.UTC)}

	tests := map[string]func(t *testing.T) test{
		"the_access_is_signed_for_the_joining_connection": func(t *testing.T) test {
			return test{signed: fresh, expected: frontendOf(fresh)}
		},
		"a_failed_signing_keeps_the_access_of_the_start": func(t *testing.T) test {
			return test{signedErr: errors.New("signing service down"), expected: startedWith}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			controller := newTestController(t, 4)
			ctx := context.Background()
			player, _, admitted := controller.joinLobby(ctx, runtime.Player{UserUUID: "user-a"})
			require.True(t, admitted)
			participants, _, ok := controller.beginStart()
			require.True(t, ok)
			controller.completeStart(participants, Game{GameVersionUUID: "game-version", ContextRevision: 1, State: []byte(`{}`)}, startedWith, map[string][]byte{"P1": []byte(`{}`)}, nil)
			player.Disconnect()
			signer := NewMockfrontendSigner(gomock.NewController(t))
			signer.EXPECT().FrontendAccess(gomock.Any(), "game-version").Return(tc.signed, tc.signedErr).Times(2)
			controller.deps.frontends = signer

			back, participant, err := controller.attachParticipant(ctx, runtime.Player{UserUUID: "user-a"})
			admin, registered := controller.admitAdmin(ctx, "creator")

			require.NoError(t, err)
			require.True(t, registered)
			require.True(t, participant)
			require.Equal(t, runtime.AttachAccepted, back.Outcome)
			start, ok := back.Connection.Started(ctx)
			require.True(t, ok)
			require.Equal(t, tc.expected, start.Frontend)
			adminStart, ok := admin.Started(ctx)
			require.True(t, ok)
			require.Equal(t, tc.expected, adminStart.Frontend, "an admin joining the running game too")
		})
	}
}

func TestJoiningTheLobbySignsNothing(t *testing.T) {
	controller := newTestController(t, 4)
	// Any signing fails the test: the mock expects none.
	controller.deps.frontends = NewMockfrontendSigner(gomock.NewController(t))

	_, participant, err := controller.attachParticipant(context.Background(), runtime.Player{UserUUID: "user-a"})

	require.NoError(t, err)
	require.False(t, participant)
}
