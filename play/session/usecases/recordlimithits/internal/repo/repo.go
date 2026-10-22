// Package repo is recordlimithits's persistence: it adds limit hits to the
// Sessions' audit records and decides nothing itself.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// Repo is recordlimithits's persistence implementation.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo bound to db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// Hit is count refusals of limit for userUUID in sessionUUID.
type Hit struct {
	SessionUUID string
	UserUUID    string
	Limit       string
	Count       int
}

// RecordHits adds every hit to its Session and user's SERVER_LIMIT record,
// creating it with the Session's version and the user's participant id when
// they have one; each limit keeps its own count in the record's detail. A hit
// of a Session that does not exist is skipped. recorded is how many were
// stored.
func (r *Repo) RecordHits(ctx context.Context, hits []Hit, recordedAt time.Time) (recorded int, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		recorded = 0
		for _, hit := range hits {
			result := tx.Exec(`
				INSERT INTO play_game_faults (session_id, game_version_id, user_uuid, participant_id, source, kind, detail,
					occurrences, first_reported_at, last_reported_at)
				SELECT s.id, s.game_version_id, ?, p.participant_id, 'SERVER', 'SERVER_LIMIT',
					jsonb_build_object('limits', jsonb_build_object(?::text, ?::bigint)), ?, ?, ?
				FROM play_sessions s
				LEFT JOIN play_session_participants p ON p.session_id = s.id AND p.user_uuid = ?
				WHERE s.uuid = ?
				ON CONFLICT (session_id, user_uuid, kind) DO UPDATE SET
					occurrences = play_game_faults.occurrences + EXCLUDED.occurrences,
					participant_id = COALESCE(play_game_faults.participant_id, EXCLUDED.participant_id),
					detail = jsonb_set(play_game_faults.detail, ARRAY['limits', ?::text],
						to_jsonb(COALESCE((play_game_faults.detail->'limits'->>?::text)::bigint, 0) + ?::bigint)),
					last_reported_at = EXCLUDED.last_reported_at
			`, hit.UserUUID, hit.Limit, hit.Count, hit.Count, recordedAt, recordedAt, hit.UserUUID, hit.SessionUUID,
				hit.Limit, hit.Limit, hit.Count)
			if result.Error != nil {
				return fmt.Errorf("recording limit hits: %w", result.Error)
			}
			recorded += int(result.RowsAffected)
		}
		return nil
	})
	return recorded, err
}
