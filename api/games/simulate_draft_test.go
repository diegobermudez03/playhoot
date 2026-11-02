package games

import (
	"encoding/json"
	"net/http"
	"testing"

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
	}
	post := func(body string) test {
		return test{method: http.MethodPost, target: "/", gameUUID: gameUUID, body: body, noStore: true}
	}
	tests := map[string]func(t *testing.T, deps dependencies) test{
		"simulated": func(t *testing.T, deps dependencies) test {
			deps.reader.EXPECT().SimulateDraft(gomock.Any(), callerUUID, gameUUID, int64(0), scenario).
				Return(composergameauthoring.DraftSimulation{Access: game.EditableVersionFound, Revision: 3, BuildStatus: sessionpkg.DraftBuildSucceeded, Ready: true, Result: ran}, nil)
			tc := post(body)
			tc.expectedCode = http.StatusOK
			tc.expectedBody = `{"format":"playhoot.simulation-result.v1","revision":3,"players":["P1","P2"],"seed":1,"finished":true,"finished_at_ms":150,
				"stopped":"finished","simulated_ms":150,"boundaries":2,
				"observations":[{"requested_at_ms":100,"at_ms":100,"state":{"n":0},"views":{"P1":{"me":"P1"}}}],
				"final":{"requested_at_ms":150,"at_ms":150,"state":{"n":1},"views":{"P1":{"me":"P1"}}},
				"commands":[{"at_ms":150,"player":"P1","name":"tap","response":{"ok":true}}],
				"rejected":[{"at_ms":120,"player":"P2","kind":"inputs","reason":"invalid","detail":"x is undeclared"}],
				"events":[],"errors":[]}`
			return tc
		},
		"another_format": func(t *testing.T, deps dependencies) test {
			tc := post(`{"format":"playhoot.scenario.v9"}`)
			tc.expectedCode = http.StatusUnprocessableEntity
			tc.expectedBody = `{"code":"invalid_scenario","message":"the scenario was not simulated; see diagnostics","diagnostics":[{"field":"format","message":"must be playhoot.scenario.v1"}]}`
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
