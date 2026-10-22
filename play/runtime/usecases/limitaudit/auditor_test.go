package limitaudit

import (
	"context"
	"errors"
	"sort"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func sorted(hits []sessionpkg.LimitHit) []sessionpkg.LimitHit {
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].UserUUID != hits[j].UserUUID {
			return hits[i].UserUUID < hits[j].UserUUID
		}
		return hits[i].Limit < hits[j].Limit
	})
	return hits
}

func TestFlushRecordsGatheredHitsCounted(t *testing.T) {
	store := NewMockhitStore(gomock.NewController(t))
	auditor := New(store)
	ctx := context.Background()
	for range 40 {
		auditor.Hit(ctx, "s1", "u1", "api.command.rate")
	}
	auditor.Hit(ctx, "s1", "u1", "runtime.sample_rate")
	auditor.Hit(ctx, "s1", "u2", "api.command.rate")
	var recorded []sessionpkg.LimitHit
	store.EXPECT().RecordLimitHits(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, hits []sessionpkg.LimitHit) (int, error) {
		recorded = hits
		return len(hits), nil
	})

	auditor.Flush(ctx)

	require.Equal(t, []sessionpkg.LimitHit{
		{SessionUUID: "s1", UserUUID: "u1", Limit: "api.command.rate", Count: 40},
		{SessionUUID: "s1", UserUUID: "u1", Limit: "runtime.sample_rate", Count: 1},
		{SessionUUID: "s1", UserUUID: "u2", Limit: "api.command.rate", Count: 1},
	}, sorted(recorded))
}

func TestFlushWithNothingGatheredStoresNothing(t *testing.T) {
	auditor := New(NewMockhitStore(gomock.NewController(t)))

	auditor.Flush(context.Background())
}

func TestFailedFlushIsDroppedNotRetried(t *testing.T) {
	store := NewMockhitStore(gomock.NewController(t))
	auditor := New(store)
	ctx := context.Background()
	auditor.Hit(ctx, "s1", "u1", "api.command.rate")
	store.EXPECT().RecordLimitHits(ctx, gomock.Any()).Return(0, errors.New("database down"))

	auditor.Flush(ctx)
	auditor.Flush(ctx)
}

func TestHitsBeyondThePendingBoundAreDropped(t *testing.T) {
	auditor := New(NewMockhitStore(gomock.NewController(t)))
	ctx := context.Background()
	for i := range maxPendingKeys + 5 {
		auditor.Hit(ctx, "s1", "u1", string(rune('a'+i%26))+string(rune(i)))
	}
	auditor.Hit(ctx, "s1", "u1", string(rune('a'))+string(rune(0)))

	require.Len(t, auditor.pending, maxPendingKeys)
	require.Equal(t, 2, auditor.pending[hitKey{sessionUUID: "s1", userUUID: "u1", limit: "a" + string(rune(0))}])
}

func TestRunRecordsWhatIsLeftWhenStopped(t *testing.T) {
	store := NewMockhitStore(gomock.NewController(t))
	auditor := New(store)
	ctx, cancel := context.WithCancel(context.Background())
	auditor.Hit(ctx, "s1", "u1", "api.command.rate")
	store.EXPECT().RecordLimitHits(gomock.Any(), []sessionpkg.LimitHit{
		{SessionUUID: "s1", UserUUID: "u1", Limit: "api.command.rate", Count: 1},
	}).Return(1, nil)
	cancel()

	auditor.Run(ctx)
}
