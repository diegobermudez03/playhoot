package businessservice

import "github.com/diegobermudez03/playhoot/game/game"

// ValidateVisibility returns the visibility and boolean indicating if its valid or not
func ValidateVisibility(visibility string) (game.VisibilityType, bool) {
	switch game.VisibilityType(visibility) {
	case game.Draft:
		return game.Draft, true
	case game.Private:
		return game.Draft, true
	case game.Hidden:
		return game.Draft, true
	case game.Public:
		return game.Public, true
	}

	return "", false
}
