// Package signinmethods tells an account which ways in it has and takes one
// away, keeping at least one so the account stays reachable.
package signinmethods

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/identity/internal/business/method"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/utils"
)

var (
	removeOutcomeLabel = metrics.NewLabel("outcome", string(identity.MethodRemoved), string(identity.MethodUnknown), string(identity.MethodLastOne))

	removalsTotal = metrics.NewCounter(
		"identity_method_removals_total",
		"Sign-in methods a person asked to unlink, by what was decided.",
		removeOutcomeLabel,
	)
)

// Service reads and removes sign-in methods.
type Service struct {
	db  *gorm.DB
	now func() time.Time
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{db: db, now: time.Now}
}

// GetDB returns the database, for transactions.
func (s *Service) GetDB() *gorm.DB {
	return s.db
}

type methodRow struct {
	ID        int64
	Issuer    string
	Subject   string
	Email     string
	Name      string
	CreatedAt time.Time
}

// ListMethods returns the live ways into userUUID's account, oldest first.
func (s *Service) ListMethods(ctx context.Context, userUUID string) ([]identity.SignInMethod, error) {
	defer logging.Step(ctx, "identity.list_methods").Close()

	rows, err := s.live(ctx, s.db, userUUID, false)
	if err != nil {
		return nil, err
	}
	methods := make([]identity.SignInMethod, 0, len(rows))
	for _, row := range rows {
		address := row.Email
		if identity.Provider(row.Issuer) == identity.ProviderEmail {
			address = row.Subject
		}
		methods = append(methods, identity.SignInMethod{
			MethodID: method.ID(row.ID),
			Issuer:   identity.Provider(row.Issuer),
			Address:  address,
			Name:     row.Name,
			AddedAt:  row.CreatedAt,
		})
	}
	return methods, nil
}

// RemoveMethod unlinks methodID from userUUID's account, unless it is the
// only way in left.
func (s *Service) RemoveMethod(ctx context.Context, userUUID string, methodID string) (identity.RemoveMethodOutcome, error) {
	defer logging.Step(ctx, "identity.remove_method").Close()

	outcome, err := s.remove(ctx, userUUID, methodID)
	if err != nil {
		return "", err
	}
	removalsTotal.Inc(ctx, removeOutcomeLabel.Value(string(outcome)))
	logging.LogFields(ctx, logging.Field("remove_method", string(outcome)))
	return outcome, nil
}

func (s *Service) remove(ctx context.Context, userUUID string, methodID string) (identity.RemoveMethodOutcome, error) {
	wanted, ok := method.Parse(methodID)
	if !ok {
		return identity.MethodUnknown, nil
	}
	return utils.RunInDBTransaction(ctx, s, func(ctx context.Context, tx *gorm.DB) (identity.RemoveMethodOutcome, error) {
		rows, err := s.live(ctx, tx, userUUID, true)
		if err != nil {
			return "", err
		}
		found := false
		for _, row := range rows {
			if row.ID == wanted {
				found = true
			}
		}
		if !found {
			return identity.MethodUnknown, nil
		}
		if len(rows) == 1 {
			return identity.MethodLastOne, nil
		}
		err = tx.WithContext(ctx).Exec(`
			UPDATE identity_external_identities SET removed_at = ? WHERE id = ? AND removed_at IS NULL`,
			s.now().UTC(), wanted,
		).Error
		if err != nil {
			return "", fmt.Errorf("removing a sign-in method: %w", err)
		}
		return identity.MethodRemoved, nil
	})
}

// live returns the account's methods that have not been removed. lock holds
// them for the rest of the transaction, so two removals at once cannot both
// believe another method is left.
func (s *Service) live(ctx context.Context, handle *gorm.DB, userUUID string, lock bool) ([]methodRow, error) {
	query := `
		SELECT bound.id, bound.issuer, bound.subject, COALESCE(bound.email, '') AS email, COALESCE(bound.name, '') AS name, bound.created_at
		FROM identity_external_identities bound JOIN identity_users users ON users.id = bound.user_id
		WHERE users.user_uuid = ? AND bound.removed_at IS NULL
		ORDER BY bound.id`
	if lock {
		query += ` FOR UPDATE OF bound`
	}
	var rows []methodRow
	err := handle.WithContext(ctx).Raw(query, userUUID).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("reading sign-in methods: %w", err)
	}
	return rows, nil
}
