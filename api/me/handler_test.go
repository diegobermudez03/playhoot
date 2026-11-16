package me

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/compliance"
	"github.com/diegobermudez03/playhoot/composer/mydata"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/plan"
)

const (
	testUserUUID = "0f8fad5b-d9cb-469f-a165-70867728950e"
	testToken    = "access-jwt"
)

// stubAuthorizer answers for one token; anything else is an anonymous
// caller, which never gets past the route's authorization.
type stubAuthorizer struct {
	level identity.Level
}

func (s stubAuthorizer) Authorize(_ context.Context, token string, operation identity.Operation) (identity.User, identity.AccessDecision, error) {
	if token != testToken {
		return identity.User{}, identity.AccessUnauthenticated, nil
	}
	if s.level == identity.LevelGuest && (operation == identity.OperationSetOwnMode || operation == identity.OperationSetInterfaceLanguage) {
		return identity.User{}, identity.AccessForbidden, nil
	}
	return identity.User{UUID: testUserUUID, DisplayName: "Ana", Level: s.level}, identity.AccessAllowed, nil
}

func serve(t *testing.T, handler *Handler, method, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	for _, route := range handler.RESTRoutes() {
		mux.HandleFunc(route.Pattern, route.Authorization(route.Handler))
	}
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	return recorder
}

func TestMyData(t *testing.T) {
	limits := plan.Limits{SavedGames: 5, CreationsPerWindow: 3, SessionsPerWindow: 10, WindowDays: 30, PlayersPerSession: 30, SimultaneousRooms: 1, StorageMB: 100}
	limitsJSON := `{"saved_games":5,"creations_per_window":3,"sessions_per_window":10,"window_days":30,"players_per_session":30,"simultaneous_rooms":1,"storage_mb":100}`

	t.Run("an anonymous caller is sent to authenticate", func(t *testing.T) {
		handler := NewHandler(stubAuthorizer{level: identity.LevelGuest}, Dependencies{})

		recorder := serve(t, handler, http.MethodGet, "/v1/me", "", "")

		require.Equal(t, http.StatusUnauthorized, recorder.Code)
	})

	t.Run("a guest gets its profile, no declaration and its capacities", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		data := NewMockdataReader(ctrl)
		data.EXPECT().ReadMyData(gomock.Any(), testUserUUID).Return(mydata.MyData{
			Profile:          identity.Profile{UUID: testUserUUID, DisplayName: "Ana", Level: identity.LevelGuest, Mode: identity.ModeNormal},
			DeclarationKnown: true,
			Limits:           limits,
			LimitsKnown:      true,
		}, nil)
		handler := NewHandler(stubAuthorizer{level: identity.LevelGuest}, Dependencies{Data: data})

		recorder := serve(t, handler, http.MethodGet, "/v1/me", testToken, "")

		require.Equal(t, http.StatusOK, recorder.Code)
		require.JSONEq(t, `{"profile":{"user_uuid":"`+testUserUUID+`","display_name":"Ana","level":"guest","mode":"normal","language":"","methods":[]},
			"declaration_known":true,"declaration":null,"limits_known":true,"limits":`+limitsJSON+`}`, recorder.Body.String())
	})

	t.Run("an account carries its mode, language, methods and declaration", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		data := NewMockdataReader(ctrl)
		added := time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)
		data.EXPECT().ReadMyData(gomock.Any(), testUserUUID).Return(mydata.MyData{
			Profile: identity.Profile{
				UUID: testUserUUID, DisplayName: "Ana", Level: identity.LevelAccount, Mode: identity.ModeTeacher, Language: "es",
				Methods: []identity.SignInMethod{{MethodID: "m1", Issuer: identity.ProviderEmail, Address: "ana@x.com", Name: "Ana", AddedAt: added}},
			},
			Declaration: compliance.CurrentDeclaration{
				Found: true, Country: "CO", MeetsAge: true, ThresholdAge: 18, DeclaredAt: added,
			},
			DeclarationKnown: true,
			Limits:           limits,
			LimitsKnown:      true,
		}, nil)
		handler := NewHandler(stubAuthorizer{level: identity.LevelAccount}, Dependencies{Data: data})

		recorder := serve(t, handler, http.MethodGet, "/v1/me", testToken, "")

		require.Equal(t, http.StatusOK, recorder.Code)
		require.JSONEq(t, `{"profile":{"user_uuid":"`+testUserUUID+`","display_name":"Ana","level":"account","mode":"teacher","language":"es",
			"methods":[{"method_id":"m1","issuer":"email","address":"ana@x.com","name":"Ana","added_at":"2026-11-10T12:00:00Z"}]},
			"declaration_known":true,
			"declaration":{"country":"CO","subdivision":"","meets_age":true,"threshold_age":18,"declared_at":"2026-11-10T12:00:00Z"},
			"limits_known":true,"limits":`+limitsJSON+`}`, recorder.Body.String())
	})

	t.Run("a piece that could not be read is answered as missing", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		data := NewMockdataReader(ctrl)
		data.EXPECT().ReadMyData(gomock.Any(), testUserUUID).Return(mydata.MyData{
			Profile: identity.Profile{UUID: testUserUUID, DisplayName: "Ana", Level: identity.LevelAccount, Mode: identity.ModeNormal},
		}, nil)
		handler := NewHandler(stubAuthorizer{level: identity.LevelAccount}, Dependencies{Data: data})

		recorder := serve(t, handler, http.MethodGet, "/v1/me", testToken, "")

		require.Equal(t, http.StatusOK, recorder.Code)
		require.Contains(t, recorder.Body.String(), `"declaration_known":false`)
		require.Contains(t, recorder.Body.String(), `"limits_known":false`)
	})
}

func TestSetMode(t *testing.T) {
	t.Run("the fresh token comes back with the new mode", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		modes := NewMockmodeSetter(ctrl)
		issued := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
		modes.EXPECT().SetMode(gomock.Any(), testUserUUID, "teacher").Return(identity.SetModeResult{
			Outcome: identity.ModeSet,
			Credentials: identity.Credentials{
				User:            identity.User{UUID: testUserUUID, DisplayName: "Ana", Level: identity.LevelAccount, Mode: identity.ModeTeacher},
				AccessToken:     "fresh-jwt",
				AccessIssuedAt:  issued,
				AccessExpiresAt: issued.Add(15 * time.Minute),
			},
		}, nil)
		handler := NewHandler(stubAuthorizer{level: identity.LevelAccount}, Dependencies{Modes: modes})

		recorder := serve(t, handler, http.MethodPut, "/v1/identity/me/mode", testToken, `{"mode":"teacher"}`)

		require.Equal(t, http.StatusOK, recorder.Code)
		require.JSONEq(t, `{"mode":"teacher","access_token":"fresh-jwt","access_expires_at":"2026-11-16T12:15:00Z","access_expires_in":900}`, recorder.Body.String())
	})

	t.Run("a guest never reaches the operation", func(t *testing.T) {
		handler := NewHandler(stubAuthorizer{level: identity.LevelGuest}, Dependencies{})

		recorder := serve(t, handler, http.MethodPut, "/v1/identity/me/mode", testToken, `{"mode":"teacher"}`)

		require.Equal(t, http.StatusForbidden, recorder.Code)
	})

	t.Run("an unknown mode is refused", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		modes := NewMockmodeSetter(ctrl)
		modes.EXPECT().SetMode(gomock.Any(), testUserUUID, "student").Return(identity.SetModeResult{Outcome: identity.ModeInvalid}, nil)
		handler := NewHandler(stubAuthorizer{level: identity.LevelAccount}, Dependencies{Modes: modes})

		recorder := serve(t, handler, http.MethodPut, "/v1/identity/me/mode", testToken, `{"mode":"student"}`)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Contains(t, recorder.Body.String(), "invalid_mode")
	})
}

func TestSetLanguage(t *testing.T) {
	t.Run("an account stores the language it reads in", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		languages := NewMocklanguageSetter(ctrl)
		languages.EXPECT().SetLanguage(gomock.Any(), testUserUUID, "pt").Return(identity.LanguageSet, nil)
		handler := NewHandler(stubAuthorizer{level: identity.LevelAccount}, Dependencies{Languages: languages})

		recorder := serve(t, handler, http.MethodPut, "/v1/identity/me/language", testToken, `{"language":"pt"}`)

		require.Equal(t, http.StatusOK, recorder.Code)
		require.JSONEq(t, `{"language":"pt"}`, recorder.Body.String())
	})

	t.Run("a language Playhoot does not speak is refused", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		languages := NewMocklanguageSetter(ctrl)
		languages.EXPECT().SetLanguage(gomock.Any(), testUserUUID, "fr").Return(identity.LanguageInvalid, nil)
		handler := NewHandler(stubAuthorizer{level: identity.LevelAccount}, Dependencies{Languages: languages})

		recorder := serve(t, handler, http.MethodPut, "/v1/identity/me/language", testToken, `{"language":"fr"}`)

		require.Equal(t, http.StatusBadRequest, recorder.Code)
		require.Contains(t, recorder.Body.String(), "invalid_language")
	})
}
