// Package resourcegrants is Identity's single-use grants: a short-lived
// secret that lets whoever holds it do one thing, once, for its User on one
// resource. Identity never knows what the resource or the thing is.
package resourcegrants

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/resourcegrant"
	"github.com/diegobermudez03/playhoot/identity/internal/business/resourcelink"
	internalrepo "github.com/diegobermudez03/playhoot/identity/usecases/resourcegrants/internal/repo"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=resourcegrants -destination=service_mock_test.go . grantStore

// grantStore persists grants.
type grantStore interface {
	LockUser(ctx context.Context, tx *gorm.DB, userUUID string) (int64, bool, error)
	DeleteExpiredGrants(ctx context.Context, tx *gorm.DB, userID int64, before time.Time) error
	CountLiveGrants(ctx context.Context, tx *gorm.DB, userID int64, now time.Time) (int64, error)
	CreateGrant(ctx context.Context, tx *gorm.DB, grantUUID string, userID int64, kind string, resourceID string, scope string, tokenSHA256 string, createdAt time.Time, expiresAt time.Time) error
	UseGrant(ctx context.Context, tokenSHA256 string, now time.Time) (*internalrepo.Grant, error)
	FindGrantByHash(ctx context.Context, tokenSHA256 string) (*internalrepo.Grant, error)
	ReleaseGrant(ctx context.Context, grantUUID string) (bool, error)
}

var (
	operationLabel = metrics.NewLabel("operation", "created", "refused_too_many", "claimed", "claim_unknown", "claim_expired", "claim_used", "released")

	grantsTotal = metrics.NewCounter(
		"identity_resource_grants_total",
		"Single-use grant operations: grants created or refused for holding too many, tokens claimed by what they turned out to be, and uses released.",
		operationLabel,
	)
)

// expiredRetention is how long an expired grant is kept before its User's
// next grant deletes it.
const expiredRetention = 24 * time.Hour

// Service is Identity's single-use grants use case.
type Service struct {
	db       utils.DBServicer
	grants   grantStore
	now      func() time.Time
	newToken func() (string, error)
	newUUID  func() string
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	repo := internalrepo.New(db)
	return &Service{db: repo, grants: repo, now: time.Now, newToken: resourcegrant.NewToken, newUUID: uuid.NewString}
}

// CreateResourceGrant creates a grant for userUUID to do scope, once, on the
// resource, for 15 minutes. identity.ErrUserNotFound when there is no such
// User; identity.ErrTooManyResourceGrants when they already hold 20 live
// ones.
func (s *Service) CreateResourceGrant(ctx context.Context, userUUID string, kind string, resourceID string, scope string) (identity.CreatedResourceGrant, error) {
	defer logging.Step(ctx, "resourcegrants.create").Close()
	if !resourcelink.ValidResource(kind, resourceID) || !resourcegrant.ValidScope(scope) {
		return identity.CreatedResourceGrant{}, fmt.Errorf("invalid grant %q %q %q", kind, resourceID, scope)
	}
	token, err := s.newToken()
	if err != nil {
		return identity.CreatedResourceGrant{}, err
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	created := identity.CreatedResourceGrant{GrantUUID: s.newUUID(), Token: token, ExpiresAt: now.Add(resourcegrant.Lifetime)}
	_, err = utils.RunInDBTransaction(ctx, s.db, func(ctx context.Context, tx *gorm.DB) (struct{}, error) {
		userID, found, err := s.grants.LockUser(ctx, tx, userUUID)
		if err != nil {
			return struct{}{}, err
		}
		if !found {
			return struct{}{}, identity.ErrUserNotFound
		}
		if err := s.grants.DeleteExpiredGrants(ctx, tx, userID, now.Add(-expiredRetention)); err != nil {
			return struct{}{}, err
		}
		live, err := s.grants.CountLiveGrants(ctx, tx, userID, now)
		if err != nil {
			return struct{}{}, err
		}
		if live >= resourcegrant.MaxLive {
			return struct{}{}, identity.ErrTooManyResourceGrants
		}
		return struct{}{}, s.grants.CreateGrant(ctx, tx, created.GrantUUID, userID, kind, resourceID, scope, resourcegrant.Hash(token), now, created.ExpiresAt)
	})
	if errors.Is(err, identity.ErrTooManyResourceGrants) {
		grantsTotal.Inc(ctx, operationLabel.Value("refused_too_many"))
	}
	if err != nil {
		return identity.CreatedResourceGrant{}, err
	}
	grantsTotal.Inc(ctx, operationLabel.Value("created"))
	logging.LogFields(ctx, logging.Field("resource_grant_uuid", created.GrantUUID))
	return created, nil
}

// ClaimResourceGrant uses token's grant if it is live, answering what it
// allows; a token is claimed at most once until released. The token itself
// is never logged.
func (s *Service) ClaimResourceGrant(ctx context.Context, token string) (identity.ClaimedResourceGrant, error) {
	claimed := identity.ClaimedResourceGrant{Outcome: identity.ResourceGrantUnknown}
	if resourcegrant.WellFormed(token) {
		hash := resourcegrant.Hash(token)
		grant, err := s.grants.UseGrant(ctx, hash, s.now().UTC())
		if err != nil {
			return identity.ClaimedResourceGrant{}, err
		}
		if grant != nil {
			claimed = identity.ClaimedResourceGrant{Outcome: identity.ResourceGrantClaimed, GrantUUID: grant.UUID, UserUUID: grant.UserUUID, Kind: grant.Kind, ResourceID: grant.ResourceID, Scope: grant.Scope}
		} else {
			// Not claimable: say why.
			grant, err = s.grants.FindGrantByHash(ctx, hash)
			if err != nil {
				return identity.ClaimedResourceGrant{}, err
			}
			switch {
			case grant == nil:
			case grant.UsedAt != nil:
				claimed.Outcome = identity.ResourceGrantUsed
			default:
				claimed.Outcome = identity.ResourceGrantExpired
			}
		}
	}
	operation := "claim_" + string(claimed.Outcome)
	if claimed.Outcome == identity.ResourceGrantClaimed {
		operation = "claimed"
	}
	grantsTotal.Inc(ctx, operationLabel.Value(operation))
	logging.LogFields(ctx, logging.Field("resource_grant", string(claimed.Outcome)), logging.Field("resource_grant_uuid", claimed.GrantUUID))
	return claimed, nil
}

// ReleaseResourceGrant makes a claimed grant usable again, for a use its
// holder could not complete (the upload broke, the service was busy). It
// still expires when it would have.
func (s *Service) ReleaseResourceGrant(ctx context.Context, grantUUID string) error {
	released, err := s.grants.ReleaseGrant(ctx, grantUUID)
	if err != nil {
		return err
	}
	if released {
		grantsTotal.Inc(ctx, operationLabel.Value("released"))
	}
	return nil
}
