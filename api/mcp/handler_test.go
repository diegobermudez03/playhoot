package mcp

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/api/games"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/identity"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

const user = "6d0796d4-36b8-4817-b1cb-6522ce6b18ae"

type fakeVerifier struct{}

func (fakeVerifier) VerifyAccessToken(ctx context.Context, token string) (identity.AIAccess, bool, error) {
	switch token {
	case "good":
		return identity.AIAccess{UserUUID: user, ConnectionID: "c1", Scopes: []string{identity.AIScopeGames}, ExpiresAt: time.Now().Add(time.Hour)}, true, nil
	case "no-scope":
		return identity.AIAccess{UserUUID: user, ConnectionID: "c1", ExpiresAt: time.Now().Add(time.Hour)}, true, nil
	default:
		return identity.AIAccess{}, false, nil
	}
}

// fakeGames records the operations called and answers what the test sets.
type fakeGames struct {
	mu       sync.Mutex
	callers  []games.Caller
	users    []string
	changes  []games.ChangeRequest
	gameIDs  []string
	revision *int64
	since    int64
	from     int64
	paths    []string
	before   int64
	limit    int
	inline   *bool
	wait     time.Duration
	upload   []byte
	key      string
	idemKey  string

	change  games.ChangeResponse
	asset   games.AssetEditResponse
	refusal *games.Refusal
}

func (f *fakeGames) ListGames(ctx context.Context, userUUID string) ([]games.GameSummaryResponse, *games.Refusal) {
	panic("not used")
}

func (f *fakeGames) CreateGame(ctx context.Context, userUUID string, req games.CreateGameRequest) (games.CreateGameResponse, *games.Refusal) {
	panic("not used")
}

func (f *fakeGames) ReadContext(ctx context.Context, userUUID string, gameUUID string, linkToken string, inlineSources bool) (games.AIContext, *games.Refusal) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users = append(f.users, userUUID)
	f.inline = &inlineSources
	return games.AIContext{Response: games.AIContextResponse{Format: "playhoot.ai-context.v1", Revision: 4}}, f.refusal
}

func (f *fakeGames) ReadFiles(ctx context.Context, userUUID string, gameUUID string, paths []string, revision *int64) (*games.FilesResponse, *games.Refusal) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paths, f.revision = paths, revision
	return &games.FilesResponse{Revision: 4, Files: nil, Missing: []string{}}, f.refusal
}

func (f *fakeGames) ReadChanges(ctx context.Context, userUUID string, gameUUID string, since int64, revision *int64) (*games.RevisionChangesResponse, *games.Refusal) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.since, f.revision = since, revision
	return &games.RevisionChangesResponse{Since: since, Revision: 4}, f.refusal
}

func (f *fakeGames) ApplyChanges(ctx context.Context, by games.Caller, gameUUID string, req games.ChangeRequest) (games.ChangeResponse, *games.Refusal) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callers = append(f.callers, by)
	f.gameIDs = append(f.gameIDs, gameUUID)
	f.changes = append(f.changes, req)
	return f.change, f.refusal
}

func (f *fakeGames) RestoreRevision(ctx context.Context, by games.Caller, gameUUID string, req games.RestoreRequest) (games.ChangeResponse, *games.Refusal) {
	panic("not used")
}

func (f *fakeGames) ReadDiff(ctx context.Context, userUUID string, gameUUID string, from int64, to *int64, onlyPath string) (*games.DiffResponse, *games.Refusal) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.from, f.revision, f.paths = from, to, []string{onlyPath}
	return &games.DiffResponse{From: from, To: 5, Files: []games.FileDiffEntryBody{}}, f.refusal
}

func (f *fakeGames) ListRevisions(ctx context.Context, userUUID string, gameUUID string, before int64, limit int) ([]games.RevisionResponse, *games.Refusal) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.before, f.limit = before, limit
	return []games.RevisionResponse{}, f.refusal
}

func (f *fakeGames) UploadAsset(ctx context.Context, by games.Caller, gameUUID string, key string, idempotencyKey string, content io.Reader) (games.AssetEditResponse, *games.Refusal) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callers = append(f.callers, by)
	f.key, f.idemKey = key, idempotencyKey
	f.upload, _ = io.ReadAll(content)
	return f.asset, f.refusal
}

func (f *fakeGames) RemoveAsset(ctx context.Context, by games.Caller, gameUUID string, key string, idempotencyKey string) (games.AssetEditResponse, *games.Refusal) {
	panic("not used")
}

func (f *fakeGames) ReadBuild(ctx context.Context, userUUID string, gameUUID string, revision *int64, wait time.Duration) (*games.BuildResponse, *games.Refusal) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.users = append(f.users, userUUID)
	f.revision, f.wait = revision, wait
	return nil, f.refusal
}

func (f *fakeGames) Simulate(ctx context.Context, userUUID string, gameUUID string, req games.ScenarioRequest) (games.SimulationResponse, *games.Refusal) {
	panic("not used")
}

type bearer struct{ token string }

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(r)
}

func serve(t *testing.T, operations gameOperations) *httptest.Server {
	t.Helper()
	h := NewHandler(fakeVerifier{}, operations, Config{ResourceMetadataURL: "https://api.test/.well-known/oauth-protected-resource/mcp", GuideURL: "https://web.test/docs/sdk/v1/index.md"})
	mux := http.NewServeMux()
	for _, route := range h.RESTRoutes() {
		mux.HandleFunc(route.Pattern, route.Authorization(route.Handler))
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return server
}

func connect(t *testing.T, url string, token string) *sdk.ClientSession {
	t.Helper()
	client := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(context.Background(), &sdk.StreamableClientTransport{
		Endpoint: url + "/mcp", HTTPClient: &http.Client{Transport: bearer{token: token}}, DisableStandaloneSSE: true, MaxRetries: -1,
	}, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func TestUnauthenticatedRequestsLearnWhereToAuthorize(t *testing.T) {
	server := serve(t, &fakeGames{})
	for token, status := range map[string]int{"": http.StatusUnauthorized, "bad": http.StatusUnauthorized, "no-scope": http.StatusForbidden} {
		req, err := http.NewRequest(http.MethodPost, server.URL+"/mcp", strings.NewReader(`{}`))
		require.NoError(t, err)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = res.Body.Close()
		require.Equal(t, status, res.StatusCode, token)
		require.Contains(t, res.Header.Get("WWW-Authenticate"), `resource_metadata="https://api.test/.well-known/oauth-protected-resource/mcp"`, token)
	}
}

func TestToolsCallTheirGamesOperations(t *testing.T) {
	ctx := context.Background()
	fake := &fakeGames{change: games.ChangeResponse{Outcome: "applied", Revision: 2}}
	session := connect(t, serve(t, fake).URL, "good")

	listed, err := session.ListTools(ctx, nil)
	require.NoError(t, err)
	var names []string
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}
	sort.Strings(names)
	require.Equal(t, []string{"apply_changes", "create_game", "diff_revisions", "get_build", "get_game_context", "list_changes_since", "list_my_games", "list_revisions", "read_files", "remove_asset", "restore_revision", "simulate", "upload_asset"}, names)
	require.NotContains(t, names, "publish")

	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "apply_changes", Arguments: map[string]any{
		"game_uuid": "g1", "base_revision": 1, "files": []map[string]any{{"path": "notes.md", "content": "# Notes"}}, "idempotency_key": "k1",
		"edits": []map[string]any{{"path": "server/game.ts", "find": "a", "replace": "b"}, {"path": "server/game.ts", "find": "c", "replace": "d", "all": true}},
	}})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.JSONEq(t, `{"outcome":"applied","revision":2}`, result.Content[0].(*sdk.TextContent).Text)
	require.Len(t, fake.changes, 1)
	require.Equal(t, "g1", fake.gameIDs[0])
	require.Equal(t, games.Caller{UserUUID: user, Origin: sessionpkg.DraftOriginMCP}, fake.callers[0], "the call acts for the token's user, from MCP")
	change := fake.changes[0]
	require.Equal(t, "playhoot.project-change.v1", change.Format)
	require.Equal(t, "k1", change.IdempotencyKey)
	require.Equal(t, int64(1), *change.BaseRevision)
	require.Equal(t, []string{}, change.Delete)
	require.Equal(t, []games.ChangeFile{{Path: "notes.md", Content: "# Notes"}}, change.Files)
	require.Equal(t, []games.ChangeEdit{{Path: "server/game.ts", Find: "a", Replace: "b"}, {Path: "server/game.ts", Find: "c", Replace: "d", All: true}}, change.Edits)

	missing, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "apply_changes", Arguments: map[string]any{"game_uuid": "g1", "base_revision": 1, "files": []any{}}})
	require.True(t, err != nil || missing.IsError, "idempotency_key is required by the tool's schema")
	require.Len(t, fake.changes, 1, "a call missing its idempotency key never reaches the operation")
}

func TestReadToolsAnswerOnlyWhatIsAsked(t *testing.T) {
	ctx := context.Background()
	fake := &fakeGames{}
	session := connect(t, serve(t, fake).URL, "good")
	call := func(name string, arguments map[string]any) string {
		t.Helper()
		result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: name, Arguments: arguments})
		require.NoError(t, err)
		require.False(t, result.IsError, name)
		return result.Content[0].(*sdk.TextContent).Text
	}

	call("get_game_context", map[string]any{"game_uuid": "g1"})
	require.NotNil(t, fake.inline)
	require.False(t, *fake.inline, "the context through MCP never carries the sources")

	call("read_files", map[string]any{"game_uuid": "g1", "paths": []string{"server/game.ts", "notes.md"}})
	require.Equal(t, []string{"server/game.ts", "notes.md"}, fake.paths)
	require.Nil(t, fake.revision, "no revision means the current one")

	answer := call("list_changes_since", map[string]any{"game_uuid": "g1", "since": 2, "revision": 4})
	require.Equal(t, int64(2), fake.since)
	require.Equal(t, int64(4), *fake.revision)
	require.Contains(t, answer, `"since":2`)

	call("list_revisions", map[string]any{"game_uuid": "g1", "before": 9, "limit": 5})
	require.Equal(t, int64(9), fake.before)
	require.Equal(t, 5, fake.limit)

	answer = call("diff_revisions", map[string]any{"game_uuid": "g1", "from": 2, "path": "notes.md"})
	require.Equal(t, int64(2), fake.from)
	require.Nil(t, fake.revision, "no to means the current one")
	require.Equal(t, []string{"notes.md"}, fake.paths)
	require.Contains(t, answer, `"to":5`)
}

func TestRefusalsAreToolErrors(t *testing.T) {
	ctx := context.Background()
	fake := &fakeGames{refusal: &games.Refusal{Status: http.StatusConflict, Body: map[string]any{"code": "revision_conflict", "current_revision": 3}}}
	session := connect(t, serve(t, fake).URL, "good")

	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "get_build", Arguments: map[string]any{"game_uuid": "g1", "revision": 2, "wait_seconds": 20}})
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.JSONEq(t, `{"status":409,"answer":{"code":"revision_conflict","current_revision":3}}`, result.Content[0].(*sdk.TextContent).Text)
	require.Equal(t, int64(2), *fake.revision)
	require.Equal(t, 20*time.Second, fake.wait)
	require.Equal(t, user, fake.users[0])

	tooLong, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "get_build", Arguments: map[string]any{"game_uuid": "g1", "revision": 2, "wait_seconds": 30}})
	require.NoError(t, err)
	require.True(t, tooLong.IsError)
	require.JSONEq(t, `{"status":400,"answer":{"message":"wait must be a whole number of seconds from 0 to 20"}}`, tooLong.Content[0].(*sdk.TextContent).Text)
	require.Len(t, fake.users, 1, "a wait over the limit never reaches the operation")
}

func TestUploadDecodesBase64AndRefusesOversizedContentUnread(t *testing.T) {
	ctx := context.Background()
	fake := &fakeGames{asset: games.AssetEditResponse{Outcome: "attached", Revision: 3}}
	session := connect(t, serve(t, fake).URL, "good")

	result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: "upload_asset", Arguments: map[string]any{
		"game_uuid": "g1", "key": "cardBack", "content_base64": base64.StdEncoding.EncodeToString([]byte("PNG bytes")), "idempotency_key": "k2",
	}})
	require.NoError(t, err)
	require.False(t, result.IsError)
	require.Equal(t, []byte("PNG bytes"), fake.upload)
	require.Equal(t, "k2", fake.idemKey)
	require.Equal(t, "cardBack", fake.key)

	result, err = session.CallTool(ctx, &sdk.CallToolParams{Name: "upload_asset", Arguments: map[string]any{
		"game_uuid": "g1", "key": "cardBack", "content_base64": "%%%", "idempotency_key": "k3",
	}})
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Len(t, fake.callers, 1, "invalid base64 never reaches the operation")
	require.Greater(t, maxAssetBase64, sessionpkg.MaxDraftAssetBytes)
}

type allowAll struct{}

func (allowAll) Authorize(ctx context.Context, token string, operation identity.Operation) (identity.User, identity.AccessDecision, error) {
	return identity.User{UUID: user, Level: identity.LevelAccount}, identity.AccessAllowed, nil
}

// The same bad input gets the same refusal through the HTTP route and the
// MCP tool: both translate the one games operation. These inputs are refused
// before any domain is called, so the handler needs none.
func TestHTTPAndMCPRefuseAlike(t *testing.T) {
	ctx := context.Background()
	gamesHandler := games.NewHandler(allowAll{}, nil, nil, nil, games.AILinks{}, games.Cards{})
	mux := http.NewServeMux()
	for _, route := range gamesHandler.RESTRoutes() {
		mux.HandleFunc(route.Pattern, route.Authorization(route.Handler))
	}
	api := httptest.NewServer(mux)
	t.Cleanup(api.Close)
	session := connect(t, serve(t, gamesHandler).URL, "good")
	const game = "7c9e6679-7425-40de-944b-e07fc1f90ae7"

	cases := map[string]struct {
		method, path, body string
		tool               string
		arguments          map[string]any
	}{
		"a game nobody can have": {
			method: http.MethodPost, path: "/v1/games/not-a-uuid/draft/changes",
			body: `{"format":"playhoot.project-change.v1","base_revision":1,"files":[],"idempotency_key":"k"}`,
			tool: "apply_changes", arguments: map[string]any{"game_uuid": "not-a-uuid", "base_revision": 1, "files": []any{}, "idempotency_key": "k"},
		},
		"restoring revision 0": {
			method: http.MethodPost, path: "/v1/games/" + game + "/draft/restore",
			body: `{"revision":0,"base_revision":1,"idempotency_key":"k"}`,
			tool: "restore_revision", arguments: map[string]any{"game_uuid": game, "revision": 0, "base_revision": 1, "idempotency_key": "k"},
		},
		"waiting too long for a build": {
			method: http.MethodGet, path: "/v1/games/" + game + "/draft/build?revision=1&wait=30",
			tool: "get_build", arguments: map[string]any{"game_uuid": game, "revision": 1, "wait_seconds": 30},
		},
		"a scenario of another format": {
			method: http.MethodPost, path: "/v1/games/" + game + "/draft/simulations",
			body: `{"format":"other","players":2,"duration_ms":1000}`,
			tool: "simulate", arguments: map[string]any{"game_uuid": game, "scenario": map[string]any{"format": "other", "players": 2, "duration_ms": 1000}},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, api.URL+tc.path, strings.NewReader(tc.body))
			require.NoError(t, err)
			req.Header.Set("Authorization", "Bearer web")
			res, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			body, err := io.ReadAll(res.Body)
			_ = res.Body.Close()
			require.NoError(t, err)
			require.GreaterOrEqual(t, res.StatusCode, 400)

			result, err := session.CallTool(ctx, &sdk.CallToolParams{Name: tc.tool, Arguments: tc.arguments})
			require.NoError(t, err)
			require.True(t, result.IsError)
			require.JSONEq(t, `{"status":`+strconv.Itoa(res.StatusCode)+`,"answer":`+string(body)+`}`, result.Content[0].(*sdk.TextContent).Text)
		})
	}
}

// The answer a refused tool call carries is exactly what its HTTP route
// answers.
func TestRefusedAnswerIsTheHTTPBody(t *testing.T) {
	refusal := &games.Refusal{Status: http.StatusBadRequest, Body: httpx.ErrorResponse{Message: "base_revision is required"}}
	recorder := httptest.NewRecorder()
	httpx.WriteJSON(recorder, refusal.Status, refusal.Body)
	text := refused(refusal).Content[0].(*sdk.TextContent).Text
	require.JSONEq(t, `{"status":400,"answer":`+recorder.Body.String()+`}`, text)
}
