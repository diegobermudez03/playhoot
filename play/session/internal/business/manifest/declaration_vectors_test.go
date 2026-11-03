package manifest

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// declarationCase is one runtime manifest the backend and the frontend's SDK build
// (packages/game-core validateManifest) must decide the same way.
type declarationCase struct {
	Name     string          `json:"name"`
	Manifest json.RawMessage `json:"manifest"`
	Valid    bool            `json:"valid"`
	Defaults *struct {
		Inputs   map[string]json.RawMessage            `json:"inputs"`
		Commands map[string]map[string]json.RawMessage `json:"commands"`
	} `json:"defaults"`
}

func TestSharedDeclarationVectors(t *testing.T) {
	raw, err := os.ReadFile("testdata/declaration-vectors.json")
	require.NoError(t, err)
	var vectors struct {
		Cases []declarationCase `json:"cases"`
	}
	require.NoError(t, json.Unmarshal(raw, &vectors))
	require.NotEmpty(t, vectors.Cases)

	for _, tc := range vectors.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			parsed, err := Parse(tc.Manifest)
			if !tc.Valid {
				require.ErrorIs(t, err, ErrInvalid)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, tc.Defaults, "a valid case states its defaults")

			inputs := make(map[string]json.RawMessage)
			for _, declaration := range parsed.Inputs {
				inputs[declaration.Name] = declaration.Default
			}
			require.Len(t, inputs, len(tc.Defaults.Inputs))
			for name, expected := range tc.Defaults.Inputs {
				require.JSONEq(t, string(expected), string(inputs[name]), name)
			}

			require.Len(t, parsed.Commands, len(tc.Defaults.Commands))
			for _, command := range parsed.Commands {
				expected, found := tc.Defaults.Commands[command.Name]
				require.True(t, found, command.Name)
				require.Len(t, command.Payload, len(expected))
				for _, field := range command.Payload {
					require.JSONEq(t, string(expected[field.Name]), string(field.Default), command.Name+"."+field.Name)
				}
			}
		})
	}
}

// The frontend keeps an identical copy for its own test, checked here when its checkout sits
// next to this one.
func TestSharedDeclarationVectorsMatchTheFrontendCopy(t *testing.T) {
	frontend, err := os.ReadFile("../../../../../../playhoot_frontend/packages/game-core/src/testdata/declaration-vectors.json")
	if os.IsNotExist(err) {
		t.Skip("the frontend checkout is not next to this one")
	}
	require.NoError(t, err)
	backend, err := os.ReadFile("testdata/declaration-vectors.json")
	require.NoError(t, err)
	require.Equal(t, string(bytes.ReplaceAll(backend, []byte("\r\n"), []byte("\n"))), string(bytes.ReplaceAll(frontend, []byte("\r\n"), []byte("\n"))))
}
