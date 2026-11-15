// Package secondfactor is the route group of the administrator's second
// factor: enrolling a TOTP credential, confirming it, and verifying a code
// for an admin-mode token or a single-use step-up proof.
package secondfactor

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/api/internal/throttle"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/utils/ratelimit"
)

// authorizer resolves the caller of a request.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identity.Operation) (identity.User, identity.AccessDecision, error)
}

// secondFactor enrolls and verifies the caller's TOTP credential.
type secondFactor interface {
	EnrollTotp(ctx context.Context, userUUID string, currentCode string) (identity.TotpEnrollment, error)
	ConfirmTotp(ctx context.Context, userUUID string, code string) (identity.TotpConfirmation, error)
	VerifyTotp(ctx context.Context, userUUID string, code string, purpose identity.SecondFactorPurpose) (identity.TotpVerification, error)
}

var (
	enrollLimits = routing.RequestLimits{PerUser: []ratelimit.Limit{{Count: 5, Per: time.Minute}}, PerClient: throttle.DefaultLimits.PerClient}
	verifyLimits = routing.RequestLimits{PerUser: []ratelimit.Limit{{Count: 10, Per: time.Minute}}, PerClient: throttle.DefaultLimits.PerClient}
)

const maxBodyBytes = 1 << 10

// Handler exposes the second factor endpoints.
type Handler struct {
	authorizer   authorizer
	secondFactor secondFactor
}

// NewHandler constructs a Handler.
func NewHandler(authorizer authorizer, secondFactor secondFactor) *Handler {
	return &Handler{authorizer: authorizer, secondFactor: secondFactor}
}

// RESTRoutes returns this group's endpoints, for any account: only the
// administrator gets past them.
func (h *Handler) RESTRoutes() []routing.Route {
	account := auth.Require(h.authorizer, identity.OperationUseSecondFactor)
	return []routing.Route{
		{Pattern: "POST /v1/identity/totp/enroll", Handler: h.handleEnroll, Authorization: account, Limits: &enrollLimits},
		{Pattern: "POST /v1/identity/totp/confirm", Handler: h.handleConfirm, Authorization: account, Limits: &enrollLimits},
		{Pattern: "POST /v1/identity/totp/verify", Handler: h.handleVerify, Authorization: account, Limits: &verifyLimits},
	}
}

// WebSocketRoutes returns no endpoints: this group has no live ones.
func (h *Handler) WebSocketRoutes() []routing.Route {
	return nil
}

type codeRequest struct {
	Code        string `json:"code"`
	CurrentCode string `json:"current_code"`
	Purpose     string `json:"purpose"`
}

type enrollResponse struct {
	Secret     string `json:"secret"`
	OtpauthURI string `json:"otpauth_uri"`
	ExpiresAt  string `json:"expires_at"`
}

type confirmResponse struct {
	Outcome string `json:"outcome"`
}

type adminTokenResponse struct {
	Purpose         string `json:"purpose"`
	UserUUID        string `json:"user_uuid"`
	Level           string `json:"level"`
	AccessToken     string `json:"access_token"`
	AccessExpiresAt string `json:"access_expires_at"`
	AccessExpiresIn int64  `json:"access_expires_in"`
}

type stepUpResponse struct {
	Purpose     string `json:"purpose"`
	StepUpProof string `json:"step_up_proof"`
	ExpiresAt   string `json:"expires_at"`
}

func (h *Handler) handleEnroll(w http.ResponseWriter, r *http.Request) {
	caller, req, ok := readRequest(w, r)
	if !ok {
		return
	}
	result, err := h.secondFactor.EnrollTotp(r.Context(), caller.UUID, req.CurrentCode)
	if failed(w, r, err) {
		return
	}
	if result.Outcome != identity.SecondFactorAccepted {
		writeRefusal(w, result.Outcome, result.LockedUntil)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, enrollResponse{Secret: result.Secret, OtpauthURI: result.URI, ExpiresAt: result.ExpiresAt.UTC().Format(time.RFC3339)})
}

func (h *Handler) handleConfirm(w http.ResponseWriter, r *http.Request) {
	caller, req, ok := readRequest(w, r)
	if !ok {
		return
	}
	result, err := h.secondFactor.ConfirmTotp(r.Context(), caller.UUID, req.Code)
	if failed(w, r, err) {
		return
	}
	if result.Outcome != identity.SecondFactorAccepted {
		writeRefusal(w, result.Outcome, result.LockedUntil)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, confirmResponse{Outcome: "confirmed"})
}

func (h *Handler) handleVerify(w http.ResponseWriter, r *http.Request) {
	caller, req, ok := readRequest(w, r)
	if !ok {
		return
	}
	purpose := identity.SecondFactorPurpose(req.Purpose)
	if purpose != identity.PurposeAdminToken && purpose != identity.PurposeStepUp {
		httpx.WriteError(w, http.StatusBadRequest, "purpose must be admin_token or step_up")
		return
	}
	result, err := h.secondFactor.VerifyTotp(r.Context(), caller.UUID, req.Code, purpose)
	if failed(w, r, err) {
		return
	}
	if result.Outcome != identity.SecondFactorAccepted {
		writeRefusal(w, result.Outcome, result.LockedUntil)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if purpose == identity.PurposeStepUp {
		httpx.WriteJSON(w, http.StatusOK, stepUpResponse{Purpose: string(purpose), StepUpProof: result.StepUpProof, ExpiresAt: result.StepUpExpiry.UTC().Format(time.RFC3339)})
		return
	}
	credentials := result.Credentials
	httpx.WriteJSON(w, http.StatusOK, adminTokenResponse{
		Purpose: string(purpose), UserUUID: credentials.User.UUID, Level: string(credentials.User.Level),
		AccessToken:     credentials.AccessToken,
		AccessExpiresAt: credentials.AccessExpiresAt.UTC().Format(time.RFC3339),
		AccessExpiresIn: int64(credentials.AccessExpiresAt.Sub(credentials.AccessIssuedAt) / time.Second),
	})
}

func readRequest(w http.ResponseWriter, r *http.Request) (identity.User, codeRequest, bool) {
	caller, ok := auth.Caller(r.Context())
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "caller not resolved")
		return identity.User{}, codeRequest{}, false
	}
	var req codeRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(&req); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return identity.User{}, codeRequest{}, false
	}
	return caller, req, true
}

func failed(w http.ResponseWriter, r *http.Request, err error) bool {
	if err == nil {
		return false
	}
	logging.LogError(r.Context(), err)
	if errors.Is(err, identity.ErrUserNotFound) {
		httpx.WriteError(w, http.StatusInternalServerError, "caller not found")
		return true
	}
	httpx.WriteError(w, http.StatusServiceUnavailable, "second factor unavailable")
	return true
}

// writeRefusal answers a refused outcome with its code; the message never
// says whether a code was wrong or already used.
func writeRefusal(w http.ResponseWriter, outcome identity.SecondFactorOutcome, lockedUntil time.Time) {
	switch outcome {
	case identity.SecondFactorNotAdmin:
		httpx.WriteCodedError(w, http.StatusForbidden, string(outcome), "only the administrator has a second factor")
	case identity.SecondFactorNotEnrolled, identity.SecondFactorNoPendingEnrollment:
		httpx.WriteCodedError(w, http.StatusConflict, string(outcome), "enroll the second factor first")
	case identity.SecondFactorCurrentCodeRequired:
		httpx.WriteCodedError(w, http.StatusBadRequest, string(outcome), "a code from the current authenticator is required")
	case identity.SecondFactorLocked:
		seconds := int64(time.Until(lockedUntil).Seconds()) + 1
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
		httpx.WriteCodedError(w, http.StatusTooManyRequests, string(outcome), "too many wrong codes, try again later")
	default:
		httpx.WriteCodedError(w, http.StatusBadRequest, string(identity.SecondFactorInvalidCode), "the code is not valid")
	}
}
