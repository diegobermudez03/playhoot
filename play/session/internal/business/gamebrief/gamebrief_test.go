package gamebrief

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

type briefCase struct {
	Name   string          `json:"name"`
	Brief  json.RawMessage `json:"brief"`
	Valid  bool            `json:"valid"`
	Fields []string        `json:"fields"`
}

func TestSharedBriefVectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/brief-vectors.json")
	require.NoError(t, err)
	var vectors struct {
		Cases []briefCase `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(raw, &vectors))
	require.NotEmpty(t, vectors.Cases)

	for _, tc := range vectors.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			_, problems := Parse(tc.Brief)
			fields := []string{}
			for _, problem := range problems {
				fields = append(fields, problem.Field)
			}
			require.Equal(t, tc.Valid, len(problems) == 0, "%v", problems)
			require.Equal(t, tc.Fields, fields)
		})
	}
}

func TestPhase(t *testing.T) {
	brief, problems := Parse([]byte(`{"format": "playhoot.brief.v1"}`))
	require.Empty(t, problems)
	require.Equal(t, PhaseBrief, brief.Phase, "a brief without a phase is in the brief phase")

	brief, problems = Parse([]byte(`{"format": "playhoot.brief.v1", "phase": "assets"}`))
	require.Empty(t, problems)
	require.Equal(t, PhaseAssets, brief.Phase)
}

// The frontend keeps an identical copy for its own test, checked here when its checkout sits
// next to this one.
func TestSharedBriefVectorsMatchTheFrontendCopy(t *testing.T) {
	frontend, err := os.ReadFile("../../../../../../playhoot_frontend/packages/game-core/src/testdata/brief-vectors.json")
	if os.IsNotExist(err) {
		t.Skip("the frontend checkout is not next to this one")
	}
	require.NoError(t, err)
	backend, err := os.ReadFile("testdata/brief-vectors.json")
	require.NoError(t, err)
	require.Equal(t, string(bytes.ReplaceAll(backend, []byte("\r\n"), []byte("\n"))), string(bytes.ReplaceAll(frontend, []byte("\r\n"), []byte("\n"))))
}
