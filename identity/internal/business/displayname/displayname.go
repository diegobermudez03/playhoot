// Package displayname holds the rules of the name other people see for a
// User.
package displayname

import (
	"strings"
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
