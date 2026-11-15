package controls

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/budget"
)

func usage(hour, day, month, reserved float64) Usage {
	return Usage{
		Spend:    map[budget.Window]float64{budget.WindowHour: hour, budget.WindowDay: day, budget.WindowMonth: month},
		Started:  map[budget.Window]int{},
		Reserved: reserved,
	}
}

func TestAdmit(t *testing.T) {
	caps := Caps{
		budget.WindowHour:  {Scope: budget.ScopeGlobal, Window: budget.WindowHour, USDLimit: 1},
		budget.WindowDay:   {Scope: budget.ScopeGlobal, Window: budget.WindowDay, USDLimit: 8},
		budget.WindowMonth: {Scope: budget.ScopeGlobal, Window: budget.WindowMonth, USDLimit: 120},
	}
	tests := map[string]func(t *testing.T){
		"a cost inside every window minus the margin is admitted": func(t *testing.T) {
			_, ok := Admit(0.5, usage(0.4, 1, 10, 0), caps)
			require.True(t, ok) // hour remainder 0.6, usable 0.54
		},
		"the margin keeps a tenth of the hour remainder free": func(t *testing.T) {
			reason, ok := Admit(0.55, usage(0.4, 1, 10, 0), caps)
			require.False(t, ok)
			require.Equal(t, budget.RefusedCapReached, reason)
		},
		"reservations occupy the window like spend": func(t *testing.T) {
			_, ok := Admit(0.5, usage(0, 0, 0, 0.5), caps)
			require.False(t, ok)
		},
		"the day window refuses when the hour has room": func(t *testing.T) {
			_, ok := Admit(0.1, usage(0, 7.95, 10, 0), caps)
			require.False(t, ok)
		},
		"the month window refuses when hour and day have room": func(t *testing.T) {
			_, ok := Admit(0.1, usage(0, 0, 119.95, 0), caps)
			require.False(t, ok)
		},
		"no caps admit nothing": func(t *testing.T) {
			reason, ok := Admit(0, usage(0, 0, 0, 0), Caps{})
			require.False(t, ok)
			require.Equal(t, budget.RefusedNoControl, reason)
		},
		"a zero cap admits nothing, even free work": func(t *testing.T) {
			reason, ok := Admit(0, usage(0, 0, 0, 0), Caps{budget.WindowDay: {USDLimit: 0, Window: budget.WindowDay}})
			require.False(t, ok)
			require.Equal(t, budget.RefusedNoControl, reason)
		},
		"concurrency and volume caps apply alongside money": func(t *testing.T) {
			u := usage(0, 0, 0, 0)
			u.Active = 2
			reason, ok := Admit(0, u, Caps{budget.WindowDay: {Window: budget.WindowDay, USDLimit: 1, MaxConcurrency: 2}})
			require.False(t, ok)
			require.Equal(t, budget.RefusedConcurrency, reason)

			u = usage(0, 0, 0, 0)
			u.Started[budget.WindowDay] = 5
			reason, ok = Admit(0, u, Caps{budget.WindowDay: {Window: budget.WindowDay, USDLimit: 1, MaxVolume: 5}})
			require.False(t, ok)
			require.Equal(t, budget.RefusedVolume, reason)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}

func TestGate(t *testing.T) {
	tests := map[string]func(t *testing.T){
		"shutdown refuses everything": func(t *testing.T) {
			reason, ok := Gate(budget.LadderState{Level: budget.LevelShutdown}, budget.CategoryEmail, false)
			require.False(t, ok)
			require.Equal(t, budget.RefusedShutdown, reason)
		},
		"restricted refuses discretionary work only": func(t *testing.T) {
			_, ok := Gate(budget.LadderState{Level: budget.LevelRestricted}, budget.CategoryEmail, true)
			require.False(t, ok)
			_, ok = Gate(budget.LadderState{Level: budget.LevelRestricted}, budget.CategoryEmail, false)
			require.True(t, ok)
		},
		"paused and still-closed categories are refused": func(t *testing.T) {
			_, ok := Gate(budget.LadderState{Level: budget.LevelPaused, PausedCategories: []budget.Category{budget.CategoryAI}}, budget.CategoryAI, false)
			require.False(t, ok)
			_, ok = Gate(budget.LadderState{Level: budget.LevelRestricted, ReopenClosed: []budget.Category{budget.CategoryEmail}}, budget.CategoryEmail, false)
			require.False(t, ok)
			_, ok = Gate(budget.LadderState{Level: budget.LevelPaused, PausedCategories: []budget.Category{budget.CategoryAI}}, budget.CategoryEmail, false)
			require.True(t, ok)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}

func TestEvaluateAndNext(t *testing.T) {
	global := Caps{
		budget.WindowHour:  {Window: budget.WindowHour, USDLimit: 1},
		budget.WindowDay:   {Window: budget.WindowDay, USDLimit: 8},
		budget.WindowMonth: {Window: budget.WindowMonth, USDLimit: 120},
	}
	emailCaps := map[budget.Category]Caps{budget.CategoryEmail: {budget.WindowDay: {Window: budget.WindowDay, USDLimit: 1}}}
	signals := func(globalUsage, email Usage) Signals {
		return Signals{Global: globalUsage, GlobalCaps: global, Categories: map[budget.Category]Usage{budget.CategoryEmail: email}, CategoryCap: emailCaps, HoursLeftInMonth: 1}
	}
	normal := budget.LadderState{Level: budget.LevelNormal}
	tests := map[string]func(t *testing.T){
		"no pressure is normal": func(t *testing.T) {
			require.Equal(t, budget.LevelNormal, Evaluate(signals(usage(0.1, 1, 10, 0), usage(0, 0.1, 0.1, 0))).Level)
		},
		"pressure on a cap restricts before it is reached": func(t *testing.T) {
			require.Equal(t, budget.LevelRestricted, Evaluate(signals(usage(0.85, 1, 10, 0), usage(0, 0, 0, 0))).Level)
		},
		"spend speed projecting the month cap restricts": func(t *testing.T) {
			s := signals(usage(0.1, 1, 100, 0), usage(0, 0, 0, 0))
			s.LastHourUSD, s.HoursLeftInMonth = 0.5, 100
			require.Equal(t, budget.LevelRestricted, Evaluate(s).Level)
		},
		"missing rates restrict": func(t *testing.T) {
			s := signals(usage(0, 0, 0, 0), usage(0, 0, 0, 0))
			s.RateMissing = []budget.Category{budget.CategoryAI}
			require.Equal(t, budget.LevelRestricted, Evaluate(s).Level)
		},
		"a category near its cap is paused": func(t *testing.T) {
			target := Evaluate(signals(usage(0.1, 1, 10, 0), usage(0, 0.9, 0.9, 0.06)))
			require.Equal(t, budget.LevelPaused, target.Level)
			require.Equal(t, []budget.Category{budget.CategoryEmail}, target.Paused)
		},
		"spend reaching a global cap shuts down": func(t *testing.T) {
			require.Equal(t, budget.LevelShutdown, Evaluate(signals(usage(1, 1, 10, 0), usage(0, 0, 0, 0))).Level)
		},
		"pressure clearing de-escalates automatically": func(t *testing.T) {
			next, changed := Next(budget.LadderState{Level: budget.LevelPaused, PausedCategories: []budget.Category{budget.CategoryEmail}}, Target{Level: budget.LevelNormal})
			require.True(t, changed)
			require.Equal(t, budget.LevelNormal, next.Level)
			require.Equal(t, budget.ActorAutomatic, next.Actor)
		},
		"shutdown never lifts automatically": func(t *testing.T) {
			_, changed := Next(budget.LadderState{Level: budget.LevelShutdown}, Target{Level: budget.LevelNormal})
			require.False(t, changed)
		},
		"a gradual reopening keeps its closed categories and at least restricted": func(t *testing.T) {
			current := budget.LadderState{Level: budget.LevelRestricted, ReopenClosed: []budget.Category{budget.CategoryAI}, PausedCategories: []budget.Category{}}
			_, changed := Next(current, Target{Level: budget.LevelNormal})
			require.False(t, changed)
			next, changed := Next(current, Target{Level: budget.LevelPaused, Paused: []budget.Category{budget.CategoryEmail}})
			require.True(t, changed)
			require.Equal(t, []budget.Category{budget.CategoryAI}, next.ReopenClosed)
		},
		"the same step appends nothing": func(t *testing.T) {
			_, changed := Next(normal, Target{Level: budget.LevelNormal})
			require.False(t, changed)
		},
	}
	for name, test := range tests {
		t.Run(name, test)
	}
}

func TestStarts(t *testing.T) {
	now := time.Date(2030, 3, 14, 15, 9, 26, 0, time.UTC)
	starts := Starts(now)
	require.Equal(t, time.Date(2030, 3, 14, 15, 0, 0, 0, time.UTC), starts[budget.WindowHour])
	require.Equal(t, time.Date(2030, 3, 14, 0, 0, 0, 0, time.UTC), starts[budget.WindowDay])
	require.Equal(t, time.Date(2030, 3, 1, 0, 0, 0, 0, time.UTC), starts[budget.WindowMonth])
}
