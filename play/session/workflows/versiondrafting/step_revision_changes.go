package versiondrafting

import (
	"context"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=versiondrafting -destination=step_revision_changes_mock_test.go . revisionChangesRepo

// revisionChangesRepo is RevisionChanges' own narrow persistence contract.
type revisionChangesRepo interface {
	FindVersion(ctx context.Context, tx *gorm.DB, versionUUID string) (*internalrepo.Version, error)
	FindRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64) (draftsources.Revision, bool, error)
}

// RevisionChanges returns the files of revision upTo of versionUUID (its
// current revision when upTo is 0), without contents, and the paths that
// changed since its earlier revision since (0: since the beginning, every
// file added). A revision the version lacks, or since past upTo, is
// sessionpkg.ErrDraftRevisionNotFound.
func (m *Manager) RevisionChanges(ctx context.Context, versionUUID string, since int64, upTo int64) (sessionpkg.DraftRevisionChanges, error) {
	defer logging.Step(ctx, "versiondrafting.revision_changes").Close()

	parsed, err := uuid.Parse(versionUUID)
	if err != nil {
		return sessionpkg.DraftRevisionChanges{}, fmt.Errorf("version uuid is not a valid UUID")
	}
	if since < 0 || upTo < 0 {
		return sessionpkg.DraftRevisionChanges{}, fmt.Errorf("revisions must not be negative")
	}
	// One transaction, so both revisions are read against the same current
	// one.
	return utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (sessionpkg.DraftRevisionChanges, error) {
		return m.revisionChangesInTx(ctx, tx, parsed.String(), since, upTo)
	})
}

// revisionChangesInTx is RevisionChanges' work inside its transaction.
func (m *Manager) revisionChangesInTx(ctx context.Context, tx *gorm.DB, versionUUID string, since int64, upTo int64) (sessionpkg.DraftRevisionChanges, error) {
	version, err := m.revisionChangesRepo.FindVersion(ctx, tx, versionUUID)
	if err != nil {
		return sessionpkg.DraftRevisionChanges{}, err
	}
	if version == nil {
		return sessionpkg.DraftRevisionChanges{}, fmt.Errorf("version %s: %w", versionUUID, sessionpkg.ErrDraftNotFound)
	}
	var current int64
	if version.CurrentRevision != nil {
		current = *version.CurrentRevision
	}
	if upTo == 0 {
		upTo = current
	}
	if upTo > current || since > upTo || upTo == 0 {
		return sessionpkg.DraftRevisionChanges{}, fmt.Errorf("version %s revisions %d..%d: %w", versionUUID, since, upTo, sessionpkg.ErrDraftRevisionNotFound)
	}
	to, err := m.revisionFiles(ctx, tx, version, upTo)
	if err != nil {
		return sessionpkg.DraftRevisionChanges{}, err
	}
	var from map[string]draftsources.File
	if since > 0 {
		if from, err = m.revisionFiles(ctx, tx, version, since); err != nil {
			return sessionpkg.DraftRevisionChanges{}, err
		}
	}
	changes := draftsources.Changes(from, to)
	return sessionpkg.DraftRevisionChanges{
		Since: since, Revision: upTo, Files: fileList(to),
		Added: changes.Added, Modified: changes.Modified, Deleted: changes.Deleted,
	}, nil
}

// revisionFiles is the files of revision number of version, which must
// exist since number is at most its current revision.
func (m *Manager) revisionFiles(ctx context.Context, tx *gorm.DB, version *internalrepo.Version, number int64) (map[string]draftsources.File, error) {
	revision, found, err := m.revisionChangesRepo.FindRevision(ctx, tx, version.ID, number)
	if err != nil {
		return nil, err
	}
	if !found {
		monitoring.Alert(ctx, "play draft version is missing a revision up to its current one")
		return nil, fmt.Errorf("version %s has no revision %d", version.UUID, number)
	}
	return revision.Files, nil
}

// fileList is files sorted by path, without contents.
func fileList(files map[string]draftsources.File) []sessionpkg.DraftFile {
	list := make([]sessionpkg.DraftFile, 0, len(files))
	for filePath, file := range files {
		list = append(list, sessionpkg.DraftFile{Path: filePath, SHA256: file.SHA256, Size: file.Size})
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Path < list[j].Path })
	return list
}

// writtenChanges are the files of revision, which a write decided, and what
// changed since: the revision before it when the write made it, none when it
// kept the revision as it was.
func (m *Manager) writtenChanges(ctx context.Context, versionUUID string, revision int64, made bool) (*sessionpkg.DraftRevisionChanges, error) {
	since := revision
	if made {
		since = revision - 1
	}
	changes, err := m.RevisionChanges(ctx, versionUUID, since, revision)
	if err != nil {
		return nil, fmt.Errorf("reading the files of the written revision: %w", err)
	}
	return &changes, nil
}
