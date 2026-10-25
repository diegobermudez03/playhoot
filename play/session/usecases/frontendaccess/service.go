// Package frontendaccess grants a Session's clients access to the frontend
// package and assets of its game version, through signed URLs shared by every
// Session of the version.
package frontendaccess

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
	"github.com/diegobermudez03/playhoot/play/session/storage"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/frontendaccess/internal/repo"
)

//go:generate mockgen -package=frontendaccess -destination=service_mock_test.go . frontendFinder,signedURLStore,urlSigner

// frontendFinder reads where a Session's game frontend and its assets are
// stored.
type frontendFinder interface {
	FindFrontend(ctx context.Context, sessionUUID string) (*internalrepo.Frontend, error)
	FindAssets(ctx context.Context, gameVersionID int64) ([]internalrepo.Asset, error)
}

// signedURLStore keeps each game version's current batch of signed URLs.
type signedURLStore interface {
	FindSignedURLs(ctx context.Context, gameVersionID int64) (*internalrepo.SignedURLs, error)
	SaveSignedURLs(ctx context.Context, gameVersionID int64, urls internalrepo.SignedURLs, signedAt time.Time) error
}

// urlSigner issues read URLs to stored objects that stop working at a given
// time.
type urlSigner interface {
	PresignGet(ctx context.Context, key string, expiresAt time.Time) (string, error)
}

// batchTTL is how long a newly signed batch works. It outlives any Session
// alive when it is signed, so a URL handed to a client never expires before
// its Session ends, and one batch serves the version's Sessions for a day.
const batchTTL = 24 * time.Hour

// batchExpiryMargin is how much longer than a Session a batch must work to be
// reused for it, so clocks that disagree a little never hand out a URL that
// expires before the Session does.
const batchExpiryMargin = 5 * time.Minute

var (
	accessOutcomeLabel = metrics.NewLabel("outcome", "reused", "signed", "sign_failed")

	accessGranted = metrics.NewCounter(
		"play_session_frontend_access_total",
		"Frontend access granted to a Session, by whether its version's current signed URLs were reused or signed again, or the signing failed.",
		accessOutcomeLabel,
	)
)

// Service is the frontend access use case.
type Service struct {
	frontends frontendFinder
	batches   signedURLStore
	signer    urlSigner
	now       func() time.Time
}

// New constructs a Service reading from db and signing URLs to store.
func New(db *gorm.DB, store storage.Store) *Service {
	repo := internalrepo.New(db)
	return &Service{frontends: repo, batches: repo, signer: store, now: time.Now}
}

// FrontendAccess returns signed URLs to the frontend package and assets of
// sessionUUID's game version, working until after that Session's maximum
// lifetime, with their content hashes and the viewport the frontend is shown
// in. An unknown Session is sessionpkg.ErrSessionNotFound.
func (s *Service) FrontendAccess(ctx context.Context, sessionUUID string) (sessionpkg.FrontendAccess, error) {
	if _, err := uuid.Parse(sessionUUID); err != nil {
		return sessionpkg.FrontendAccess{}, sessionpkg.ErrSessionNotFound
	}
	frontend, err := s.frontends.FindFrontend(ctx, sessionUUID)
	if err != nil {
		return sessionpkg.FrontendAccess{}, err
	}
	if frontend == nil {
		return sessionpkg.FrontendAccess{}, sessionpkg.ErrSessionNotFound
	}
	access, versionID, err := pinnedFrontend(ctx, sessionUUID, frontend)
	if err != nil {
		return sessionpkg.FrontendAccess{}, err
	}
	stored, err := s.frontends.FindAssets(ctx, versionID)
	if err != nil {
		return sessionpkg.FrontendAccess{}, err
	}
	assets, err := deliveredAssets(stored)
	if err != nil {
		monitoring.Alert(ctx, "play executable game version has assets breaking the asset rules")
		return sessionpkg.FrontendAccess{}, fmt.Errorf("game version of session %s: %w", sessionUUID, err)
	}

	batch, err := s.batches.FindSignedURLs(ctx, versionID)
	if err != nil {
		return sessionpkg.FrontendAccess{}, err
	}
	outcome := "reused"
	if !covers(batch, assets, frontend.SessionExpiresAt.Add(batchExpiryMargin)) {
		if batch, err = s.signBatch(ctx, versionID, *frontend.ObjectKey, assets); err != nil {
			accessGranted.Inc(ctx, accessOutcomeLabel.Value("sign_failed"))
			return sessionpkg.FrontendAccess{}, err
		}
		outcome = "signed"
	}
	accessGranted.Inc(ctx, accessOutcomeLabel.Value(outcome))

	access.URL, access.ExpiresAt = batch.FrontendURL, batch.ExpiresAt.UTC()
	access.Assets = make([]sessionpkg.FrontendAsset, len(assets))
	for i, asset := range assets {
		access.Assets[i] = sessionpkg.FrontendAsset{
			Key: asset.Key, Type: asset.Type, MIMEType: asset.MIMEType, SHA256: asset.SHA256, Size: asset.Size,
			URL: batch.AssetURLs[asset.Key],
		}
	}
	return access, nil
}

// pinnedFrontend checks that the Session's pinned version can be delivered
// and returns its access without URLs, and the version's id. A version that
// cannot is broken data: a Session is only ever pinned to an executable one.
func pinnedFrontend(ctx context.Context, sessionUUID string, frontend *internalrepo.Frontend) (sessionpkg.FrontendAccess, int64, error) {
	if frontend.GameVersionID == nil || frontend.Status == nil {
		monitoring.Alert(ctx, "play session is pinned to a game version that does not exist")
		return sessionpkg.FrontendAccess{}, 0, fmt.Errorf("session %s is pinned to a missing game version", sessionUUID)
	}
	if !gameversion.Status(*frontend.Status).AllowsSessions() {
		monitoring.Alert(ctx, "play session is pinned to a game version that is not executable")
		return sessionpkg.FrontendAccess{}, 0, fmt.Errorf("game version of session %s is %s, not executable", sessionUUID, *frontend.Status)
	}
	if frontend.ObjectKey == nil || frontend.SHA256 == nil {
		monitoring.Alert(ctx, "play executable game version has no frontend package")
		return sessionpkg.FrontendAccess{}, 0, fmt.Errorf("game version of session %s has no frontend package", sessionUUID)
	}
	viewport, ok := viewportOf(frontend.AspectRatio)
	if !ok {
		monitoring.Alert(ctx, "play executable game version has no offered aspect ratio")
		return sessionpkg.FrontendAccess{}, 0, fmt.Errorf("game version of session %s has no offered aspect ratio", sessionUUID)
	}
	return sessionpkg.FrontendAccess{SHA256: *frontend.SHA256, Viewport: viewport}, *frontend.GameVersionID, nil
}

// deliveredAsset is an asset whose object is stored, so clients may load it.
type deliveredAsset struct {
	Key       string
	Type      string
	MIMEType  string
	ObjectKey string
	SHA256    string
	Size      int64
}

// deliveredAssets are the stored assets whose object exists, which together
// must keep Playhoot's asset rules. One without its object is a record of an
// asset no longer stored, and is not delivered.
func deliveredAssets(stored []internalrepo.Asset) ([]deliveredAsset, error) {
	assets := make([]deliveredAsset, 0, len(stored))
	checked := make([]gameversion.Asset, 0, len(stored))
	for _, asset := range stored {
		if asset.ObjectKey == nil {
			continue
		}
		assets = append(assets, deliveredAsset{
			Key: asset.Key, Type: asset.Type, MIMEType: asset.MIMEType, ObjectKey: *asset.ObjectKey, SHA256: asset.SHA256, Size: asset.Size,
		})
		checked = append(checked, gameversion.Asset{Key: asset.Key, Type: gameversion.AssetType(asset.Type), MIMEType: asset.MIMEType, SHA256: asset.SHA256, Size: asset.Size})
	}
	if err := gameversion.ValidateAssets(checked); err != nil {
		return nil, err
	}
	return assets, nil
}

// covers reports whether batch can be handed out: it has a URL for every
// asset and works past until.
func covers(batch *internalrepo.SignedURLs, assets []deliveredAsset, until time.Time) bool {
	if batch == nil || !batch.ExpiresAt.After(until) {
		return false
	}
	for _, asset := range assets {
		if batch.AssetURLs[asset.Key] == "" {
			return false
		}
	}
	return true
}

// signBatch signs every URL of a version together, with one expiry, and makes
// them the version's current batch.
func (s *Service) signBatch(ctx context.Context, versionID int64, frontendKey string, assets []deliveredAsset) (*internalrepo.SignedURLs, error) {
	// Whole seconds, as signatures and the database keep them, so this batch
	// reads the same before and after it is stored.
	signedAt := s.now().UTC().Truncate(time.Second)
	batch := &internalrepo.SignedURLs{ExpiresAt: signedAt.Add(batchTTL), AssetURLs: make(map[string]string, len(assets))}
	var err error
	if batch.FrontendURL, err = s.signer.PresignGet(ctx, frontendKey, batch.ExpiresAt); err != nil {
		return nil, fmt.Errorf("signing frontend package url: %w", err)
	}
	for _, asset := range assets {
		url, err := s.signer.PresignGet(ctx, asset.ObjectKey, batch.ExpiresAt)
		if err != nil {
			return nil, fmt.Errorf("signing url of asset %s: %w", asset.Key, err)
		}
		batch.AssetURLs[asset.Key] = url
	}
	// The batch works whether or not it is kept: failing to keep it only
	// means the next Session signs again.
	if err := s.batches.SaveSignedURLs(ctx, versionID, *batch, signedAt); err != nil {
		logging.LogError(ctx, fmt.Errorf("keeping the signed frontend urls of game version %d: %w", versionID, err))
	}
	return batch, nil
}

// viewportOf is the viewport of a stored aspect ratio; ok is false when there
// is none or Playhoot does not offer it.
func viewportOf(aspectRatio *string) (viewport sessionpkg.Viewport, ok bool) {
	if aspectRatio == nil {
		return sessionpkg.Viewport{}, false
	}
	resolution, ok := gameversion.AspectRatio(*aspectRatio).Resolution()
	if !ok {
		return sessionpkg.Viewport{}, false
	}
	return sessionpkg.Viewport{AspectRatio: *aspectRatio, Width: resolution.Width, Height: resolution.Height}, true
}
