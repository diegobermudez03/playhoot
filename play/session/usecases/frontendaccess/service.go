// Package frontendaccess grants clients short-lived access to the frontend
// package of a game version.
package frontendaccess

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/monitoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	"github.com/diegobermudez03/playhoot/play/session/storage"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/usecases/frontendaccess/internal/repo"
)

//go:generate mockgen -package=frontendaccess -destination=service_mock_test.go . frontendFinder,urlSigner

// frontendFinder reads where a game version's frontend package is stored.
type frontendFinder interface {
	FindFrontend(ctx context.Context, gameVersionUUID string) (*internalrepo.Frontend, error)
}

// urlSigner issues expiring read URLs to stored objects.
type urlSigner interface {
	PresignGet(ctx context.Context, key string, ttl time.Duration) (objectstore.SignedURL, error)
}

// urlTTL is how long a granted URL works: long enough to load the game, short
// enough that a leaked URL soon stops working.
const urlTTL = 15 * time.Minute

var errGameVersionNotFound = errors.New("game version not found")

// Service is the frontend access use case.
type Service struct {
	frontends frontendFinder
	signer    urlSigner
	ttl       time.Duration
}

// New constructs a Service reading from db and signing URLs to store.
func New(db *gorm.DB, store storage.Store) *Service {
	return &Service{frontends: internalrepo.New(db), signer: store, ttl: urlTTL}
}

// FrontendAccess returns a signed, expiring URL to gameVersionUUID's frontend
// package, with the package's content hash. Only an executable version, one a
// Session can be pinned to, is granted.
func (s *Service) FrontendAccess(ctx context.Context, gameVersionUUID string) (sessionpkg.FrontendAccess, error) {
	if _, err := uuid.Parse(gameVersionUUID); err != nil {
		return sessionpkg.FrontendAccess{}, errGameVersionNotFound
	}
	frontend, err := s.frontends.FindFrontend(ctx, gameVersionUUID)
	if err != nil {
		return sessionpkg.FrontendAccess{}, err
	}
	if frontend == nil {
		return sessionpkg.FrontendAccess{}, errGameVersionNotFound
	}
	if !gameversion.Status(frontend.Status).AllowsSessions() {
		return sessionpkg.FrontendAccess{}, fmt.Errorf("game version %s is %s, not executable", gameVersionUUID, frontend.Status)
	}
	if frontend.ObjectKey == nil || frontend.SHA256 == nil {
		monitoring.Alert(ctx, "play executable game version has no frontend package")
		return sessionpkg.FrontendAccess{}, fmt.Errorf("game version %s has no frontend package", gameVersionUUID)
	}
	signed, err := s.signer.PresignGet(ctx, *frontend.ObjectKey, s.ttl)
	if err != nil {
		return sessionpkg.FrontendAccess{}, fmt.Errorf("signing frontend package url: %w", err)
	}
	return sessionpkg.FrontendAccess{URL: signed.URL, SHA256: *frontend.SHA256, ExpiresAt: signed.ExpiresAt}, nil
}
