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

func TestRecordLimitHits(t *testing.T) {
	type test struct {
		hits []sessionpkg.LimitHit

		expected  int
		expectErr bool
	}

	const sessionUUID = "7c9e6679-7425-40de-944b-e07fc1f90ae7"
	const userUUID = "aaaaaaaa-aaaa-4aaa-8aaa-000000000002"
	ctx := context.Background()
	now := time.Date(2026, 10, 21, 12, 0, 0, 123456789, time.UTC)
	recordedAt := now.Truncate(time.Microsecond)
	valid := sessionpkg.LimitHit{SessionUUID: sessionUUID, UserUUID: userUUID, Limit: "api.command.rate", Count: 40}
	stored := internalrepo.Hit{SessionUUID: sessionUUID, UserUUID: userUUID, Limit: "api.command.rate", Count: 40}

	tests := map[string]func(t *testing.T, store *MockhitStore) test{
		"valid_hits_are_stored": func(t *testing.T, store *MockhitStore) test {
			store.EXPECT().RecordHits(ctx, []internalrepo.Hit{stored}, recordedAt).Return(1, nil)
			return test{hits: []sessionpkg.LimitHit{valid}, expected: 1}
		},
		"invalid_hits_are_skipped": func(t *testing.T, store *MockhitStore) test {
			store.EXPECT().RecordHits(ctx, []internalrepo.Hit{stored}, recordedAt).Return(1, nil)
			return test{hits: []sessionpkg.LimitHit{
				valid,
				{SessionUUID: "nope", UserUUID: userUUID, Limit: "x", Count: 1},
				{SessionUUID: sessionUUID, UserUUID: "nope", Limit: "x", Count: 1},
				{SessionUUID: sessionUUID, UserUUID: userUUID, Limit: "", Count: 1},
				{SessionUUID: sessionUUID, UserUUID: userUUID, Limit: strings.Repeat("x", 65), Count: 1},
				{SessionUUID: sessionUUID, UserUUID: userUUID, Limit: "x", Count: 0},
			}, expected: 1}
		},
		"nothing_valid_stores_nothing": func(t *testing.T, store *MockhitStore) test {
			return test{hits: []sessionpkg.LimitHit{{SessionUUID: "nope"}}}
		},
		"storing_fails": func(t *testing.T, store *MockhitStore) test {
			store.EXPECT().RecordHits(ctx, []internalrepo.Hit{stored}, recordedAt).Return(0, errors.New("database down"))
			return test{hits: []sessionpkg.LimitHit{valid}, expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			store := NewMockhitStore(gomock.NewController(t))
			tc := setup(t, store)
			service := &Service{hits: store, now: func() time.Time { return now }}

			recorded, err := service.RecordLimitHits(ctx, tc.hits)

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expected, recorded)
		})
	}
}
