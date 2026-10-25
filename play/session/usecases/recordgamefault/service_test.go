package recordgamefault

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/recordgamefault/internal/repo"
)

func TestRecordGameFault(t *testing.T) {
	type test struct {
		sessionUUID string
		fault       sessionpkg.GameFault

		expected  sessionpkg.RecordGameFaultOutcome
		expectErr bool
	}

	const sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	const userUUID = "aaaaaaaa-aaaa-4aaa-8aaa-000000000002"
	ctx := context.Background()
	now := time.Date(2026, 10, 20, 12, 0, 0, 123456789, time.UTC)
	reportedAt := now.Truncate(time.Microsecond)
	abuse := sessionpkg.GameFault{
		UserUUID:      userUUID,
		ParticipantID: "P2",
		Source:        sessionpkg.GameFaultFromClient,
		Kind:          sessionpkg.GameFaultBridgeAbuse,
		Detail:        []byte(`{"refused":{"sample":900}}`),
	}
	stored := internalrepo.Fault{UserUUID: userUUID, ParticipantID: "P2", Source: "CLIENT", Kind: "BRIDGE_ABUSE", Detail: abuse.Detail}
	invalidSample := sessionpkg.GameFault{
		UserUUID:      userUUID,
		ParticipantID: "P2",
		Source:        sessionpkg.GameFaultFromServer,
		Kind:          sessionpkg.GameFaultInvalidSample,
		Detail:        []byte(`{"reason":"jump is not a declared input"}`),
	}

	tests := map[string]func(t *testing.T, faults *MockfaultStore) test{
		"recorded": func(t *testing.T, faults *MockfaultStore) test {
			faults.EXPECT().RecordFault(ctx, sessionUUID, stored, reportedAt).Return(true, nil)
			return test{sessionUUID: sessionUUID, fault: abuse, expected: sessionpkg.GameFaultRecorded}
		},
		"a_server_fault_is_recorded_as_the_server_s": func(t *testing.T, faults *MockfaultStore) test {
			faults.EXPECT().RecordFault(ctx, sessionUUID, internalrepo.Fault{
				UserUUID: userUUID, ParticipantID: "P2", Source: "SERVER", Kind: "INVALID_SAMPLE", Detail: invalidSample.Detail,
			}, reportedAt).Return(true, nil)
			return test{sessionUUID: sessionUUID, fault: invalidSample, expected: sessionpkg.GameFaultRecorded}
		},
		"a_server_kind_from_the_client": func(t *testing.T, faults *MockfaultStore) test {
			fault := invalidSample
			fault.Source = sessionpkg.GameFaultFromClient
			return test{sessionUUID: sessionUUID, fault: fault, expectErr: true}
		},
		"a_client_kind_from_the_server": func(t *testing.T, faults *MockfaultStore) test {
			fault := abuse
			fault.Source = sessionpkg.GameFaultFromServer
			return test{sessionUUID: sessionUUID, fault: fault, expectErr: true}
		},
		"unknown_source": func(t *testing.T, faults *MockfaultStore) test {
			fault := abuse
			fault.Source = ""
			return test{sessionUUID: sessionUUID, fault: fault, expectErr: true}
		},
		"session_not_running_or_not_a_participant": func(t *testing.T, faults *MockfaultStore) test {
			faults.EXPECT().RecordFault(ctx, sessionUUID, stored, reportedAt).Return(false, nil)
			return test{sessionUUID: sessionUUID, fault: abuse, expected: sessionpkg.GameFaultNotRecorded}
		},
		"invalid_session_uuid": func(t *testing.T, faults *MockfaultStore) test {
			return test{sessionUUID: "not-a-uuid", fault: abuse, expectErr: true}
		},
		"reporter_without_participant": func(t *testing.T, faults *MockfaultStore) test {
			fault := abuse
			fault.ParticipantID = ""
			return test{sessionUUID: sessionUUID, fault: fault, expectErr: true}
		},
		"unknown_kind": func(t *testing.T, faults *MockfaultStore) test {
			fault := abuse
			fault.Kind = "CHEATING"
			return test{sessionUUID: sessionUUID, fault: fault, expectErr: true}
		},
		"detail_not_an_object": func(t *testing.T, faults *MockfaultStore) test {
			fault := abuse
			fault.Detail = []byte(`"text"`)
			return test{sessionUUID: sessionUUID, fault: fault, expectErr: true}
		},
		"storing_fails": func(t *testing.T, faults *MockfaultStore) test {
			faults.EXPECT().RecordFault(ctx, sessionUUID, stored, reportedAt).Return(false, errors.New("database down"))
			return test{sessionUUID: sessionUUID, fault: abuse, expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			faults := NewMockfaultStore(gomock.NewController(t))
			tc := setup(t, faults)
			service := &Service{faults: faults, now: func() time.Time { return now }}

			outcome, err := service.RecordGameFault(ctx, tc.sessionUUID, tc.fault)

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, outcome)
		})
	}
}
