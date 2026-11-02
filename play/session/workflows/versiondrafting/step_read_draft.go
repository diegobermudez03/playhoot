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
	"github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/changeattempt"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=versiondrafting -destination=step_read_draft_mock_test.go . readDraftRepo

// readDraftRepo is ReadDraft's own narrow persistence contract.
type readDraftRepo interface {
	FindVersion(ctx context.Context, tx *gorm.DB, versionUUID string) (*internalrepo.Version, error)
	FindRevisionFiles(ctx context.Context, tx *gorm.DB, versionID uint, number int64) (map[string]draftsources.File, bool, error)
	FindSourceContents(ctx context.Context, tx *gorm.DB, hashes []string) (map[string]string, error)
	RecentChangeAttempts(ctx context.Context, tx *gorm.DB, versionID uint, limit int) ([]internalrepo.ChangeAttempt, error)
}

// ReadDraft returns versionUUID's sources at revision, or at its current
// revision when revision is 0, with their contents when withContent, and its
// latest change attempts. A revision it lacks is
// sessionpkg.ErrDraftRevisionNotFound.
func (m *Manager) ReadDraft(ctx context.Context, versionUUID string, revision int64, withContent bool) (sessionpkg.DraftSnapshot, error) {
	defer logging.Step(ctx, "versiondrafting.read_draft").Close()

	parsed, err := uuid.Parse(versionUUID)
	if err != nil {
		return sessionpkg.DraftSnapshot{}, fmt.Errorf("version uuid is not a valid UUID")
	}
	if revision < 0 {
		return sessionpkg.DraftSnapshot{}, fmt.Errorf("revision must not be negative")
	}
	// One transaction, so the files, contents and attempts are read together.
	return utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (sessionpkg.DraftSnapshot, error) {
		return m.readDraftInTx(ctx, tx, parsed.String(), revision, withContent)
	})
}

// readDraftInTx is ReadDraft's work inside its transaction.
func (m *Manager) readDraftInTx(ctx context.Context, tx *gorm.DB, versionUUID string, revision int64, withContent bool) (sessionpkg.DraftSnapshot, error) {
	version, err := m.readDraftRepo.FindVersion(ctx, tx, versionUUID)
	if err != nil {
		return sessionpkg.DraftSnapshot{}, err
	}
	if version == nil {
		return sessionpkg.DraftSnapshot{}, fmt.Errorf("version %s: %w", versionUUID, sessionpkg.ErrDraftNotFound)
	}
	snapshot := sessionpkg.DraftSnapshot{VersionUUID: version.UUID, GameUUID: version.GameUUID, Status: version.Status}
	if version.CurrentRevision != nil {
		snapshot.CurrentRevision = *version.CurrentRevision
	}
	if revision == 0 {
		revision = snapshot.CurrentRevision
	}
	if revision > snapshot.CurrentRevision {
		return sessionpkg.DraftSnapshot{}, fmt.Errorf("version %s revision %d: %w", versionUUID, revision, sessionpkg.ErrDraftRevisionNotFound)
	}
	snapshot.Revision = revision

	if revision > 0 {
		files, found, err := m.readDraftRepo.FindRevisionFiles(ctx, tx, version.ID, revision)
		if err != nil {
			return sessionpkg.DraftSnapshot{}, err
		}
		if !found {
			monitoring.Alert(ctx, "play draft version is missing a revision up to its current one")
			return sessionpkg.DraftSnapshot{}, fmt.Errorf("version %s has no revision %d", versionUUID, revision)
		}
		snapshot.Files, err = m.draftFiles(ctx, tx, files, withContent)
		if err != nil {
			return sessionpkg.DraftSnapshot{}, err
		}
	}

	attempts, err := m.readDraftRepo.RecentChangeAttempts(ctx, tx, version.ID, maxRecentAttempts)
	if err != nil {
		return sessionpkg.DraftSnapshot{}, err
	}
	for _, attempt := range attempts {
		snapshot.Attempts = append(snapshot.Attempts, publicAttempt(attempt))
	}
	return snapshot, nil
}

// draftFiles is a revision's files sorted by path, with their contents when
// asked.
func (m *Manager) draftFiles(ctx context.Context, tx *gorm.DB, files map[string]draftsources.File, withContent bool) ([]sessionpkg.DraftFile, error) {
	paths := make([]string, 0, len(files))
	for filePath := range files {
		paths = append(paths, filePath)
	}
	sort.Strings(paths)
	var contents map[string]string
	if withContent {
		hashes := make([]string, 0, len(files))
		for _, file := range files {
			hashes = append(hashes, file.SHA256)
		}
		var err error
		contents, err = m.readDraftRepo.FindSourceContents(ctx, tx, hashes)
		if err != nil {
			return nil, err
		}
	}
	result := make([]sessionpkg.DraftFile, len(paths))
	for i, filePath := range paths {
		file := files[filePath]
		result[i] = sessionpkg.DraftFile{Path: filePath, SHA256: file.SHA256, Size: file.Size}
		if !withContent {
			continue
		}
		content, found := contents[file.SHA256]
		if !found {
			monitoring.Alert(ctx, "play draft revision references a source content that is not stored")
			return nil, fmt.Errorf("source content %s of %s is not stored", file.SHA256, filePath)
		}
		result[i].Content = &content
	}
	return result, nil
}

// publicAttempt is a completed attempt in its public form; its revision is
// shown only when it applied.
func publicAttempt(attempt internalrepo.ChangeAttempt) sessionpkg.DraftChangeAttempt {
	public := sessionpkg.DraftChangeAttempt{
		BaseRevision: attempt.BaseRevision,
		Origin:       attempt.Origin,
		Diagnostics:  changeattempt.Public(attempt.Diagnostics),
		CreatedAt:    attempt.CreatedAt,
	}
	if attempt.Outcome != nil {
		public.Outcome = sessionpkg.DraftChangeOutcome(*attempt.Outcome)
	}
	if public.Outcome == sessionpkg.DraftChangeApplied || public.Outcome == sessionpkg.DraftChangeUnchanged {
		public.Revision = attempt.Revision
	}
	return public
}
