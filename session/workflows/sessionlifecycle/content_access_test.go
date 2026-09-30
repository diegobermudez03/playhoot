package sessionlifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/session"
	"github.com/diegobermudez03/playhoot/session/internal/objectstore"
	internalrepo "github.com/diegobermudez03/playhoot/session/workflows/sessionlifecycle/internal/repo"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// signFailingStore is an objectstore.Store whose PresignGet always fails.
type signFailingStore struct {
	objectstore.Store
	err error
}

func (s signFailingStore) PresignGet(ctx context.Context, key string, ttl time.Duration) (objectstore.SignedURL, error) {
	return objectstore.SignedURL{}, s.err
}

// recordingStore records the key and ttl PresignGet was asked to sign.
type recordingStore struct {
	objectstore.Store
	key string
	ttl time.Duration
}

func (s *recordingStore) PresignGet(ctx context.Context, key string, ttl time.Duration) (objectstore.SignedURL, error) {
	s.key, s.ttl = key, ttl
	return objectstore.SignedURL{URL: "https://signed.example/" + key, ExpiresAt: time.Now().Add(ttl)}, nil
}

// TestManagerContentAccess covers GetFrontendScriptAccess/GetAssetAccess's
// decision logic against mocked persistence: they open no transaction, so
// everything but real SQL is mockable here.
func TestManagerContentAccess(t *testing.T) {
	repoErr := errors.New("repo failed")
	locator := objectstore.Locator{ObjectKey: "scripts/abc", SHA256: "abc", Size: 7}
	found := &internalrepo.Session{ID: 1, GameDefinitionUUID: "def-uuid"}

	t.Run("session_not_found", func(t *testing.T) {
		repo := NewMockcontentAccessRepoAPI(gomock.NewController(t))
		repo.EXPECT().ResolveSessionForClientState(gomock.Any(), "s").Return(nil, nil)
		m := &Manager{contentAccessRepo: repo, dbServicer: fakeDBServicer{}, objectStore: objectstore.NewFake()}

		_, err := m.GetFrontendScriptAccess(context.Background(), "s", "u")
		require.ErrorIs(t, err, session.ErrSessionNotFound)
	})

	t.Run("propagates_repo_errors_without_signing", func(t *testing.T) {
		repo := NewMockcontentAccessRepoAPI(gomock.NewController(t))
		repo.EXPECT().ResolveSessionForClientState(gomock.Any(), "s").Return(found, nil)
		repo.EXPECT().FindActor(gomock.Any(), gomock.Any(), uint(1), "u").Return(&internalrepo.Actor{ID: 5}, nil)
		repo.EXPECT().ResolveFrontendScriptObject(gomock.Any(), "def-uuid").Return(nil, repoErr)
		store := &recordingStore{Store: objectstore.NewFake()}
		m := &Manager{contentAccessRepo: repo, dbServicer: fakeDBServicer{}, objectStore: store}

		_, err := m.GetFrontendScriptAccess(context.Background(), "s", "u")
		require.ErrorIs(t, err, repoErr)
		require.Empty(t, store.key, "nothing may be signed when resolution failed")
	})

	t.Run("non_participant_is_declined_and_nothing_is_resolved_or_signed", func(t *testing.T) {
		repo := NewMockcontentAccessRepoAPI(gomock.NewController(t))
		repo.EXPECT().ResolveSessionForClientState(gomock.Any(), "s").Return(found, nil)
		repo.EXPECT().FindActor(gomock.Any(), gomock.Any(), uint(1), "u").Return(nil, nil)
		store := &recordingStore{Store: objectstore.NewFake()}
		m := &Manager{contentAccessRepo: repo, dbServicer: fakeDBServicer{}, objectStore: store}

		result, err := m.GetAssetAccess(context.Background(), "s", "u", "card-back")
		require.NoError(t, err)
		require.Equal(t, session.ContentAccessOutcomeNotAParticipant, result.Outcome)
		require.Empty(t, store.key)
	})

	t.Run("undeclared_content_is_reported_not_found_and_not_signed", func(t *testing.T) {
		repo := NewMockcontentAccessRepoAPI(gomock.NewController(t))
		repo.EXPECT().ResolveSessionForClientState(gomock.Any(), "s").Return(found, nil)
		repo.EXPECT().FindActor(gomock.Any(), gomock.Any(), uint(1), "u").Return(&internalrepo.Actor{ID: 5}, nil)
		repo.EXPECT().ResolveAssetObject(gomock.Any(), "def-uuid", "nope").Return(nil, nil)
		store := &recordingStore{Store: objectstore.NewFake()}
		m := &Manager{contentAccessRepo: repo, dbServicer: fakeDBServicer{}, objectStore: store}

		result, err := m.GetAssetAccess(context.Background(), "s", "u", "nope")
		require.NoError(t, err)
		require.Equal(t, session.ContentAccessOutcomeContentNotFound, result.Outcome)
		require.Empty(t, store.key)
	})

	t.Run("signing_failure_is_returned_as_an_error", func(t *testing.T) {
		repo := NewMockcontentAccessRepoAPI(gomock.NewController(t))
		repo.EXPECT().ResolveSessionForClientState(gomock.Any(), "s").Return(found, nil)
		repo.EXPECT().FindActor(gomock.Any(), gomock.Any(), uint(1), "u").Return(&internalrepo.Actor{ID: 5}, nil)
		repo.EXPECT().ResolveFrontendScriptObject(gomock.Any(), "def-uuid").Return(&internalrepo.ContentObject{Locator: locator, ContentType: "text/javascript"}, nil)
		signErr := errors.New("iam signBlob denied")
		m := &Manager{contentAccessRepo: repo, dbServicer: fakeDBServicer{}, objectStore: signFailingStore{Store: objectstore.NewFake(), err: signErr}}

		result, err := m.GetFrontendScriptAccess(context.Background(), "s", "u")
		require.ErrorIs(t, err, signErr)
		require.Empty(t, result.URL)
	})

	t.Run("grants_a_url_signed_for_exactly_the_resolved_object_with_the_configured_ttl", func(t *testing.T) {
		repo := NewMockcontentAccessRepoAPI(gomock.NewController(t))
		repo.EXPECT().ResolveSessionForClientState(gomock.Any(), "s").Return(found, nil)
		repo.EXPECT().FindActor(gomock.Any(), gomock.Any(), uint(1), "u").Return(&internalrepo.Actor{ID: 5}, nil)
		repo.EXPECT().ResolveAssetObject(gomock.Any(), "def-uuid", "card-back").Return(&internalrepo.ContentObject{Locator: locator, ContentType: "image/png"}, nil)
		store := &recordingStore{Store: objectstore.NewFake()}
		m := &Manager{contentAccessRepo: repo, dbServicer: fakeDBServicer{}, objectStore: store, signedURLTTL: 45 * time.Second}

		result, err := m.GetAssetAccess(context.Background(), "s", "u", "card-back")
		require.NoError(t, err)
		require.Equal(t, session.ContentAccessOutcomeGranted, result.Outcome)
		require.Equal(t, "scripts/abc", store.key)
		require.Equal(t, 45*time.Second, store.ttl)
		require.Equal(t, "https://signed.example/scripts/abc", result.URL)
		require.Equal(t, "abc", result.SHA256)
		require.Equal(t, "image/png", result.ContentType)
		require.Equal(t, int64(7), result.Size)
	})
}
