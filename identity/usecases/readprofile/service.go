// Package readprofile tells a person who they are on Playhoot: the name
// others see, whether they are a guest or an account, the mode and language
// of that account, and the ways into it.
package readprofile

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
)

//go:generate mockgen -package=readprofile -destination=service_mock_test.go . methodLister

// methodLister lists the live ways into an account.
type methodLister interface {
	ListMethods(ctx context.Context, userUUID string) ([]identity.SignInMethod, error)
}

// Service reads a person's own profile.
type Service struct {
	db      *gorm.DB
	methods methodLister
}

// New constructs a Service on db, listing methods with methods.
func New(db *gorm.DB, methods methodLister) *Service {
	return &Service{db: db, methods: methods}
}

type profileRow struct {
	DisplayName string
	Language    string
	HasAccount  bool
	Mode        string
}

// ReadProfile returns who userUUID is. identity.ErrUserNotFound when there
// is no such User. A guest is normal, has no language of their own and no
// methods: their only way in is the credential their browser keeps.
func (s *Service) ReadProfile(ctx context.Context, userUUID string) (identity.Profile, error) {
	defer logging.Step(ctx, "identity.read_profile").Close()

	if _, err := uuid.Parse(userUUID); err != nil {
		return identity.Profile{}, identity.ErrUserNotFound
	}
	var rows []profileRow
	err := s.db.WithContext(ctx).Raw(`
		SELECT COALESCE(users.display_name, '') AS display_name, users.interface_language AS language,
			account.id IS NOT NULL AS has_account, COALESCE(account.mode, '') AS mode
		FROM identity_users users
		LEFT JOIN identity_accounts account ON account.user_id = users.id
		WHERE users.user_uuid = ?`,
		userUUID,
	).Scan(&rows).Error
	if err != nil {
		return identity.Profile{}, fmt.Errorf("reading profile: %s", err)
	}
	if len(rows) == 0 {
		return identity.Profile{}, identity.ErrUserNotFound
	}
	row := rows[0]
	profile := identity.Profile{
		UUID:        userUUID,
		DisplayName: row.DisplayName,
		Level:       identity.LevelGuest,
		Mode:        identity.ModeNormal,
		Methods:     []identity.SignInMethod{},
	}
	if !row.HasAccount {
		return profile, nil
	}
	profile.Level = identity.LevelAccount
	profile.Mode = identity.NormalizeMode(identity.Mode(row.Mode))
	profile.Language = row.Language
	methods, err := s.methods.ListMethods(ctx, userUUID)
	if err != nil {
		return identity.Profile{}, err
	}
	if methods != nil {
		profile.Methods = methods
	}
	return profile, nil
}
