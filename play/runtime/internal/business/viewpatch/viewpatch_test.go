package viewpatch

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDiff(t *testing.T) {
	type test struct {
		previous string
		next     string

		expectedPatch string
		expectedFull  bool
		expectErr     bool
	}

	tests := map[string]func(t *testing.T) test{
		"first_view_is_sent_whole": func(t *testing.T) test {
			return test{next: `{"round":1}`, expectedPatch: `{"round":1}`, expectedFull: true}
		},
		"unchanged_view_is_an_empty_patch": func(t *testing.T) test {
			return test{previous: `{"round":1,"me":{"x":1}}`, next: `{"round":1,"me":{"x":1}}`, expectedPatch: `{}`}
		},
		"only_changed_fields": func(t *testing.T) test {
			return test{previous: `{"round":1,"score":3,"title":"a"}`, next: `{"round":2,"score":3,"title":"a"}`, expectedPatch: `{"round":2}`}
		},
		"nested_objects_are_patched_too": func(t *testing.T) test {
			return test{
				previous:      `{"me":{"x":1,"y":2},"others":{"p2":{"x":5}}}`,
				next:          `{"me":{"x":1,"y":3},"others":{"p2":{"x":5}}}`,
				expectedPatch: `{"me":{"y":3}}`,
			}
		},
		"changed_list_is_sent_whole": func(t *testing.T) test {
			return test{previous: `{"hand":["a","b","c"],"round":1}`, next: `{"hand":["a","c"],"round":1}`, expectedPatch: `{"hand":["a","c"]}`}
		},
		"added_field": func(t *testing.T) test {
			return test{previous: `{"round":1}`, next: `{"round":1,"winner":"P1"}`, expectedPatch: `{"winner":"P1"}`}
		},
		"object_replaced_by_a_value": func(t *testing.T) test {
			return test{previous: `{"target":{"x":1}}`, next: `{"target":null}`, expectedPatch: `{"target":null}`}
		},
		"removed_field_falls_back_to_a_full_view": func(t *testing.T) test {
			return test{previous: `{"round":1,"hint":"x"}`, next: `{"round":1}`, expectedPatch: `{"round":1}`, expectedFull: true}
		},
		"removed_nested_field_falls_back_to_a_full_view": func(t *testing.T) test {
			return test{previous: `{"me":{"x":1,"y":2}}`, next: `{"me":{"x":1}}`, expectedPatch: `{"me":{"x":1}}`, expectedFull: true}
		},
		"view_that_is_not_an_object_is_sent_whole": func(t *testing.T) test {
			return test{previous: `[1]`, next: `[2]`, expectedPatch: `[2]`, expectedFull: true}
		},
		"invalid_view": func(t *testing.T) test {
			return test{previous: `{"round":1}`, next: `{`, expectErr: true}
		},
	}

	for name, setup := range tests {
		t.Run(name, func(t *testing.T) {
			tc := setup(t)
			var previous []byte
			if tc.previous != "" {
				previous = []byte(tc.previous)
			}

			patch, full, err := Diff(previous, []byte(tc.next))

			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expectedFull, full)
			require.JSONEq(t, tc.expectedPatch, string(patch))
		})
	}
}
