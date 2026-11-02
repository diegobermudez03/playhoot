// Package changeattempt is what the steps that change a DRAFT's sources
// share about their attempts: the fingerprint of a request, replaying an
// attempt completed under the same idempotency key, and diagnostics in their
// public form.
package changeattempt

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/diegobermudez03/playhoot/monitoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
)

// Fingerprint is the SHA-256 of a step's request payload as JSON, which
// identifies the same request across retries.
func Fingerprint(payload any) (string, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encoding request: %s", err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

// Replay answers a retry with what the attempt that holds its idempotency
// key decided, or reports that the key was used for another request.
func Replay(ctx context.Context, existing *internalrepo.ChangeAttempt, requestSHA256 string) (sessionpkg.DraftChangeResult, error) {
	if existing.RequestSHA256 != requestSHA256 {
		return sessionpkg.DraftChangeResult{}, fmt.Errorf("change attempt: %w", sessionpkg.ErrIdempotencyConflict)
	}
	if existing.Outcome == nil || existing.Revision == nil {
		monitoring.Alert(ctx, "play draft change attempt is claimed without an outcome")
		return sessionpkg.DraftChangeResult{}, fmt.Errorf("change attempt %d has no outcome", existing.ID)
	}
	return sessionpkg.DraftChangeResult{
		Outcome:     sessionpkg.DraftChangeOutcome(*existing.Outcome),
		Revision:    *existing.Revision,
		Diagnostics: Public(existing.Diagnostics),
		Replayed:    true,
	}, nil
}

// Public is diagnostics in their public form.
func Public(diagnostics []draftsources.Diagnostic) []sessionpkg.DraftDiagnostic {
	if len(diagnostics) == 0 {
		return nil
	}
	public := make([]sessionpkg.DraftDiagnostic, len(diagnostics))
	for i, diagnostic := range diagnostics {
		public[i] = sessionpkg.DraftDiagnostic{Path: diagnostic.Path, Code: diagnostic.Code, Message: diagnostic.Message}
	}
	return public
}

// ValidOrigin reports whether origin is one a change may come from.
func ValidOrigin(origin string) bool {
	return origin == sessionpkg.DraftOriginWeb || origin == sessionpkg.DraftOriginAPI || origin == sessionpkg.DraftOriginMCP
}
