package draftsources

import (
	"reflect"
	"testing"
)

func TestChanges(t *testing.T) {
	from := map[string]File{
		"game.project.json": {SHA256: "p", Size: 1},
		"server/game.ts":    {SHA256: "s1", Size: 2},
		"client/Game.tsx":   {SHA256: "c", Size: 3},
		"shared/old.ts":     {SHA256: "o", Size: 4},
	}
	to := map[string]File{
		"game.project.json": {SHA256: "p", Size: 1},
		"server/game.ts":    {SHA256: "s2", Size: 5},
		"client/Game.tsx":   {SHA256: "c", Size: 3},
		"shared/b.ts":       {SHA256: "b", Size: 6},
		"shared/a.ts":       {SHA256: "a", Size: 7},
	}
	got := Changes(from, to)
	want := FileChanges{Added: []string{"shared/a.ts", "shared/b.ts"}, Modified: []string{"server/game.ts"}, Deleted: []string{"shared/old.ts"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Changes = %+v, want %+v", got, want)
	}
}

func TestChangesFromNothingAddsEverything(t *testing.T) {
	got := Changes(nil, map[string]File{"b": {SHA256: "1"}, "a": {SHA256: "2"}})
	want := FileChanges{Added: []string{"a", "b"}, Modified: []string{}, Deleted: []string{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Changes = %+v, want %+v", got, want)
	}
}

func TestChangesOfTheSameFilesIsEmpty(t *testing.T) {
	files := map[string]File{"a": {SHA256: "1"}}
	got := Changes(files, files)
	if len(got.Added)+len(got.Modified)+len(got.Deleted) != 0 {
		t.Fatalf("Changes = %+v, want none", got)
	}
}
