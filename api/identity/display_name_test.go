package identity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	identitypkg "github.com/diegobermudez03/playhoot/identity"
)

func TestHandleSetDisplayName(t *testing.T) {
	type test struct {
		body         string
		expectedCode int
		expectedBody string
	}

	caller := identitypkg.User{UUID: "0f8fad5b-d9cb-469f-a165-70867728950e", Level: identitypkg.LevelAccount}
	ctx := auth.WithCaller(context.Background(), caller)
	named := identitypkg.User{UUID: caller.UUID, DisplayName: "Bea", Level: identitypkg.LevelAccount}

	tests := map[string]func(t *testing.T, names *MockdisplayNameSetter) test{
		"names_the_caller": func(t *testing.T, names *MockdisplayNameSetter) test {
			names.EXPECT().SetDisplayName(ctx, caller.UUID, " Bea ").
				Return(identitypkg.SetDisplayNameResult{Outcome: identitypkg.DisplayNameSet, User: named}, nil)
			return test{body: `{"display_name":" Bea "}`, expectedCode: http.StatusOK,
				expectedBody: `{"user":{"user_uuid":"0f8fad5b-d9cb-469f-a165-70867728950e","display_name":"Bea","level":"account","mode":"normal"}}`}
		},
		"name_identity_refuses": func(t *testing.T, names *MockdisplayNameSetter) test {
			names.EXPECT().SetDisplayName(ctx, caller.UUID, "\t").
				Return(identitypkg.SetDisplayNameResult{Outcome: identitypkg.DisplayNameInvalid}, nil)
			return test{body: `{"display_name":"\t"}`, expectedCode: http.StatusBadRequest,
				expectedBody: `{"code":"invalid_display_name","message":"display_name is not allowed"}`}
		},
		"the_change_limit_says_when_the_next_one_is_possible": func(t *testing.T, names *MockdisplayNameSetter) test {
			names.EXPECT().SetDisplayName(ctx, caller.UUID, "Bea").
				Return(identitypkg.SetDisplayNameResult{
					Outcome:      identitypkg.DisplayNameLimitReached,
					NextChangeAt: time.Date(2026, 12, 1, 12, 0, 0, 0, time.UTC),
				}, nil)
			return test{body: `{"display_name":"Bea"}`, expectedCode: http.StatusTooManyRequests,
				expectedBody: `{"code":"change_limit_reached","message":"you have changed your name as often as allowed for now","next_change_at":"2026-12-01T12:00:00Z"}`}
		},
		"missing_or_too_long": func(t *testing.T, names *MockdisplayNameSetter) test {
			return test{body: `{"display_name":"` + strings.Repeat("x", 65) + `"}`, expectedCode: http.StatusBadRequest,
				expectedBody: `{"message":"display_name is required and must be at most 64 characters"}`}
		},
		"identity_failure_is_transient": func(t *testing.T, names *MockdisplayNameSetter) test {
			names.EXPECT().SetDisplayName(ctx, caller.UUID, "Bea").Return(identitypkg.SetDisplayNameResult{}, errors.New("db down"))
			return test{body: `{"display_name":"Bea"}`, expectedCode: http.StatusServiceUnavailable,
				expectedBody: `{"message":"choosing the name failed"}`}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			names := NewMockdisplayNameSetter(gomock.NewController(t))
			tc := setup(t, names)

			request := httptest.NewRequest(http.MethodPut, "/v1/identity/me/display-name", strings.NewReader(tc.body)).WithContext(ctx)
			recorder := httptest.NewRecorder()
			NewHandler(nil, nil, nil, nil, names, nil, Logins{}, EmailLogins{}, Methods{}).handleSetDisplayName(recorder, request)

			require.Equal(t, tc.expectedCode, recorder.Code)
			require.JSONEq(t, tc.expectedBody, recorder.Body.String())
		})
	}
}
