package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/diegobermudez03/playhoot/api"
	"github.com/diegobermudez03/playhoot/session"
	"github.com/stretchr/testify/require"
)

// fakeContentAccessor is a hand-written test double for
// api/session.ContentAccessor. It records what the handler passed it so a
// test can assert the path and query were decoded correctly.
type fakeContentAccessor struct {
	result session.ContentAccessResult
	err    error

	gotSession session.SessionUUID
	gotUser    session.UserUUID
	gotKey     string
	calls      int
}

func (f *fakeContentAccessor) GetFrontendScriptAccess(ctx context.Context, sessionUUID session.SessionUUID, userUUID session.UserUUID) (session.ContentAccessResult, error) {
	f.calls++
	f.gotSession, f.gotUser = sessionUUID, userUUID
	return f.result, f.err
}

func (f *fakeContentAccessor) GetAssetAccess(ctx context.Context, sessionUUID session.SessionUUID, userUUID session.UserUUID, key string) (session.ContentAccessResult, error) {
	f.calls++
	f.gotSession, f.gotUser, f.gotKey = sessionUUID, userUUID, key
	return f.result, f.err
}

func TestContentAccessEndpoints(t *testing.T) {
	expiresAt := time.Now().UTC().Add(2 * time.Minute).Truncate(time.Second)
	granted := session.ContentAccessResult{
		Outcome:     session.ContentAccessOutcomeGranted,
		URL:         "https://storage.example/signed",
		ExpiresAt:   expiresAt,
		SHA256:      "abc123",
		ContentType: "image/png",
		Size:        42,
	}

	tests := map[string]struct {
		path       string
		accessor   *fakeContentAccessor
		wantStatus int
		check      func(t *testing.T, accessor *fakeContentAccessor, body map[string]any)
	}{
		"frontend script granted": {
			path:       "/sessions/s-1/frontend-script?user_uuid=u-1",
			accessor:   &fakeContentAccessor{result: granted},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, accessor *fakeContentAccessor, body map[string]any) {
				require.Equal(t, session.SessionUUID("s-1"), accessor.gotSession)
				require.Equal(t, session.UserUUID("u-1"), accessor.gotUser)
				require.Equal(t, "https://storage.example/signed", body["url"])
				require.Equal(t, "abc123", body["sha256"])
				require.Equal(t, "image/png", body["content_type"])
				require.Equal(t, float64(42), body["size"])
			},
		},
		"asset granted decodes the logical key": {
			path:       "/sessions/s-1/assets/card-back?user_uuid=u-1",
			accessor:   &fakeContentAccessor{result: granted},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, accessor *fakeContentAccessor, body map[string]any) {
				require.Equal(t, "card-back", accessor.gotKey)
				require.Equal(t, session.SessionUUID("s-1"), accessor.gotSession)
			},
		},
		"non participant is forbidden": {
			path:       "/sessions/s-1/frontend-script?user_uuid=u-1",
			accessor:   &fakeContentAccessor{result: session.ContentAccessResult{Outcome: session.ContentAccessOutcomeNotAParticipant}},
			wantStatus: http.StatusForbidden,
		},
		"undeclared asset is not found": {
			path:       "/sessions/s-1/assets/nope?user_uuid=u-1",
			accessor:   &fakeContentAccessor{result: session.ContentAccessResult{Outcome: session.ContentAccessOutcomeContentNotFound}},
			wantStatus: http.StatusNotFound,
		},
		"unknown session is not found": {
			path:       "/sessions/missing/frontend-script?user_uuid=u-1",
			accessor:   &fakeContentAccessor{err: session.ErrSessionNotFound},
			wantStatus: http.StatusNotFound,
		},
		"unexpected failure is a 500": {
			path:       "/sessions/s-1/frontend-script?user_uuid=u-1",
			accessor:   &fakeContentAccessor{err: errors.New("signing failed")},
			wantStatus: http.StatusInternalServerError,
		},
		"missing user_uuid is rejected before any lookup (frontend script)": {
			path:       "/sessions/s-1/frontend-script",
			accessor:   &fakeContentAccessor{result: granted},
			wantStatus: http.StatusBadRequest,
			check: func(t *testing.T, accessor *fakeContentAccessor, _ map[string]any) {
				require.Zero(t, accessor.calls)
			},
		},
		"missing user_uuid is rejected before any lookup (asset)": {
			path:       "/sessions/s-1/assets/card-back",
			accessor:   &fakeContentAccessor{result: granted},
			wantStatus: http.StatusBadRequest,
			check: func(t *testing.T, accessor *fakeContentAccessor, _ map[string]any) {
				require.Zero(t, accessor.calls)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			srv := api.NewServer(&fakeSessionCreator{}, tc.accessor)
			ts := httptest.NewServer(srv.Routes())
			defer ts.Close()

			resp, err := http.Get(ts.URL + tc.path)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, tc.wantStatus, resp.StatusCode)

			var body map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&body)
			if tc.check != nil {
				tc.check(t, tc.accessor, body)
			}
		})
	}
}
