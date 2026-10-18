package sessionlifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/sessionlifecycle/internal/repo"
)

func TestCheckStartEligibility(t *testing.T) {
	type dependencies struct {
		repo    *MockcheckStartRepo
		scripts *MockscriptLoader
	}
	type test struct {
		sessionUUID string
		userUUID    string
		playerCount int

		expected  sessionpkg.StartEligibility
		expectErr bool
	}

	ctx := context.Background()
	key, sha, size := "game-versions/backend-scripts/abc", "abc", int64(12)
	locator := objectstore.Locator{ObjectKey: key, SHA256: sha, Size: size}
	candidate := func() *internalrepo.StartCandidate {
		return &internalrepo.StartCandidate{
			Status:            "LOBBY",
			CreatedByUserUUID: startCreatorUUID,
			LobbyExpiresAt:    startNow.Add(time.Minute),
			GameVersionUUID:   startVersionUUID,
			VersionStatus:     "EXECUTABLE",
			Manifest:          []byte(`{"schema_version":1,"players":{"min":2,"max":4}}`),
			ScriptObjectKey:   &key,
			ScriptSHA256:      &sha,
			ScriptSize:        &size,
		}
	}
	finds := func(deps dependencies, found *internalrepo.StartCandidate) {
		deps.repo.EXPECT().FindStartCandidate(ctx, startSessionUUID).Return(found, nil)
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"eligible_with_the_versions_script": func(t *testing.T, deps dependencies) test {
			finds(deps, candidate())
			deps.scripts.EXPECT().Load(ctx, locator).Return([]byte(`function start() {}`), nil)
			return test{
				playerCount: 2,
				expected:    sessionpkg.StartEligibility{Outcome: sessionpkg.StartEligible, GameVersionUUID: startVersionUUID, Script: []byte(`function start() {}`)},
			}
		},
		"creator_uuid_in_another_textual_form_is_the_creator": func(t *testing.T, deps dependencies) test {
			finds(deps, candidate())
			deps.scripts.EXPECT().Load(ctx, locator).Return([]byte(`function start() {}`), nil)
			return test{
				userUUID:    "0F8FAD5B-D9CB-469F-A165-70867728950E",
				playerCount: 4,
				expected:    sessionpkg.StartEligibility{Outcome: sessionpkg.StartEligible, GameVersionUUID: startVersionUUID, Script: []byte(`function start() {}`)},
			}
		},
		"invalid_session_uuid_is_not_found": func(t *testing.T, deps dependencies) test {
			return test{sessionUUID: "not-a-uuid", playerCount: 2, expected: sessionpkg.StartEligibility{Outcome: sessionpkg.StartSessionNotFound}}
		},
		"unknown_session": func(t *testing.T, deps dependencies) test {
			finds(deps, nil)
			return test{playerCount: 2, expected: sessionpkg.StartEligibility{Outcome: sessionpkg.StartSessionNotFound}}
		},
		"caller_is_not_the_creator": func(t *testing.T, deps dependencies) test {
			finds(deps, candidate())
			return test{userUUID: startPlayerA, playerCount: 2, expected: sessionpkg.StartEligibility{Outcome: sessionpkg.StartNotCreator}}
		},
		"running_session_already_started": func(t *testing.T, deps dependencies) test {
			running := candidate()
			running.Status = "RUNNING"
			finds(deps, running)
			return test{playerCount: 2, expected: sessionpkg.StartEligibility{Outcome: sessionpkg.StartAlreadyStarted}}
		},
		"ended_session": func(t *testing.T, deps dependencies) test {
			aborted := candidate()
			aborted.Status = "ABORTED"
			finds(deps, aborted)
			return test{playerCount: 2, expected: sessionpkg.StartEligibility{Outcome: sessionpkg.StartNotInLobby}}
		},
		"overdue_lobby_has_expired": func(t *testing.T, deps dependencies) test {
			overdue := candidate()
			overdue.LobbyExpiresAt = startNow
			finds(deps, overdue)
			return test{playerCount: 2, expected: sessionpkg.StartEligibility{Outcome: sessionpkg.StartNotInLobby}}
		},
		"fewer_players_than_the_manifest_allows": func(t *testing.T, deps dependencies) test {
			finds(deps, candidate())
			return test{playerCount: 1, expected: sessionpkg.StartEligibility{Outcome: sessionpkg.StartInvalidPlayerCount}}
		},
		"more_players_than_the_manifest_allows": func(t *testing.T, deps dependencies) test {
			finds(deps, candidate())
			return test{playerCount: 5, expected: sessionpkg.StartEligibility{Outcome: sessionpkg.StartInvalidPlayerCount}}
		},
		"version_not_executable_is_broken_data": func(t *testing.T, deps dependencies) test {
			draft := candidate()
			draft.VersionStatus = "DRAFT"
			finds(deps, draft)
			return test{playerCount: 2, expectErr: true}
		},
		"invalid_manifest_is_broken_data": func(t *testing.T, deps dependencies) test {
			broken := candidate()
			broken.Manifest = []byte(`{"schema_version":1}`)
			finds(deps, broken)
			return test{playerCount: 2, expectErr: true}
		},
		"version_without_script_is_broken_data": func(t *testing.T, deps dependencies) test {
			scriptless := candidate()
			scriptless.ScriptObjectKey = nil
			finds(deps, scriptless)
			return test{playerCount: 2, expectErr: true}
		},
		"loading_the_script_fails": func(t *testing.T, deps dependencies) test {
			finds(deps, candidate())
			deps.scripts.EXPECT().Load(ctx, locator).Return(nil, objectstore.ErrHashMismatch)
			return test{playerCount: 2, expectErr: true}
		},
		"finding_the_session_fails": func(t *testing.T, deps dependencies) test {
			deps.repo.EXPECT().FindStartCandidate(ctx, startSessionUUID).Return(nil, errors.New("database down"))
			return test{playerCount: 2, expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{repo: NewMockcheckStartRepo(ctrl), scripts: NewMockscriptLoader(ctrl)}
			tc := setup(t, deps)
			if tc.sessionUUID == "" {
				tc.sessionUUID = startSessionUUID
			}
			if tc.userUUID == "" {
				tc.userUUID = startCreatorUUID
			}
			manager := &Manager{checkStartRepo: deps.repo, scripts: deps.scripts, now: func() time.Time { return startNow }}

			eligibility, err := manager.CheckStartEligibility(ctx, tc.sessionUUID, tc.userUUID, tc.playerCount)

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, eligibility)
		})
	}
}
