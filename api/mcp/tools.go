package mcp

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/diegobermudez03/playhoot/api/games"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

// The change envelope's format (the games operation's own).
const changeFormat = "playhoot.project-change.v1"

// maxAssetBase64 is the longest base64 an upload may carry: the largest
// asset plus the one byte that tells a file over the limit, encoded.
var maxAssetBase64 = base64.StdEncoding.EncodedLen(sessionpkg.MaxDraftAssetBytes + 1)

type listGamesInput struct{}

type createGameInput struct {
	Name           string `json:"name" jsonschema:"the game's name, 1 to 80 characters; the user can change it later"`
	IdempotencyKey string `json:"idempotency_key" jsonschema:"a new random string for this creation; resend the same one only to retry it"`
}

type gameInput struct {
	GameUUID string `json:"game_uuid" jsonschema:"the game's uuid, from list_my_games or create_game"`
}

type readFileInput struct {
	GameUUID string `json:"game_uuid" jsonschema:"the game's uuid"`
	Path     string `json:"path" jsonschema:"the file's path in the project, such as server/game.ts"`
	Revision int64  `json:"revision,omitempty" jsonschema:"the revision to read; the current one when omitted"`
}

type changeFile struct {
	Path    string `json:"path" jsonschema:"the file's path in the project"`
	Content string `json:"content" jsonschema:"the file's complete new content"`
}

type applyChangesInput struct {
	GameUUID       string       `json:"game_uuid" jsonschema:"the game's uuid"`
	BaseRevision   int64        `json:"base_revision" jsonschema:"the revision you read; a revision_conflict answer means the game changed since: read it again"`
	Files          []changeFile `json:"files" jsonschema:"only the files that change, each with its whole new content"`
	Delete         []string     `json:"delete,omitempty" jsonschema:"paths of files to remove"`
	IdempotencyKey string       `json:"idempotency_key" jsonschema:"a new random string for this change; resend the same one only to retry it"`
}

type restoreRevisionInput struct {
	GameUUID       string `json:"game_uuid" jsonschema:"the game's uuid"`
	Revision       int64  `json:"revision" jsonschema:"the earlier revision whose files and assets come back"`
	BaseRevision   int64  `json:"base_revision" jsonschema:"the current revision you read"`
	IdempotencyKey string `json:"idempotency_key" jsonschema:"a new random string for this restore"`
}

type uploadAssetInput struct {
	GameUUID       string `json:"game_uuid" jsonschema:"the game's uuid"`
	Key            string `json:"key" jsonschema:"the asset's key, as game.project.json declares it"`
	ContentBase64  string `json:"content_base64" jsonschema:"the file's bytes in standard base64; its real type must match the declared one (image, audio or video) and stay within its size limit"`
	IdempotencyKey string `json:"idempotency_key" jsonschema:"a new random string for this upload"`
}

type removeAssetInput struct {
	GameUUID       string `json:"game_uuid" jsonschema:"the game's uuid"`
	Key            string `json:"key" jsonschema:"the asset's key"`
	IdempotencyKey string `json:"idempotency_key" jsonschema:"a new random string for this removal"`
}

type getBuildInput struct {
	GameUUID    string `json:"game_uuid" jsonschema:"the game's uuid"`
	Revision    int64  `json:"revision" jsonschema:"the revision whose build to read"`
	WaitSeconds int    `json:"wait_seconds,omitempty" jsonschema:"wait up to this many seconds (at most 20) for the build to end"`
}

type simulateInput struct {
	GameUUID string         `json:"game_uuid" jsonschema:"the game's uuid"`
	Scenario map[string]any `json:"scenario" jsonschema:"a playhoot.scenario.v1 document, as the guide's scenarios page describes"`
}

// addTools declares every tool on server. Each one translates its arguments
// to its games operation, and the operation's answer to its result.
func (h *Handler) addTools(server *sdk.Server) {
	sdk.AddTool(server, &sdk.Tool{Name: "list_my_games", Description: "Lists the user's games, the most recently changed first, with whether each is published and has a version being edited."},
		func(ctx context.Context, req *sdk.CallToolRequest, _ listGamesInput) (*sdk.CallToolResult, any, error) {
			user, err := userOf(req)
			if err != nil {
				return nil, nil, err
			}
			return answer(h.games.ListGames(ctx, user))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "create_game", Description: "Creates a new, unpublished game for the user, with a starter project as revision 1 (it builds automatically). Answers its game_uuid."},
		func(ctx context.Context, req *sdk.CallToolRequest, in createGameInput) (*sdk.CallToolResult, any, error) {
			user, err := userOf(req)
			if err != nil {
				return nil, nil, err
			}
			return answer(h.games.CreateGame(ctx, user, games.CreateGameRequest{Name: in.Name, IdempotencyKey: in.IdempotencyKey}))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "get_game_context", Description: "Reads everything needed to change a game: its current revision and when it was observed, the build of that revision with its diagnostics, the declared assets, the recent attempts, notes.md and the sources (inline when small), and the SDK guide's address. Read it again after any change."},
		func(ctx context.Context, req *sdk.CallToolRequest, in gameInput) (*sdk.CallToolResult, any, error) {
			user, err := userOf(req)
			if err != nil {
				return nil, nil, err
			}
			// No link: the tools replace the link's addresses.
			read, refusal := h.games.ReadContext(ctx, user, in.GameUUID, "")
			return answer(read.Response, refusal)
		})
	sdk.AddTool(server, &sdk.Tool{Name: "read_file", Description: "Reads one file of a revision of the game as plain text."},
		func(ctx context.Context, req *sdk.CallToolRequest, in readFileInput) (*sdk.CallToolResult, any, error) {
			user, err := userOf(req)
			if err != nil {
				return nil, nil, err
			}
			var revision *int64
			if in.Revision > 0 {
				revision = &in.Revision
			}
			file, refusal := h.games.ReadFile(ctx, user, in.GameUUID, in.Path, revision)
			if refusal != nil {
				return refused(refusal), nil, nil
			}
			return text(file.Content), nil, nil
		})
	sdk.AddTool(server, &sdk.Tool{Name: "apply_changes", Description: "Changes the game's sources on the revision you read: the files given replace theirs whole, the paths in delete are removed. Applied, it makes a new revision, which builds automatically (wait for it with get_build)."},
		func(ctx context.Context, req *sdk.CallToolRequest, in applyChangesInput) (*sdk.CallToolResult, any, error) {
			by, err := callerOf(req)
			if err != nil {
				return nil, nil, err
			}
			change := games.ChangeRequest{Format: changeFormat, BaseRevision: &in.BaseRevision, Delete: in.Delete, IdempotencyKey: in.IdempotencyKey}
			if change.Delete == nil {
				change.Delete = []string{}
			}
			for _, file := range in.Files {
				change.Files = append(change.Files, games.ChangeFile{Path: file.Path, Content: file.Content})
			}
			return answer(h.games.ApplyChanges(ctx, by, in.GameUUID, change))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "restore_revision", Description: "Makes a new revision with an earlier revision's files and assets, to recover from a broken change. History is never rewritten."},
		func(ctx context.Context, req *sdk.CallToolRequest, in restoreRevisionInput) (*sdk.CallToolResult, any, error) {
			by, err := callerOf(req)
			if err != nil {
				return nil, nil, err
			}
			return answer(h.games.RestoreRevision(ctx, by, in.GameUUID, games.RestoreRequest{Revision: in.Revision, BaseRevision: &in.BaseRevision, IdempotencyKey: in.IdempotencyKey}))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "list_revisions", Description: "Lists the game's revisions, newest first, with where each came from."},
		func(ctx context.Context, req *sdk.CallToolRequest, in gameInput) (*sdk.CallToolResult, any, error) {
			user, err := userOf(req)
			if err != nil {
				return nil, nil, err
			}
			return answer(h.games.ListRevisions(ctx, user, in.GameUUID))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "upload_asset", Description: "Uploads the file of an asset game.project.json declares, in base64. Limits by type: image 5 MiB, audio 10 MiB, video 50 MiB, 100 MiB in all. If you cannot produce the file, ask the user to upload it in Recursos."},
		func(ctx context.Context, req *sdk.CallToolRequest, in uploadAssetInput) (*sdk.CallToolResult, any, error) {
			by, err := callerOf(req)
			if err != nil {
				return nil, nil, err
			}
			if len(in.ContentBase64) > maxAssetBase64 {
				return refusal("asset_too_large", fmt.Sprintf("the file is larger than the largest asset allowed (%d bytes)", sessionpkg.MaxDraftAssetBytes)), nil, nil
			}
			content, err := base64.StdEncoding.DecodeString(in.ContentBase64)
			if err != nil {
				return refusal("invalid_base64", "content_base64 must be standard base64"), nil, nil
			}
			return answer(h.games.UploadAsset(ctx, by, in.GameUUID, in.Key, in.IdempotencyKey, bytes.NewReader(content)))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "remove_asset", Description: "Removes the file bound to an asset key in a new revision."},
		func(ctx context.Context, req *sdk.CallToolRequest, in removeAssetInput) (*sdk.CallToolResult, any, error) {
			by, err := callerOf(req)
			if err != nil {
				return nil, nil, err
			}
			return answer(h.games.RemoveAsset(ctx, by, in.GameUUID, in.Key, in.IdempotencyKey))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "get_build", Description: "Reads the build of a revision: QUEUED, RUNNING, SUCCEEDED, FAILED or SUPERSEDED, with diagnostics by file and line. With wait_seconds it answers as soon as the build ends."},
		func(ctx context.Context, req *sdk.CallToolRequest, in getBuildInput) (*sdk.CallToolResult, any, error) {
			user, err := userOf(req)
			if err != nil {
				return nil, nil, err
			}
			var wait time.Duration
			if in.WaitSeconds > 0 {
				var refusal *games.Refusal
				if wait, refusal = games.BuildWait(in.WaitSeconds); refusal != nil {
					return refused(refusal), nil, nil
				}
			}
			return answer(h.games.ReadBuild(ctx, user, in.GameUUID, &in.Revision, wait))
		})
	sdk.AddTool(server, &sdk.Tool{Name: "simulate", Description: "Runs a scenario on the current revision's succeeded build with a virtual clock, and answers the global state and each player's view at the instants asked for, the command answers, refusals, events and errors. Same seed, same result."},
		func(ctx context.Context, req *sdk.CallToolRequest, in simulateInput) (*sdk.CallToolResult, any, error) {
			user, err := userOf(req)
			if err != nil {
				return nil, nil, err
			}
			scenario, refusal := games.DecodeScenario(mustJSON(in.Scenario))
			if refusal != nil {
				return refused(refusal), nil, nil
			}
			return answer(h.games.Simulate(ctx, user, in.GameUUID, scenario))
		})
}

// callerOf is the User a tool call acts for, changing as an AI application
// connected through MCP.
func callerOf(req *sdk.CallToolRequest) (games.Caller, error) {
	user, err := userOf(req)
	if err != nil {
		return games.Caller{}, err
	}
	return games.Caller{UserUUID: user, Origin: sessionpkg.DraftOriginMCP}, nil
}

// answer is a tool's result for an operation's answer: its value as JSON, or
// an error result with the status and body its HTTP route answers with.
func answer[T any](value T, refusal *games.Refusal) (*sdk.CallToolResult, any, error) {
	if refusal != nil {
		return refused(refusal), nil, nil
	}
	var encoded bytes.Buffer
	if err := json.NewEncoder(&encoded).Encode(value); err != nil {
		return nil, nil, fmt.Errorf("encoding the tool's answer: %w", err)
	}
	return text(encoded.String()), nil, nil
}

// refused is the error result of a refusal: {status, answer}, answer being
// the body the HTTP route answers.
func refused(refusal *games.Refusal) *sdk.CallToolResult {
	var body any
	if json.Unmarshal(mustJSON(refusal.Body), &body) != nil {
		body = refusal.Body
	}
	return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: string(mustJSON(map[string]any{"status": refusal.Status, "answer": body}))}}}
}

func text(content string) *sdk.CallToolResult {
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: content}}}
}

// refusal is an error result decided before any operation ran.
func refusal(code string, message string) *sdk.CallToolResult {
	return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: string(mustJSON(map[string]any{"code": code, "message": message}))}}}
}

func mustJSON(value any) []byte {
	encoded, err := json.Marshal(value)
	if err != nil {
		return []byte("null")
	}
	return encoded
}
