package recordlimithits

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/recordlimithits/internal/repo"
)

func TestRecordLimitHitsSkipsWhatIsNotValid(t *testing.T) {
	const sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	const userUUID = "aaaaaaaa-aaaa-4aaa-8aaa-000000000002"
	service := &Service{hits: NewMockhitStore(gomock.NewController(t)), now: time.Now}

	recorded, err := service.RecordLimitHits(context.Background(), []sessionpkg.LimitHit{
		{SessionUUID: "nope", UserUUID: userUUID, Limit: "x", Count: 1},
		{SessionUUID: sessionUUID, UserUUID: "nope", Limit: "x", Count: 1},
		{SessionUUID: sessionUUID, UserUUID: userUUID, Limit: "", Count: 1},
		{SessionUUID: sessionUUID, UserUUID: userUUID, Limit: strings.Repeat("x", 65), Count: 1},
		{SessionUUID: sessionUUID, UserUUID: userUUID, Limit: "x", Count: 0},
	})

	require.NoError(t, err)
	require.Zero(t, recorded, "nothing valid opens no transaction")
}

func TestRecordInTx(t *testing.T) {
	type test struct {
		expected  int
		expectErr bool
	}

	const sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	const userUUID = "aaaaaaaa-aaaa-4aaa-8aaa-000000000002"
	ctx := context.Background()
	recordedAt := time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)
	hit := sessionpkg.LimitHit{SessionUUID: sessionUUID, UserUUID: userUUID, Limit: "api.command.rate", Count: 40}
	p2 := "P2"
	report := internalrepo.Report{
		SessionID: 7, GameVersionID: 3, UserUUID: userUUID, ParticipantID: &p2,
		Source: "SERVER", Kind: "SERVER_LIMIT", Detail: `{"limit":"api.command.rate"}`, Occurrences: 40, ReportedAt: recordedAt,
	}

	tests := map[string]func(t *testing.T, store *MockhitStore) test{
		"each_hit_is_a_report_with_its_count": func(t *testing.T, store *MockhitStore) test {
			store.EXPECT().FindTarget(ctx, nil, sessionUUID, userUUID).Return(&internalrepo.Target{SessionID: 7, GameVersionID: 3, ParticipantID: &p2}, nil)
			store.EXPECT().CreateReport(ctx, nil, report).Return(nil)
			return test{expected: 1}
		},
		"a_user_who_never_played_is_reported_without_participant": func(t *testing.T, store *MockhitStore) test {
			store.EXPECT().FindTarget(ctx, nil, sessionUUID, userUUID).Return(&internalrepo.Target{SessionID: 7, GameVersionID: 3}, nil)
			lobby := report
			lobby.ParticipantID = nil
			store.EXPECT().CreateReport(ctx, nil, lobby).Return(nil)
			return test{expected: 1}
		},
		"a_session_that_does_not_exist_is_skipped": func(t *testing.T, store *MockhitStore) test {
			store.EXPECT().FindTarget(ctx, nil, sessionUUID, userUUID).Return(nil, nil)
			return test{expected: 0}
		},
		"storing_fails": func(t *testing.T, store *MockhitStore) test {
			store.EXPECT().FindTarget(ctx, nil, sessionUUID, userUUID).Return(&internalrepo.Target{SessionID: 7, GameVersionID: 3, ParticipantID: &p2}, nil)
			store.EXPECT().CreateReport(ctx, nil, report).Return(errors.New("database down"))
			return test{expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			store := NewMockhitStore(gomock.NewController(t))
			tc := setup(t, store)
			service := &Service{hits: store, now: func() time.Time { return recordedAt }}

			recorded, err := service.recordInTx(ctx, nil, []sessionpkg.LimitHit{hit}, recordedAt)

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, recorded)
		})
	}
}
