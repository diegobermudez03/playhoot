package versiondrafting

import (
	"fmt"
	"testing"

	"go.uber.org/mock/gomock"

	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
)

// revisionWithFiles matches a draftsources.Revision holding files files.
func revisionWithFiles(files int) gomock.Matcher {
	return revisionMatcher{files: files}
}

type revisionMatcher struct {
	files int
}

func (m revisionMatcher) Matches(x any) bool {
	revision, ok := x.(draftsources.Revision)
	return ok && len(revision.Files) == m.files
}

func (m revisionMatcher) String() string {
	return fmt.Sprintf("is a revision with %d files", m.files)
}

// templateContent is the content of the starter template's file at filePath.
func templateContent(t *testing.T, filePath string) string {
	t.Helper()
	for _, write := range draftsources.Template() {
		if write.Path == filePath {
			return write.Content
		}
	}
	t.Fatalf("the template has no %s", filePath)
	return ""
}
