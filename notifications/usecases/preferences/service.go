// Package preferences reads and appends a user's optional email choices.
// Necessary notifications have no switch; marketing is only effective with
// the marketing acceptance, which acceptance checks per notice.
package preferences

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/notifications"
	"github.com/diegobermudez03/playhoot/notifications/internal/prefs"
	internalrepo "github.com/diegobermudez03/playhoot/notifications/internal/repo"
)

var changesTotal = metrics.NewCounter("notifications_preference_changes_total", "Preference rows appended.")

// Service reads and sets preferences.
type Service struct {
	repo *internalrepo.Repo
	now  func() time.Time
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{repo: internalrepo.New(db), now: time.Now}
}

// ReadPreferences returns userUUID's current preferences.
func (s *Service) ReadPreferences(ctx context.Context, userUUID string) (notifications.Preferences, error) {
	defer logging.Step(ctx, "notifications.read_preferences").Close()
	return prefs.Current(ctx, s.repo, s.repo.GetDB(), userUUID)
}

// SetPreferences appends p as userUUID's current preferences; saving the
// same values again appends nothing.
func (s *Service) SetPreferences(ctx context.Context, userUUID string, p notifications.Preferences) (notifications.Preferences, error) {
	defer logging.Step(ctx, "notifications.set_preferences").Close()
	db := s.repo.GetDB()
	current, err := prefs.Current(ctx, s.repo, db, userUUID)
	if err != nil || current == p {
		return current, err
	}
	row := internalrepo.Preferences{
		UserUUID: userUUID, AIEmail: p.AI, CapacityEmail: p.Capacity, ReviewEmail: p.Review, MarketingEmail: p.Marketing, CreatedAt: s.now().UTC(),
	}
	if err := s.repo.InsertPreferences(ctx, db, &row); err != nil {
		return notifications.Preferences{}, err
	}
	changesTotal.Inc(ctx)
	return p, nil
}
