package inputs

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// vectorCase is one field value the backend and the frontend's SDK (packages/game-core
// validateValues) must decide the same way.
type vectorCase struct {
	Name      string          `json:"name"`
	Field     inputDocument   `json:"field"`
	Valid     bool            `json:"valid"`
	Value     json.RawMessage `json:"value"`
	Absent    bool            `json:"absent"`
	Payload   json.RawMessage `json:"payload"`
	Canonical json.RawMessage `json:"canonical"`
}

// inputDocument is a field as a manifest declares it.
type inputDocument struct {
	Type     Type            `json:"type"`
	Nullable bool            `json:"nullable"`
	Min      *float64        `json:"min"`
	Max      *float64        `json:"max"`
	Values   []string        `json:"values"`
	Default  json.RawMessage `json:"default"`
}

func TestSharedFieldVectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/vectors.json")
	require.NoError(t, err)
	var vectors struct {
		Cases []vectorCase `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(raw, &vectors))
	require.NotEmpty(t, vectors.Cases)

	for _, tc := range vectors.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			schema, err := NewSchema([]Declaration{{
				Name: "f", Type: tc.Field.Type, Nullable: tc.Field.Nullable,
				Min: tc.Field.Min, Max: tc.Field.Max, Values: tc.Field.Values, Default: tc.Field.Default,
			}})
			require.NoError(t, err, "every vector declares a valid field")

			payload := []byte(`{}`)
			switch {
			case len(tc.Payload) > 0:
				payload = tc.Payload
			case !tc.Absent:
				payload = append(append([]byte(`{"f":`), tc.Value...), '}')
			}
			snapshot, err := schema.Normalize(payload)
			if !tc.Valid {
				require.ErrorIs(t, err, ErrInvalidValue)
				return
			}
			require.NoError(t, err)
			var got struct {
				F json.RawMessage `json:"f"`
			}
			require.NoError(t, json.Unmarshal([]byte(snapshot), &got))
			require.JSONEq(t, string(tc.Canonical), string(got.F))
		})
	}
}

// The frontend keeps an identical copy for its own test, checked here when its checkout sits
// next to this one.
func TestSharedFieldVectorsMatchTheFrontendCopy(t *testing.T) {
	frontend, err := os.ReadFile("../../../../../playhoot_frontend/packages/game-core/src/testdata/field-vectors.json")
	if os.IsNotExist(err) {
		t.Skip("the frontend checkout is not next to this one")
	}
	require.NoError(t, err)
	backend, err := os.ReadFile("testdata/vectors.json")
	require.NoError(t, err)
	require.Equal(t, string(bytes.ReplaceAll(backend, []byte("\r\n"), []byte("\n"))), string(bytes.ReplaceAll(frontend, []byte("\r\n"), []byte("\n"))))
}
