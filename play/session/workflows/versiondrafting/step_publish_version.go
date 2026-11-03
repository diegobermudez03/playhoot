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
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/play/internal/business/inputrequirements"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox/scriptcheck"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/build"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=versiondrafting -destination=step_publish_version_mock_test.go . publishVersionRepo

// publishVersionRepo is CheckPublication's and FreezeVersion's own narrow
// persistence contract.
type publishVersionRepo interface {
	revisionReader
	FindVersion(ctx context.Context, tx *gorm.DB, versionUUID string) (*internalrepo.Version, error)
	FindBuiltArtifacts(ctx context.Context, tx *gorm.DB, versionUUID string, revision int64) (*internalrepo.BuiltArtifacts, error)
	FindAssetObjectKeys(ctx context.Context, tx *gorm.DB, hashes []string) (map[string]string, error)
	LockGame(ctx context.Context, tx *gorm.DB, gameUUID string) (uint, error)
	FreezeVersion(ctx context.Context, tx *gorm.DB, versionID uint, from string, to string, frozen internalrepo.FrozenArtifacts, at time.Time) (bool, error)
	CreateVersionAssets(ctx context.Context, tx *gorm.DB, versionID uint, assets []internalrepo.VersionAsset, at time.Time) error
	CreateInputRequirements(ctx context.Context, tx *gorm.DB, versionID uint, requirements internalrepo.InputRequirements, at time.Time) error
	SetCurrentVersion(ctx context.Context, tx *gorm.DB, gameID uint, versionID uint, at time.Time) error
}

var (
	freezeOutcomeLabel = metrics.NewLabel("outcome", "frozen", "already_frozen", "not_ready")

	versionsFrozenTotal = metrics.NewCounter(
		"play_versions_frozen_total",
		"Requests to freeze a DRAFT into an executable version, by outcome.",
		freezeOutcomeLabel,
	)
)

// sdkInputRequirements is what an sdk-v1 game asks of the device: its client
// is a page of buttons and taps, so pointer press, on desktop and mobile.
func sdkInputRequirements() inputrequirements.Requirements {
	return inputrequirements.Requirements{
		UsesPointerPress: true,
		Devices:          inputrequirements.DevicesFor(false, false, true),
	}
}

// CheckPublication tells whether the DRAFT versionUUID's current revision
// may become an executable version, and why not when it may not.
func (m *Manager) CheckPublication(ctx context.Context, versionUUID string) (sessionpkg.PublishReadiness, error) {
	defer logging.Step(ctx, "versiondrafting.check_publication").Close()

	parsed, err := uuid.Parse(versionUUID)
	if err != nil {
		return sessionpkg.PublishReadiness{}, fmt.Errorf("version uuid is not a valid UUID")
	}
	db := m.dbServicer.GetDB()
	version, err := m.publishVersionRepo.FindVersion(ctx, db, parsed.String())
	if err != nil {
		return sessionpkg.PublishReadiness{}, err
	}
	if version == nil {
		return sessionpkg.PublishReadiness{}, fmt.Errorf("version %s: %w", versionUUID, sessionpkg.ErrDraftNotFound)
	}
	if gameversion.Status(version.Status) != gameversion.Draft || version.CurrentRevision == nil {
		return sessionpkg.PublishReadiness{Outcome: sessionpkg.PublishNotEditable}, nil
	}
	checked, err := m.checkRevision(ctx, db, version, *version.CurrentRevision)
	if err != nil {
		return sessionpkg.PublishReadiness{}, err
	}
	outcome := sessionpkg.PublishReady
	if len(checked.blockers) > 0 {
		outcome = sessionpkg.PublishNotReady
	}
	logging.LogFields(ctx, logging.Field("publication_readiness", string(outcome)), logging.Field("revision", checked.revision))
	return sessionpkg.PublishReadiness{Outcome: outcome, Revision: checked.revision, Blockers: checked.blockers}, nil
}

// FreezeVersion turns the DRAFT versionUUID of gameUUID, at revision, into an
// executable version, immutable from now on, with its build's artifacts, its
// assets and its input requirements, and makes it the game's current
// version, in one transaction. Freezing a version again answers the same.
func (m *Manager) FreezeVersion(ctx context.Context, gameUUID string, versionUUID string, revision int64) (sessionpkg.FrozenVersion, error) {
	defer logging.Step(ctx, "versiondrafting.freeze_version").Close()

	parsedGame, err := uuid.Parse(gameUUID)
	if err != nil {
		return sessionpkg.FrozenVersion{}, fmt.Errorf("game uuid is not a valid UUID")
	}
	parsedVersion, err := uuid.Parse(versionUUID)
	if err != nil {
		return sessionpkg.FrozenVersion{}, fmt.Errorf("version uuid is not a valid UUID")
	}
	frozen, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (sessionpkg.FrozenVersion, error) {
		return m.freezeInTx(ctx, tx, parsedGame.String(), parsedVersion.String(), revision)
	})
	if err != nil {
		return sessionpkg.FrozenVersion{}, err
	}
	versionsFrozenTotal.Inc(ctx, freezeOutcomeLabel.Value(string(frozen.Outcome)))
	logging.LogFields(ctx, logging.Field("freeze_outcome", string(frozen.Outcome)), logging.Field("revision", frozen.Revision))
	return frozen, nil
}

func (m *Manager) freezeInTx(ctx context.Context, tx *gorm.DB, gameUUID string, versionUUID string, revision int64) (sessionpkg.FrozenVersion, error) {
	gameID, err := m.publishVersionRepo.LockGame(ctx, tx, gameUUID)
	if err != nil {
		return sessionpkg.FrozenVersion{}, err
	}
	version, err := m.publishVersionRepo.FindVersion(ctx, tx, versionUUID)
	if err != nil {
		return sessionpkg.FrozenVersion{}, err
	}
	if version == nil {
		return sessionpkg.FrozenVersion{}, fmt.Errorf("version %s: %w", versionUUID, sessionpkg.ErrDraftNotFound)
	}
	if version.GameUUID != gameUUID {
		return sessionpkg.FrozenVersion{}, fmt.Errorf("version %s: %w", versionUUID, sessionpkg.ErrDraftVersionConflict)
	}
	var current int64
	if version.CurrentRevision != nil {
		current = *version.CurrentRevision
	}
	switch gameversion.Status(version.Status) {
	case gameversion.Executable:
		return sessionpkg.FrozenVersion{Outcome: sessionpkg.FreezeAlreadyFrozen, Revision: current}, nil
	case gameversion.Draft:
	default:
		monitoring.Alert(ctx, "play version has an unknown status")
		return sessionpkg.FrozenVersion{}, fmt.Errorf("version %s has unknown status %q", versionUUID, version.Status)
	}
	if version.CurrentRevision == nil {
		monitoring.Alert(ctx, "play draft version has no current revision")
		return sessionpkg.FrozenVersion{}, fmt.Errorf("draft version %s has no current revision", versionUUID)
	}
	if current != revision {
		return sessionpkg.FrozenVersion{Outcome: sessionpkg.FreezeNotReady, Revision: current, Blockers: []sessionpkg.PublishBlocker{{Code: sessionpkg.PublishRevisionChanged}}}, nil
	}
	checked, err := m.checkRevision(ctx, tx, version, current)
	if err != nil {
		return sessionpkg.FrozenVersion{}, err
	}
	if len(checked.blockers) > 0 {
		return sessionpkg.FrozenVersion{Outcome: sessionpkg.FreezeNotReady, Revision: current, Blockers: checked.blockers}, nil
	}

	at := m.now().UTC().Truncate(time.Microsecond)
	artifacts, err := frozenArtifacts(ctx, versionUUID, checked.built)
	if err != nil {
		return sessionpkg.FrozenVersion{}, err
	}
	froze, err := m.publishVersionRepo.FreezeVersion(ctx, tx, version.ID, string(gameversion.Draft), string(gameversion.Executable), artifacts, at)
	if err != nil {
		return sessionpkg.FrozenVersion{}, err
	}
	if !froze {
		return sessionpkg.FrozenVersion{}, fmt.Errorf("version %s stopped being a DRAFT while locked", versionUUID)
	}
	assets, err := m.versionAssets(ctx, tx, checked)
	if err != nil {
		return sessionpkg.FrozenVersion{}, err
	}
	if err := m.publishVersionRepo.CreateVersionAssets(ctx, tx, version.ID, assets, at); err != nil {
		return sessionpkg.FrozenVersion{}, err
	}
	requirements, err := versionInputRequirements(checked.built.ScriptProfile)
	if err != nil {
		return sessionpkg.FrozenVersion{}, err
	}
	if err := m.publishVersionRepo.CreateInputRequirements(ctx, tx, version.ID, requirements, at); err != nil {
		return sessionpkg.FrozenVersion{}, err
	}
	if err := m.publishVersionRepo.SetCurrentVersion(ctx, tx, gameID, version.ID, at); err != nil {
		return sessionpkg.FrozenVersion{}, err
	}
	return sessionpkg.FrozenVersion{Outcome: sessionpkg.FreezeFrozen, Revision: current}, nil
}

// checkedRevision is what checking a revision for publication found.
type checkedRevision struct {
	revision int64
	built    *internalrepo.BuiltArtifacts
	files    draftsources.Revision
	declared map[string]draftsources.AssetDeclaration
	blockers []sessionpkg.PublishBlocker
}

// checkRevision finds why revision of version may not be published: no
// succeeded build of exactly it, or required assets without content.
func (m *Manager) checkRevision(ctx context.Context, tx *gorm.DB, version *internalrepo.Version, revision int64) (checkedRevision, error) {
	checked := checkedRevision{revision: revision}
	built, err := m.publishVersionRepo.FindBuiltArtifacts(ctx, tx, version.UUID, revision)
	if err != nil {
		return checkedRevision{}, err
	}
	switch {
	case built == nil:
		checked.blockers = append(checked.blockers, sessionpkg.PublishBlocker{Code: sessionpkg.PublishBuildMissing})
	case build.Status(built.Status) != build.Succeeded:
		checked.blockers = append(checked.blockers, sessionpkg.PublishBlocker{Code: sessionpkg.PublishBuildNotSucceeded, BuildStatus: sessionpkg.DraftBuildStatus(built.Status)})
	default:
		checked.built = built
	}
	files, found, err := m.publishVersionRepo.FindRevision(ctx, tx, version.ID, revision)
	if err != nil {
		return checkedRevision{}, err
	}
	if !found {
		monitoring.Alert(ctx, "play draft version is missing its current revision")
		return checkedRevision{}, fmt.Errorf("version %s has no revision %d", version.UUID, revision)
	}
	checked.files = files
	checked.declared, err = declarationsOf(ctx, tx, m.publishVersionRepo, files)
	if err != nil {
		return checkedRevision{}, err
	}
	var missing []string
	for key, declaration := range checked.declared {
		if _, bound := files.Assets[key]; !bound && !declaration.Optional {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		checked.blockers = append(checked.blockers, sessionpkg.PublishBlocker{Code: sessionpkg.PublishMissingRequiredAssets, Keys: missing})
	}
	return checked, nil
}

// frozenArtifacts is what the version keeps from its succeeded build; a
// succeeded build without them is broken data.
func frozenArtifacts(ctx context.Context, versionUUID string, built *internalrepo.BuiltArtifacts) (internalrepo.FrozenArtifacts, error) {
	if built.ServerObjectKey == nil || built.ServerSHA256 == nil || built.ServerSize == nil ||
		built.ClientObjectKey == nil || built.ClientSHA256 == nil || built.ClientSize == nil ||
		built.AspectRatio == nil || len(built.Manifest) == 0 {
		monitoring.Alert(ctx, "play draft build succeeded without its artifacts")
		return internalrepo.FrozenArtifacts{}, fmt.Errorf("version %s revision %d: a succeeded build is missing artifacts", versionUUID, built.Revision)
	}
	return internalrepo.FrozenArtifacts{
		Manifest:        built.Manifest,
		ScriptObjectKey: *built.ServerObjectKey,
		ScriptSHA256:    *built.ServerSHA256,
		ScriptSize:      *built.ServerSize,
		FrontObjectKey:  *built.ClientObjectKey,
		FrontSHA256:     *built.ClientSHA256,
		FrontSize:       *built.ClientSize,
		AspectRatio:     *built.AspectRatio,
	}, nil
}

// versionAssets is the assets the frozen version is delivered with: every
// declared key the revision binds content to. A binding whose declaration
// was removed is left behind.
func (m *Manager) versionAssets(ctx context.Context, tx *gorm.DB, checked checkedRevision) ([]internalrepo.VersionAsset, error) {
	keys := make([]string, 0, len(checked.files.Assets))
	hashes := make([]string, 0, len(checked.files.Assets))
	for key, binding := range checked.files.Assets {
		if _, declared := checked.declared[key]; declared {
			keys = append(keys, key)
			hashes = append(hashes, binding.SHA256)
		}
	}
	sort.Strings(keys)
	objectKeys, err := m.publishVersionRepo.FindAssetObjectKeys(ctx, tx, hashes)
	if err != nil {
		return nil, err
	}
	assets := make([]internalrepo.VersionAsset, 0, len(keys))
	for _, key := range keys {
		binding := checked.files.Assets[key]
		objectKey, stored := objectKeys[binding.SHA256]
		if !stored {
			monitoring.Alert(ctx, "play draft revision binds an asset that is not registered")
			return nil, fmt.Errorf("asset %s binds %s, which is not registered", key, binding.SHA256)
		}
		assets = append(assets, internalrepo.VersionAsset{
			Key: key, Type: string(checked.declared[key].Type), MIMEType: binding.MIMEType,
			ObjectKey: objectKey, SHA256: binding.SHA256, Size: binding.Size,
		})
	}
	return assets, nil
}

// versionInputRequirements is what a frozen version asks of the device, by
// the profile its script was built with.
func versionInputRequirements(profile string) (internalrepo.InputRequirements, error) {
	if scriptcheck.Profile(profile) != scriptcheck.ProfileSDKV1 {
		return internalrepo.InputRequirements{}, fmt.Errorf("only sdk-v1 drafts are published, not %q", profile)
	}
	requirements := sdkInputRequirements()
	if err := requirements.Validate(); err != nil {
		return internalrepo.InputRequirements{}, err
	}
	return internalrepo.InputRequirements{
		UsesKeyboard:     requirements.UsesKeyboard,
		UsesPointerHover: requirements.UsesPointerHover,
		UsesPointerPress: requirements.UsesPointerPress,
		Devices:          string(requirements.Devices),
	}, nil
}
