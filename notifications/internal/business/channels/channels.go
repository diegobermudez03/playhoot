// Package channels decides where an entry goes: the type's row of the
// matrix first, then the recipient's preferences, which only trim optional
// email.
package channels

import "github.com/diegobermudez03/playhoot/notifications"

// Decision is the channels of one entry.
type Decision struct {
	InApp bool
	Email bool
}

// Decide applies spec to a recipient: no address or a student under a
// no-student type means no email; necessary and standard types keep email;
// optional types need their group on, and marketing also the consent.
func Decide(spec notifications.TypeSpec, hasEmail, student, marketingConsent bool, prefs notifications.Preferences) Decision {
	d := Decision{InApp: spec.InApp}
	if !hasEmail || (student && spec.NoEmailToStudents) {
		return d
	}
	switch spec.Email {
	case notifications.EmailNecessary, notifications.EmailStandard:
		d.Email = true
	case notifications.EmailOptional:
		d.Email = prefs.Allows(spec.Group) && (spec.Group != notifications.GroupMarketing || marketingConsent)
	}
	return d
}
