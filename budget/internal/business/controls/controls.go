// Package controls holds Budget's pure rules: window boundaries, whether a
// cost fits the caps, and which ladder step the numbers call for.
package controls

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/diegobermudez03/playhoot/budget"
)

// Margin is the share of each window's remainder kept free so the cut can
// still be executed while work in flight finishes.
const Margin = 0.10

// Escalation thresholds, as a share of a cap committed (spend plus
// reservations): restrict from RestrictAt, pause a category from PauseAt.
// Spend alone at or over a global cap means control failed: shutdown.
const (
	RestrictAt = 0.80
	PauseAt    = 0.95
)

// MaxReasonLen bounds an admin reason; MaxServiceLen a service or unit name.
const (
	MaxReasonLen  = 500
	MaxServiceLen = 80
)

// Starts returns the start of each window containing now, in UTC.
func Starts(now time.Time) map[budget.Window]time.Time {
	now = now.UTC()
	return map[budget.Window]time.Time{
		budget.WindowHour:  now.Truncate(time.Hour),
		budget.WindowDay:   time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC),
		budget.WindowMonth: time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC),
	}
}

// HoursLeftInMonth is how long until the month window ends.
func HoursLeftInMonth(now time.Time) float64 {
	start := Starts(now)[budget.WindowMonth]
	return start.AddDate(0, 1, 0).Sub(now.UTC()).Hours()
}

// Usage is one scope's consumption: spend per window (measured plus
// estimated not replaced), reservations still active, admissions started
// per window and active admissions.
type Usage struct {
	Spend    map[budget.Window]float64
	Reserved float64
	Started  map[budget.Window]int
	Active   int
}

// Add sums other into u.
func (u Usage) Add(other Usage) Usage {
	sum := Usage{Spend: map[budget.Window]float64{}, Started: map[budget.Window]int{}, Reserved: u.Reserved + other.Reserved, Active: u.Active + other.Active}
	for _, w := range budget.Windows {
		sum.Spend[w] = u.Spend[w] + other.Spend[w]
		sum.Started[w] = u.Started[w] + other.Started[w]
	}
	return sum
}

// Caps are the current caps of one scope, by window.
type Caps map[budget.Window]budget.Cap

// Admit decides whether cost fits one scope's caps. A scope without caps, or
// with a zero or negative limit, admits nothing: never unlimited.
func Admit(cost float64, usage Usage, caps Caps) (budget.RefusalReason, bool) {
	if len(caps) == 0 {
		return budget.RefusedNoControl, false
	}
	for _, w := range budget.Windows {
		c, ok := caps[w]
		if !ok {
			continue
		}
		if c.USDLimit <= 0 {
			return budget.RefusedNoControl, false
		}
		remainder := c.USDLimit - usage.Spend[w] - usage.Reserved
		if remainder <= 0 || cost > remainder*(1-Margin) {
			return budget.RefusedCapReached, false
		}
		if c.MaxConcurrency > 0 && usage.Active >= c.MaxConcurrency {
			return budget.RefusedConcurrency, false
		}
		if c.MaxVolume > 0 && usage.Started[w] >= c.MaxVolume {
			return budget.RefusedVolume, false
		}
	}
	return "", true
}

// Gate applies the ladder to a request before any cap is looked at.
func Gate(state budget.LadderState, category budget.Category, discretionary bool) (budget.RefusalReason, bool) {
	switch {
	case state.Level == budget.LevelShutdown:
		return budget.RefusedShutdown, false
	case contains(state.PausedCategories, category) || contains(state.ReopenClosed, category):
		return budget.RefusedPaused, false
	case state.Level != budget.LevelNormal && discretionary:
		return budget.RefusedRestricted, false
	}
	return "", true
}

// Signals are the numbers the ladder evaluation reads.
type Signals struct {
	Global      Usage
	GlobalCaps  Caps
	Categories  map[budget.Category]Usage
	CategoryCap map[budget.Category]Caps
	RateMissing []budget.Category
	// LastHourUSD is global spend over the last sixty minutes.
	LastHourUSD      float64
	HoursLeftInMonth float64
}

// Target is the step the numbers call for, before the current state is
// considered.
type Target struct {
	Level  budget.Level
	Paused []budget.Category
	Reason string
}

// Evaluate computes the target step: shutdown when spend alone reaches a
// global cap; paused for each category close to or over its cap; restricted
// when any cap is under pressure, the month projection exceeds the global
// month cap, or cost information is missing.
func Evaluate(s Signals) Target {
	for _, w := range budget.Windows {
		if c, ok := s.GlobalCaps[w]; ok && c.USDLimit > 0 && s.Global.Spend[w] >= c.USDLimit {
			return Target{Level: budget.LevelShutdown, Reason: fmt.Sprintf("global %s spend reached its cap", w)}
		}
	}
	var paused []budget.Category
	reasons := []string{}
	for _, category := range budget.Categories {
		usage := s.Categories[category]
		for _, w := range budget.Windows {
			c, ok := s.CategoryCap[category][w]
			if !ok || c.USDLimit <= 0 {
				continue
			}
			if committedShare(usage, w, c) >= PauseAt {
				paused = append(paused, category)
				reasons = append(reasons, fmt.Sprintf("%s %s cap nearly exhausted", category, w))
				break
			}
		}
	}
	if len(paused) > 0 {
		return Target{Level: budget.LevelPaused, Paused: paused, Reason: join(reasons)}
	}
	for _, w := range budget.Windows {
		if c, ok := s.GlobalCaps[w]; ok && c.USDLimit > 0 && committedShare(s.Global, w, c) >= RestrictAt {
			reasons = append(reasons, fmt.Sprintf("global %s cap under pressure", w))
		}
	}
	for _, category := range budget.Categories {
		for _, w := range budget.Windows {
			c, ok := s.CategoryCap[category][w]
			if ok && c.USDLimit > 0 && committedShare(s.Categories[category], w, c) >= RestrictAt {
				reasons = append(reasons, fmt.Sprintf("%s %s cap under pressure", category, w))
			}
		}
	}
	if c, ok := s.GlobalCaps[budget.WindowMonth]; ok && c.USDLimit > 0 && s.Global.Spend[budget.WindowMonth]+s.LastHourUSD*s.HoursLeftInMonth > c.USDLimit {
		reasons = append(reasons, "spend speed projects the month cap exhausted")
	}
	if len(s.RateMissing) > 0 {
		reasons = append(reasons, fmt.Sprintf("cost rates missing for %v", s.RateMissing))
	}
	if len(reasons) > 0 {
		return Target{Level: budget.LevelRestricted, Reason: join(reasons)}
	}
	return Target{Level: budget.LevelNormal, Reason: "pressure cleared"}
}

// Next merges the target with the current state: shutdown never lifts here;
// categories still closed by a gradual reopening stay closed and keep the
// level at least restricted. It answers false when nothing changes.
func Next(current budget.LadderState, target Target) (budget.LadderState, bool) {
	if current.Level == budget.LevelShutdown {
		return current, false
	}
	next := budget.LadderState{Level: target.Level, PausedCategories: sorted(target.Paused), ReopenClosed: sorted(current.ReopenClosed), Reason: target.Reason, Actor: budget.ActorAutomatic}
	if next.Level == budget.LevelNormal && len(next.ReopenClosed) > 0 {
		next.Level = budget.LevelRestricted
		next.Reason = "gradual reopening in progress"
	}
	if next.Level == current.Level && equal(next.PausedCategories, sorted(current.PausedCategories)) && equal(next.ReopenClosed, sorted(current.ReopenClosed)) {
		return current, false
	}
	return next, true
}

// RemainingUSD is how much of one window's cap is not yet committed.
func RemainingUSD(usage Usage, w budget.Window, c budget.Cap) float64 {
	return math.Max(0, c.USDLimit-usage.Spend[w]-usage.Reserved)
}

func committedShare(usage Usage, w budget.Window, c budget.Cap) float64 {
	return (usage.Spend[w] + usage.Reserved) / c.USDLimit
}

func contains(list []budget.Category, c budget.Category) bool {
	for _, item := range list {
		if item == c {
			return true
		}
	}
	return false
}

func sorted(list []budget.Category) []budget.Category {
	out := append([]budget.Category{}, list...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func equal(a, b []budget.Category) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func join(reasons []string) string {
	out := ""
	for i, r := range reasons {
		if i > 0 {
			out += "; "
		}
		out += r
	}
	return out
}

// ValidCap reports whether a cap can be recorded.
func ValidCap(c budget.Cap) bool {
	if c.Scope != budget.ScopeGlobal && !budget.ValidCategory(budget.Category(c.Scope)) {
		return false
	}
	valid := false
	for _, w := range budget.Windows {
		valid = valid || c.Window == w
	}
	return valid && c.USDLimit >= 0 && !math.IsNaN(c.USDLimit) && !math.IsInf(c.USDLimit, 0) && c.MaxConcurrency >= 0 && c.MaxVolume >= 0
}

// ValidRate reports whether a rate can be recorded.
func ValidRate(r budget.Rate) bool {
	return budget.ValidCategory(r.Category) && r.Service != "" && len(r.Service) <= MaxServiceLen &&
		r.Unit != "" && len(r.Unit) <= MaxServiceLen && r.USDPerUnit >= 0 && !math.IsNaN(r.USDPerUnit) && !math.IsInf(r.USDPerUnit, 0)
}
