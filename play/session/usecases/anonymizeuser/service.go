// Package anonymizeuser takes a deleted person's name out of every roster
// they appear in. The rows stay: a game is still read turn by turn, and the
// participant identifier still tells two deleted players apart within it.
package anonymizeuser

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
)

var clearedTotal = metrics.NewCounter(
	"play_session_participant_names_cleared_total",
	"Roster name snapshots cleared because the person's identity was deleted.",
)

// Service clears the name snapshots of a deleted person.
type Service struct {
	db *gorm.DB
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{db: db}
}

// AnonymizeUser clears the name snapshot userUUID left on every roster,
// answering how many it cleared. A snapshot already cleared stays cleared,
// so running it again clears nothing new.
func (s *Service) AnonymizeUser(ctx context.Context, userUUID string) (int64, error) {
	defer logging.Step(ctx, "play.session.anonymize_user").Close()

	result := s.db.WithContext(ctx).Exec(`
		UPDATE play_session_participants SET display_name = ''
		WHERE user_uuid = ? AND display_name <> ''`,
		userUUID,
	)
	if result.Error != nil {
		return 0, fmt.Errorf("clearing the roster names of a deleted person: %s", result.Error)
	}
	if result.RowsAffected > 0 {
		clearedTotal.Add(ctx, float64(result.RowsAffected))
	}
	logging.LogFields(ctx, logging.Field("participant_names_cleared", result.RowsAffected))
	return result.RowsAffected, nil
}
