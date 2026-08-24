package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
)

func TestRequestLogSlogArgsBuildsOrderedStructuredScopes(t *testing.T) {
	ctx := Start(context.Background())

	LogFields(ctx, logField{Key: "game_uuid", Value: "game-1"})
	step := Step(ctx, "decode")
	LogFields(ctx,
		logField{Key: "status", Value: "started"},
		logField{Key: "status", Value: "finished"},
	)
	LogError(ctx, errors.New("decode warning"))
	step.Close()
	LogFields(ctx, logField{Key: "game_uuid", Value: "game-2"})

	args := requestLogFromContextForTest(t, ctx).SlogArgs()

	assertArgKeys(t, args, []string{
		"started_at",
		"ended_at",
		"duration_ms",
		"game_uuid",
		"decode",
		"game_uuid_2",
		"error",
		"logging_errors",
	})
	assertSlogArg(t, args, "game_uuid", "game-1")
	assertSlogArg(t, args, "game_uuid_2", "game-2")
	assertSlogArg(t, args, "error", "decode warning")

	logged := renderJSONLog(t, args)
	decode := logged["decode"].(map[string]any)
	if decode["status"] != "started" {
		t.Fatalf("decode.status = %#v, want started", decode["status"])
	}
	if decode["status_2"] != "finished" {
		t.Fatalf("decode.status_2 = %#v, want finished", decode["status_2"])
	}
	assertJSONList(t, decode["errors"], []string{"decode warning"})
}

func TestRequestLogSlogArgsBuildsNestedStepsAndLoopLists(t *testing.T) {
	ctx := Start(context.Background())

	load := Step(ctx, "load")
	LogFields(ctx, logField{Key: "repo", Value: "games"})
	decode := Step(ctx, "decode")
	LogFields(ctx, logField{Key: "format", Value: "json"})
	decode.Close()
	load.Close()

	LogLoopFields(ctx, "versions", logField{Key: "uuid", Value: "v1"}, logField{Key: "index", Value: 1})
	LogLoopFields(ctx, "versions", logField{Key: "uuid", Value: "v2"}, logField{Key: "index", Value: 2})

	args := requestLogFromContextForTest(t, ctx).SlogArgs()
	assertArgKeys(t, args, []string{
		"started_at",
		"ended_at",
		"duration_ms",
		"load",
		"versions",
		"logging_errors",
	})

	logged := renderJSONLog(t, args)
	loadLog := logged["load"].(map[string]any)
	if loadLog["repo"] != "games" {
		t.Fatalf("load.repo = %#v, want games", loadLog["repo"])
	}
	decodeLog := loadLog["decode"].(map[string]any)
	if decodeLog["format"] != "json" {
		t.Fatalf("load.decode.format = %#v, want json", decodeLog["format"])
	}

	versions := logged["versions"].([]any)
	if len(versions) != 2 {
		t.Fatalf("versions length = %d, want 2", len(versions))
	}
	first := versions[0].(map[string]any)
	if first["uuid"] != "v1" || first["index"] != float64(1) {
		t.Fatalf("first version = %#v, want uuid v1 and index 1", first)
	}
	second := versions[1].(map[string]any)
	if second["uuid"] != "v2" || second["index"] != float64(2) {
		t.Fatalf("second version = %#v, want uuid v2 and index 2", second)
	}
}

func TestRequestLogSlogArgsReportsUnclosedSteps(t *testing.T) {
	ctx := Start(context.Background())

	Step(ctx, "decode")
	Step(ctx, "validate")

	args := requestLogFromContextForTest(t, ctx).SlogArgs()
	logged := renderJSONLog(t, args)

	assertJSONList(t, logged["logging_errors"], []string{
		"step validate wasnt closed",
		"step decode wasnt closed",
	})
}

func TestRequestLogHelpersNoopWhenContextHasNoRequestLog(t *testing.T) {
	ctx := context.Background()

	LogFields(ctx, logField{Key: "ignored", Value: true})
	LogError(ctx, errors.New("ignored"))
	LogLoopFields(ctx, "ignored", logField{Key: "ignored", Value: true})
	FinishRequestLog(ctx, slog.Default(), "ignored")

	if _, ok := requestLogFromContext(ctx); ok {
		t.Fatal("unexpected request log in context")
	}
}

func TestFinishRequestLogWritesSingleStructuredLog(t *testing.T) {
	ctx := Start(context.Background())
	LogFields(ctx, logField{Key: "game_uuid", Value: "game-1"})

	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, &slog.HandlerOptions{}))

	FinishRequestLog(ctx, logger, "request completed")

	logged := output.String()
	for _, want := range []string{
		"msg=\"request completed\"",
		"game_uuid=game-1",
		"duration_ms=",
		"started_at=",
		"ended_at=",
	} {
		if !strings.Contains(logged, want) {
			t.Fatalf("log output %q does not contain %q", logged, want)
		}
	}
}

func TestRequestLogSupportsConcurrentWrites(t *testing.T) {
	ctx := Start(context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			LogFields(ctx, logField{Key: "request_id", Value: "request-1"})
			LogError(ctx, errors.New("temporary failure"))
		}()
	}
	wg.Wait()

	logged := renderJSONLog(t, requestLogFromContextForTest(t, ctx).SlogArgs())
	if logged["error"] != "temporary failure" {
		t.Fatalf("error = %#v, want temporary failure", logged["error"])
	}
	errorsLog := logged["errors"].([]any)
	if len(errorsLog) != 50 {
		t.Fatalf("errors length = %d, want 50", len(errorsLog))
	}
}

func requestLogFromContextForTest(t *testing.T, ctx context.Context) *RequestLog {
	t.Helper()

	requestLog, ok := requestLogFromContext(ctx)
	if !ok {
		t.Fatal("request log was not found in context")
	}
	return requestLog
}

func renderJSONLog(t *testing.T, args []any) map[string]any {
	t.Helper()

	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{}))
	logger.Info("request completed", args...)

	var logged map[string]any
	if err := json.Unmarshal(output.Bytes(), &logged); err != nil {
		t.Fatalf("decode JSON log %q: %v", output.String(), err)
	}
	return logged
}

func assertArgKeys(t *testing.T, args []any, want []string) {
	t.Helper()

	got := make([]string, 0, len(args)/2)
	for i := 0; i < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			t.Fatalf("arg key at index %d = %#v, want string", i, args[i])
		}
		got = append(got, key)
	}

	if len(got) != len(want) {
		t.Fatalf("arg keys = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("arg keys = %#v, want %#v", got, want)
		}
	}
}

func assertSlogArg(t *testing.T, args []any, key string, want any) {
	t.Helper()

	for i := 0; i < len(args)-1; i += 2 {
		if args[i] == key {
			got := slogValueAny(args[i+1])
			if got != want {
				t.Fatalf("%s = %#v, want %#v", key, got, want)
			}
			return
		}
	}

	t.Fatalf("missing slog arg %q in %#v", key, args)
}

func slogValueAny(value any) any {
	slogValue, ok := value.(slog.Value)
	if !ok {
		return value
	}
	return slogValue.Any()
}

func assertJSONList(t *testing.T, got any, want []string) {
	t.Helper()

	items, ok := got.([]any)
	if !ok {
		t.Fatalf("value = %#v, want JSON list", got)
	}
	if len(items) != len(want) {
		t.Fatalf("list = %#v, want %#v", items, want)
	}
	for i := range want {
		if items[i] != want[i] {
			t.Fatalf("list = %#v, want %#v", items, want)
		}
	}
}
