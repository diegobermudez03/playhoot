package secondfactor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/identity"
)

type fakeSecondFactor struct {
	enrollment   identity.TotpEnrollment
	confirmation identity.TotpConfirmation
	verification identity.TotpVerification
	err          error
	calls        []string
}

func (f *fakeSecondFactor) EnrollTotp(_ context.Context, userUUID string, currentCode string) (identity.TotpEnrollment, error) {
	f.calls = append(f.calls, "enroll "+userUUID+" "+currentCode)
	return f.enrollment, f.err
}

func (f *fakeSecondFactor) ConfirmTotp(_ context.Context, userUUID string, code string) (identity.TotpConfirmation, error) {
	f.calls = append(f.calls, "confirm "+userUUID+" "+code)
	return f.confirmation, f.err
}

func (f *fakeSecondFactor) VerifyTotp(_ context.Context, userUUID string, code string, purpose identity.SecondFactorPurpose) (identity.TotpVerification, error) {
	f.calls = append(f.calls, "verify "+userUUID+" "+code+" "+string(purpose))
	return f.verification, f.err
}

func TestHandlers(t *testing.T) {
	caller := identity.User{UUID: "0f8fad5b-d9cb-469f-a165-70867728950e", Level: identity.LevelAccount}
	issued := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	type test struct {
		handle        func(h *Handler) http.HandlerFunc
		body          string
		fake          *fakeSecondFactor
		expectedCode  int
		expectedBody  string
		expectedCalls []string
	}
	enroll := func(h *Handler) http.HandlerFunc { return h.handleEnroll }
	confirm := func(h *Handler) http.HandlerFunc { return h.handleConfirm }
	verify := func(h *Handler) http.HandlerFunc { return h.handleVerify }
	tests := map[string]func(t *testing.T) test{
		"enroll_returns_the_secret": func(t *testing.T) test {
			return test{handle: enroll, body: `{}`, fake: &fakeSecondFactor{enrollment: identity.TotpEnrollment{
				Outcome: identity.SecondFactorAccepted, Secret: "ABC", URI: "otpauth://totp/x", ExpiresAt: issued}},
				expectedCode: http.StatusOK, expectedBody: `{"secret":"ABC","otpauth_uri":"otpauth://totp/x","expires_at":"2026-11-15T10:00:00Z"}`,
				expectedCalls: []string{"enroll " + caller.UUID + " "}}
		},
		"enroll_refused_without_grant": func(t *testing.T) test {
			return test{handle: enroll, body: `{"current_code":"123456"}`, fake: &fakeSecondFactor{enrollment: identity.TotpEnrollment{Outcome: identity.SecondFactorNotAdmin}},
				expectedCode: http.StatusForbidden, expectedBody: `{"code":"not_admin","message":"only the administrator has a second factor"}`,
				expectedCalls: []string{"enroll " + caller.UUID + " 123456"}}
		},
		"confirm_with_a_wrong_code": func(t *testing.T) test {
			return test{handle: confirm, body: `{"code":"000000"}`, fake: &fakeSecondFactor{confirmation: identity.TotpConfirmation{Outcome: identity.SecondFactorInvalidCode}},
				expectedCode: http.StatusBadRequest, expectedBody: `{"code":"invalid_code","message":"the code is not valid"}`,
				expectedCalls: []string{"confirm " + caller.UUID + " 000000"}}
		},
		"confirm_accepted": func(t *testing.T) test {
			return test{handle: confirm, body: `{"code":"123456"}`, fake: &fakeSecondFactor{confirmation: identity.TotpConfirmation{Outcome: identity.SecondFactorAccepted}},
				expectedCode: http.StatusOK, expectedBody: `{"outcome":"confirmed"}`, expectedCalls: []string{"confirm " + caller.UUID + " 123456"}}
		},
		"verify_for_the_admin_token": func(t *testing.T) test {
			admin := identity.User{UUID: caller.UUID, Level: identity.LevelAdmin}
			return test{handle: verify, body: `{"code":"123456","purpose":"admin_token"}`, fake: &fakeSecondFactor{verification: identity.TotpVerification{
				Outcome: identity.SecondFactorAccepted, Credentials: identity.Credentials{User: admin, AccessToken: "jwt", AccessIssuedAt: issued, AccessExpiresAt: issued.Add(15 * time.Minute)}}},
				expectedCode:  http.StatusOK,
				expectedBody:  `{"purpose":"admin_token","user_uuid":"` + caller.UUID + `","level":"admin","access_token":"jwt","access_expires_at":"2026-11-15T10:15:00Z","access_expires_in":900}`,
				expectedCalls: []string{"verify " + caller.UUID + " 123456 admin_token"}}
		},
		"verify_for_a_step_up_proof": func(t *testing.T) test {
			return test{handle: verify, body: `{"code":"123456","purpose":"step_up"}`, fake: &fakeSecondFactor{verification: identity.TotpVerification{
				Outcome: identity.SecondFactorAccepted, StepUpProof: "proof", StepUpExpiry: issued}},
				expectedCode: http.StatusOK, expectedBody: `{"purpose":"step_up","step_up_proof":"proof","expires_at":"2026-11-15T10:00:00Z"}`,
				expectedCalls: []string{"verify " + caller.UUID + " 123456 step_up"}}
		},
		"verify_while_locked": func(t *testing.T) test {
			return test{handle: verify, body: `{"code":"123456","purpose":"step_up"}`, fake: &fakeSecondFactor{verification: identity.TotpVerification{
				Outcome: identity.SecondFactorLocked, LockedUntil: time.Now().Add(time.Minute)}},
				expectedCode: http.StatusTooManyRequests, expectedBody: `{"code":"locked","message":"too many wrong codes, try again later"}`,
				expectedCalls: []string{"verify " + caller.UUID + " 123456 step_up"}}
		},
		"verify_with_an_unknown_purpose": func(t *testing.T) test {
			return test{handle: verify, body: `{"code":"123456","purpose":"root"}`, fake: &fakeSecondFactor{},
				expectedCode: http.StatusBadRequest, expectedBody: `{"message":"purpose must be admin_token or step_up"}`}
		},
		"identity_failure_is_transient": func(t *testing.T) test {
			return test{handle: verify, body: `{"code":"123456","purpose":"step_up"}`, fake: &fakeSecondFactor{err: errors.New("db down")},
				expectedCode: http.StatusServiceUnavailable, expectedBody: `{"message":"second factor unavailable"}`,
				expectedCalls: []string{"verify " + caller.UUID + " 123456 step_up"}}
		},
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			ctx := auth.WithCaller(context.Background(), caller)
			request := httptest.NewRequest(http.MethodPost, "/v1/identity/totp", strings.NewReader(tc.body)).WithContext(ctx)
			recorder := httptest.NewRecorder()
			tc.handle(NewHandler(nil, tc.fake))(recorder, request)

			require.Equal(t, tc.expectedCode, recorder.Code)
			require.JSONEq(t, tc.expectedBody, recorder.Body.String())
			require.Equal(t, tc.expectedCalls, tc.fake.calls)
		})
	}
}
