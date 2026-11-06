package draftsources

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestResolveEdits(t *testing.T) {
	type test struct {
		edits []Edit

		expectedWrites []Write
		expectedCodes  []string
		expectedIndex  []int
	}

	contents := map[string]string{
		"server/game.ts": "const a = 1;\nconst b = 2;\nconst a2 = 1;\n",
		"notes.md":       "# Notes\n",
	}
	current := make(map[string]File, len(contents))
	byHash := make(map[string]string, len(contents))
	for filePath, content := range contents {
		hash := SHA256Hex(content)
		current[filePath] = File{SHA256: hash, Size: int64(len(content))}
		byHash[hash] = content
	}

	tests := map[string]func(t *testing.T) test{
		"applies_edits_in_order_on_the_edited_content": func(t *testing.T) test {
			return test{
				edits: []Edit{
					{Path: "server/game.ts", Find: "const b = 2;", Replace: "const b = 3;"},
					{Path: "notes.md", Find: "# Notes", Replace: "# Notes\nmore"},
					{Path: "server/game.ts", Find: "b = 3", Replace: "b = 4"},
				},
				expectedWrites: []Write{
					{Path: "server/game.ts", Content: "const a = 1;\nconst b = 4;\nconst a2 = 1;\n"},
					{Path: "notes.md", Content: "# Notes\nmore\n"},
				},
			}
		},
		"all_replaces_every_occurrence": func(t *testing.T) test {
			return test{
				edits:          []Edit{{Path: "server/game.ts", Find: "= 1", Replace: "= 9", All: true}},
				expectedWrites: []Write{{Path: "server/game.ts", Content: "const a = 9;\nconst b = 2;\nconst a2 = 9;\n"}},
			}
		},
		"refuses_an_ambiguous_find": func(t *testing.T) test {
			return test{
				edits:         []Edit{{Path: "server/game.ts", Find: "= 1", Replace: "= 9"}},
				expectedCodes: []string{CodeEditAmbiguous},
				expectedIndex: []int{0},
			}
		},
		"refuses_a_find_the_file_lacks": func(t *testing.T) test {
			return test{
				edits: []Edit{
					{Path: "notes.md", Find: "# Notes", Replace: "# N"},
					{Path: "server/game.ts", Find: "const c", Replace: "const d"},
				},
				expectedCodes: []string{CodeEditNotFound},
				expectedIndex: []int{1},
			}
		},
		"checks_later_edits_against_earlier_ones": func(t *testing.T) test {
			return test{
				edits: []Edit{
					{Path: "notes.md", Find: "# Notes", Replace: "# Other"},
					{Path: "notes.md", Find: "# Notes", Replace: "# N"},
				},
				expectedCodes: []string{CodeEditNotFound},
				expectedIndex: []int{1},
			}
		},
		"skips_the_edits_after_a_failed_one_of_the_same_file": func(t *testing.T) test {
			return test{
				edits: []Edit{
					{Path: "notes.md", Find: "missing", Replace: "x"},
					{Path: "notes.md", Find: "also missing", Replace: "y"},
				},
				expectedCodes: []string{CodeEditNotFound},
				expectedIndex: []int{0},
			}
		},
		"refuses_empty_finds_missing_files_and_bad_paths": func(t *testing.T) test {
			return test{
				edits: []Edit{
					{Path: "notes.md", Find: "", Replace: "x"},
					{Path: "server/other.ts", Find: "a", Replace: "b"},
					{Path: "../etc", Find: "a", Replace: "b"},
				},
				expectedCodes: []string{CodeInvalidEdit, CodeNotFound, CodeInvalidPath},
				expectedIndex: []int{0, 1, 2},
			}
		},
		"refuses_a_result_larger_than_a_file": func(t *testing.T) test {
			return test{
				edits:         []Edit{{Path: "notes.md", Find: "Notes", Replace: strings.Repeat("x", MaxFileBytes)}},
				expectedCodes: []string{CodeFileTooLarge},
				expectedIndex: []int{0},
			}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			test := setup(t)

			writes, diagnostics, err := ResolveEdits(current, byHash, test.edits)

			require.NoError(t, err)
			require.Equal(t, test.expectedWrites, writes)
			var codes []string
			var indexes []int
			for _, diagnostic := range diagnostics {
				codes = append(codes, diagnostic.Code)
				require.NotNil(t, diagnostic.Edit)
				indexes = append(indexes, *diagnostic.Edit)
			}
			require.Equal(t, test.expectedCodes, codes)
			require.Equal(t, test.expectedIndex, indexes)
		})
	}
}

func TestResolveEditsWithoutTheContent(t *testing.T) {
	current := map[string]File{"notes.md": {SHA256: SHA256Hex("x"), Size: 1}}

	_, _, err := ResolveEdits(current, map[string]string{}, []Edit{{Path: "notes.md", Find: "x", Replace: "y"}})

	require.Error(t, err)
}

func TestEditedHashes(t *testing.T) {
	current := map[string]File{"a.ts": {SHA256: "h1"}, "b.ts": {SHA256: "h1"}, "c.ts": {SHA256: "h2"}}

	hashes := EditedHashes(current, []Edit{{Path: "a.ts"}, {Path: "b.ts"}, {Path: "missing.ts"}, {Path: "c.ts"}, {Path: "a.ts"}})

	require.Equal(t, []string{"h1", "h2"}, hashes)
}

func TestDiff(t *testing.T) {
	diff := Diff("notes.md", "a\nb\nc\n", "a\nB\nc\n")

	require.False(t, diff.Truncated)
	require.Equal(t, "--- a/notes.md\n+++ b/notes.md\n@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n", diff.Diff)

	var before, after strings.Builder
	for i := 0; i < 2000; i++ {
		before.WriteString("line one\n")
		after.WriteString("line two\n")
	}
	long := Diff("notes.md", before.String(), after.String())
	require.True(t, long.Truncated)
	require.LessOrEqual(t, len(long.Diff), MaxDiffBytes)
	require.True(t, strings.HasSuffix(long.Diff, "\n"))
}
