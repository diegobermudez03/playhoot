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

//go:generate mockgen -package=versiondrafting -destination=step_diff_revisions_mock_test.go . diffRevisionsRepo

// diffRevisionsRepo is DiffRevisions' own narrow persistence contract.
type diffRevisionsRepo interface {
	FindVersion(ctx context.Context, tx *gorm.DB, versionUUID string) (*internalrepo.Version, error)
	FindRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64) (draftsources.Revision, bool, error)
	FindSourceContents(ctx context.Context, tx *gorm.DB, hashes []string) (map[string]string, error)
}

// DiffRevisions compares revision from of versionUUID (0: the empty project)
// with revision to (0: the current one), either order, every differing file
// or only onlyPath. A revision past the current one is
// sessionpkg.ErrDraftRevisionNotFound.
func (m *Manager) DiffRevisions(ctx context.Context, versionUUID string, from int64, to int64, onlyPath string) (sessionpkg.DraftRevisionDiff, error) {
	defer logging.Step(ctx, "versiondrafting.diff_revisions").Close()

	parsed, err := uuid.Parse(versionUUID)
	if err != nil {
		return sessionpkg.DraftRevisionDiff{}, fmt.Errorf("version uuid is not a valid UUID")
	}
	if from < 0 || to < 0 {
		return sessionpkg.DraftRevisionDiff{}, fmt.Errorf("revisions must not be negative")
	}
	return utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (sessionpkg.DraftRevisionDiff, error) {
		return m.diffRevisionsInTx(ctx, tx, parsed.String(), from, to, onlyPath)
	})
}

// diffRevisionsInTx is DiffRevisions' work inside its transaction.
func (m *Manager) diffRevisionsInTx(ctx context.Context, tx *gorm.DB, versionUUID string, from int64, to int64, onlyPath string) (sessionpkg.DraftRevisionDiff, error) {
	version, err := m.diffRevisionsRepo.FindVersion(ctx, tx, versionUUID)
	if err != nil {
		return sessionpkg.DraftRevisionDiff{}, err
	}
	if version == nil {
		return sessionpkg.DraftRevisionDiff{}, fmt.Errorf("version %s: %w", versionUUID, sessionpkg.ErrDraftNotFound)
	}
	var current int64
	if version.CurrentRevision != nil {
		current = *version.CurrentRevision
	}
	if to == 0 {
		to = current
	}
	if to == 0 || to > current || from > current {
		return sessionpkg.DraftRevisionDiff{}, fmt.Errorf("version %s revisions %d and %d: %w", versionUUID, from, to, sessionpkg.ErrDraftRevisionNotFound)
	}
	before, err := m.diffedFiles(ctx, tx, version, from)
	if err != nil {
		return sessionpkg.DraftRevisionDiff{}, err
	}
	after, err := m.diffedFiles(ctx, tx, version, to)
	if err != nil {
		return sessionpkg.DraftRevisionDiff{}, err
	}

	entries := differing(draftsources.Changes(before, after), onlyPath)
	var hashes []string
	for _, entry := range entries {
		if file, found := before[entry.Path]; found {
			hashes = append(hashes, file.SHA256)
		}
		if file, found := after[entry.Path]; found {
			hashes = append(hashes, file.SHA256)
		}
	}
	contents, err := m.diffRevisionsRepo.FindSourceContents(ctx, tx, hashes)
	if err != nil {
		return sessionpkg.DraftRevisionDiff{}, err
	}
	content := func(files map[string]draftsources.File, filePath string) (string, error) {
		file, found := files[filePath]
		if !found {
			return "", nil
		}
		text, stored := contents[file.SHA256]
		if !stored {
			monitoring.Alert(ctx, "play draft revision references a source content that is not stored")
			return "", fmt.Errorf("no content for %s (%s)", filePath, file.SHA256)
		}
		return text, nil
	}

	used := 0
	for i := range entries {
		entry := &entries[i]
		old, err := content(before, entry.Path)
		if err != nil {
			return sessionpkg.DraftRevisionDiff{}, err
		}
		updated, err := content(after, entry.Path)
		if err != nil {
			return sessionpkg.DraftRevisionDiff{}, err
		}
		diff := draftsources.Diff(entry.Path, old, updated)
		if used+len(diff.Diff) > sessionpkg.MaxDraftDiffBytes {
			entry.Omitted = true
			continue
		}
		used += len(diff.Diff)
		entry.Diff, entry.Truncated = diff.Diff, diff.Truncated
	}
	return sessionpkg.DraftRevisionDiff{From: from, To: to, Files: entries}, nil
}

// diffedFiles is the files of revision number of version; none for 0, the
// empty project.
func (m *Manager) diffedFiles(ctx context.Context, tx *gorm.DB, version *internalrepo.Version, number int64) (map[string]draftsources.File, error) {
	if number == 0 {
		return nil, nil
	}
	revision, found, err := m.diffRevisionsRepo.FindRevision(ctx, tx, version.ID, number)
	if err != nil {
		return nil, err
	}
	if !found {
		monitoring.Alert(ctx, "play draft version is missing a revision up to its current one")
		return nil, fmt.Errorf("version %s has no revision %d", version.UUID, number)
	}
	return revision.Files, nil
}

// differing is the files changes names, sorted by path, with their status;
// only onlyPath when it is set.
func differing(changes draftsources.FileChanges, onlyPath string) []sessionpkg.DraftRevisionFileDiff {
	entries := []sessionpkg.DraftRevisionFileDiff{}
	add := func(paths []string, status string) {
		for _, filePath := range paths {
			if onlyPath == "" || filePath == onlyPath {
				entries = append(entries, sessionpkg.DraftRevisionFileDiff{Path: filePath, Status: status})
			}
		}
	}
	add(changes.Added, sessionpkg.DraftFileAdded)
	add(changes.Modified, sessionpkg.DraftFileModified)
	add(changes.Deleted, sessionpkg.DraftFileDeleted)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	return entries
}
