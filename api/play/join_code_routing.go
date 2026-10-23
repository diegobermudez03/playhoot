package play

import (
	"errors"
	"net/http"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// maxJoinCodeLength bounds what is worth asking the Session about; real codes
// are much shorter.
const maxJoinCodeLength = 32

// Reasons a join by code is rejected, on the wire, besides the Runtime's own.
const (
	rejectInvalidJoinCode = "invalid_join_code"
	rejectJoinCodeExpired = "join_code_expired"
)

// joinCodeMalformed is the logged outcome of a code not worth resolving.
const joinCodeMalformed = "malformed"

// joinCodeRejections are the wire reasons of the join code outcomes that
// refuse the join.
var joinCodeRejections = map[sessionpkg.JoinCodeOutcome]string{
	sessionpkg.JoinCodeInvalid:        rejectInvalidJoinCode,
	sessionpkg.JoinCodeExpired:        rejectJoinCodeExpired,
	sessionpkg.JoinCodeSessionStarted: string(runtime.RejectSessionStarted),
}

var errJoinCodeWithoutSession = errors.New("join code found without a session UUID")

// joinRefuser answers an authorized live request that may not join, with
// reason as on the wire.
type joinRefuser func(w http.ResponseWriter, r *http.Request, reason string)

// resolveJoinCode replaces the route's codeValue wildcard with the Session it
// names, set as the sessionValue path value requireOwner reads. A code that
// does not let the caller join is answered through refuse, never revealing
// its Session. A join code is only an identifier, never proof of
// authorization.
func resolveJoinCode(resolver joinCodeResolver, refuse joinRefuser, codeValue string, sessionValue string) routing.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			joinCode := r.PathValue(codeValue)
			if joinCode == "" || len(joinCode) > maxJoinCodeLength {
				logging.LogFields(ctx, logging.Field("join_code_outcome", joinCodeMalformed))
				refuse(w, r, rejectInvalidJoinCode)
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

			if resolution.Outcome == sessionpkg.JoinCodeFound {
				r.SetPathValue(sessionValue, resolution.SessionUUID)
				next(w, r)
				return
			}
			reason, refused := joinCodeRejections[resolution.Outcome]
			if !refused {
				httpx.WriteError(w, http.StatusInternalServerError, "resolving join code failed")
				return
			}
			refuse(w, r, reason)
		}
	}
}
