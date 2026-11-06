package draftsources

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
)

func TestApply(t *testing.T) {
	type test struct {
		writes  []Write
		deletes []string

		expectedFiles map[string]File
		expectedCodes []string
	}

	base := func() map[string]File {
		files := make(map[string]File)
		for _, write := range Template() {
			files[write.Path] = File{SHA256: SHA256Hex(write.Content), Size: int64(len(write.Content))}
		}
		return files
	}
	tests := map[string]func(t *testing.T) test{
		"adds_replaces_and_deletes": func(t *testing.T) test {
			current := base()
			current["shared/old.ts"] = File{SHA256: "abc", Size: 3}
			expected := base()
			expected["server/game.ts"] = File{SHA256: SHA256Hex("x"), Size: 1}
			expected["scenarios/win.json"] = File{SHA256: SHA256Hex(validScenario), Size: int64(len(validScenario))}
			return test{
				writes:        []Write{{Path: "server/game.ts", Content: "x"}, {Path: "scenarios/win.json", Content: validScenario}},
				deletes:       []string{"shared/old.ts"},
				expectedFiles: expected,
			}
		},
		"empty_change": func(t *testing.T) test {
			return test{expectedCodes: []string{CodeEmptyChange}}
		},
		"invalid_paths": func(t *testing.T) test {
			return test{
				writes: []Write{
					{Path: "/server/a.ts"}, {Path: "server/../a.ts"}, {Path: `server\a.ts`},
					{Path: "server//a.ts"}, {Path: ""},
				},
				expectedCodes: []string{CodeInvalidPath, CodeInvalidPath, CodeInvalidPath, CodeInvalidPath, CodeInvalidPath},
			}
		},
		"paths_not_allowed": func(t *testing.T) test {
			return test{
				writes:        []Write{{Path: "dist/server.js"}, {Path: "server/game.js"}, {Path: "scenarios/a.ts"}, {Path: "README.md"}},
				expectedCodes: []string{CodePathNotAllowed, CodePathNotAllowed, CodePathNotAllowed, CodePathNotAllowed},
			}
		},
		"duplicates_across_writes_and_deletes": func(t *testing.T) test {
			return test{
				writes:        []Write{{Path: "shared/a.ts", Content: "1"}, {Path: "shared/a.ts", Content: "2"}},
				deletes:       []string{"shared/a.ts"},
				expectedCodes: []string{CodeDuplicatePath, CodeDuplicatePath},
			}
		},
		"deleting_required_or_missing_files": func(t *testing.T) test {
			return test{
				deletes:       []string{"server/game.ts", "shared/missing.ts"},
				expectedCodes: []string{CodeRequiredFile, CodeNotFound},
			}
		},
		"invalid_text_and_too_large_file": func(t *testing.T) test {
			return test{
				writes:        []Write{{Path: "shared/a.ts", Content: "\xff"}, {Path: "shared/b.ts", Content: strings.Repeat("a", MaxFileBytes+1)}},
				expectedCodes: []string{CodeInvalidUTF8, CodeFileTooLarge},
			}
		},
		"too_many_files": func(t *testing.T) test {
			var writes []Write
			for i := 0; i < MaxFiles; i++ {
				writes = append(writes, Write{Path: "shared/f" + strconv.Itoa(i) + ".ts"})
			}
			return test{writes: writes, expectedCodes: []string{CodeTooManyFiles}}
		},
		"project_too_large": func(t *testing.T) test {
			var writes []Write
			for i := 0; i < 9; i++ {
				writes = append(writes, Write{Path: "shared/big" + strconv.Itoa(i) + ".ts", Content: strings.Repeat("a", MaxFileBytes)})
			}
			return test{writes: writes, expectedCodes: []string{CodeProjectTooLarge}}
		},
	}
	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			current := base()
			if name == "adds_replaces_and_deletes" {
				current["shared/old.ts"] = File{SHA256: "abc", Size: 3}
			}
			applied := Apply(current, tc.writes, tc.deletes)
			codes := make([]string, len(applied.Diagnostics))
			for i, diagnostic := range applied.Diagnostics {
				codes[i] = diagnostic.Code
			}
			if len(tc.expectedCodes) > 0 {
				require.Equal(t, tc.expectedCodes, codes)
				require.Nil(t, applied.Files)
				return
			}
			require.Empty(t, codes)
			require.Equal(t, tc.expectedFiles, applied.Files)
			for _, write := range tc.writes {
				require.Equal(t, write.Content, applied.Contents[SHA256Hex(write.Content)])
			}
		})
	}
}

func TestTemplate(t *testing.T) {
	writes := Template()
	paths := make([]string, len(writes))
	for i, write := range writes {
		paths[i] = write.Path
	}
	require.Equal(t, []string{"client/Game.tsx", "game.project.json", "notes.md", "scenarios/first-to-goal.json", "server/game.ts", "shared/actions.ts"}, paths)
	applied := Apply(nil, writes, nil)
	require.Empty(t, applied.Diagnostics, "the template must be a valid project")
}

func TestDeclaredAssets(t *testing.T) {
	t.Run("reads the declared assets and an absent declaration as none", func(t *testing.T) {
		declared, diagnostics := DeclaredAssets(`{"format":"playhoot.project.v1","assets":{"cardBack":{"type":"image","description":"Card back, 512x768"},"flip":{"type":"audio","description":"Flip sound","optional":true}}}`)
		require.Empty(t, diagnostics)
		require.Equal(t, map[string]AssetDeclaration{
			"cardBack": {Type: gameversion.AssetImage, Description: "Card back, 512x768"},
			"flip":     {Type: gameversion.AssetAudio, Description: "Flip sound", Optional: true},
		}, declared)

		none, diagnostics := DeclaredAssets(`{"format":"playhoot.project.v1"}`)
		require.Empty(t, diagnostics)
		require.Empty(t, none)
	})

	t.Run("names every invalid declaration", func(t *testing.T) {
		_, diagnostics := DeclaredAssets(`{"assets":{"1bad":{"type":"image","description":"x"},"noType":{"type":"model","description":"x"},"noDescription":{"type":"image","description":" "},"notObject":"image"}}`)
		require.Len(t, diagnostics, 4)
		for _, diagnostic := range diagnostics {
			require.Equal(t, ProjectFile, diagnostic.Path)
			require.Equal(t, CodeInvalidAsset, diagnostic.Code)
		}
		_, diagnostics = DeclaredAssets(`not json`)
		require.Equal(t, CodeInvalidProjectFile, diagnostics[0].Code)
	})

	t.Run("a change writing an invalid project file is refused", func(t *testing.T) {
		applied := Apply(map[string]File{}, []Write{{Path: ProjectFile, Content: `{"languages":["es"],"assets":{"x":{"type":"image"}}}`}}, nil)
		require.Equal(t, CodeInvalidAsset, applied.Diagnostics[0].Code)
	})
}

func TestSameRevision(t *testing.T) {
	files := map[string]File{"a.ts": {SHA256: "1", Size: 1}}
	with := Revision{Files: files, Assets: map[string]AssetBinding{"cardBack": {SHA256: "p", Size: 3, MIMEType: "image/png"}}}
	require.True(t, SameRevision(with, Revision{Files: files, Assets: map[string]AssetBinding{"cardBack": {SHA256: "p", Size: 3, MIMEType: "image/png"}}}))
	require.False(t, SameRevision(with, Revision{Files: files}))
	require.False(t, SameRevision(with, Revision{Files: files, Assets: map[string]AssetBinding{"cardBack": {SHA256: "q"}}}))
}

func TestAssetStates(t *testing.T) {
	declared := map[string]AssetDeclaration{
		"cardBack": {Type: gameversion.AssetImage, Description: "back"},
		"flip":     {Type: gameversion.AssetAudio, Description: "flip", Optional: true},
	}
	bindings := map[string]AssetBinding{"cardBack": {SHA256: "p"}, "old": {SHA256: "o"}}
	states := AssetStates(declared, bindings)
	require.Len(t, states, 3)
	require.Equal(t, "cardBack", states[0].Key)
	require.Equal(t, AssetReady, states[0].Status)
	require.Equal(t, AssetMissing, states[1].Status)
	require.Equal(t, "old", states[2].Key)
	require.Equal(t, AssetUndeclared, states[2].Status)
}

func TestDeclaredLanguages(t *testing.T) {
	languages, diagnostics := DeclaredLanguages(`{"format":"playhoot.project.v1","languages":["es","en"]}`)
	require.Empty(t, diagnostics)
	require.Equal(t, []gameversion.Language{"es", "en"}, languages)

	for name, project := range map[string]string{
		"absent":      `{"format":"playhoot.project.v1"}`,
		"empty":       `{"languages":[]}`,
		"not_a_list":  `{"languages":"es"}`,
		"unsupported": `{"languages":["es","fr"]}`,
		"repeated":    `{"languages":["es","es"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			languages, diagnostics := DeclaredLanguages(project)
			require.Nil(t, languages)
			require.Len(t, diagnostics, 1)
			require.Equal(t, CodeInvalidLanguages, diagnostics[0].Code)
		})
	}

	t.Run("a change of the project file without them is invalid", func(t *testing.T) {
		applied := Apply(nil, []Write{{Path: ProjectFile, Content: `{"format":"playhoot.project.v1"}`}}, nil)
		require.NotEmpty(t, applied.Diagnostics)
		require.Equal(t, CodeInvalidLanguages, applied.Diagnostics[0].Code)
	})
}

const validScenario = `{"format": "playhoot.scenario.v1", "players": 2, "duration_ms": 1000}`

func TestApplyValidatesSavedScenarios(t *testing.T) {
	applied := Apply(nil, append(Template(), Write{Path: "scenarios/broken.json", Content: `{"format": "playhoot.scenario.v1", "players": 2, "expect": [{"path": "a", "op": "matches"}]}`}), nil)

	codes := make([]string, len(applied.Diagnostics))
	for i, diagnostic := range applied.Diagnostics {
		require.Equal(t, "scenarios/broken.json", diagnostic.Path)
		codes[i] = diagnostic.Code
	}
	require.Equal(t, []string{CodeInvalidScenario, CodeInvalidScenario}, codes)
	require.Equal(t, "duration_ms: is required", applied.Diagnostics[0].Message)
}
