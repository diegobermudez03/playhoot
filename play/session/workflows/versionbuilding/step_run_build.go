package versionbuilding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	"github.com/diegobermudez03/playhoot/play/runtime/sandbox/scriptcheck"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/build"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/manifest"
	"github.com/diegobermudez03/playhoot/play/session/internal/gamebuild"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versionbuilding/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=versionbuilding -destination=step_run_build_mock_test.go . runBuildRepo,artifactStorage

// runBuildRepo is RunNextBuild's own narrow persistence contract.
type runBuildRepo interface {
	ClaimOldest(ctx context.Context, tx *gorm.DB, from string, to string) (*internalrepo.ClaimedBuild, error)
	FindRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64) (draftsources.Revision, bool, error)
	FindSourceContents(ctx context.Context, tx *gorm.DB, hashes []string) (map[string]string, error)
	FindReusable(ctx context.Context, tx *gorm.DB, versionID uint, inputsSHA256 string, status string) (*internalrepo.ReusableBuild, error)
	RegisterArtifactObject(ctx context.Context, tx *gorm.DB, sha256 string, size int64, contentType string, objectKey string) error
	FinishBuild(ctx context.Context, tx *gorm.DB, buildID uint, from string, to string, artifacts internalrepo.Artifacts, finishedAt time.Time) (bool, error)
}

// artifactStorage stores built content.
type artifactStorage interface {
	Put(ctx context.Context, key string, content []byte, contentType string) error
}

// Content types of the stored artifacts.
const (
	serverContentType = "text/javascript"
	clientContentType = "text/html"
)

var (
	buildOutcomeLabel = metrics.NewLabel("outcome", "succeeded", "reused", "failed", "infrastructure")

	draftBuildsTotal = metrics.NewCounter(
		"play_draft_builds_total",
		"Finished DRAFT builds: built, reused from a build with the same inputs, failed on the game's code, or failed on the platform (infrastructure).",
		buildOutcomeLabel,
	)
	draftBuildDurationSeconds = metrics.NewHistogram(
		"play_draft_build_duration_seconds",
		"Time from a DRAFT build starting to its end.",
		[]float64{0.1, 0.5, 1, 2, 5, 10, 20, 40, 60},
		buildOutcomeLabel,
	)
	draftBuildQueueWaitSeconds = metrics.NewHistogram(
		"play_draft_build_queue_wait_seconds",
		"Time a DRAFT build waited queued before it started.",
		[]float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 300},
	)
)

// buildEnd is what running one build decided.
type buildEnd struct {
	status    build.Status
	outcome   string
	artifacts internalrepo.Artifacts
}

// RunNextBuild runs the oldest queued build, if any, to its end, and reports
// whether there was one. Every end is recorded: a failure of the game's code
// with its diagnostics, a failure of the platform as infrastructure.
func (m *Manager) RunNextBuild(ctx context.Context) (bool, error) {
	claimed, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (*internalrepo.ClaimedBuild, error) {
		return m.runBuildRepo.ClaimOldest(ctx, tx, string(build.Queued), string(build.Running))
	})
	if err != nil || claimed == nil {
		return false, err
	}
	defer logging.Step(ctx, "versionbuilding.run_build").Close()
	started := m.now()
	draftBuildQueueWaitSeconds.ObserveDuration(ctx, started.Sub(claimed.CreatedAt))
	logging.LogFields(ctx,
		logging.Field("build_uuid", claimed.UUID),
		logging.Field("version_uuid", claimed.VersionUUID),
		logging.Field("revision", claimed.Revision),
	)

	end, err := m.runBuild(ctx, claimed)
	if err != nil {
		// Recorded as the platform's failure, so the build never stays
		// running; the author can apply again.
		logging.LogError(ctx, err)
		end = infrastructureEnd(build.CodeInternal, "the build failed on the platform; this is not the game's problem, apply a change to build again", internalrepo.Artifacts{})
	}
	finished, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (bool, error) {
		return m.runBuildRepo.FinishBuild(ctx, tx, claimed.ID, string(build.Running), string(end.status), end.artifacts, m.now())
	})
	if err != nil {
		return true, err
	}
	if !finished {
		// Requeued meanwhile: whoever runs it again records its end.
		logging.LogFields(ctx, logging.Field("build_finish_skipped", true))
		return true, nil
	}
	draftBuildsTotal.Inc(ctx, buildOutcomeLabel.Value(end.outcome))
	draftBuildDurationSeconds.ObserveDuration(ctx, m.now().Sub(started), buildOutcomeLabel.Value(end.outcome))
	logging.LogFields(ctx, logging.Field("build_status", string(end.status)), logging.Field("build_outcome", end.outcome))
	return true, nil
}

// runBuild decides a claimed build's end; an error is unexpected (the
// database failing, broken data), every expected failure is an end.
func (m *Manager) runBuild(ctx context.Context, claimed *internalrepo.ClaimedBuild) (buildEnd, error) {
	db := m.dbServicer.GetDB()
	revision, found, err := m.runBuildRepo.FindRevision(ctx, db, claimed.GameVersionID, claimed.Revision)
	if err != nil {
		return buildEnd{}, err
	}
	if !found {
		monitoring.Alert(ctx, "play build refers to a revision its version lacks")
		return buildEnd{}, fmt.Errorf("version %s has no revision %d", claimed.VersionUUID, claimed.Revision)
	}
	toolchain, err := m.builder.Version(ctx)
	if err != nil {
		return infrastructureEnd(build.CodeToolchainUnavailable, "the game toolchain is not available on this server: "+err.Error(), internalrepo.Artifacts{}), nil
	}
	inputs := build.InputsSHA256(revision, toolchain)
	decided := internalrepo.Artifacts{InputsSHA256: inputs, ToolchainVersion: toolchain}

	reusable, err := m.runBuildRepo.FindReusable(ctx, db, claimed.GameVersionID, inputs, string(build.Succeeded))
	if err != nil {
		return buildEnd{}, err
	}
	if reusable != nil {
		decided.ServerObjectKey, decided.ServerSHA256, decided.ServerSize = &reusable.ServerObjectKey, &reusable.ServerSHA256, &reusable.ServerSize
		decided.ClientObjectKey, decided.ClientSHA256, decided.ClientSize = &reusable.ClientObjectKey, &reusable.ClientSHA256, &reusable.ClientSize
		decided.Manifest, decided.AspectRatio, decided.ReusedBuildID = reusable.Manifest, &reusable.AspectRatio, &reusable.ID
		return buildEnd{status: build.Succeeded, outcome: "reused", artifacts: decided}, nil
	}

	contents, err := m.runBuildRepo.FindSourceContents(ctx, db, revisionHashes(revision))
	if err != nil {
		return buildEnd{}, err
	}
	output, err := m.buildSources(ctx, revision, contents)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return infrastructureEnd(build.CodeTimeout, fmt.Sprintf("the build took longer than %s; try again, and keep the project small", gamebuild.DefaultTimeout), decided), nil
	case errors.Is(err, gamebuild.ErrUnavailable):
		return infrastructureEnd(build.CodeToolchainUnavailable, "the game toolchain is not available on this server: "+err.Error(), decided), nil
	case err != nil:
		logging.LogError(ctx, err)
		return infrastructureEnd(build.CodeToolchainFailed, "the game toolchain failed; this is the platform's problem, not the game's", decided), nil
	}
	if !output.OK {
		return failedEnd(output.Diagnostics, decided)
	}
	if problems := checkOutput(output); len(problems) > 0 {
		return failedEnd(problems, decided)
	}
	if err := m.storeArtifacts(ctx, output, &decided); err != nil {
		logging.LogError(ctx, err)
		return infrastructureEnd(build.CodeStorageFailed, "storing the built game failed; this is the platform's problem, not the game's", decided), nil
	}
	return buildEnd{status: build.Succeeded, outcome: "succeeded", artifacts: decided}, nil
}

// buildSources writes the revision's sources into a directory of their own
// and builds them there.
func (m *Manager) buildSources(ctx context.Context, revision draftsources.Revision, contents map[string]string) (gamebuild.Output, error) {
	dir, err := os.MkdirTemp("", "playhoot-build-")
	if err != nil {
		return gamebuild.Output{}, fmt.Errorf("creating build directory: %w", err)
	}
	defer os.RemoveAll(dir)
	for path, file := range revision.Files {
		content, found := contents[file.SHA256]
		if !found {
			return gamebuild.Output{}, fmt.Errorf("source content %s of %s is not stored", file.SHA256, path)
		}
		// Paths were checked when the change was applied; a stored path
		// that escapes the directory is broken data.
		target := filepath.Join(dir, filepath.FromSlash(path))
		if !strings.HasPrefix(target, dir+string(filepath.Separator)) {
			return gamebuild.Output{}, fmt.Errorf("source path %q leaves the project", path)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return gamebuild.Output{}, fmt.Errorf("creating source folder: %w", err)
		}
		if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
			return gamebuild.Output{}, fmt.Errorf("writing source: %w", err)
		}
	}
	return m.builder.Build(ctx, gamebuild.Input{Dir: dir})
}

// checkOutput is what the runtime would refuse of a built game: a script
// outside its compile profile, a manifest it cannot parse, a viewport it
// does not offer.
func checkOutput(output gamebuild.Output) []build.Diagnostic {
	var problems []build.Diagnostic
	for _, violation := range scriptcheck.Validate(scriptcheck.ProfileSDKV1, output.Server) {
		problems = append(problems, build.Diagnostic{
			Stage: build.StageSandbox, Code: violation.Rule, File: "server.js", Line: violation.Line, Column: violation.Column,
			Message: violation.Message,
		})
	}
	if _, err := manifest.Parse(output.Manifest); err != nil {
		problems = append(problems, build.Diagnostic{Stage: build.StageManifest, File: "server/game.ts", Message: err.Error()})
	}
	if _, ok := gameversion.AspectRatio(output.AspectRatio).Resolution(); !ok {
		problems = append(problems, build.Diagnostic{Stage: build.StageManifest, File: "game.project.json", Message: fmt.Sprintf("aspect ratio %q is not one Playhoot offers", output.AspectRatio)})
	}
	return problems
}

// storeArtifacts stores the built script and page content-addressed, each
// registered before it is stored, and records where in decided.
func (m *Manager) storeArtifacts(ctx context.Context, output gamebuild.Output, decided *internalrepo.Artifacts) error {
	server := objectstore.LocatorFor(backendScriptPrefix, output.Server)
	client := objectstore.LocatorFor(frontendPackagePrefix, output.Client)
	if _, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (struct{}, error) {
		if err := m.runBuildRepo.RegisterArtifactObject(ctx, tx, server.SHA256, server.Size, serverContentType, server.ObjectKey); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, m.runBuildRepo.RegisterArtifactObject(ctx, tx, client.SHA256, client.Size, clientContentType, client.ObjectKey)
	}); err != nil {
		return err
	}
	if err := m.artifacts.Put(ctx, server.ObjectKey, output.Server, serverContentType); err != nil {
		return fmt.Errorf("storing server.js: %w", err)
	}
	if err := m.artifacts.Put(ctx, client.ObjectKey, output.Client, clientContentType); err != nil {
		return fmt.Errorf("storing client.html: %w", err)
	}
	aspectRatio := output.AspectRatio
	decided.ServerObjectKey, decided.ServerSHA256, decided.ServerSize = &server.ObjectKey, &server.SHA256, &server.Size
	decided.ClientObjectKey, decided.ClientSHA256, decided.ClientSize = &client.ObjectKey, &client.SHA256, &client.Size
	decided.Manifest, decided.AspectRatio = output.Manifest, &aspectRatio
	return nil
}

func failedEnd(diagnostics []build.Diagnostic, decided internalrepo.Artifacts) (buildEnd, error) {
	encoded, err := json.Marshal(diagnostics)
	if err != nil {
		return buildEnd{}, fmt.Errorf("encoding build diagnostics: %w", err)
	}
	decided.Diagnostics = encoded
	return buildEnd{status: build.Failed, outcome: "failed", artifacts: decided}, nil
}

func infrastructureEnd(code string, message string, decided internalrepo.Artifacts) buildEnd {
	encoded, _ := json.Marshal([]build.Diagnostic{{Stage: build.StageInfrastructure, Code: code, Message: message}})
	decided.Diagnostics = encoded
	return buildEnd{status: build.Failed, outcome: "infrastructure", artifacts: decided}
}

func revisionHashes(revision draftsources.Revision) []string {
	hashes := make([]string, 0, len(revision.Files))
	for _, file := range revision.Files {
		hashes = append(hashes, file.SHA256)
	}
	sort.Strings(hashes)
	return hashes
}
