// Package refreshcredentials renews a client's access token with its refresh
// token, rotating the refresh token only when it is old so a session writes
// rarely.
package refreshcredentials

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/accesstoken"
	"github.com/diegobermudez03/playhoot/identity/internal/business/refreshtoken"
	internalrepo "github.com/diegobermudez03/playhoot/identity/usecases/refreshcredentials/internal/repo"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=refreshcredentials -destination=service_mock_test.go . sessionStore,accessIssuer

// sessionStore reads and rotates refresh tokens inside a transaction.
type sessionStore interface {
	LockSessionByRefreshToken(ctx context.Context, tx *gorm.DB, refreshToken string) (*internalrepo.Session, error)
	LockSessionByID(ctx context.Context, tx *gorm.DB, id int64) (*internalrepo.Session, error)
	RotateSession(ctx context.Context, tx *gorm.DB, current internalrepo.Session, refreshToken string, issuedAt time.Time, expiresAt *time.Time) error
}

// accessIssuer issues access tokens.
type accessIssuer interface {
	Issue(user identity.User, issuedAt time.Time) (string, time.Time, error)
}

var (
	outcomeLabel = metrics.NewLabel("outcome", "renewed", "rotated", "previous", "invalid")

	refreshesTotal = metrics.NewCounter(
		"identity_refreshes_total",
		"Refresh requests, by outcome: renewed with the same refresh token, rotated, renewed with the token just replaced, or invalid.",
		outcomeLabel,
	)
)

// Service is Identity's refresh use case.
type Service struct {
	sessions   sessionStore
	db         utils.DBServicer
	access     accessIssuer
	now        func() time.Time
	newRefresh func() (string, error)
}

// New constructs a Service on db, signing access tokens with accessKey.
func New(db *gorm.DB, accessKey []byte) (*Service, error) {
	signer, err := accesstoken.NewSigner(accessKey)
	if err != nil {
		return nil, err
	}
	repo := internalrepo.New(db)
	return &Service{sessions: repo, db: repo, access: signer, now: time.Now, newRefresh: refreshtoken.New}, nil
}

// renewal is what a refresh decided inside its transaction: the session's
// User and the refresh token the client keeps. Empty token: invalid.
type renewal struct {
	user         identity.User
	refreshToken string
	outcome      string
}

// Refresh renews the credentials of refreshToken: a new access token, and the
// refresh token to keep (the same one, or its successor once rotated).
func (s *Service) Refresh(ctx context.Context, refreshToken string) (identity.RefreshResult, error) {
	now := s.now().UTC().Truncate(time.Microsecond)
	renewed, err := utils.RunInDBTransaction(ctx, s.db, func(ctx context.Context, tx *gorm.DB) (renewal, error) {
		return s.refreshInTx(ctx, tx, refreshToken, now)
	})
	if err != nil {
		return identity.RefreshResult{}, err
	}
	refreshesTotal.Inc(ctx, outcomeLabel.Value(renewed.outcome))
	if renewed.refreshToken == "" {
		return identity.RefreshResult{Outcome: identity.RefreshInvalid}, nil
	}
	accessToken, expiresAt, err := s.access.Issue(renewed.user, now)
	if err != nil {
		return identity.RefreshResult{}, err
	}
	return identity.RefreshResult{
		Outcome: identity.RefreshRenewed,
		Credentials: identity.Credentials{
			User:            renewed.user,
			AccessToken:     accessToken,
			AccessIssuedAt:  now,
			AccessExpiresAt: expiresAt,
			RefreshToken:    renewed.refreshToken,
		},
	}, nil
}

// refreshInTx decides under the presented row's lock, so two renewals of the
// same token never rotate it twice: the second sees it replaced and gets the
// successor through the previous token's grace.
func (s *Service) refreshInTx(ctx context.Context, tx *gorm.DB, refreshToken string, now time.Time) (renewal, error) {
	invalid := renewal{outcome: "invalid"}
	session, err := s.sessions.LockSessionByRefreshToken(ctx, tx, refreshToken)
	if err != nil || session == nil {
		return invalid, err
	}

	if session.DeactivatedAt == nil {
		if refreshtoken.Expired(session.ExpiresAt, now) {
			return invalid, nil
		}
		if !refreshtoken.ShouldRotate(session.IssuedAt, now) {
			return renewal{user: userOf(*session), refreshToken: session.RefreshToken, outcome: "renewed"}, nil
		}
		successor, err := s.newRefresh()
		if err != nil {
			return renewal{}, err
		}
		user := userOf(*session)
		expiresAt := refreshtoken.ExpiresAt(user.Level, now, session.StartedAt)
		if err := s.sessions.RotateSession(ctx, tx, *session, successor, now, expiresAt); err != nil {
			return renewal{}, err
		}
		return renewal{user: user, refreshToken: successor, outcome: "rotated"}, nil
	}

	// Not active: only the token just replaced still works, briefly. A signed
	// out session has no successor; any older token is refused, nothing else
	// changes.
	if session.ReplacedByID == nil || !refreshtoken.WithinPreviousGrace(*session.DeactivatedAt, now) {
		return invalid, nil
	}
	successor, err := s.sessions.LockSessionByID(ctx, tx, *session.ReplacedByID)
	if err != nil || successor == nil {
		return invalid, err
	}
	if successor.DeactivatedAt != nil || refreshtoken.Expired(successor.ExpiresAt, now) {
		return invalid, nil
	}
	return renewal{user: userOf(*successor), refreshToken: successor.RefreshToken, outcome: "previous"}, nil
}

func userOf(session internalrepo.Session) identity.User {
	level := identity.LevelGuest
	if session.HasAccount {
		level = identity.LevelAccount
	}
	return identity.User{UUID: session.UserUUID, DisplayName: session.DisplayName, Level: level}
}
