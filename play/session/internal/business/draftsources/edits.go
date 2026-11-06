package draftsources

import (
	"fmt"
	"strings"
)

// Diagnostic codes of a refused edit.
const (
	CodeInvalidEdit   = "invalid_edit"
	CodeEditNotFound  = "edit_not_found"
	CodeEditAmbiguous = "edit_ambiguous"
)

// Edit replaces Find with Replace in the file at Path: its one occurrence,
// or every occurrence when All.
type Edit struct {
	Path    string `json:"path"`
	Find    string `json:"find"`
	Replace string `json:"replace"`
	All     bool   `json:"all,omitempty"`
}

// EditedHashes are the content hashes of the current files edits touch, the
// contents ResolveEdits needs. Paths current lacks are left out.
func EditedHashes(current map[string]File, edits []Edit) []string {
	seen := make(map[string]bool, len(edits))
	var hashes []string
	for _, edit := range edits {
		file, found := current[edit.Path]
		if !found || seen[file.SHA256] {
			continue
		}
		seen[file.SHA256] = true
		hashes = append(hashes, file.SHA256)
	}
	return hashes
}

// ResolveEdits applies edits in order to the current files' contents (by
// hash), answering each edited file's whole new content as a write; any
// diagnostic (naming its edit's index) refuses the change.
func ResolveEdits(current map[string]File, contents map[string]string, edits []Edit) ([]Write, []Diagnostic, error) {
	var diagnostics []Diagnostic
	refuse := func(index int, filePath, code, message string) {
		at := index
		diagnostics = append(diagnostics, Diagnostic{Path: filePath, Code: code, Edit: &at, Message: fmt.Sprintf("edit %d: %s", index, message)})
	}

	edited := make(map[string]string, len(edits))
	var order []string
	failed := make(map[string]bool)
	for i, edit := range edits {
		pathRefused := false
		checkPath(edit.Path, func(filePath, code, message string) {
			pathRefused = true
			refuse(i, filePath, code, message)
		})
		if pathRefused || failed[edit.Path] {
			continue
		}
		content, found := edited[edit.Path]
		if !found {
			file, exists := current[edit.Path]
			if !exists {
				refuse(i, edit.Path, CodeNotFound, "the project has no such file to edit; write new files whole in files")
				failed[edit.Path] = true
				continue
			}
			if content, found = contents[file.SHA256]; !found {
				return nil, nil, fmt.Errorf("no content for %s (%s)", edit.Path, file.SHA256)
			}
			order = append(order, edit.Path)
		}
		next, ok := applyEdit(i, edit, content, refuse)
		if !ok {
			// Later edits of the file were made over this one's result; they
			// are not checked against a content that never was.
			failed[edit.Path] = true
			continue
		}
		edited[edit.Path] = next
	}
	if len(diagnostics) > 0 {
		return nil, diagnostics, nil
	}
	writes := make([]Write, len(order))
	for i, filePath := range order {
		writes[i] = Write{Path: filePath, Content: edited[filePath]}
	}
	return writes, nil, nil
}

// applyEdit is content with edit applied, refusing it when Find is empty,
// absent, found more than once without All, or the result would outgrow a
// file.
func applyEdit(index int, edit Edit, content string, refuse func(index int, filePath, code, message string)) (string, bool) {
	if edit.Find == "" {
		refuse(index, edit.Path, CodeInvalidEdit, "find is empty; it must be text the file holds")
		return "", false
	}
	count := strings.Count(content, edit.Find)
	switch {
	case count == 0:
		refuse(index, edit.Path, CodeEditNotFound, "find does not appear in the file (as edited by the edits before it); read the file again and copy the text exactly")
		return "", false
	case count > 1 && !edit.All:
		refuse(index, edit.Path, CodeEditAmbiguous, fmt.Sprintf("find appears %d times; add surrounding lines so it appears once, or set all to replace every occurrence", count))
		return "", false
	}
	if size := len(content) + count*(len(edit.Replace)-len(edit.Find)); size > MaxFileBytes {
		refuse(index, edit.Path, CodeFileTooLarge, fmt.Sprintf("the file would have %d bytes; the limit is %d", size, MaxFileBytes))
		return "", false
	}
	if edit.All {
		return strings.ReplaceAll(content, edit.Find, edit.Replace), true
	}
	return strings.Replace(content, edit.Find, edit.Replace, 1), true
}
