package play

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/runtime"
)

func TestDecodeClientMessage(t *testing.T) {
	type test struct {
		data            []byte
		expectedMessage runtime.ClientMessage
		expectedErr     error
	}

	tests := map[string]func(t *testing.T) test{
		"not_json": func(t *testing.T) test {
			return test{data: []byte(`not json`), expectedErr: errMalformedMessage}
		},
		"not_an_object": func(t *testing.T) test {
			return test{data: []byte(`["rejected"]`), expectedErr: errMalformedMessage}
		},
		"no_type": func(t *testing.T) test {
			return test{data: []byte(`{"reason":"x"}`), expectedErr: errMalformedMessage}
		},
		"type_not_a_string": func(t *testing.T) test {
			return test{data: []byte(`{"type":3}`), expectedErr: errMalformedMessage}
		},
		"unknown_type": func(t *testing.T) test {
			return test{data: []byte(`{"type":"teleport"}`), expectedErr: errUnknownMessageType}
		},
		"server_message_type_is_not_a_client_message": func(t *testing.T) test {
			return test{data: []byte(`{"type":"rejected","reason":"not_creator"}`), expectedErr: errUnknownMessageType}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			message, err := decodeClientMessage(tc.data)

			require.Equal(t, tc.expectedMessage, message)
			require.ErrorIs(t, err, tc.expectedErr)
		})
	}
}

func TestEncodeServerMessage(t *testing.T) {
	type test struct {
		message      runtime.ServerMessage
		expectedJSON string
		expectedErr  error
	}

	tests := map[string]func(t *testing.T) test{
		"rejected": func(t *testing.T) test {
			return test{
				message:      runtime.Rejected{Reason: runtime.RejectNotCreator},
				expectedJSON: `{"type":"rejected","reason":"not_creator"}`,
			}
		},
		"close_is_not_a_wire_message": func(t *testing.T) test {
			return test{message: runtime.Close{Reason: runtime.CloseAdmissionRejected}, expectedErr: errUnknownMessageType}
		},
		"nil_message": func(t *testing.T) test {
			return test{message: nil, expectedErr: errUnknownMessageType}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)

			data, err := encodeServerMessage(tc.message)

			require.ErrorIs(t, err, tc.expectedErr)
			if tc.expectedJSON != "" {
				require.JSONEq(t, tc.expectedJSON, string(data))
			} else {
				require.Nil(t, data)
			}
		})
	}
}
