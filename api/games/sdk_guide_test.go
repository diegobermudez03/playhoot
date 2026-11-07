package games

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

type fakeGuide map[string]string

func (f fakeGuide) ReadPage(ctx context.Context, page string) (sessionpkg.SDKGuidePage, error) {
	content, found := f[page]
	if !found {
		return sessionpkg.SDKGuidePage{}, sessionpkg.ErrSDKGuidePageNotFound
	}
	return sessionpkg.SDKGuidePage{Name: page, Content: content, ToolchainVersion: "tc-1"}, nil
}

func TestHandleAISDKPage(t *testing.T) {
	serve := func(guide sdkGuide, page string) *httptest.ResponseRecorder {
		h, _ := newTestHandler(gomock.NewController(t))
		h.links.Guide = guide
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.SetPathValue("page", page)
		recorder := httptest.NewRecorder()
		h.handleAISDKPage(recorder, request)
		return recorder
	}
	guide := fakeGuide{"index.md": "# Start\n", "examples/quiz.md": "# Quiz\n"}

	page := serve(guide, "examples/quiz.md")
	require.Equal(t, http.StatusOK, page.Code)
	require.Equal(t, "# Quiz\n", page.Body.String())
	require.Equal(t, "text/markdown; charset=utf-8", page.Header().Get("Content-Type"))
	require.Equal(t, "tc-1", page.Header().Get("Playhoot-Toolchain-Version"))

	start := serve(guide, "")
	require.Equal(t, "# Start\n", start.Body.String(), "the guide starts at index.md")

	missing := serve(guide, "nope.md")
	require.Equal(t, http.StatusNotFound, missing.Code)
	require.JSONEq(t, `{"code":"sdk_page_not_found","message":"the guide has no such page; capabilities.md lists them"}`, missing.Body.String())

	unconfigured := serve(nil, "index.md")
	require.Equal(t, http.StatusServiceUnavailable, unconfigured.Code)
}
