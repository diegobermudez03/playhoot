package businessservice

import "github.com/diegobermudez03/playhoot/game/management"

// ValidateVisibility returns the visibility and boolean indicating if its valid or not
func ValidateVisibility(visibility string) (management.VisibilityType, bool) {
	switch management.VisibilityType(visibility) {
	case management.Draft:
		return management.Draft, true
	case management.Private:
		return management.Private, true
	case management.Hidden:
		return management.Hidden, true
	case management.Public:
		return management.Public, true
	}

	return "", false
}

func IsPlayableVisibility(visbility management.VisibilityType) bool {
	if visbility == management.Hidden || visbility == management.Public {
		return true
	}
	return false
}
