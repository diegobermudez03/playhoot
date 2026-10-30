package identity

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
				expectedBody: `{"user":{"user_uuid":"0f8fad5b-d9cb-469f-a165-70867728950e","display_name":"Bea","level":"account"}}`}
		},
		"name_identity_refuses": func(t *testing.T, names *MockdisplayNameSetter) test {
			names.EXPECT().SetDisplayName(ctx, caller.UUID, "\t").
				Return(identitypkg.SetDisplayNameResult{Outcome: identitypkg.DisplayNameInvalid}, nil)
			return test{body: `{"display_name":"\t"}`, expectedCode: http.StatusBadRequest,
				expectedBody: `{"code":"invalid_display_name","message":"display_name is not allowed"}`}
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
			NewHandler(nil, nil, nil, nil, names, nil, Logins{}).handleSetDisplayName(recorder, request)

			require.Equal(t, tc.expectedCode, recorder.Code)
			require.JSONEq(t, tc.expectedBody, recorder.Body.String())
		})
	}
}
