package reconciliation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/session/internal/business/endcause"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/sessionstatus"
)

func TestCauseFor(t *testing.T) {
	type test struct {
		session Session

		expectedCause   endcause.Cause
		expectedMustEnd bool
	}

	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	// running is a RUNNING Session within its lifetime whose owner lease runs
	// out at lease from now.
	running := func(lease time.Duration) Session {
		return Session{Status: sessionstatus.Running, ExpiresAt: now.Add(time.Hour), OwnerLeaseExpiresAt: now.Add(lease)}
	}

	tests := map[string]func(t *testing.T) test{
		"lobby_past_its_expiry": func(t *testing.T) test {
			return test{session: Session{Status: sessionstatus.Lobby, LobbyExpiresAt: now}, expectedCause: endcause.LobbyExpired, expectedMustEnd: true}
		},
		"lobby_before_its_expiry": func(t *testing.T) test {
			return test{session: Session{Status: sessionstatus.Lobby, LobbyExpiresAt: now.Add(time.Second)}}
		},
		"running_with_its_lease_and_lifetime_left": func(t *testing.T) test {
			return test{session: running(time.Second)}
		},
		"lease_over_is_owner_lost": func(t *testing.T) test {
			return test{session: running(0), expectedCause: endcause.OwnerLost, expectedMustEnd: true}
		},
		"past_its_lifetime_expires_even_with_an_owner": func(t *testing.T) test {
			session := running(time.Minute)
			session.ExpiresAt = now
			return test{session: session, expectedCause: endcause.MaxLifetime, expectedMustEnd: true}
		},
		"lease_over_with_a_finalized_replay_finished": func(t *testing.T) test {
			session := running(-time.Minute)
			session.ReplayFinalized = true
			session.ExpiresAt = now
			return test{session: session, expectedCause: endcause.GameFinished, expectedMustEnd: true}
		},
		"finalized_replay_alone_is_no_deadline": func(t *testing.T) test {
			session := running(time.Minute)
			session.ReplayFinalized = true
			return test{session: session}
		},
		"running_without_a_lease_waits_for_its_lifetime": func(t *testing.T) test {
			return test{session: Session{Status: sessionstatus.Running, ExpiresAt: now.Add(time.Hour)}}
		},
		"ended_session_is_left_alone": func(t *testing.T) test {
			return test{session: Session{Status: sessionstatus.Cancelled, LobbyExpiresAt: now.Add(-time.Hour), ExpiresAt: now.Add(-time.Hour)}}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			cause, mustEnd := CauseFor(tc.session, now)

			require.Equal(t, tc.expectedMustEnd, mustEnd)
			require.Equal(t, tc.expectedCause, cause)
		})
	}
}
