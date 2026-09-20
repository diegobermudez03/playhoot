package sessionruntime

import (
	"errors"

	"github.com/diegobermudez03/playhoot/game/session"
	"github.com/diegobermudez03/playhoot/play"
)

// sentinelTranslations maps every game/session sentinel error a caller of
// sessionlifecycle.Manager needs to distinguish to play's own sentinel,
// so a caller of play.Coordinator (api) can branch on a specific
// business-outcome error without itself importing any `game` package.
var sentinelTranslations = []struct {
	from error
	to   error
}{
	{session.ErrGameNotFound, play.ErrGameNotFound},
	{session.ErrDefinitionDoesNotCompile, play.ErrDefinitionDoesNotCompile},
	{session.ErrJoinCodeInvalid, play.ErrJoinCodeInvalid},
	{session.ErrSessionNotFound, play.ErrSessionNotFound},
	{session.ErrInteractionNotFound, play.ErrInteractionNotFound},
	{session.ErrIdempotencyKeyRequired, play.ErrIdempotencyKeyRequired},
	{session.ErrIdempotencyConflict, play.ErrIdempotencyConflict},
	{session.ErrIdempotencyInFlight, play.ErrIdempotencyInFlight},
}

// translateError maps err to its play-owned sentinel, if it matches one
// game/session defines; any other error (an unexpected/internal failure)
// is returned unchanged - api's default handling for it is a plain 500,
// which needs no `game` sentinel to express.
func translateError(err error) error {
	if err == nil {
		return nil
	}
	for _, t := range sentinelTranslations {
		if errors.Is(err, t.from) {
			return t.to
		}
	}
	return err
}
