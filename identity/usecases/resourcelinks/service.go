// Package resourcelinks is Identity's secret links to one resource: whoever
// holds a link acts for its User on that resource only, until it expires or
// is revoked. Identity never knows what the resource is.
package resourcelinks

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/resourcelink"
	internalrepo "github.com/diegobermudez03/playhoot/identity/usecases/resourcelinks/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=resourcelinks -destination=service_mock_test.go . linkStore

// linkStore persists resource links.
type linkStore interface {
	LockUser(ctx context.Context, tx *gorm.DB, userUUID string) (int64, bool, error)
	RevokeOpenLink(ctx context.Context, tx *gorm.DB, userID int64, kind string, resourceID string, revokedAt time.Time) (bool, error)
	CreateLink(ctx context.Context, tx *gorm.DB, userID int64, kind string, resourceID string, tokenSHA256 string, createdAt time.Time, expiresAt time.Time) error
	FindOpenLink(ctx context.Context, userUUID string, kind string, resourceID string) (*internalrepo.Link, error)
	FindLinkByHash(ctx context.Context, tokenSHA256 string) (*internalrepo.Link, error)
}

var (
	operationLabel = metrics.NewLabel("operation", "created", "revoked", "resolved_valid", "resolved_unknown", "resolved_expired", "resolved_revoked")

	linksTotal = metrics.NewCounter(
		"identity_resource_links_total",
		"Resource link operations: links created (replacing any open one) and revoked, and tokens resolved, by what they turned out to be.",
		operationLabel,
	)
)

// Service is Identity's resource links use case.
type Service struct {
	db       utils.DBServicer
	links    linkStore
	now      func() time.Time
	newToken func() (string, error)
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	repo := internalrepo.New(db)
	return &Service{db: repo, links: repo, now: time.Now, newToken: resourcelink.NewToken}
}

// CreateResourceLink creates a link for userUUID to the resource, revoking the
// open one if any. identity.ErrUserNotFound when there is no such User.
func (s *Service) CreateResourceLink(ctx context.Context, userUUID string, kind string, resourceID string) (identity.CreatedResourceLink, error) {
	defer logging.Step(ctx, "resourcelinks.create").Close()
	if !resourcelink.ValidResource(kind, resourceID) {
		return identity.CreatedResourceLink{}, fmt.Errorf("invalid resource %q %q", kind, resourceID)
	}
	token, err := s.newToken()
	if err != nil {
		return identity.CreatedResourceLink{}, err
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	created := identity.CreatedResourceLink{Token: token, CreatedAt: now, ExpiresAt: now.Add(resourcelink.Lifetime)}
	replaced, err := utils.RunInDBTransaction(ctx, s.db, func(ctx context.Context, tx *gorm.DB) (bool, error) {
		userID, found, err := s.links.LockUser(ctx, tx, userUUID)
		if err != nil {
			return false, err
		}
		if !found {
			return false, identity.ErrUserNotFound
		}
		replaced, err := s.links.RevokeOpenLink(ctx, tx, userID, kind, resourceID, now)
		if err != nil {
			return false, err
		}
		return replaced, s.links.CreateLink(ctx, tx, userID, kind, resourceID, resourcelink.Hash(token), created.CreatedAt, created.ExpiresAt)
	})
	if err != nil {
		return identity.CreatedResourceLink{}, err
	}
	linksTotal.Inc(ctx, operationLabel.Value("created"))
	logging.LogFields(ctx, logging.Field("resource_link_replaced", replaced))
	return created, nil
}

// ResourceLinkStatus says whether userUUID has a live link to the resource.
func (s *Service) ResourceLinkStatus(ctx context.Context, userUUID string, kind string, resourceID string) (identity.ResourceLinkStatus, error) {
	link, err := s.links.FindOpenLink(ctx, userUUID, kind, resourceID)
	if err != nil || link == nil || !s.now().Before(link.ExpiresAt) {
		return identity.ResourceLinkStatus{}, err
	}
	return identity.ResourceLinkStatus{Active: true, CreatedAt: link.CreatedAt.UTC(), ExpiresAt: link.ExpiresAt.UTC()}, nil
}

// RevokeResourceLink revokes userUUID's open link to the resource; revoking
// when there is none does nothing.
func (s *Service) RevokeResourceLink(ctx context.Context, userUUID string, kind string, resourceID string) error {
	defer logging.Step(ctx, "resourcelinks.revoke").Close()
	now := s.now().UTC().Truncate(time.Microsecond)
	revoked, err := utils.RunInDBTransaction(ctx, s.db, func(ctx context.Context, tx *gorm.DB) (bool, error) {
		userID, found, err := s.links.LockUser(ctx, tx, userUUID)
		if err != nil || !found {
			return false, err
		}
		return s.links.RevokeOpenLink(ctx, tx, userID, kind, resourceID, now)
	})
	if err != nil {
		return err
	}
	if revoked {
		linksTotal.Inc(ctx, operationLabel.Value("revoked"))
	}
	logging.LogFields(ctx, logging.Field("resource_link_revoked", revoked))
	return nil
}

// ResolveResourceLink says who token acts for and on what resource. The
// token itself is never logged.
func (s *Service) ResolveResourceLink(ctx context.Context, token string) (identity.ResolvedResourceLink, error) {
	resolved := identity.ResolvedResourceLink{Outcome: identity.ResourceLinkUnknown}
	if resourcelink.WellFormed(token) {
		link, err := s.links.FindLinkByHash(ctx, resourcelink.Hash(token))
		if err != nil {
			return identity.ResolvedResourceLink{}, err
		}
		switch {
		case link == nil:
		case link.RevokedAt != nil:
			resolved.Outcome = identity.ResourceLinkRevoked
		case !s.now().Before(link.ExpiresAt):
			resolved.Outcome = identity.ResourceLinkExpired
		default:
			resolved = identity.ResolvedResourceLink{Outcome: identity.ResourceLinkValid, UserUUID: link.UserUUID, Kind: link.Kind, ResourceID: link.ResourceID, ExpiresAt: link.ExpiresAt.UTC()}
		}
	}
	linksTotal.Inc(ctx, operationLabel.Value("resolved_"+string(resolved.Outcome)))
	logging.LogFields(ctx, logging.Field("resource_link", string(resolved.Outcome)))
	return resolved, nil
}
