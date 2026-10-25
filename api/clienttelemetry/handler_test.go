package clienttelemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRecord(t *testing.T) {
	type test struct {
		event string

		expectedRecorded bool
		expectedKnown    bool
	}

	tests := map[string]func(t *testing.T) test{
		"artifact_load": func(t *testing.T) test {
			return test{event: `{"name":"artifact_load","duration_ms":420,"cached":false}`, expectedRecorded: true, expectedKnown: true}
		},
		"artifact_load_without_cache_flag": func(t *testing.T) test {
			return test{event: `{"name":"artifact_load","duration_ms":420}`, expectedKnown: true}
		},
		"game_ready": func(t *testing.T) test {
			return test{event: `{"name":"game_ready","duration_ms":80}`, expectedRecorded: true, expectedKnown: true}
		},
		"duration_out_of_range": func(t *testing.T) test {
			return test{event: `{"name":"game_ready","duration_ms":600001}`, expectedKnown: true}
		},
		"negative_duration": func(t *testing.T) test {
			return test{event: `{"name":"game_ready","duration_ms":-1}`, expectedKnown: true}
		},
		"connection_lost": func(t *testing.T) test {
			return test{event: `{"name":"connection_lost","close":"retryable","count":2}`, expectedRecorded: true, expectedKnown: true}
		},
		"connection_lost_of_an_unknown_close": func(t *testing.T) test {
			return test{event: `{"name":"connection_lost","close":"exploded"}`, expectedKnown: true}
		},
		"reconnect": func(t *testing.T) test {
			return test{event: `{"name":"reconnect","outcome":"gave_up"}`, expectedRecorded: true, expectedKnown: true}
		},
		"resync": func(t *testing.T) test {
			return test{event: `{"name":"resync","count":3}`, expectedRecorded: true, expectedKnown: true}
		},
		"count_out_of_range": func(t *testing.T) test {
			return test{event: `{"name":"resync","count":10001}`, expectedKnown: true}
		},
		"bridge_refusal": func(t *testing.T) test {
			return test{event: `{"name":"bridge_refusal","reason":"over_limit","count":900}`, expectedRecorded: true, expectedKnown: true}
		},
		"error_of_its_origin": func(t *testing.T) test {
			return test{event: `{"name":"error","origin":"artifact","kind":"ready_timeout"}`, expectedRecorded: true, expectedKnown: true}
		},
		"invalid_join_code_rejection": func(t *testing.T) test {
			return test{event: `{"name":"error","origin":"backend_rejection","kind":"invalid_join_code"}`, expectedRecorded: true, expectedKnown: true}
		},
		"expired_join_code_rejection": func(t *testing.T) test {
			return test{event: `{"name":"error","origin":"backend_rejection","kind":"join_code_expired"}`, expectedRecorded: true, expectedKnown: true}
		},
		"unsupported_device_rejection": func(t *testing.T) test {
			return test{event: `{"name":"error","origin":"backend_rejection","kind":"device_not_supported"}`, expectedRecorded: true, expectedKnown: true}
		},
		"unsupported_device_of_another_origin": func(t *testing.T) test {
			return test{event: `{"name":"error","origin":"bridge","kind":"device_not_supported"}`, expectedKnown: true}
		},
		"join_code_rejection_of_another_origin": func(t *testing.T) test {
			return test{event: `{"name":"error","origin":"transport","kind":"join_code_expired"}`, expectedKnown: true}
		},
		"error_kind_of_another_origin": func(t *testing.T) test {
			return test{event: `{"name":"error","origin":"presentation","kind":"ready_timeout"}`, expectedKnown: true}
		},
		"error_of_a_free_form_kind": func(t *testing.T) test {
			return test{event: `{"name":"error","origin":"transport","kind":"TypeError: Failed to fetch"}`, expectedKnown: true}
		},
		"unknown_event": func(t *testing.T) test {
			return test{event: `{"name":"mined_bitcoin"}`}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			var e event
			require.NoError(t, jsonUnmarshal(tc.event, &e))

			recorded, known := record(context.Background(), e)

			require.Equal(t, tc.expectedRecorded, recorded)
			require.Equal(t, tc.expectedKnown, known)
		})
	}
}

func TestHandleReport(t *testing.T) {
	type test struct {
		body string

		expectedStatus int
	}

	tests := map[string]func(t *testing.T) test{
		"a_batch_is_accepted_whatever_its_events_held": func(t *testing.T) test {
			return test{
				body:           `{"events":[{"name":"game_ready","duration_ms":80},{"name":"nope"},"not an event"]}`,
				expectedStatus: http.StatusNoContent,
			}
		},
		"an_empty_batch": func(t *testing.T) test {
			return test{body: `{"events":[]}`, expectedStatus: http.StatusNoContent}
		},
		"not_a_batch": func(t *testing.T) test {
			return test{body: `[1,2]`, expectedStatus: http.StatusBadRequest}
		},
		"too_many_events": func(t *testing.T) test {
			events := strings.TrimSuffix(strings.Repeat(`{"name":"resync"},`, maxEvents+1), ",")
			return test{body: `{"events":[` + events + `]}`, expectedStatus: http.StatusBadRequest}
		},
		"over_the_body_limit": func(t *testing.T) test {
			return test{body: `{"events":[{"name":"x","kind":"` + strings.Repeat("a", maxBodyBytes) + `"}]}`, expectedStatus: http.StatusBadRequest}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodPost, "/v1/client-telemetry", strings.NewReader(tc.body))

			NewHandler(nil).handleReport(recorder, request)

			require.Equal(t, tc.expectedStatus, recorder.Code)
		})
	}
}

func jsonUnmarshal(raw string, into any) error {
	return json.Unmarshal([]byte(raw), into)
}
