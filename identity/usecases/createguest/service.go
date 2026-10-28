// Package createguest creates guest Users: people playing without an account,
// who get credentials of their own that live in their browser.
package createguest

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/accesstoken"
	"github.com/diegobermudez03/playhoot/identity/internal/business/displayname"
	"github.com/diegobermudez03/playhoot/identity/internal/business/refreshtoken"
	internalrepo "github.com/diegobermudez03/playhoot/identity/usecases/createguest/internal/repo"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=createguest -destination=service_mock_test.go . guestStore,accessIssuer

// guestStore persists guest creation.
type guestStore interface {
	ClaimGuestRequest(ctx context.Context, tx *gorm.DB, guest internalrepo.NewGuest) (int64, *internalrepo.GuestRequest, error)
	CreateGuestUser(ctx context.Context, tx *gorm.DB, guest internalrepo.NewGuest) error
}

// accessIssuer issues access tokens.
type accessIssuer interface {
	Issue(user identity.User, issuedAt time.Time) (string, time.Time, error)
}

// ReplayWindow is how long the same idempotency key and name get the guest
// they created again; after it, the key is spent.
const ReplayWindow = 5 * time.Minute

var (
	outcomeLabel = metrics.NewLabel("outcome", "created", "replayed", "invalid_display_name", "conflict", "spent")

	creationsTotal = metrics.NewCounter(
		"identity_guest_creations_total",
		"Guest creation requests, by outcome.",
		outcomeLabel,
	)
)

// Service is Identity's guest creation use case.
type Service struct {
	db     utils.DBServicer
	guests guestStore
	access accessIssuer
	now    func() time.Time
}

// New constructs a Service on db, signing access tokens with accessKey.
func New(db *gorm.DB, accessKey []byte) (*Service, error) {
	signer, err := accesstoken.NewSigner(accessKey)
	if err != nil {
		return nil, err
	}
	repo := internalrepo.New(db)
	return &Service{db: repo, guests: repo, access: signer, now: time.Now}, nil
}

// CreateGuest creates a guest User named displayName, idempotently per
// idempotencyKey (a UUID), and returns their credentials. The same key and
// name within ReplayWindow, while the guest's session is active, gets the same
// guest; later the key is spent. A reused key with another name returns
// identity.ErrIdempotencyConflict.
func (s *Service) CreateGuest(ctx context.Context, displayName string, idempotencyKey string) (identity.CreateGuestResult, error) {
	name, ok := displayname.Normalize(displayName)
	if !ok {
		creationsTotal.Inc(ctx, outcomeLabel.Value("invalid_display_name"))
		return identity.CreateGuestResult{Outcome: identity.GuestInvalidDisplayName}, nil
	}
	key, err := uuid.Parse(idempotencyKey)
	if err != nil {
		return identity.CreateGuestResult{}, errors.New("idempotency key must be a UUID")
	}
	now := s.now().UTC().Truncate(time.Microsecond)
	return utils.RunInDBTransaction(ctx, s.db, func(ctx context.Context, tx *gorm.DB) (identity.CreateGuestResult, error) {
		return s.createInTx(ctx, tx, name, key.String(), now)
	})
}

// createInTx claims the key and creates the guest, or answers the request
// that claimed it before.
func (s *Service) createInTx(ctx context.Context, tx *gorm.DB, name string, key string, now time.Time) (identity.CreateGuestResult, error) {
	refreshToken, err := refreshtoken.New()
	if err != nil {
		return identity.CreateGuestResult{}, err
	}
	guest := internalrepo.NewGuest{
		IdempotencyKey: key,
		UserUUID:       uuid.NewString(),
		DisplayName:    name,
		SessionUUID:    uuid.NewString(),
		RefreshToken:   refreshToken,
		RequestedAt:    now,
	}
	_, existing, err := s.guests.ClaimGuestRequest(ctx, tx, guest)
	if err != nil {
		return identity.CreateGuestResult{}, err
	}
	if existing == nil {
		if err := s.guests.CreateGuestUser(ctx, tx, guest); err != nil {
			return identity.CreateGuestResult{}, err
		}
		creationsTotal.Inc(ctx, outcomeLabel.Value("created"))
		user := identity.User{UUID: guest.UserUUID, DisplayName: name, Level: identity.LevelGuest}
		return s.credentials(user, refreshToken, now)
	}

	// The key was claimed before: the same request again, or another one.
	if existing.DisplayName != name {
		creationsTotal.Inc(ctx, outcomeLabel.Value("conflict"))
		return identity.CreateGuestResult{}, identity.ErrIdempotencyConflict
	}
	if now.Sub(existing.RequestedAt) >= ReplayWindow || existing.ActiveRefreshToken == nil {
		creationsTotal.Inc(ctx, outcomeLabel.Value("spent"))
		return identity.CreateGuestResult{Outcome: identity.GuestIdempotencyKeySpent}, nil
	}
	creationsTotal.Inc(ctx, outcomeLabel.Value("replayed"))
	user := identity.User{UUID: existing.UserUUID, DisplayName: name, Level: identity.LevelGuest}
	return s.credentials(user, *existing.ActiveRefreshToken, now)
}

func (s *Service) credentials(user identity.User, refreshToken string, now time.Time) (identity.CreateGuestResult, error) {
	accessToken, expiresAt, err := s.access.Issue(user, now)
	if err != nil {
		return identity.CreateGuestResult{}, err
	}
	return identity.CreateGuestResult{
		Outcome: identity.GuestCreated,
		Credentials: identity.Credentials{
			User:            user,
			AccessToken:     accessToken,
			AccessIssuedAt:  now,
			AccessExpiresAt: expiresAt,
			RefreshToken:    refreshToken,
		},
	}, nil
}
