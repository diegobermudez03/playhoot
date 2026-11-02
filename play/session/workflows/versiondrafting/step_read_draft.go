package versiondrafting

import (
	"context"
	"fmt"
	"sort"
	"time"

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

//go:generate mockgen -package=versiondrafting -destination=step_read_draft_mock_test.go . readDraftRepo,assetLinkSigner

// readDraftRepo is ReadDraft's own narrow persistence contract.
type readDraftRepo interface {
	FindVersion(ctx context.Context, tx *gorm.DB, versionUUID string) (*internalrepo.Version, error)
	FindRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64) (draftsources.Revision, bool, error)
	FindSourceContents(ctx context.Context, tx *gorm.DB, hashes []string) (map[string]string, error)
	FindAssetObjectKeys(ctx context.Context, tx *gorm.DB, hashes []string) (map[string]string, error)
	RecentChangeAttempts(ctx context.Context, tx *gorm.DB, versionID uint, limit int) ([]internalrepo.ChangeAttempt, error)
}

// assetLinkSigner signs the short-lived links a read gives to asset content.
type assetLinkSigner interface {
	PresignGet(ctx context.Context, key string, expiresAt time.Time) (string, error)
}

// draftRead is what one read transaction gathered; links are signed after it.
type draftRead struct {
	snapshot   sessionpkg.DraftSnapshot
	objectKeys map[string]string
}

// ReadDraft returns versionUUID's sources and assets at revision, or at its
// current revision when revision is 0, with source contents when
// withContent, and its latest change attempts. Each asset with content
// carries a short-lived link to it. A revision it lacks is
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
	// One transaction, so the files, contents, assets and attempts are read
	// together.
	read, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (draftRead, error) {
		return m.readDraftInTx(ctx, tx, parsed.String(), revision, withContent)
	})
	if err != nil {
		return sessionpkg.DraftSnapshot{}, err
	}
	expiresAt := m.now().Add(assetLinkTTL).Truncate(time.Second)
	for i, asset := range read.snapshot.Assets {
		if asset.SHA256 == "" {
			continue
		}
		objectKey, found := read.objectKeys[asset.SHA256]
		if !found {
			monitoring.Alert(ctx, "play draft revision binds an asset content that is not stored")
			return sessionpkg.DraftSnapshot{}, fmt.Errorf("asset content %s of %s is not stored", asset.SHA256, asset.Key)
		}
		link, err := m.assetLinks.PresignGet(ctx, objectKey, expiresAt)
		if err != nil {
			return sessionpkg.DraftSnapshot{}, fmt.Errorf("signing asset link: %w", err)
		}
		read.snapshot.Assets[i].URL = link
		read.snapshot.Assets[i].URLExpiresAt = expiresAt
	}
	return read.snapshot, nil
}

// readDraftInTx is ReadDraft's work inside its transaction.
func (m *Manager) readDraftInTx(ctx context.Context, tx *gorm.DB, versionUUID string, revision int64, withContent bool) (draftRead, error) {
	version, err := m.readDraftRepo.FindVersion(ctx, tx, versionUUID)
	if err != nil {
		return draftRead{}, err
	}
	if version == nil {
		return draftRead{}, fmt.Errorf("version %s: %w", versionUUID, sessionpkg.ErrDraftNotFound)
	}
	read := draftRead{snapshot: sessionpkg.DraftSnapshot{VersionUUID: version.UUID, GameUUID: version.GameUUID, Status: version.Status}}
	if version.CurrentRevision != nil {
		read.snapshot.CurrentRevision = *version.CurrentRevision
	}
	if revision == 0 {
		revision = read.snapshot.CurrentRevision
	}
	if revision > read.snapshot.CurrentRevision {
		return draftRead{}, fmt.Errorf("version %s revision %d: %w", versionUUID, revision, sessionpkg.ErrDraftRevisionNotFound)
	}
	read.snapshot.Revision = revision

	if revision > 0 {
		pinned, found, err := m.readDraftRepo.FindRevision(ctx, tx, version.ID, revision)
		if err != nil {
			return draftRead{}, err
		}
		if !found {
			monitoring.Alert(ctx, "play draft version is missing a revision up to its current one")
			return draftRead{}, fmt.Errorf("version %s has no revision %d", versionUUID, revision)
		}
		contents, err := m.readContents(ctx, tx, pinned.Files, withContent)
		if err != nil {
			return draftRead{}, err
		}
		read.snapshot.Files, err = draftFiles(ctx, pinned.Files, contents, withContent)
		if err != nil {
			return draftRead{}, err
		}
		read.snapshot.Assets, read.snapshot.AssetDiagnostics = draftAssets(pinned, contents)
		read.objectKeys, err = m.readDraftRepo.FindAssetObjectKeys(ctx, tx, boundHashes(pinned.Assets))
		if err != nil {
			return draftRead{}, err
		}
	}

	attempts, err := m.readDraftRepo.RecentChangeAttempts(ctx, tx, version.ID, maxRecentAttempts)
	if err != nil {
		return draftRead{}, err
	}
	for _, attempt := range attempts {
		read.snapshot.Attempts = append(read.snapshot.Attempts, publicAttempt(attempt))
	}
	return read, nil
}

// readContents reads the contents a read needs: every file's when asked, and
// always the project file's, which declares the assets.
func (m *Manager) readContents(ctx context.Context, tx *gorm.DB, files map[string]draftsources.File, withContent bool) (map[string]string, error) {
	var hashes []string
	for filePath, file := range files {
		if withContent || filePath == draftsources.ProjectFile {
			hashes = append(hashes, file.SHA256)
		}
	}
	return m.readDraftRepo.FindSourceContents(ctx, tx, hashes)
}

// draftFiles is a revision's files sorted by path, with their contents when
// asked.
func draftFiles(ctx context.Context, files map[string]draftsources.File, contents map[string]string, withContent bool) ([]sessionpkg.DraftFile, error) {
	paths := make([]string, 0, len(files))
	for filePath := range files {
		paths = append(paths, filePath)
	}
	sort.Strings(paths)
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

// draftAssets is the revision's assets as the project declares them and as
// they are bound; an unreadable declaration declares none and says why.
func draftAssets(revision draftsources.Revision, contents map[string]string) ([]sessionpkg.DraftAsset, []sessionpkg.DraftDiagnostic) {
	var declared map[string]draftsources.AssetDeclaration
	var diagnostics []draftsources.Diagnostic
	if project, found := revision.Files[draftsources.ProjectFile]; found {
		declared, diagnostics = draftsources.DeclaredAssets(contents[project.SHA256])
	}
	states := draftsources.AssetStates(declared, revision.Assets)
	assets := make([]sessionpkg.DraftAsset, len(states))
	for i, state := range states {
		assets[i] = publicAsset(state)
	}
	return assets, changeattempt.Public(diagnostics)
}

// publicAsset is one asset state in its public form, without its link.
func publicAsset(state draftsources.AssetState) sessionpkg.DraftAsset {
	asset := sessionpkg.DraftAsset{Key: state.Key, Status: sessionpkg.DraftAssetStatus(state.Status)}
	if state.Declaration != nil {
		asset.Type = string(state.Declaration.Type)
		asset.Description = state.Declaration.Description
		asset.Optional = state.Declaration.Optional
	}
	if state.Binding != nil {
		asset.SHA256 = state.Binding.SHA256
		asset.Size = state.Binding.Size
		asset.MIMEType = state.Binding.MIMEType
	}
	return asset
}

// boundHashes is the content hash of every bound asset.
func boundHashes(bindings map[string]draftsources.AssetBinding) []string {
	hashes := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		hashes = append(hashes, binding.SHA256)
	}
	return hashes
}

// publicAttempt is a completed attempt in its public form; its revision is
// shown only when it changed the revision or deliberately kept it.
func publicAttempt(attempt internalrepo.ChangeAttempt) sessionpkg.DraftChangeAttempt {
	public := sessionpkg.DraftChangeAttempt{
		Kind:         sessionpkg.DraftAttemptKind(attempt.Kind),
		BaseRevision: attempt.BaseRevision,
		Origin:       attempt.Origin,
		Diagnostics:  changeattempt.Public(attempt.Diagnostics),
		CreatedAt:    attempt.CreatedAt,
	}
	if attempt.Outcome != nil {
		public.Outcome = *attempt.Outcome
	}
	switch public.Outcome {
	case string(sessionpkg.DraftChangeApplied), string(sessionpkg.DraftChangeUnchanged),
		string(sessionpkg.DraftAssetAttached), string(sessionpkg.DraftAssetRemoved):
		public.Revision = attempt.Revision
	}
	return public
}
