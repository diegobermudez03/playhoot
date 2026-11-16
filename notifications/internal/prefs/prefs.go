// Package prefs reads a user's current email preferences, the defaults
// until they save their own.
package prefs

import (
	"context"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/notifications"
	internalrepo "github.com/diegobermudez03/playhoot/notifications/internal/repo"
)

// Current returns userUUID's latest preferences or the defaults.
func Current(ctx context.Context, repo *internalrepo.Repo, tx *gorm.DB, userUUID string) (notifications.Preferences, error) {
	row, err := repo.LatestPreferences(ctx, tx, userUUID)
	if err != nil {
		return notifications.Preferences{}, err
	}
	if row == nil {
		return notifications.DefaultPreferences, nil
	}
	return notifications.Preferences{AI: row.AIEmail, Capacity: row.CapacityEmail, Review: row.ReviewEmail, Marketing: row.MarketingEmail}, nil
}
