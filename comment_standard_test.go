package main

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// internalCitationPattern matches the shapes of internal decision-tracking
// reference this repository's own engineering comment standard forbids
// citing as the reason something is true in a source comment: an ADR
// number, a WORK number, a Blocker/Slice reference, or a path into this
// repository's own docs tree. The actual reasoning must be written out in
// plain language instead of pointing at a document for it.
var internalCitationPattern = regexp.MustCompile(`(?i)\b(?:[A-Z]+-)?ADR-\d+\b|\bWORK-\d{4}\b|\bBlocker \d+\b|\bSlice \d+\b|\bdocs/(?:work|engineering|ai)/`)

// TestNoInternalDocCitationsInComments walks every tracked .go file's
// comments and fails if any cites an internal ADR/WORK/Blocker/Slice
// reference or docs-tree path, catching the mistake mechanically instead
// of relying solely on code review to notice it.
func TestNoInternalDocCitationsInComments(t *testing.T) {
	var violations []string

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
		for _, group := range file.Comments {
			for _, comment := range group.List {
				if internalCitationPattern.MatchString(comment.Text) {
					violations = append(violations, fmt.Sprintf("%s: %s", fset.Position(comment.Pos()), strings.TrimSpace(comment.Text)))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking repository for .go files: %v", err)
	}

	if len(violations) > 0 {
		t.Errorf("found %d comment(s) citing an internal ADR/WORK/Blocker/Slice/docs-path reference as justification - state the reasoning directly instead (see docs/engineering/standards/code-comments.md):\n%s",
			len(violations), strings.Join(violations, "\n"))
	}
}
