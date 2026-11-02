package draftsources

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
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
			expected["scenarios/win.json"] = File{SHA256: SHA256Hex("{}"), Size: 2}
			return test{
				writes:        []Write{{Path: "server/game.ts", Content: "x"}, {Path: "scenarios/win.json", Content: "{}"}},
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
	require.Equal(t, []string{"client/Game.tsx", "game.project.json", "notes.md", "server/game.ts", "shared/actions.ts"}, paths)
	applied := Apply(nil, writes, nil)
	require.Empty(t, applied.Diagnostics, "the template must be a valid project")
}
