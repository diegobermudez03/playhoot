package versiondrafting

import (
	"fmt"

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
