// Package gamebrief reads a project's brief.json (playhoot.brief.v1): the
// game agreed with the person before and while it is written, and the phase
// the work is in, so whoever continues it knows where it stands.
package gamebrief

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"unicode/utf8"
)

// Format is the format a brief declares.
const Format = "playhoot.brief.v1"

// Path is where a project keeps its brief.
const Path = "brief.json"

// Phases of the work, in order. A brief without one is in PhaseBrief.
const (
	PhaseBrief      = "brief"
	PhaseFoundation = "foundation"
	PhaseBuilding   = "building"
	PhaseTesting    = "testing"
	PhaseAssets     = "assets"
	PhaseReady      = "ready"
)

// Phases lists the phases in order.
var Phases = []string{PhaseBrief, PhaseFoundation, PhaseBuilding, PhaseTesting, PhaseAssets, PhaseReady}

// Limits of a brief.
const (
	MaxTextLength = 2000
	MaxItems      = 50
	MaxItemLength = 500
	MaxPlayers    = 1000
)

// textFields hold one text each; listFields lists of short texts.
var (
	textFields = []string{"goal", "loop", "rounds", "scoring", "ending", "assessment"}
	listFields = []string{"actions", "private_information", "capabilities", "assets", "open_questions"}
)

// Brief is what a valid brief declares that the platform reads: its phase.
// The rest stays as the document holds it.
type Brief struct {
	Phase string
}

// Problem is one reason a document is not a valid brief; Field is empty
// when it is about the whole document.
type Problem struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Parse reads raw as a brief, or answers every reason it is not one.
func Parse(raw []byte) (Brief, []Problem) {
	var problems []Problem
	add := func(field, message string) {
		problems = append(problems, Problem{Field: field, Message: message})
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return Brief{}, []Problem{{Message: "a brief is one JSON object"}}
	}
	known := map[string]bool{"format": true, "phase": true, "players": true}
	for _, name := range textFields {
		known[name] = true
	}
	for _, name := range listFields {
		known[name] = true
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !known[name] {
			add(name, "is not a field of a brief")
		}
	}

	var format string
	if json.Unmarshal(fields["format"], &format) != nil || format != Format {
		add("format", "must be "+Format)
	}
	brief := Brief{Phase: PhaseBrief}
	if value, present := fields["phase"]; present {
		var phase string
		if json.Unmarshal(value, &phase) != nil || !isPhase(phase) {
			add("phase", "is one of brief, foundation, building, testing, assets, ready")
		} else {
			brief.Phase = phase
		}
	}
	if value, present := fields["players"]; present {
		checkPlayers(value, add)
	}
	for _, name := range textFields {
		if value, present := fields[name]; present {
			checkText(name, value, MaxTextLength, add)
		}
	}
	for _, name := range listFields {
		value, present := fields[name]
		if !present {
			continue
		}
		var items []json.RawMessage
		if json.Unmarshal(value, &items) != nil || isNull(value) {
			add(name, "is a list of texts")
			continue
		}
		if len(items) > MaxItems {
			add(name, fmt.Sprintf("holds at most %d items", MaxItems))
		}
		for i, item := range items {
			checkText(fmt.Sprintf("%s[%d]", name, i), item, MaxItemLength, add)
		}
	}
	if len(problems) > 0 {
		// By field, so both sides of the shared vectors list them alike.
		sort.SliceStable(problems, func(i, j int) bool { return problems[i].Field < problems[j].Field })
		return Brief{}, problems
	}
	return brief, nil
}

func checkPlayers(value json.RawMessage, add func(field, message string)) {
	var players map[string]json.RawMessage
	if json.Unmarshal(value, &players) != nil || players == nil {
		add("players", "is {\"min\": n, \"max\": m}")
		return
	}
	bounds := map[string]int{}
	for _, name := range []string{"min", "max"} {
		var number float64
		raw, present := players[name]
		if !present || json.Unmarshal(raw, &number) != nil || number != float64(int(number)) || number < 1 || number > MaxPlayers {
			add("players."+name, fmt.Sprintf("is a whole number from 1 to %d", MaxPlayers))
			continue
		}
		bounds[name] = int(number)
	}
	for name := range players {
		if name != "min" && name != "max" {
			add("players."+name, "is not a field of players")
		}
	}
	if minimum, ok := bounds["min"]; ok {
		if maximum, ok := bounds["max"]; ok && minimum > maximum {
			add("players", "min is at most max")
		}
	}
}

func checkText(field string, value json.RawMessage, limit int, add func(field, message string)) {
	var text string
	if isNull(value) || json.Unmarshal(value, &text) != nil {
		add(field, "is a text")
		return
	}
	if utf8.RuneCountInString(text) > limit {
		add(field, fmt.Sprintf("holds at most %d characters", limit))
	}
}

func isPhase(phase string) bool {
	for _, known := range Phases {
		if phase == known {
			return true
		}
	}
	return false
}

func isNull(value json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}
