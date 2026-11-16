// Package method names a way into an account outside Identity: an
// identifier its owner uses to point at one of their sign-in methods.
package method

import "strconv"

// ID renders a method's identifier.
func ID(id int64) string {
	return strconv.FormatInt(id, 10)
}

// Parse reads a method identifier back; false when the value is not one.
func Parse(value string) (int64, bool) {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}
