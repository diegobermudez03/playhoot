// Package status is the control panel's numbers: the ladder, the caps and
// rates in force, each scope's window usage split measured and estimated,
// active reservations, spend speed and the month projection.
package status

import (
	"context"
	"sort"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/budget"
	"github.com/diegobermudez03/playhoot/budget/internal/business/controls"
	internalrepo "github.com/diegobermudez03/playhoot/budget/internal/repo"
	"github.com/diegobermudez03/playhoot/budget/internal/snapshot"
	"github.com/diegobermudez03/playhoot/logging"
)

// Service is the status read.
type Service struct {
	repo *internalrepo.Repo
	now  func() time.Time
}

// New constructs a Service on db.
func New(db *gorm.DB) *Service {
	return &Service{repo: internalrepo.New(db), now: time.Now}
}

// Status reads the panel at now. Usage lists global first, then every
// category with a cap or with consumption this month.
func (s *Service) Status(ctx context.Context) (budget.Status, error) {
	defer logging.Step(ctx, "budget.status").Close()
	now := s.now().UTC()
	db := s.repo.GetDB()
	snap, err := snapshot.Load(ctx, s.repo, db, now)
	if err != nil {
		return budget.Status{}, err
	}
	rates, err := s.repo.RatesAt(ctx, db, now)
	if err != nil {
		return budget.Status{}, err
	}
	result := budget.Status{Ladder: snap.Ladder, Caps: snap.Caps, LastHourUSD: snap.LastHourUSD, RateMissing: snap.RateMissing, HoursToMonthCap: -1}
	if result.Caps == nil {
		result.Caps = []budget.Cap{}
	}
	if result.RateMissing == nil {
		result.RateMissing = []budget.Category{}
	}
	result.Rates = make([]budget.Rate, len(rates))
	for i, r := range rates {
		result.Rates[i] = budget.Rate{Category: budget.Category(r.Category), Service: r.Service, Unit: r.Unit, USDPerUnit: r.USDPerUnit}
	}
	result.Usage = append(result.Usage, usage(budget.ScopeGlobal, snap.Global, snap.GlobalCaps, snap.Measured[budget.ScopeGlobal])...)
	categories := append([]budget.Category{}, budget.Categories...)
	sort.Slice(categories, func(i, j int) bool { return categories[i] < categories[j] })
	for _, c := range categories {
		u := snap.Categories[c]
		if len(snap.CategoryCap[c]) == 0 && u.Spend[budget.WindowMonth] == 0 && u.Reserved == 0 {
			continue
		}
		result.Usage = append(result.Usage, usage(string(c), u, snap.CategoryCap[c], snap.Measured[string(c)])...)
		result.ActiveReservations += u.Active
	}
	if c, ok := snap.GlobalCaps[budget.WindowMonth]; ok && snap.LastHourUSD > 0 {
		result.HoursToMonthCap = controls.RemainingUSD(snap.Global, budget.WindowMonth, c) / snap.LastHourUSD
	}
	return result, nil
}

func usage(scope string, u controls.Usage, caps controls.Caps, measured map[budget.Window]float64) []budget.WindowUsage {
	out := make([]budget.WindowUsage, 0, len(budget.Windows))
	for _, w := range budget.Windows {
		row := budget.WindowUsage{Scope: scope, Window: w, Measured: measured[w], Estimated: u.Spend[w] - measured[w], Reserved: u.Reserved}
		if c, ok := caps[w]; ok {
			row.Limit, row.HasCap = c.USDLimit, true
		}
		out = append(out, row)
	}
	return out
}
