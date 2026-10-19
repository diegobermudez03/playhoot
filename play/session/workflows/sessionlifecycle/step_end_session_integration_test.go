package sessionlifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	"github.com/diegobermudez03/playhoot/play/session/internal/testdb"
)

func TestEndSessionIntegration(t *testing.T) {
	type test struct {
		session startableSession
		end     func(session startableSession) sessionpkg.SessionEnd

		expected        sessionpkg.EndSessionResult
		expectedStatus  integrationStatus
		expectedHistory func(session startableSession) []string
		// expectedActiveCodes is how many join codes still work afterwards.
		expectedActiveCodes int
	}

	db := testdb.OpenSessionDB(t)
	store := objectstore.NewFake()
	now := time.Now().UTC()
	text := func(value string) *string { return &value }
	noHistory := func(startableSession) []string { return nil }

	tests := map[string]func(t *testing.T) test{
		"host_cancels_the_lobby_releasing_its_join_code": func(t *testing.T) test {
			return test{
				session: seedStartableSession(t, db, store, "LOBBY", now.Add(time.Hour)),
				end: func(session startableSession) sessionpkg.SessionEnd {
					return sessionpkg.SessionEnd{Cause: sessionpkg.EndHostCancelled, ChangedBy: session.CreatorUUID}
				},
				expected:       sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionEndedNow, Status: "CANCELLED", EndCause: "HOST_CANCELLED"},
				expectedStatus: integrationStatus{Status: "CANCELLED", EndCause: text("HOST_CANCELLED")},
				expectedHistory: func(session startableSession) []string {
					return []string{"LOBBY -> CANCELLED HOST_CANCELLED by " + session.CreatorUUID}
				},
			}
		},
		"running_game_expires_by_inactivity": func(t *testing.T) test {
			return test{
				session: seedStartableSession(t, db, store, "RUNNING", now.Add(time.Hour)),
				end: func(startableSession) sessionpkg.SessionEnd {
					return sessionpkg.SessionEnd{Cause: sessionpkg.EndInactivityTimeout}
				},
				expected:       sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionEndedNow, Status: "EXPIRED", EndCause: "INACTIVITY_TIMEOUT"},
				expectedStatus: integrationStatus{Status: "EXPIRED", EndCause: text("INACTIVITY_TIMEOUT")},
				expectedHistory: func(startableSession) []string {
					return []string{"RUNNING -> EXPIRED INACTIVITY_TIMEOUT by platform"}
				},
			}
		},
		"running_game_finishes": func(t *testing.T) test {
			return test{
				session: seedStartableSession(t, db, store, "RUNNING", now.Add(time.Hour)),
				end: func(startableSession) sessionpkg.SessionEnd {
					return sessionpkg.SessionEnd{Cause: sessionpkg.EndGameFinished}
				},
				expected:       sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionEndedNow, Status: "FINISHED", EndCause: "GAME_FINISHED"},
				expectedStatus: integrationStatus{Status: "FINISHED", EndCause: text("GAME_FINISHED")},
				expectedHistory: func(startableSession) []string {
					return []string{"RUNNING -> FINISHED GAME_FINISHED by platform"}
				},
			}
		},
		"running_game_is_aborted": func(t *testing.T) test {
			return test{
				session: seedStartableSession(t, db, store, "RUNNING", now.Add(time.Hour)),
				end: func(startableSession) sessionpkg.SessionEnd {
					return sessionpkg.SessionEnd{Cause: sessionpkg.EndUnrecoverableInfraFailure}
				},
				expected:       sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionEndedNow, Status: "ABORTED", EndCause: "UNRECOVERABLE_INFRA_FAILURE"},
				expectedStatus: integrationStatus{Status: "ABORTED", EndCause: text("UNRECOVERABLE_INFRA_FAILURE")},
				expectedHistory: func(startableSession) []string {
					return []string{"RUNNING -> ABORTED UNRECOVERABLE_INFRA_FAILURE by platform"}
				},
			}
		},
		"lobby_whose_game_failed_to_start_is_aborted_releasing_its_join_code": func(t *testing.T) test {
			return test{
				session: seedStartableSession(t, db, store, "LOBBY", now.Add(time.Hour)),
				end: func(startableSession) sessionpkg.SessionEnd {
					return sessionpkg.SessionEnd{Cause: sessionpkg.EndGameStartFailure}
				},
				expected:       sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionEndedNow, Status: "ABORTED", EndCause: "GAME_START_FAILURE"},
				expectedStatus: integrationStatus{Status: "ABORTED", EndCause: text("GAME_START_FAILURE")},
				expectedHistory: func(startableSession) []string {
					return []string{"LOBBY -> ABORTED GAME_START_FAILURE by platform"}
				},
			}
		},
		"running_game_is_not_aborted_as_a_start_failure": func(t *testing.T) test {
			return test{
				session: seedStartableSession(t, db, store, "RUNNING", now.Add(time.Hour)),
				end: func(startableSession) sessionpkg.SessionEnd {
					return sessionpkg.SessionEnd{Cause: sessionpkg.EndGameStartFailure}
				},
				expected:            sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionNotEnded, Status: "RUNNING"},
				expectedStatus:      integrationStatus{Status: "RUNNING"},
				expectedHistory:     noHistory,
				expectedActiveCodes: 1,
			}
		},
		"lobby_is_not_cancelled_for_players_leaving_a_game": func(t *testing.T) test {
			return test{
				session: seedStartableSession(t, db, store, "LOBBY", now.Add(time.Hour)),
				end: func(startableSession) sessionpkg.SessionEnd {
					return sessionpkg.SessionEnd{Cause: sessionpkg.EndAllPlayersLeft}
				},
				expected:            sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionNotEnded, Status: "LOBBY"},
				expectedStatus:      integrationStatus{Status: "LOBBY"},
				expectedHistory:     noHistory,
				expectedActiveCodes: 1,
			}
		},
		"ended_session_reports_how_it_ended": func(t *testing.T) test {
			session := seedStartableSession(t, db, store, "RUNNING", now.Add(time.Hour))
			_, err := New(db, store).EndSession(context.Background(), session.UUID, sessionpkg.SessionEnd{Cause: sessionpkg.EndMaxLifetime})
			require.NoError(t, err)
			return test{
				session: session,
				end: func(startableSession) sessionpkg.SessionEnd {
					return sessionpkg.SessionEnd{Cause: sessionpkg.EndPlatformTerminated}
				},
				expected:       sessionpkg.EndSessionResult{Outcome: sessionpkg.SessionNotEnded, Status: "EXPIRED", EndCause: "MAX_LIFETIME"},
				expectedStatus: integrationStatus{Status: "EXPIRED", EndCause: text("MAX_LIFETIME")},
				expectedHistory: func(startableSession) []string {
					return []string{"RUNNING -> EXPIRED MAX_LIFETIME by platform"}
				},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			manager := New(db, store)

			result, err := manager.EndSession(context.Background(), tc.session.UUID, tc.end(tc.session))

			require.NoError(t, err)
			require.Equal(t, tc.expected, result)
			status, _, _, active := startEffects(t, db, tc.session.ID)
			require.Equal(t, tc.expectedStatus, status)
			require.Len(t, active, tc.expectedActiveCodes)
			require.Equal(t, tc.expectedHistory(tc.session), statusHistory(t, db, tc.session.ID))
		})
	}
}
