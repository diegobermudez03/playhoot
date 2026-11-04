package games

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

// codeAILinkInvalid answers a link that is unknown, expired, revoked or
// replaced, never telling which.
const codeAILinkInvalid = "ai_link_invalid"

// aiLinkLimits apply to each route an AI reaches through its link, per link
// owner: more than a person's, since an agent sends changes in bursts.
var aiLinkLimits = routing.RequestLimits{
	PerUser: []ratelimit.Limit{{Count: 20, Per: time.Second}, {Count: 300, Per: time.Minute}},
}

// AIRequestLimits are aiLinkLimits for the other AI channel, the MCP server.
var AIRequestLimits = aiLinkLimits

type originContextKey struct{}

// originOf is where a change came from: an AI through its link, or the web.
func originOf(ctx context.Context) string {
	if origin, ok := ctx.Value(originContextKey{}).(string); ok {
		return origin
	}
	return sessionpkg.DraftOriginWeb
}

// aiLinkAccess authorizes a request by the link token in its path: Identity
// says whom it acts for and on which game, and the request goes on as that
// owner on that game, with the API as the origin of its changes. The token is
// never logged.
func (h *Handler) aiLinkAccess(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		// A game's sources are private: nothing may keep, index or pass on
		// this page or its address, refusals included.
		w.Header().Set("Cache-Control", "private, no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Robots-Tag", "noindex")

		resolved, err := h.links.Resolver.ResolveResourceLink(ctx, r.PathValue("token"))
		if err != nil {
			logging.LogError(ctx, err)
			httpx.WriteError(w, http.StatusInternalServerError, "reading the link failed")
			return
		}
		gameUUID, parseErr := uuid.Parse(resolved.ResourceID)
		if resolved.Outcome != identity.ResourceLinkValid || resolved.Kind != game.ResourceKind || parseErr != nil {
			httpx.WriteCodedError(w, http.StatusUnauthorized, codeAILinkInvalid, "this link no longer works: ask the game's owner for a new one in Playhoot")
			return
		}
		logging.LogFields(ctx, logging.Field("ai_link", true))
		ctx = auth.WithCaller(ctx, identity.User{UUID: resolved.UserUUID, Level: identity.LevelAccount})
		ctx = context.WithValue(ctx, originContextKey{}, sessionpkg.DraftOriginAPI)
		r = r.WithContext(ctx)
		r.SetPathValue("gameUUID", gameUUID.String())
		next(w, r)
	}
}
