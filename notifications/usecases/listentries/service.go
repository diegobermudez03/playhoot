// Package listentries reads a user's notifications with filters and the
// unread count. Listing marks nothing read.
package listentries

import (
	"context"
	"encoding/json"
	"fmt"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/notifications"
	internalrepo "github.com/diegobermudez03/playhoot/notifications/internal/repo"
)

// Service lists entries.
type Service struct {
	repo *internalrepo.Repo
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{repo: internalrepo.New(db)}
}

// ListEntries returns userUUID's in-app entries, newest first, and the
// unread count.
func (s *Service) ListEntries(ctx context.Context, userUUID string, filter notifications.EntryFilter) (notifications.EntryPage, error) {
	defer logging.Step(ctx, "notifications.list_entries").Close()
	limit := filter.Limit
	if limit <= 0 {
		limit = notifications.DefaultListLimit
	}
	if limit > notifications.MaxListLimit {
		limit = notifications.MaxListLimit
	}
	db := s.repo.GetDB()
	rows, err := s.repo.ListEntries(ctx, db, userUUID, internalrepo.EntryFilter{
		SourceDomain: filter.SourceDomain, Type: string(filter.Type), From: filter.From, To: filter.To, Before: filter.Before, Limit: limit,
	})
	if err != nil {
		return notifications.EntryPage{}, err
	}
	unread, err := s.repo.UnreadCount(ctx, db, userUUID)
	if err != nil {
		return notifications.EntryPage{}, err
	}
	page := notifications.EntryPage{Entries: make([]notifications.Entry, len(rows)), Unread: unread}
	for i, row := range rows {
		skeleton := map[string]any{}
		if err := json.Unmarshal([]byte(row.Skeleton), &skeleton); err != nil {
			return notifications.EntryPage{}, fmt.Errorf("reading skeleton of %s: %w", row.EntryUUID, err)
		}
		page.Entries[i] = notifications.Entry{
			EntryUUID: row.EntryUUID, SourceDomain: row.SourceDomain, Type: notifications.Type(row.Type), ExternalToken: row.ExternalToken,
			Skeleton: skeleton, Email: row.EmailChannel, Necessary: row.Necessary, ReadAt: row.ReadAt, CreatedAt: row.CreatedAt,
		}
	}
	return page, nil
}
