// Package readcontact tells other domains, through the orchestrator, how a
// User is reached and who the administrator is.
package readcontact

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
)

// Service reads contacts.
type Service struct {
	db *gorm.DB
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{db: db}
}

// noEmail is the stored value of a provider link that observed no address.
const noEmail = ""

type contactRow struct {
	Account bool
	Email   *string
}

// ReadContact returns how recipient is reached. It is a User's UUID, whose
// address is the one its latest live sign-in method observed; or the
// identifier of a one-time email code attempt, which is reached at the
// address the code was asked for, in the language it was asked in, although
// nobody has a User yet.
func (s *Service) ReadContact(ctx context.Context, recipient string) (identity.Contact, error) {
	defer logging.Step(ctx, "identity.read_contact").Close()
	if _, err := uuid.Parse(recipient); err != nil {
		return identity.Contact{}, nil
	}
	var rows []contactRow
	err := s.db.WithContext(ctx).Raw(`SELECT a.id IS NOT NULL AS account,
			(SELECT e.email FROM identity_external_identities e
				WHERE e.user_id = u.id AND e.removed_at IS NULL AND e.email <> ? ORDER BY e.id DESC LIMIT 1) AS email
		FROM identity_users u LEFT JOIN identity_accounts a ON a.user_id = u.id
		WHERE u.user_uuid = ?`, noEmail, recipient).Scan(&rows).Error
	if err != nil {
		return identity.Contact{}, fmt.Errorf("reading contact: %w", err)
	}
	if len(rows) == 0 {
		return s.readAttemptContact(ctx, recipient)
	}
	contact := identity.Contact{Found: true, Account: rows[0].Account}
	if rows[0].Account && rows[0].Email != nil {
		contact.Email = *rows[0].Email
	}
	return contact, nil
}

type attemptRow struct {
	Email    string
	Language string
}

// readAttemptContact resolves an email-code attempt to where its code goes.
// Not found when no attempt has that identifier either.
func (s *Service) readAttemptContact(ctx context.Context, attemptUUID string) (identity.Contact, error) {
	var rows []attemptRow
	err := s.db.WithContext(ctx).Raw(`SELECT email, language FROM identity_email_codes WHERE attempt_uuid = ?`, attemptUUID).
		Scan(&rows).Error
	if err != nil {
		return identity.Contact{}, fmt.Errorf("reading an access code's contact: %w", err)
	}
	if len(rows) == 0 {
		return identity.Contact{}, nil
	}
	return identity.Contact{Found: true, Email: rows[0].Email, Language: rows[0].Language}, nil
}

// AdminUserUUID returns the account holding the active administrator
// grant; found is false while there is none.
func (s *Service) AdminUserUUID(ctx context.Context) (userUUID string, found bool, err error) {
	defer logging.Step(ctx, "identity.read_admin").Close()
	var uuids []string
	err = s.db.WithContext(ctx).Raw(`SELECT u.user_uuid FROM identity_admin_grants g JOIN identity_users u ON u.id = g.user_id
		WHERE g.revoked_at IS NULL ORDER BY g.id DESC LIMIT 1`).Scan(&uuids).Error
	if err != nil {
		return "", false, fmt.Errorf("reading administrator: %w", err)
	}
	if len(uuids) == 0 {
		return "", false, nil
	}
	return uuids[0], true, nil
}
