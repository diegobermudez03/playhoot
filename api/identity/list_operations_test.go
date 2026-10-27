package identity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	identitypkg "github.com/diegobermudez03/playhoot/identity"
)

func TestHandleListOperations(t *testing.T) {
	type dependencies struct {
		operations *MockoperationLister
	}
	type test struct {
		expectedBody string
	}

	tests := map[string]func(t *testing.T, deps dependencies) test{
		"catalog_listed_in_order": func(t *testing.T, deps dependencies) test {
			deps.operations.EXPECT().ListOperations(gomock.Any()).Return([]identitypkg.OperationLevel{
				{Operation: identitypkg.OperationCreateSession, RequiredLevel: identitypkg.LevelAccount},
				{Operation: identitypkg.OperationJoinSessionAsPlayer, RequiredLevel: identitypkg.LevelGuest},
			})
			return test{
				expectedBody: `{"operations":[{"operation":"CREATE_SESSION","required_level":"account"},{"operation":"JOIN_SESSION_AS_PLAYER","required_level":"guest"}]}`,
			}
		},
		"empty_catalog_is_an_empty_list": func(t *testing.T, deps dependencies) test {
			deps.operations.EXPECT().ListOperations(gomock.Any()).Return(nil)
			return test{expectedBody: `{"operations":[]}`}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			deps := dependencies{operations: NewMockoperationLister(ctrl)}
			tc := setup(t, deps)

			// No Authorization header: the catalog is public.
			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/identity/operations", nil)
			rec := httptest.NewRecorder()
			NewHandler(NewMockguestCreator(ctrl), deps.operations).handleListOperations(rec, req)

			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, "public, max-age=300", rec.Header().Get("Cache-Control"))
			require.JSONEq(t, tc.expectedBody, rec.Body.String())
		})
	}
}
