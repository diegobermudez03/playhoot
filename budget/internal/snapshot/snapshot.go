// Package snapshot reads Budget's numbers at one instant: the ladder, the
// caps in force, every scope's window usage and the categories missing
// rates. Admission, ladder evaluation and status all decide from it.
package snapshot

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/budget/internal/business/controls"
	internalrepo "github.com/diegobermudez03/playhoot/budget/internal/repo"
)

// Snapshot is Budget at one instant.
type Snapshot struct {
	Now         time.Time
	Ladder      budget.LadderState
	LadderRowID int64
	Caps        []budget.Cap
	GlobalCaps  controls.Caps
	CategoryCap map[budget.Category]controls.Caps
	Categories  map[budget.Category]controls.Usage
	Global      controls.Usage
	// Measured and Estimated split each scope's month spend; MeasuredDay
	// and MeasuredHour the day and hour spend.
	Measured    map[string]map[budget.Window]float64
	RateMissing []budget.Category
	LastHourUSD float64
}

// CategoryNames is every category as stored.
func CategoryNames() []string {
	names := make([]string, len(budget.Categories))
	for i, c := range budget.Categories {
		names[i] = string(c)
	}
	return names
}

// Load reads the snapshot at now on db (a transaction holding the budget
// lock when the caller decides from it).
func Load(ctx context.Context, repo *internalrepo.Repo, db *gorm.DB, now time.Time) (Snapshot, error) {
	now = now.UTC()
	s := Snapshot{Now: now, CategoryCap: map[budget.Category]controls.Caps{}, Categories: map[budget.Category]controls.Usage{}, GlobalCaps: controls.Caps{}, Measured: map[string]map[budget.Window]float64{}}
	ladder, rowID, err := LoadLadder(ctx, repo, db)
	if err != nil {
		return s, err
	}
	s.Ladder, s.LadderRowID = ladder, rowID

	caps, err := repo.CapsAt(ctx, db, now)
	if err != nil {
		return s, err
	}
	for _, row := range caps {
		c := budget.Cap{Scope: row.Scope, Window: budget.Window(row.CapWindow), USDLimit: row.USDLimit}
		if row.MaxConcurrency != nil {
			c.MaxConcurrency = *row.MaxConcurrency
		}
		if row.MaxVolume != nil {
			c.MaxVolume = *row.MaxVolume
		}
		s.Caps = append(s.Caps, c)
		if c.Scope == budget.ScopeGlobal {
			s.GlobalCaps[c.Window] = c
			continue
		}
		category := budget.Category(c.Scope)
		if s.CategoryCap[category] == nil {
			s.CategoryCap[category] = controls.Caps{}
		}
		s.CategoryCap[category][c.Window] = c
	}

	starts := controls.Starts(now)
	names := CategoryNames()
	spend, err := repo.SpendTotals(ctx, db, names, starts[budget.WindowMonth], starts[budget.WindowDay], starts[budget.WindowHour], now.Add(-time.Hour))
	if err != nil {
		return s, err
	}
	reservations, err := repo.ReservationTotals(ctx, db, names, now, starts[budget.WindowMonth], starts[budget.WindowDay], starts[budget.WindowHour])
	if err != nil {
		return s, err
	}
	for _, c := range budget.Categories {
		s.Categories[c] = controls.Usage{Spend: map[budget.Window]float64{}, Started: map[budget.Window]int{}}
	}
	s.Measured[budget.ScopeGlobal] = map[budget.Window]float64{}
	for _, row := range spend {
		category := budget.Category(row.Category)
		usage := s.Categories[category]
		usage.Spend[budget.WindowMonth] = row.Month
		usage.Spend[budget.WindowDay] = row.Day
		usage.Spend[budget.WindowHour] = row.Hour
		s.LastHourUSD += row.Trailing
		s.Measured[row.Category] = map[budget.Window]float64{budget.WindowMonth: row.MeasuredMonth, budget.WindowDay: row.MeasuredDay, budget.WindowHour: row.MeasuredHour}
		for w, v := range s.Measured[row.Category] {
			s.Measured[budget.ScopeGlobal][w] += v
		}
	}
	for _, row := range reservations {
		category := budget.Category(row.Category)
		usage := s.Categories[category]
		usage.Reserved = row.Reserved
		usage.Active = row.Active
		usage.Started[budget.WindowMonth] = row.StartedMonth
		usage.Started[budget.WindowDay] = row.StartedDay
		usage.Started[budget.WindowHour] = row.StartedHour
		s.Categories[category] = usage
	}
	s.Global = controls.Usage{Spend: map[budget.Window]float64{}, Started: map[budget.Window]int{}}
	for _, c := range budget.Categories {
		s.Global = s.Global.Add(s.Categories[c])
	}

	missing, err := repo.RateMissingCategories(ctx, db, names, starts[budget.WindowMonth], now)
	if err != nil {
		return s, err
	}
	for _, m := range missing {
		s.RateMissing = append(s.RateMissing, budget.Category(m))
	}
	return s, nil
}

// Signals is the snapshot as the ladder evaluation reads it.
func (s Snapshot) Signals() controls.Signals {
	return controls.Signals{
		Global: s.Global, GlobalCaps: s.GlobalCaps, Categories: s.Categories, CategoryCap: s.CategoryCap,
		RateMissing: s.RateMissing, LastHourUSD: s.LastHourUSD, HoursLeftInMonth: controls.HoursLeftInMonth(s.Now),
	}
}

// LoadLadder reads the ladder's current state; before any row it is normal.
func LoadLadder(ctx context.Context, repo *internalrepo.Repo, db *gorm.DB) (budget.LadderState, int64, error) {
	row, err := repo.LatestChange(ctx, db)
	if err != nil {
		return budget.LadderState{}, 0, err
	}
	if row == nil {
		return budget.LadderState{Level: budget.LevelNormal, Actor: budget.ActorAutomatic}, 0, nil
	}
	state := budget.LadderState{Level: budget.Level(row.Level), Reason: row.Reason, Actor: row.Actor, Since: row.CreatedAt}
	if err := decode(row.PausedCategories, &state.PausedCategories); err != nil {
		return state, 0, err
	}
	if err := decode(row.ReopenClosed, &state.ReopenClosed); err != nil {
		return state, 0, err
	}
	return state, row.ID, nil
}

// ChangeRow turns a ladder state into the row that records it.
func ChangeRow(state budget.LadderState, at time.Time) internalrepo.Change {
	return internalrepo.Change{
		Level: string(state.Level), PausedCategories: encode(state.PausedCategories), ReopenClosed: encode(state.ReopenClosed),
		Reason: state.Reason, Actor: state.Actor, CreatedAt: at,
	}
}

func decode(raw string, into *[]budget.Category) error {
	if raw == "" {
		*into = []budget.Category{}
		return nil
	}
	if err := json.Unmarshal([]byte(raw), into); err != nil {
		return fmt.Errorf("reading ladder categories: %s", err)
	}
	return nil
}

func encode(list []budget.Category) string {
	if list == nil {
		list = []budget.Category{}
	}
	body, _ := json.Marshal(list)
	return string(body)
}
