// Package repo persists Analytics' event store.
package repo

import (
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Event is an analytics_events row. Properties holds a JSON object.
type Event struct {
	ID             int64
	EventType      string
	UserRef        string
	AccountKind    string
	PlanKind       string
	EntityRef      string
	Properties     string
	SourceDomain   string
	IdempotencyKey string
	OccurredAt     time.Time
	CreatedAt      time.Time
}

func (Event) TableName() string { return "analytics_events" }

// Repo persists the event store on one database.
type Repo struct {
	db *gorm.DB
}

// New constructs a Repo on db.
func New(db *gorm.DB) *Repo {
	return &Repo{db: db}
}

// InsertEvents stores events in one statement, skipping those whose source
// domain and key were stored before, and reports how many were new.
func (r *Repo) InsertEvents(ctx context.Context, events []Event) (int, error) {
	if len(events) == 0 {
		return 0, nil
	}
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&events)
	if result.Error != nil {
		return 0, fmt.Errorf("inserting analytics events: %s", result.Error)
	}
	return int(result.RowsAffected), nil
}
