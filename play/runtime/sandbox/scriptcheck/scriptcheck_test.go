package scriptcheck

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// entrypointStubs completes a script with the entrypoints it does not test.
const entrypointStubs = `
function project(state, participant) { return { me: participant }; }
`

func TestValidate(t *testing.T) {
	type test struct {
		script string

		// expectedRules are the rules broken, in position order; none means
		// the script is valid.
		expectedRules []string
	}

	tests := map[string]func(t *testing.T) test{
		"pure_game_is_valid": func(t *testing.T) test {
			return test{script: `"use strict";
				const ROUNDS = 3;
				const label = (n) => "round " + n;
				function start(participants, context) {
					const scores = {};
					for (const p of participants) { scores[p] = 0; }
					const order = participants.slice().map((p) => p);
					let round = 1;
					round++;
					return { state: { round, scores, rounds: ROUNDS, title: label(round), order } };
				}
				function project(state, participant) {
					const { scores } = state;
					const others = Object.keys(scores).filter((p) => p !== participant);
					const view = { mine: scores[participant], others: others.length };
					view.round = state.round;
					return view;
				}`}
		},
		"missing_entrypoints": func(t *testing.T) test {
			return test{script: `const x = 1;`, expectedRules: []string{RuleEntrypoint, RuleEntrypoint}}
		},
		"top_level_let_is_module_state": func(t *testing.T) test {
			return test{script: `let turns = 0; function start() { return { state: {} }; }` + entrypointStubs, expectedRules: []string{RuleModuleState}}
		},
		"top_level_var_is_module_state": func(t *testing.T) test {
			return test{script: `var turns = 0; function start() { return { state: {} }; }` + entrypointStubs, expectedRules: []string{RuleModuleState}}
		},
		"top_level_const_object_is_module_state": func(t *testing.T) test {
			return test{script: `const cache = {}; function start() { return { state: {} }; }` + entrypointStubs, expectedRules: []string{RuleModuleState}}
		},
		"top_level_computed_const_is_module_state": func(t *testing.T) test {
			return test{script: `const counter = (() => { let n = 0; return () => n++; })(); function start() { return { state: {} }; }` + entrypointStubs, expectedRules: []string{RuleModuleState}}
		},
		"top_level_statement": func(t *testing.T) test {
			return test{script: `start(); function start() { return { state: {} }; }` + entrypointStubs, expectedRules: []string{RuleModuleState}}
		},
		"reassigning_a_top_level_function": func(t *testing.T) test {
			return test{script: `function start() { project = null; return { state: {} }; }` + entrypointStubs, expectedRules: []string{RuleModuleState}}
		},
		"implicit_global": func(t *testing.T) test {
			return test{script: `function start() { leaked = 1; return { state: {} }; }` + entrypointStubs, expectedRules: []string{RuleGlobalAccess}}
		},
		"changing_a_builtin": func(t *testing.T) test {
			return test{script: `function start() { Math.cache = 1; return { state: {} }; }` + entrypointStubs, expectedRules: []string{RuleGlobalAccess}}
		},
		"global_this": func(t *testing.T) test {
			return test{script: `function start() { return { state: { g: typeof globalThis } }; }` + entrypointStubs, expectedRules: []string{RuleGlobalAccess}}
		},
		"eval": func(t *testing.T) test {
			return test{script: `function start() { return { state: eval("1") }; }` + entrypointStubs, expectedRules: []string{RuleGlobalAccess}}
		},
		"constructor_access": func(t *testing.T) test {
			return test{script: `function start() { const F = (() => 0).constructor; return { state: {} }; }` + entrypointStubs, expectedRules: []string{RuleGlobalAccess}}
		},
		"dynamic_import_does_not_parse": func(t *testing.T) test {
			return test{script: `function start() { import("qjs:os"); return { state: {} }; }` + entrypointStubs, expectedRules: []string{RuleSyntax}}
		},
		"assigning_into_the_state": func(t *testing.T) test {
			return test{script: `function start() { return { state: {} }; }
				function project(state) { state.round = 2; return {}; }`, expectedRules: []string{RuleMutation}}
		},
		"assigning_through_an_alias": func(t *testing.T) test {
			return test{script: `function start() { return { state: {} }; }
				function project(state) { const s = state; const scores = s.scores; scores.p1 = 0; return {}; }`, expectedRules: []string{RuleMutation}}
		},
		"assigning_through_destructuring": func(t *testing.T) test {
			return test{script: `function start() { return { state: {} }; }
				function project({ scores }) { scores.p1 = 0; return {}; }`, expectedRules: []string{RuleMutation}}
		},
		"element_found_in_the_state": func(t *testing.T) test {
			return test{script: `function start() { return { state: {} }; }
				function project(state) { const first = state.items.find((i) => i.id === 1); first.done = true; return {}; }`, expectedRules: []string{RuleMutation}}
		},
		"mutating_method_on_the_input": func(t *testing.T) test {
			return test{script: `function start(participants) { participants.push("P9"); return { state: {} }; }` + entrypointStubs, expectedRules: []string{RuleMutation}}
		},
		"sorting_the_input_in_place": func(t *testing.T) test {
			return test{script: `function start(participants) { participants.sort(); return { state: {} }; }` + entrypointStubs, expectedRules: []string{RuleMutation}}
		},
		"deleting_from_the_state": func(t *testing.T) test {
			return test{script: `function start() { return { state: {} }; }
				function project(state) { delete state.secret; return {}; }`, expectedRules: []string{RuleMutation}}
		},
		"incrementing_into_the_state": func(t *testing.T) test {
			return test{script: `function start() { return { state: {} }; }
				function project(state) { state.views++; return {}; }`, expectedRules: []string{RuleMutation}}
		},
		"object_assign_onto_the_state": func(t *testing.T) test {
			return test{script: `function start() { return { state: {} }; }
				function project(state) { Object.assign(state, { seen: true }); return {}; }`, expectedRules: []string{RuleMutation}}
		},
		"loop_variable_over_the_state": func(t *testing.T) test {
			return test{script: `function start() { return { state: {} }; }
				function project(state) { for (const player of state.players) { player.seen = true; } return {}; }`, expectedRules: []string{RuleMutation}}
		},
		"helper_mutating_its_parameter": func(t *testing.T) test {
			return test{script: `function bump(scores) { scores.total = 1; }
				function start() { return { state: {} }; }` + entrypointStubs, expectedRules: []string{RuleMutation}}
		},
		"local_container_that_received_an_input": func(t *testing.T) test {
			return test{script: `function start(participants) {
					const list = [];
					list.push(participants);
					list[0].push("P9");
					return { state: {} };
				}` + entrypointStubs, expectedRules: []string{RuleMutation}}
		},
		"this": func(t *testing.T) test {
			return test{script: `function start() { return { state: { self: this } }; }` + entrypointStubs, expectedRules: []string{RuleThis}}
		},
		"async_entrypoint": func(t *testing.T) test {
			return test{script: `async function start() { return { state: {} }; }` + entrypointStubs, expectedRules: []string{RuleAsync}}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			violations := Validate([]byte(tc.script))

			rules := []string{}
			for _, violation := range violations {
				rules = append(rules, violation.Rule)
			}
			expected := tc.expectedRules
			if expected == nil {
				expected = []string{}
			}
			require.Equal(t, expected, rules, "%+v", violations)
		})
	}
}

func TestValidateReportsPositions(t *testing.T) {
	violations := Validate([]byte("function start() { return { state: {} }; }\nfunction project(state) {\n  state.x = 1;\n  return {};\n}"))

	require.Equal(t, []Violation{{Line: 3, Column: 3, Rule: RuleMutation, Message: "assignment changes state, which is an input or reaches one; return a new value instead"}}, violations)
}
