package games

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"go.uber.org/mock/gomock"

	composergameauthoring "github.com/diegobermudez03/playhoot/composer/gameauthoring"
	"github.com/diegobermudez03/playhoot/game"
	"github.com/diegobermudez03/playhoot/play/runtime"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
)

func TestHandleSimulateDraft(t *testing.T) {
	const body = `{"format":"playhoot.scenario.v1","players":2,"duration_ms":300,
		"steps":[{"at_ms":150,"player":"P1","command":{"name":"tap"}},{"at_ms":120,"player":"P2","inputs":{"x":1}}],"observe_at_ms":[100]}`
	scenario := runtime.Scenario{Players: 2, Seed: 1, DurationMs: 300, ObserveAtMs: []int64{100}, Steps: []runtime.ScenarioStep{
		{AtMs: 150, Player: "P1", Command: &runtime.ScenarioCommand{Name: "tap"}},
		{AtMs: 120, Player: "P2", Inputs: json.RawMessage(`{"x":1}`)},
	}}
	finishedAt := int64(150)
	ran := runtime.SimulationResult{
		Outcome: runtime.SimulationRan, Players: []string{"P1", "P2"}, Seed: 1, Finished: true, FinishedAtMs: &finishedAt,
		Stopped: runtime.SimulationStoppedFinished, SimulatedMs: 150, Boundaries: 2,
		Observations: []runtime.SimulationObservation{{RequestedAtMs: 100, AtMs: 100, State: json.RawMessage(`{"n":0}`), Views: map[string]json.RawMessage{"P1": json.RawMessage(`{"me":"P1"}`)}}},
		Final:        runtime.SimulationObservation{RequestedAtMs: 150, AtMs: 150, State: json.RawMessage(`{"n":1}`), Views: map[string]json.RawMessage{"P1": json.RawMessage(`{"me":"P1"}`)}},
		Commands:     []runtime.SimulatedCommand{{AtMs: 150, Player: "P1", Name: "tap", Response: json.RawMessage(`{"ok":true}`)}},
		Rejected:     []runtime.SimulationRejection{{AtMs: 120, Player: "P2", Kind: "inputs", Reason: "invalid", Detail: "x is undeclared"}},
		ID:           "r-1",
		ExpiresAt:    time.Date(2026, 11, 6, 12, 30, 0, 0, time.UTC),
	}
	no, yes := false, true
	end := int64(150)
	expected := ran
	expected.Passed = &no
	expected.Expectations = []runtime.ExpectationResult{
		{Index: 0, Expectation: runtime.ScenarioExpectation{AtMs: &end, Of: "global", Path: "n", Op: "eq", Value: json.RawMessage(`2`)}, ObservedAtMs: 150, Actual: json.RawMessage(`1`)},
		{Index: 1, Expectation: runtime.ScenarioExpectation{Of: "P1", Path: "me", Op: "exists"}, Passed: true, ObservedAtMs: 150, Final: true, Actual: json.RawMessage(`"P1"`)},
	}
	saved := func(path string, result runtime.SimulationResult) composergameauthoring.SavedScenarioRun {
		return composergameauthoring.SavedScenarioRun{Path: path, Result: result}
	}
	passing := ran
	passing.Passed = &yes
	post := func(body string) test {
		return test{method: http.MethodPost, target: "/", gameUUID: gameUUID, body: body, noStore: true}
	}
	tests := map[string]func(t *testing.T, deps dependencies) test{
		"simulated_answers_a_summary": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().SimulateDraft(gomock.Any(), callerUUID, gameUUID, int64(0), scenario).
				Return(composergameauthoring.DraftSimulation{Access: game.EditableVersionFound, Revision: 3, BuildStatus: sessionpkg.DraftBuildSucceeded, Ready: true, ToolchainVersion: "tc-1", Result: expected}, nil)
			tc := post(body)
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"format":"playhoot.simulation-summary.v1","result_id":"r-1","expires_at":"2026-11-06T12:30:00Z","revision":3,"toolchain_version":"tc-1",
				"players":["P1","P2"],"seed":1,"passed":false,
				"expectations":{"total":2,"passed":1,"failed":[{"index":0,"at_ms":150,"of":"global","path":"n","op":"eq","expected":2,"passed":false,"observed_at_ms":150,"actual":1}]},
				"finished":true,"finished_at_ms":150,"stopped":"finished","simulated_ms":150,"boundaries":2,"observed_at_ms":[100],
				"counts":{"commands":1,"rejected":1,"events":0,"errors":0},
				"rejected":[{"at_ms":120,"player":"P2","kind":"inputs","reason":"invalid","detail":"x is undeclared"}],"errors":[]}`
			return tc
		},
		"full_detail_answers_everything": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().SimulateDraft(gomock.Any(), callerUUID, gameUUID, int64(0), scenario).
				Return(composergameauthoring.DraftSimulation{Access: game.EditableVersionFound, Revision: 3, BuildStatus: sessionpkg.DraftBuildSucceeded, Ready: true, Result: ran}, nil)
			tc := post(body[:len(body)-1] + `,"detail":"full"}`)
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"format":"playhoot.simulation-result.v1","result_id":"r-1","expires_at":"2026-11-06T12:30:00Z","revision":3,"players":["P1","P2"],"seed":1,"finished":true,"finished_at_ms":150,
				"stopped":"finished","simulated_ms":150,"boundaries":2,
				"observations":[{"requested_at_ms":100,"at_ms":100,"state":{"n":0},"views":{"P1":{"me":"P1"}}}],
				"final":{"requested_at_ms":150,"at_ms":150,"state":{"n":1},"views":{"P1":{"me":"P1"}}},
				"commands":[{"at_ms":150,"player":"P1","name":"tap","response":{"ok":true}}],
				"rejected":[{"at_ms":120,"player":"P2","kind":"inputs","reason":"invalid","detail":"x is undeclared"}],
				"events":[],"errors":[]}`
			return tc
		},
		"another_format": func(t *testing.T, deps dependencies) test {
			tc := post(`{"format":"playhoot.scenario.v9","players":1,"duration_ms":10}`)
			tc.expectedCode = http.StatusUnprocessableEntity
			tc.expectedBody = `{"code":"invalid_scenario","message":"the scenario was not simulated; see diagnostics","diagnostics":[{"field":"format","message":"must be playhoot.scenario.v1"}]}`
			return tc
		},
		"a_saved_scenario_by_path": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().SimulateSavedScenarios(gomock.Any(), callerUUID, gameUUID, int64(2), "scenarios/win.json").
				Return(composergameauthoring.DraftSavedSimulations{Access: game.EditableVersionFound, Revision: 2, BuildStatus: sessionpkg.DraftBuildSucceeded, Ready: true, Runs: []composergameauthoring.SavedScenarioRun{saved("scenarios/win.json", passing)}}, nil)
			tc := post(`{"revision":2,"scenario_path":"scenarios/win.json"}`)
			tc.expectedCode = http.StatusOK
			return tc
		},
		"a_saved_scenario_that_does_not_exist": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().SimulateSavedScenarios(gomock.Any(), callerUUID, gameUUID, int64(0), "scenarios/none.json").
				Return(composergameauthoring.DraftSavedSimulations{Access: game.EditableVersionFound, Ready: true, NotFound: true}, nil)
			tc := post(`{"scenario_path":"scenarios/none.json"}`)
			tc.expectedCode = http.StatusNotFound
			tc.expectedBody = `{"code":"scenario_not_found","message":"the revision has no saved scenario at scenario_path (scenarios/<name>.json)"}`
			return tc
		},
		"every_saved_scenario": func(t *testing.T, deps dependencies) test {
			invalid := runtime.SimulationResult{Outcome: runtime.SimulationInvalid, Diagnostics: []runtime.SimulationDiagnostic{{Field: "players", Message: "1 to 8"}}}
			deps.reader.EXPECT().SimulateSavedScenarios(gomock.Any(), callerUUID, gameUUID, int64(0), "").
				Return(composergameauthoring.DraftSavedSimulations{Access: game.EditableVersionFound, Revision: 3, BuildStatus: sessionpkg.DraftBuildSucceeded, Ready: true, ToolchainVersion: "tc-1",
					Runs: []composergameauthoring.SavedScenarioRun{
						saved("scenarios/a.json", passing),
						{Path: "scenarios/b.json", Diagnostics: []runtime.SimulationDiagnostic{{Field: "format", Message: "must be playhoot.scenario.v1"}}},
						saved("scenarios/c.json", invalid),
					},
					Skipped: []string{"scenarios/z.json"},
				}, nil)
			tc := post(`{"all":true}`)
			tc.expectedCode = http.StatusOK
			tc.check = func(t *testing.T, body string) {
				var batch SimulationBatch
				require.NoError(t, json.Unmarshal([]byte(body), &batch))
				require.Equal(t, "playhoot.simulation-batch.v1", batch.Format)
				require.Equal(t, "tc-1", batch.ToolchainVersion)
				require.False(t, *batch.Passed, "an invalid saved scenario fails the batch")
				require.Len(t, batch.Scenarios, 3)
				require.True(t, *batch.Scenarios[0].Summary.Passed)
				require.Equal(t, "format", batch.Scenarios[1].Diagnostics[0].Field)
				require.Equal(t, "players", batch.Scenarios[2].Diagnostics[0].Field)
				require.Equal(t, []string{"scenarios/z.json"}, batch.Skipped)
			}
			return tc
		},
		"path_and_all_together": func(t *testing.T, deps dependencies) test {
			tc := post(`{"all":true,"scenario_path":"scenarios/a.json"}`)
			tc.expectedCode = http.StatusBadRequest
			return tc
		},
		"an_unknown_detail": func(t *testing.T, deps dependencies) test {
			tc := post(`{"format":"playhoot.scenario.v1","players":1,"duration_ms":100,"detail":"all"}`)
			tc.expectedCode = http.StatusBadRequest
			return tc
		},
		"invalid_body": func(t *testing.T, deps dependencies) test {
			tc := post(`{`)
			tc.expectedCode = http.StatusBadRequest
			return tc
		},
		"an_invalid_scenario": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().SimulateDraft(gomock.Any(), callerUUID, gameUUID, int64(0), gomock.Any()).
				Return(composergameauthoring.DraftSimulation{Access: game.EditableVersionFound, Ready: true, Result: runtime.SimulationResult{
					Outcome: runtime.SimulationInvalid, Diagnostics: []runtime.SimulationDiagnostic{{Field: "players", Message: "this game plays with 1 to 8 players"}},
				}}, nil)
			tc := post(`{"format":"playhoot.scenario.v1","players":9,"duration_ms":100}`)
			tc.expectedCode = http.StatusUnprocessableEntity
			tc.expectedBody = `{"code":"invalid_scenario","message":"the scenario was not simulated; see diagnostics","diagnostics":[{"field":"players","message":"this game plays with 1 to 8 players"}]}`
			return tc
		},
		"a_build_that_did_not_succeed": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().SimulateDraft(gomock.Any(), callerUUID, gameUUID, int64(4), gomock.Any()).
				Return(composergameauthoring.DraftSimulation{Access: game.EditableVersionFound, Revision: 4, BuildStatus: sessionpkg.DraftBuildFailed}, nil)
			tc := post(`{"format":"playhoot.scenario.v1","revision":4,"players":1,"duration_ms":100}`)
			tc.expectedCode = http.StatusConflict
			tc.expectedBody = `{"code":"build_not_ready","message":"only a revision whose build SUCCEEDED can be simulated","revision":4,"build_status":"FAILED"}`
			return tc
		},
		"a_revision_without_build": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().SimulateDraft(gomock.Any(), callerUUID, gameUUID, int64(9), gomock.Any()).
				Return(composergameauthoring.DraftSimulation{}, sessionpkg.ErrDraftBuildNotFound)
			tc := post(`{"format":"playhoot.scenario.v1","revision":9,"players":1,"duration_ms":100}`)
			tc.expectedCode = http.StatusConflict
			tc.expectedBody = `{"code":"build_not_ready","message":"that revision has no build to simulate"}`
			return tc
		},
		"busy": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().SimulateDraft(gomock.Any(), callerUUID, gameUUID, int64(0), gomock.Any()).
				Return(composergameauthoring.DraftSimulation{}, runtime.ErrSimulationsBusy)
			tc := post(`{"format":"playhoot.scenario.v1","players":1,"duration_ms":100}`)
			tc.expectedCode = http.StatusServiceUnavailable
			tc.expectedBody = `{"code":"simulations_busy","message":"too many simulations in progress; try again in a few seconds"}`
			return tc
		},
		"a_version_missing_in_play_is_created_and_simulated": func(t *testing.T, deps dependencies) test {
			gomock.InOrder(
				deps.reader.EXPECT().SimulateDraft(gomock.Any(), callerUUID, gameUUID, int64(0), gomock.Any()).
					Return(composergameauthoring.DraftSimulation{Access: game.EditableVersionFound, Missing: true}, nil),
				deps.editor.EXPECT().EnsureDraft(gomock.Any(), callerUUID, gameUUID).Return(game.EditableVersionFound, nil),
				deps.reader.EXPECT().SimulateDraft(gomock.Any(), callerUUID, gameUUID, int64(0), gomock.Any()).
					Return(composergameauthoring.DraftSimulation{Access: game.EditableVersionFound, Revision: 1, BuildStatus: sessionpkg.DraftBuildQueued}, nil),
			)
			tc := post(`{"format":"playhoot.scenario.v1","players":1,"duration_ms":100}`)
			tc.expectedCode = http.StatusConflict
			return tc
		},
		"not_owner": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().SimulateDraft(gomock.Any(), callerUUID, gameUUID, int64(0), gomock.Any()).
				Return(composergameauthoring.DraftSimulation{Access: game.EditableVersionNotFound}, nil)
			tc := post(`{"format":"playhoot.scenario.v1","players":1,"duration_ms":100}`)
			tc.expectedCode = http.StatusNotFound
			return tc
		},
	}
	run(t, tests, func(h *Handler) http.HandlerFunc { return h.handleSimulateDraft })
}

func TestHandleReadSimulationState(t *testing.T) {
	get := func(target string) test {
		return test{method: http.MethodGet, target: target, gameUUID: gameUUID, noStore: true, pathValues: map[string]string{"resultID": "r-1"}}
	}
	at := int64(300)
	five := 5
	tests := map[string]func(t *testing.T, deps dependencies) test{
		"one_value_at_an_instant": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadSimulationState(gomock.Any(), callerUUID, gameUUID, "r-1", &at, "P1", "hand[0]").
				Return(composergameauthoring.DraftSimulationState{Access: game.EditableVersionFound, Read: runtime.SimulationStateRead{
					Outcome: runtime.StateReadFound, Of: "P1", Path: "hand[0]", RequestedAtMs: 300, AtMs: 300, Value: json.RawMessage(`{"rank":3}`),
				}}, nil)
			tc := get("/?at_ms=300&of=P1&path=hand[0]")
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"result_id":"r-1","of":"P1","path":"hand[0]","requested_at_ms":300,"at_ms":300,"value":{"rank":3}}`
			return tc
		},
		"the_end_of_the_global_state_too_large": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadSimulationState(gomock.Any(), callerUUID, gameUUID, "r-1", nil, "global", "").
				Return(composergameauthoring.DraftSimulationState{Access: game.EditableVersionFound, Read: runtime.SimulationStateRead{
					Outcome: runtime.StateReadFound, Of: "global", AtMs: 900, Final: true, TooLarge: true, Size: 70000, Length: &five,
				}}, nil)
			tc := get("/")
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"result_id":"r-1","of":"global","path":"","requested_at_ms":null,"at_ms":900,"final":true,"too_large":true,"size":70000,"length":5}`
			return tc
		},
		"an_instant_not_observed": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadSimulationState(gomock.Any(), callerUUID, gameUUID, "r-1", &at, "global", "").
				Return(composergameauthoring.DraftSimulationState{Access: game.EditableVersionFound, Read: runtime.SimulationStateRead{Outcome: runtime.StateReadNotObserved, Observed: []int64{100}}}, nil)
			tc := get("/?at_ms=300")
			tc.expectedCode = http.StatusNotFound
			tc.expectedBody = `{"code":"not_observed","message":"the simulation did not observe that instant; read one of observed_at_ms, or leave at_ms out for the end","observed_at_ms":[100]}`
			return tc
		},
		"a_path_with_nothing": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadSimulationState(gomock.Any(), callerUUID, gameUUID, "r-1", nil, "global", "x").
				Return(composergameauthoring.DraftSimulationState{Access: game.EditableVersionFound, Read: runtime.SimulationStateRead{Outcome: runtime.StateReadPathNotFound}}, nil)
			tc := get("/?path=x")
			tc.expectedCode = http.StatusNotFound
			return tc
		},
		"an_expired_result": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().ReadSimulationState(gomock.Any(), callerUUID, gameUUID, "r-1", nil, "global", "").
				Return(composergameauthoring.DraftSimulationState{}, runtime.ErrSimulationResultNotFound)
			tc := get("/")
			tc.expectedCode = http.StatusNotFound
			tc.expectedBody = `{"code":"simulation_not_found","message":"no kept simulation result has that id: it expired or is unknown; simulate again"}`
			return tc
		},
		"a_bad_instant": func(t *testing.T, deps dependencies) test {
			tc := get("/?at_ms=soon")
			tc.expectedCode = http.StatusBadRequest
			return tc
		},
	}
	run(t, tests, func(h *Handler) http.HandlerFunc { return h.handleReadSimulationState })
}
