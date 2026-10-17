package play

import (
	"errors"
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// maxJoinCodeLength bounds what is worth asking the Session about; real codes
// are much shorter.
const maxJoinCodeLength = 32

var errJoinCodeWithoutSession = errors.New("join code found without a session UUID")

// resolveJoinCode replaces the route's codeValue wildcard with the Session it
// names, set as the sessionValue path value requireOwner reads. A join code is
// only an identifier, never proof of authorization.
func resolveJoinCode(resolver joinCodeResolver, codeValue string, sessionValue string) routing.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			joinCode := r.PathValue(codeValue)
			if joinCode == "" || len(joinCode) > maxJoinCodeLength {
				httpx.WriteError(w, http.StatusBadRequest, "join code is invalid")
				return
			}
			logging.LogFields(ctx, logging.Field("join_code", joinCode))

			resolution, err := resolver.ResolveJoinCode(ctx, joinCode)
			if err == nil && resolution.Outcome == sessionpkg.JoinCodeFound && resolution.SessionUUID == "" {
				err = errJoinCodeWithoutSession
			}
			if err != nil {
				logging.LogError(ctx, err)
				httpx.WriteError(w, http.StatusInternalServerError, "resolving join code failed")
				return
			}
			logging.LogFields(ctx, logging.Field("join_code_outcome", string(resolution.Outcome)))

			switch resolution.Outcome {
			case sessionpkg.JoinCodeFound:
				r.SetPathValue(sessionValue, resolution.SessionUUID)
				next(w, r)
			case sessionpkg.JoinCodeNotFound:
				httpx.WriteError(w, http.StatusNotFound, "join code not found or expired")
			default:
				httpx.WriteError(w, http.StatusInternalServerError, "resolving join code failed")
			}
		}
	}
}
