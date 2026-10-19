package jsonpatch

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiff(t *testing.T) {
	type test struct {
		before string
		after  string

		expected string
	}

	tests := map[string]func(t *testing.T) test{
		"equal_documents_need_no_operation": func(t *testing.T) test {
			return test{before: `{"a":1,"b":[1,{"c":null}]}`, after: `{"b":[1,{"c":null}],"a":1}`, expected: `null`}
		},
		"changed_field": func(t *testing.T) test {
			return test{before: `{"a":1,"b":2}`, after: `{"a":1,"b":3}`, expected: `[{"op":"replace","path":"/b","value":3}]`}
		},
		"removed_field": func(t *testing.T) test {
			return test{before: `{"a":1,"b":2}`, after: `{"a":1}`, expected: `[{"op":"remove","path":"/b"}]`}
		},
		"added_field": func(t *testing.T) test {
			return test{before: `{"a":1}`, after: `{"a":1,"b":{"c":true}}`, expected: `[{"op":"add","path":"/b","value":{"c":true}}]`}
		},
		"field_set_to_null_keeps_the_null": func(t *testing.T) test {
			return test{before: `{"a":1}`, after: `{"a":null}`, expected: `[{"op":"replace","path":"/a","value":null}]`}
		},
		"nested_change": func(t *testing.T) test {
			return test{before: `{"p":{"x":1,"y":2}}`, after: `{"p":{"x":1,"y":5}}`, expected: `[{"op":"replace","path":"/p/y","value":5}]`}
		},
		"reordered_list": func(t *testing.T) test {
			return test{before: `{"l":[1,2,3]}`, after: `{"l":[3,1,2]}`, expected: `[{"op":"replace","path":"/l/0","value":3},{"op":"replace","path":"/l/1","value":1},{"op":"replace","path":"/l/2","value":2}]`}
		},
		"shorter_list_removes_its_tail_from_the_end": func(t *testing.T) test {
			return test{before: `[1,2,3,4]`, after: `[1,2]`, expected: `[{"op":"remove","path":"/3"},{"op":"remove","path":"/2"}]`}
		},
		"longer_list_appends": func(t *testing.T) test {
			return test{before: `[1]`, after: `[1,{"a":2},null]`, expected: `[{"op":"add","path":"/1","value":{"a":2}},{"op":"add","path":"/2","value":null}]`}
		},
		"keys_with_pointer_characters_are_escaped": func(t *testing.T) test {
			return test{before: `{"a/b":1,"c~d":1}`, after: `{"a/b":2,"c~d":2}`, expected: `[{"op":"replace","path":"/a~1b","value":2},{"op":"replace","path":"/c~0d","value":2}]`}
		},
		"type_change_replaces_the_value": func(t *testing.T) test {
			return test{before: `{"a":[1]}`, after: `{"a":{"0":1}}`, expected: `[{"op":"replace","path":"/a","value":{"0":1}}]`}
		},
		"whole_document_replaced": func(t *testing.T) test {
			return test{before: `{"a":1}`, after: `[1]`, expected: `[{"op":"replace","path":"","value":[1]}]`}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			before, after := mustParse(t, tc.before), mustParse(t, tc.after)

			operations := Diff(before, after)

			encoded, err := json.Marshal(operations)
			require.NoError(t, err)
			require.JSONEq(t, tc.expected, string(encoded))
		})
	}
}

// TestApply checks that applying a diff, after a round trip through JSON,
// always rebuilds the target document.
func TestApply(t *testing.T) {
	type test struct {
		before string
		after  string
	}

	tests := map[string]func(t *testing.T) test{
		"fields_added_removed_and_nulled": func(t *testing.T) test {
			return test{before: `{"a":1,"b":2,"c":{"d":3}}`, after: `{"a":null,"c":{"e":[1,2]},"f":"x"}`}
		},
		"lists_reordered_shrunk_and_grown": func(t *testing.T) test {
			return test{before: `{"l":[1,2,3,4],"m":[{"a":1}],"n":[[1,2],[3]]}`, after: `{"l":[4,3],"m":[{"a":2},{"b":1},null],"n":[[2],[3,4,5]]}`}
		},
		"root_list": func(t *testing.T) test {
			return test{before: `[1,[2,3],{"a":[]}]`, after: `[[9],{"a":[1,2]}]`}
		},
		"escaped_keys": func(t *testing.T) test {
			return test{before: `{"a/b":{"~":1}}`, after: `{"a/b":{"~":2,"/":3}}`}
		},
		"numbers_keep_their_precision": func(t *testing.T) test {
			return test{before: `{"n":12345678901234567890}`, after: `{"n":12345678901234567891,"f":0.1000000000000000055511151231257827}`}
		},
		"scalar_to_object": func(t *testing.T) test {
			return test{before: `3`, after: `{"a":3}`}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			encoded, err := json.Marshal(Diff(mustParse(t, tc.before), mustParse(t, tc.after)))
			require.NoError(t, err)
			var operations []Operation
			require.NoError(t, json.Unmarshal(encoded, &operations))

			patched, err := Apply(mustParse(t, tc.before), operations)

			require.NoError(t, err)
			result, err := json.Marshal(patched)
			require.NoError(t, err)
			require.JSONEq(t, tc.after, string(result))
			require.Equal(t, mustParse(t, tc.after), patched)
		})
	}
}

func TestApplyRejectsInvalidPaths(t *testing.T) {
	tests := map[string]Operation{
		"replace_missing_field": {Op: OpReplace, Path: "/missing", Value: 1},
		"remove_missing_field":  {Op: OpRemove, Path: "/missing"},
		"index_out_of_range":    {Op: OpReplace, Path: "/l/5", Value: 1},
		"path_inside_a_scalar":  {Op: OpAdd, Path: "/a/b", Value: 1},
		"path_without_slash":    {Op: OpAdd, Path: "a", Value: 1},
		"remove_whole_document": {Op: OpRemove, Path: ""},
		"unknown_operation":     {Op: "move", Path: "/a"},
		"missing_parent":        {Op: OpAdd, Path: "/x/y", Value: 1},
		"index_not_a_number":    {Op: OpRemove, Path: "/l/x"},
		"add_past_the_list_end": {Op: OpAdd, Path: "/l/3", Value: 1},
	}

	for name, operation := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Apply(mustParse(t, `{"a":1,"l":[1,2]}`), []Operation{operation})

			require.Error(t, err)
		})
	}
}

func mustParse(t *testing.T, raw string) any {
	t.Helper()
	value, err := Parse([]byte(raw))
	require.NoError(t, err)
	return value
}
