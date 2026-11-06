package scenariodoc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecode(t *testing.T) {
	t.Run("reads_steps_holds_observations_and_expectations", func(t *testing.T) {
		scenario, diagnostics := Decode([]byte(`{
			"format": "playhoot.scenario.v1", "players": 2, "duration_ms": 3000, "revision": 4,
			"steps": [{"at_ms": 100, "player": "P1", "command": {"name": "tap"}}],
			"holds": [{"from_ms": 1000, "to_ms": 1800, "player": "P2", "inputs": {"right": true}}],
			"observe_at_ms": [500],
			"expect": [{"at_ms": 500, "of": "P1", "path": "taps.P1", "op": "eq", "value": 1}, {"path": "winner", "op": "exists"}]
		}`))

		require.Empty(t, diagnostics)
		at := int64(500)
		require.Equal(t, Scenario{
			Players: 2, Seed: DefaultSeed, DurationMs: 3000,
			Steps:       []Step{{AtMs: 100, Player: "P1", Command: &Command{Name: "tap"}}},
			Holds:       []Hold{{FromMs: 1000, ToMs: 1800, Player: "P2", Inputs: json.RawMessage(`{"right": true}`)}},
			ObserveAtMs: []int64{500},
			Expect: []Expectation{
				{AtMs: &at, Of: "P1", Path: "taps.P1", Op: "eq", Value: json.RawMessage(`1`)},
				{Of: "global", Path: "winner", Op: "exists"},
			},
		}, scenario)
	})

	t.Run("names_every_shape_problem", func(t *testing.T) {
		_, diagnostics := Decode([]byte(`{
			"format": "x", "players": 1,
			"steps": [{"at_ms": 0, "player": "P1"}, {"at_ms": 0, "player": "P1", "command": {"name": ""}}],
			"holds": [{"from_ms": 5, "to_ms": 5, "player": "P1"}],
			"expect": [
				{"of": "everyone", "path": "a", "op": "eq", "value": 1},
				{"path": "a..b", "op": "exists"},
				{"path": "a", "op": "gt", "value": "big"},
				{"path": "a", "op": "exists", "value": 1},
				{"path": "a", "op": "matches", "value": 1},
				{"path": "a", "op": "len", "value": 1.5},
				{"path": "a", "op": "in", "value": 3}
			]
		}`))

		fields := make([]string, len(diagnostics))
		for i, diagnostic := range diagnostics {
			fields[i] = diagnostic.Field
		}
		require.Equal(t, []string{"format", "duration_ms", "steps[0]", "steps[1].command.name", "holds[0]", "holds[0].inputs",
			"expect[0].of", "expect[1].path", "expect[2]", "expect[3]", "expect[4]", "expect[5]", "expect[6]"}, fields)
	})

	t.Run("not_a_json_object", func(t *testing.T) {
		_, diagnostics := Decode([]byte(`[1]`))
		require.Len(t, diagnostics, 1)
	})
}

func TestParsePathAndResolve(t *testing.T) {
	document := json.RawMessage(`{"players": {"P1": {"hand": [{"rank": 3}, {"rank": 9}]}}, "list": [[1, 2]], "empty": null}`)
	for path, expected := range map[string]string{
		"":                       string(document),
		"players.P1.hand[1].rank": "9",
		"list[0][1]":             "2",
		"empty":                  "null",
	} {
		segments, err := ParsePath(path)
		require.NoError(t, err, path)
		value, found := Resolve(document, segments)
		require.True(t, found, path)
		require.JSONEq(t, expected, string(value), path)
	}
	for _, path := range []string{"players.P2", "list[3]", "players.P1.hand.rank", "list.x"} {
		segments, err := ParsePath(path)
		require.NoError(t, err, path)
		_, found := Resolve(document, segments)
		require.False(t, found, path)
	}
	for _, path := range []string{"a.", ".a", "a[x]", "a[1", "a[-1]", "a[1]b"} {
		_, err := ParsePath(path)
		require.Error(t, err, path)
	}
}

func TestCheck(t *testing.T) {
	document := json.RawMessage(`{"score": 3, "name": "Ana", "hand": [1, 2], "winner": null}`)
	expect := func(path, op, value string) Expectation {
		return Expectation{Of: OfGlobal, Path: path, Op: op, Value: json.RawMessage(value)}
	}
	for _, passing := range []Expectation{
		expect("score", OpEq, `3`), expect("hand", OpEq, `[1, 2]`), expect("score", OpNe, `4`),
		expect("winner", OpExists, ``), expect("loser", OpAbsent, ``), expect("score", OpLt, `4`),
		expect("score", OpLte, `3`), expect("score", OpGt, `2`), expect("score", OpGte, `3`),
		expect("name", OpIn, `["Bo", "Ana"]`), expect("hand", OpLen, `2`), expect("name", OpLen, `3`),
	} {
		verdict := Check(passing, document)
		require.True(t, verdict.Passed, "%s %s %s", passing.Path, passing.Op, passing.Value)
	}

	failed := Check(expect("score", OpEq, `4`), document)
	require.False(t, failed.Passed)
	require.JSONEq(t, `3`, string(failed.Actual))
	require.Empty(t, failed.Problem)

	missing := Check(expect("loser", OpEq, `1`), document)
	require.False(t, missing.Passed)
	require.Equal(t, "nothing at that path", missing.Problem)

	notNumber := Check(expect("name", OpGt, `1`), document)
	require.False(t, notNumber.Passed)
	require.NotEmpty(t, notNumber.Problem)
}
