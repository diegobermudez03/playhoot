package scriptcheck

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSDKProfile(t *testing.T) {
	t.Run("the built Tap Race script passes sdk-v1 and not the handwritten profile", func(t *testing.T) {
		script, err := os.ReadFile("../testdata/tap_race_sdk_v1.js")
		require.NoError(t, err)
		require.Empty(t, Validate(ProfileSDKV1, script))
		require.NotEmpty(t, Validate(ProfileHandwritten, script))
	})

	const tail = `
function start(p) { return { state: {} }; }
function project(s) { return {}; }
`
	cases := []struct {
		name   string
		script string
		rules  []string
	}{
		{
			name: "inputs may change and constants may be literal objects and definitions",
			script: `const GOAL = 20;
const QUESTIONS = [{ q: "a", options: ["x", "y"] }, ...[]];
const commands = defineCommands({ tap: {} });
const game = defineGame({ commands, setup: (p) => ({}), on: { tap(state, c) { state.n += 1; state.list.push(c); } } });
function defineGame(d) { return d; }
function defineCommands(d) { return d; }
function helper(state) { const copy = state.items.slice(); copy.sort(); state.items = copy; this; }
function __playhoot_module_values() { return [QUESTIONS, commands, game]; }` + tail,
		},
		{
			name: "constants may read properties of other constants",
			script: `const config = { rounds: 3, timing: { seconds: 20 } };
const ROUNDS = config.rounds;
const game = defineGame({ phases: { play: { seconds: config.timing.seconds } }, rounds: ROUNDS });
function defineGame(d) { return d; }
function __playhoot_module_values() { return [config, ROUNDS, game]; }` + tail,
		},
		{
			name: "property reads reach no constructor and literals hold no getters",
			script: `const A = { a: 1 };
const B = A.constructor;
const C = A["a"];
const D = { get x() { return 1; } };
function __playhoot_module_values() { return [A]; }` + tail,
			// Reading constructor anywhere is also a global access.
			rules: []string{RuleModuleState, RuleGlobalAccess, RuleModuleState, RuleModuleState},
		},
		{
			name: "computed constants and calls other than definitions are refused",
			script: `const made = make();
const size = GOAL * 2;
function make() { return {}; }
function __playhoot_module_values() { return [made]; }` + tail,
			// The refused constant is not one the list may hold either.
			rules: []string{RuleModuleState, RuleModuleState, RuleModuleState},
		},
		{
			name: "the list of values must be exactly the non-primitive constants",
			script: `const A = [1];
const B = { b: 1 };
const C = 3;
function __playhoot_module_values() { return [A]; }` + tail,
			rules: []string{RuleModuleState},
		},
		{
			name:   "the list of values is required",
			script: `const A = [1];` + tail,
			rules:  []string{RuleEntrypoint},
		},
		{
			name: "changing a top-level constant directly is still refused",
			script: `const A = [1];
function add() { A.push(2); }
function __playhoot_module_values() { return [A]; }` + tail,
			rules: []string{RuleModuleState},
		},
		{
			name: "module state and escapes are still refused",
			script: `let n = 0;
function f(o) { eval("1"); return o.constructor; }
async function g() {}
function __playhoot_module_values() { return []; }` + tail,
			rules: []string{RuleModuleState, RuleGlobalAccess, RuleGlobalAccess, RuleAsync},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			violations := Validate(ProfileSDKV1, []byte(tc.script))
			rules := make([]string, len(violations))
			messages := make([]string, len(violations))
			for i, violation := range violations {
				rules[i] = violation.Rule
				messages[i] = violation.Message
			}
			if len(tc.rules) == 0 {
				require.Empty(t, violations, strings.Join(messages, "\n"))
				return
			}
			require.ElementsMatch(t, tc.rules, rules, strings.Join(messages, "\n"))
		})
	}

	t.Run("an unknown profile is refused", func(t *testing.T) {
		require.Equal(t, RuleUnsupported, Validate("sdk-v9", []byte(tail))[0].Rule)
	})
}
