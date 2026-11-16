// Package displayname holds the rules of the name other people see for a
// User.
package displayname

import (
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// MaxLength is the longest display name, in characters.
const MaxLength = 64

// Normalize returns name trimmed of surrounding spaces, and whether it is a
// valid display name: 1 to MaxLength characters, without control characters.
func Normalize(name string) (string, bool) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || !utf8.ValidString(trimmed) || utf8.RuneCountInString(trimmed) > MaxLength {
		return "", false
	}
	for _, r := range trimmed {
		if unicode.IsControl(r) {
			return "", false
		}
	}
	return trimmed, true
}

// ChangeWindow is how far back changes are counted, and MaxChangesPerWindow
// how many a User may make inside it.
const (
	ChangeWindow        = 30 * 24 * time.Hour
	MaxChangesPerWindow = 3
)

// NextChangeAt returns when the User may change their name again, given the
// times they changed it inside the window, oldest first. The zero time means
// they may change it now.
func NextChangeAt(inWindow []time.Time, now time.Time) time.Time {
	if len(inWindow) < MaxChangesPerWindow {
		return time.Time{}
	}
	// The oldest change of the window leaves it first, freeing a place.
	oldest := inWindow[len(inWindow)-MaxChangesPerWindow]
	next := oldest.Add(ChangeWindow)
	if !next.After(now) {
		return time.Time{}
	}
	return next
}
