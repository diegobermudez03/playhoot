package versiondrafting

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"syscall"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/diegobermudez03/playhoot/logging"
	"github.com/diegobermudez03/playhoot/monitoring"
	"github.com/diegobermudez03/playhoot/monitoring/metrics"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/draftsources"
	"github.com/diegobermudez03/playhoot/play/session/internal/business/gameversion"
	"github.com/diegobermudez03/playhoot/play/session/internal/objectstore"
	"github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/changeattempt"
	internalrepo "github.com/diegobermudez03/playhoot/play/session/workflows/versiondrafting/internal/repo"
	"github.com/diegobermudez03/playhoot/utils"
)

//go:generate mockgen -package=versiondrafting -destination=step_upload_asset_mock_test.go . uploadAssetRepo,assetStorage

// uploadAssetRepo is UploadAsset's own narrow persistence contract.
type uploadAssetRepo interface {
	FindVersion(ctx context.Context, tx *gorm.DB, versionUUID string) (*internalrepo.Version, error)
	FindChangeAttempt(ctx context.Context, tx *gorm.DB, versionID uint, idempotencyKey string) (*internalrepo.ChangeAttempt, error)
	ClaimChangeAttempt(ctx context.Context, tx *gorm.DB, versionID uint, kind string, idempotencyKey string, requestSHA256 string, baseRevision int64, origin string) (uint, *internalrepo.ChangeAttempt, error)
	FindRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64) (draftsources.Revision, bool, error)
	FindSourceContents(ctx context.Context, tx *gorm.DB, hashes []string) (map[string]string, error)
	RegisterAssetObject(ctx context.Context, tx *gorm.DB, sha256 string, size int64, mimeType string, objectKey string, registeredAt time.Time) error
	AdvanceRevision(ctx context.Context, tx *gorm.DB, versionID uint, status string, base int64) (bool, error)
	CreateRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64, revision draftsources.Revision, source string) error
	QueueBuild(ctx context.Context, tx *gorm.DB, versionID uint, revision int64, buildUUID string, queued string, superseded string) (int, error)
	CompleteChangeAttempt(ctx context.Context, tx *gorm.DB, attemptID uint, outcome string, revision *int64, diagnostics []draftsources.Diagnostic) error
}

// assetStorage stores checked asset content.
type assetStorage interface {
	Put(ctx context.Context, key string, content []byte, contentType string) error
}

// assetRevisionSource is the revision source of a revision that attached or
// removed an asset.
const assetRevisionSource = "asset"

// maxRevisionRaces bounds how many times attaching retries when a change of
// the sources commits a new revision while it decides.
const maxRevisionRaces = 3

// Bounds of the uploads one instance holds in memory at once: at most
// maxConcurrentUploads, each waiting at most uploadSlotWait for its turn.
const (
	maxConcurrentUploads = 4
	uploadSlotWait       = 2 * time.Second
)

// refusedAttemptKeyPrefix marks the attempt recorded for an upload refused
// before its content was known, which never holds the caller's key: a retry
// with that key is decided again.
const refusedAttemptKeyPrefix = "refused-before-content:"

// Diagnostic codes of a refused asset.
const (
	codeAssetEmpty       = "asset_empty"
	codeAssetTooLarge    = "asset_too_large"
	codeWrongContentType = "wrong_content_type"
	codeAssetsTooLarge   = "assets_too_large"
)

var (
	assetOutcomeLabel = metrics.NewLabel("outcome", "attached", "removed", "unchanged", "invalid", "too_large", "not_editable", "not_declared", "replayed")

	draftAssetEditsTotal = metrics.NewCounter(
		"play_draft_asset_edits_total",
		"Uploads and removals of DRAFT assets, by outcome; replayed counts retries answered from an earlier attempt.",
		assetOutcomeLabel,
	)

	uploadInterruptionLabel = metrics.NewLabel("cause", "busy", "read_timeout", "client_gone", "read_failed")

	draftAssetUploadsInterruptedTotal = metrics.NewCounter(
		"play_draft_asset_uploads_interrupted_total",
		"DRAFT asset uploads lost before a decision: no upload slot in time (busy), the request's read deadline (read_timeout), the client closing the upload (client_gone) or another read failure.",
		uploadInterruptionLabel,
	)

	draftAssetUploadSlotWaitSeconds = metrics.NewHistogram(
		"play_draft_asset_upload_slot_wait_seconds",
		"Time a DRAFT asset upload waited for a slot to hold its content in memory.",
		[]float64{0.01, 0.05, 0.1, 0.25, 0.5, 1, 2},
	)

	draftAssetUploadReadSeconds = metrics.NewHistogram(
		"play_draft_asset_upload_read_seconds",
		"Time spent receiving a DRAFT asset upload's content, whether it completed or not.",
		[]float64{0.1, 0.5, 1, 5, 15, 30, 60, 120, 300},
	)
)

// uploadRequest is what makes two uploads the same request: the key and the
// content, by its hash.
type uploadRequest struct {
	Kind   string `json:"kind"`
	Key    string `json:"key"`
	SHA256 string `json:"sha256"`
}

// uploadLookup is what an upload finds before reading any byte: the version,
// the attempt already holding the key, and the key's declared type, or the
// outcome refusing the upload without reading it.
type uploadLookup struct {
	version     *internalrepo.Version
	existing    *internalrepo.ChangeAttempt
	declaration *draftsources.AssetDeclaration
	refusal     sessionpkg.DraftAssetOutcome
}

// receivedUpload is what reading the content decided before any write: the
// content and its binding, or the outcome refusing it.
type receivedUpload struct {
	key         string
	content     []byte
	refusal     sessionpkg.DraftAssetOutcome
	diagnostics []draftsources.Diagnostic
	binding     draftsources.AssetBinding
}

// UploadAsset receives the content of the asset key the DRAFT versionUUID
// declares, cut at the declared type's limit and hashed while read, and
// binds it in a new revision made on the current one. Bytes are stored
// before any reference to them, so no reference can point to nothing.
func (m *Manager) UploadAsset(ctx context.Context, versionUUID string, key string, content io.Reader, idempotencyKey string, origin string) (sessionpkg.DraftAssetResult, error) {
	defer logging.Step(ctx, "versiondrafting.upload_asset").Close()

	parsed, err := uuid.Parse(versionUUID)
	if err != nil {
		return sessionpkg.DraftAssetResult{}, fmt.Errorf("version uuid is not a valid UUID")
	}
	if idempotencyKey == "" {
		return sessionpkg.DraftAssetResult{}, fmt.Errorf("idempotency key is required")
	}
	if !changeattempt.ValidOrigin(origin) {
		return sessionpkg.DraftAssetResult{}, fmt.Errorf("unknown change origin %q", origin)
	}

	// Nothing is read before this lookup, so a version Play does not have
	// yet can be created and the upload sent again with the same content.
	found, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (uploadLookup, error) {
		return m.lookUpUpload(ctx, tx, parsed.String(), key, idempotencyKey)
	})
	if err != nil {
		return sessionpkg.DraftAssetResult{}, err
	}
	result, err := m.upload(ctx, found, key, content, idempotencyKey, origin)
	if err != nil {
		return sessionpkg.DraftAssetResult{}, err
	}
	countAssetEdit(ctx, result)
	if result.Outcome == sessionpkg.DraftAssetAttached || result.Outcome == sessionpkg.DraftAssetRemoved {
		if result.Build, err = m.builtRevision(ctx, parsed.String(), result.Revision, result.Replayed); err != nil {
			return sessionpkg.DraftAssetResult{}, err
		}
	}
	return result, nil
}

// upload decides an upload the lookup did not refuse at once, reading its
// content within its slot.
func (m *Manager) upload(ctx context.Context, found uploadLookup, key string, content io.Reader, idempotencyKey string, origin string) (sessionpkg.DraftAssetResult, error) {
	if found.existing == nil && found.refusal != "" {
		return m.recordRefusal(ctx, found.version, key, found.refusal, nil, origin)
	}
	release, err := m.takeUploadSlot(ctx)
	if err != nil {
		return sessionpkg.DraftAssetResult{}, err
	}
	defer release()

	var limit int64 = sessionpkg.MaxDraftAssetBytes
	if found.declaration != nil {
		limit = gameversion.MaxAssetBytes(found.declaration.Type)
	}
	began := m.now()
	received, err := receive(key, found.declaration, content, limit)
	draftAssetUploadReadSeconds.ObserveDuration(ctx, m.now().Sub(began))
	if err != nil {
		draftAssetUploadsInterruptedTotal.Inc(ctx, uploadInterruptionLabel.Value(readInterruptionCause(ctx, err)))
		return sessionpkg.DraftAssetResult{}, err
	}
	if found.existing != nil {
		return m.replayUpload(ctx, found.version.ID, found.existing, key, received)
	}
	if received.refusal == sessionpkg.DraftAssetTooLarge {
		return m.recordRefusal(ctx, found.version, key, received.refusal, received.diagnostics, origin)
	}
	fingerprint, err := changeattempt.Fingerprint(uploadRequest{Kind: string(sessionpkg.DraftAttemptAssetUpload), Key: key, SHA256: received.binding.SHA256})
	if err != nil {
		return sessionpkg.DraftAssetResult{}, err
	}
	if received.refusal == "" {
		if err := m.storeAsset(ctx, received); err != nil {
			return sessionpkg.DraftAssetResult{}, err
		}
	}
	done, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (committed[sessionpkg.DraftAssetResult], error) {
		return m.attachInTx(ctx, tx, found.version.UUID, received, idempotencyKey, fingerprint, origin)
	})
	if err != nil {
		return sessionpkg.DraftAssetResult{}, err
	}
	countSuperseded(ctx, done.superseded)
	return done.result, nil
}

// lookUpUpload finds the version, an attempt already holding the key, and
// what the current revision declares for key.
func (m *Manager) lookUpUpload(ctx context.Context, tx *gorm.DB, versionUUID string, key string, idempotencyKey string) (uploadLookup, error) {
	version, err := m.uploadAssetRepo.FindVersion(ctx, tx, versionUUID)
	if err != nil {
		return uploadLookup{}, err
	}
	if version == nil {
		return uploadLookup{}, fmt.Errorf("version %s: %w", versionUUID, sessionpkg.ErrDraftNotFound)
	}
	found := uploadLookup{version: version}
	found.existing, err = m.uploadAssetRepo.FindChangeAttempt(ctx, tx, version.ID, idempotencyKey)
	if err != nil {
		return uploadLookup{}, err
	}
	if gameversion.Status(version.Status) != gameversion.Draft || version.CurrentRevision == nil {
		found.refusal = sessionpkg.DraftAssetNotEditable
		return found, nil
	}
	declared, err := currentDeclarations(ctx, tx, m.uploadAssetRepo, version)
	if err != nil {
		return uploadLookup{}, err
	}
	declaration, isDeclared := declared[key]
	if !isDeclared {
		found.refusal = sessionpkg.DraftAssetNotDeclared
		return found, nil
	}
	found.declaration = &declaration
	return found, nil
}

// takeUploadSlot waits briefly for a turn to hold an upload in memory and
// returns its release, or sessionpkg.ErrUploadsBusy.
func (m *Manager) takeUploadSlot(ctx context.Context) (func(), error) {
	began := m.now()
	timer := time.NewTimer(uploadSlotWait)
	defer timer.Stop()
	defer func() { draftAssetUploadSlotWaitSeconds.ObserveDuration(ctx, m.now().Sub(began)) }()
	select {
	case m.uploadSlots <- struct{}{}:
		return func() { <-m.uploadSlots }, nil
	case <-timer.C:
		draftAssetUploadsInterruptedTotal.Inc(ctx, uploadInterruptionLabel.Value("busy"))
		return nil, sessionpkg.ErrUploadsBusy
	case <-ctx.Done():
		draftAssetUploadsInterruptedTotal.Inc(ctx, uploadInterruptionLabel.Value("client_gone"))
		return nil, ctx.Err()
	}
}

// readInterruptionCause classifies why receiving an upload's content failed:
// the request's read deadline, the client going away, or anything else.
func readInterruptionCause(ctx context.Context, err error) string {
	var netErr net.Error
	switch {
	case errors.Is(err, os.ErrDeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		return "read_timeout"
	case ctx.Err() != nil, errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, context.Canceled), errors.Is(err, syscall.ECONNRESET):
		return "client_gone"
	default:
		return "read_failed"
	}
}

// receive reads content up to limit, hashing it, and checks what it really
// is against the declaration; declaration is nil only for a retry of a key
// the project no longer declares, which is never attached anew.
func receive(key string, declaration *draftsources.AssetDeclaration, content io.Reader, limit int64) (receivedUpload, error) {
	received := receivedUpload{key: key}
	path := assetPath(key)
	refuse := func(outcome sessionpkg.DraftAssetOutcome, code string, message string) (receivedUpload, error) {
		received.refusal = outcome
		received.diagnostics = []draftsources.Diagnostic{{Path: path, Code: code, Message: message}}
		return received, nil
	}
	hash := sha256.New()
	var buffer bytes.Buffer
	read, err := io.Copy(io.MultiWriter(&buffer, hash), io.LimitReader(content, limit+1))
	if err != nil {
		return receivedUpload{}, fmt.Errorf("reading uploaded asset: %w", err)
	}
	if read > limit {
		return refuse(sessionpkg.DraftAssetTooLarge, codeAssetTooLarge, fmt.Sprintf("the file is larger than %d bytes, the limit for this asset", limit))
	}
	received.content = buffer.Bytes()
	mimeType := gameversion.SniffMIMEType(received.content)
	received.binding = draftsources.AssetBinding{SHA256: hex.EncodeToString(hash.Sum(nil)), Size: read, MIMEType: mimeType}
	if read == 0 {
		return refuse(sessionpkg.DraftAssetInvalid, codeAssetEmpty, "the uploaded file is empty")
	}
	if declaration != nil && !gameversion.AcceptsMIMEType(declaration.Type, mimeType) {
		detected := mimeType
		if detected == "" {
			detected = "an unsupported format"
		}
		return refuse(sessionpkg.DraftAssetInvalid, codeWrongContentType, fmt.Sprintf("the file is %s, not a supported %s (its bytes decide, not its name or header)", detected, declaration.Type))
	}
	return received, nil
}

// storeAsset registers the content in the object registry, then stores it
// content-addressed: a failure between both leaves a registered object that
// nothing references, which its owner's cleanup removes.
func (m *Manager) storeAsset(ctx context.Context, received receivedUpload) error {
	locator := objectstore.LocatorFor(assetObjectPrefix, received.content)
	if _, err := utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (struct{}, error) {
		return struct{}{}, m.uploadAssetRepo.RegisterAssetObject(ctx, tx, locator.SHA256, locator.Size, received.binding.MIMEType, locator.ObjectKey, m.now())
	}); err != nil {
		return err
	}
	if err := m.assetStorage.Put(ctx, locator.ObjectKey, received.content, received.binding.MIMEType); err != nil {
		return fmt.Errorf("storing asset: %w", err)
	}
	return nil
}

// recordRefusal records an upload refused before its content was known and
// answers it. The attempt never holds the caller's key, which stays free.
func (m *Manager) recordRefusal(ctx context.Context, version *internalrepo.Version, key string, refusal sessionpkg.DraftAssetOutcome, diagnostics []draftsources.Diagnostic, origin string) (sessionpkg.DraftAssetResult, error) {
	fingerprint, err := changeattempt.Fingerprint(uploadRequest{Kind: string(sessionpkg.DraftAttemptAssetUpload), Key: key})
	if err != nil {
		return sessionpkg.DraftAssetResult{}, err
	}
	var current int64
	if version.CurrentRevision != nil {
		current = *version.CurrentRevision
	}
	return utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (sessionpkg.DraftAssetResult, error) {
		attemptID, _, err := m.uploadAssetRepo.ClaimChangeAttempt(ctx, tx, version.ID, string(sessionpkg.DraftAttemptAssetUpload), refusedAttemptKeyPrefix+m.newUUID(), fingerprint, current, origin)
		if err != nil {
			return sessionpkg.DraftAssetResult{}, err
		}
		if err := m.uploadAssetRepo.CompleteChangeAttempt(ctx, tx, attemptID, string(refusal), &current, diagnostics); err != nil {
			return sessionpkg.DraftAssetResult{}, err
		}
		return sessionpkg.DraftAssetResult{Outcome: refusal, Revision: current, Diagnostics: changeattempt.Public(diagnostics)}, nil
	})
}

// replayUpload answers a retry from the attempt holding its key, when the
// retry sent the same content.
func (m *Manager) replayUpload(ctx context.Context, versionID uint, existing *internalrepo.ChangeAttempt, key string, received receivedUpload) (sessionpkg.DraftAssetResult, error) {
	if received.refusal == sessionpkg.DraftAssetTooLarge {
		return sessionpkg.DraftAssetResult{}, fmt.Errorf("asset attempt: %w", sessionpkg.ErrIdempotencyConflict)
	}
	fingerprint, err := changeattempt.Fingerprint(uploadRequest{Kind: string(sessionpkg.DraftAttemptAssetUpload), Key: key, SHA256: received.binding.SHA256})
	if err != nil {
		return sessionpkg.DraftAssetResult{}, err
	}
	return utils.RunInDBTransaction(ctx, m.dbServicer, func(ctx context.Context, tx *gorm.DB) (sessionpkg.DraftAssetResult, error) {
		return m.replayAttempt(ctx, tx, versionID, existing, fingerprint, key)
	})
}

// replayAttempt answers from existing, with the asset it bound.
func (m *Manager) replayAttempt(ctx context.Context, tx *gorm.DB, versionID uint, existing *internalrepo.ChangeAttempt, fingerprint string, key string) (sessionpkg.DraftAssetResult, error) {
	replayed, err := changeattempt.ReplayAsset(ctx, existing, fingerprint)
	if err != nil {
		return sessionpkg.DraftAssetResult{}, err
	}
	if replayed.Outcome != sessionpkg.DraftAssetAttached && replayed.Outcome != sessionpkg.DraftAssetUnchanged {
		return replayed, nil
	}
	asset, err := m.assetAt(ctx, tx, versionID, replayed.Revision, key)
	if err != nil {
		return sessionpkg.DraftAssetResult{}, err
	}
	replayed.Asset = asset
	return replayed, nil
}

// attachInTx records the attempt and, when the content may be bound, binds
// it in a new revision.
func (m *Manager) attachInTx(ctx context.Context, tx *gorm.DB, versionUUID string, received receivedUpload, idempotencyKey string, fingerprint string, origin string) (committed[sessionpkg.DraftAssetResult], error) {
	version, err := m.uploadAssetRepo.FindVersion(ctx, tx, versionUUID)
	if err != nil {
		return committed[sessionpkg.DraftAssetResult]{}, err
	}
	if version == nil {
		return committed[sessionpkg.DraftAssetResult]{}, fmt.Errorf("version %s: %w", versionUUID, sessionpkg.ErrDraftNotFound)
	}
	var current int64
	if version.CurrentRevision != nil {
		current = *version.CurrentRevision
	}
	attemptID, existing, err := m.uploadAssetRepo.ClaimChangeAttempt(ctx, tx, version.ID, string(sessionpkg.DraftAttemptAssetUpload), idempotencyKey, fingerprint, current, origin)
	if err != nil {
		return committed[sessionpkg.DraftAssetResult]{}, err
	}
	if existing != nil {
		replayed, err := m.replayAttempt(ctx, tx, version.ID, existing, fingerprint, received.key)
		return committed[sessionpkg.DraftAssetResult]{result: replayed}, err
	}

	decided, diagnostics, err := m.decideAttach(ctx, tx, version, received)
	if err != nil {
		return committed[sessionpkg.DraftAssetResult]{}, err
	}
	var superseded int
	if decided.Outcome == sessionpkg.DraftAssetAttached {
		if superseded, err = m.queueBuild(ctx, tx, m.uploadAssetRepo, version.ID, decided.Revision); err != nil {
			return committed[sessionpkg.DraftAssetResult]{}, err
		}
	}
	revision := decided.Revision
	if err := m.uploadAssetRepo.CompleteChangeAttempt(ctx, tx, attemptID, string(decided.Outcome), &revision, diagnostics); err != nil {
		return committed[sessionpkg.DraftAssetResult]{}, err
	}
	return committed[sessionpkg.DraftAssetResult]{result: decided, superseded: superseded}, nil
}

// decideAttach decides the upload against the version's current revision
// and, when it attaches, writes the new revision.
func (m *Manager) decideAttach(ctx context.Context, tx *gorm.DB, version *internalrepo.Version, received receivedUpload) (sessionpkg.DraftAssetResult, []draftsources.Diagnostic, error) {
	for race := 0; race < maxRevisionRaces; race++ {
		var current int64
		if version.CurrentRevision != nil {
			current = *version.CurrentRevision
		}
		if gameversion.Status(version.Status) != gameversion.Draft || version.CurrentRevision == nil {
			return sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetNotEditable, Revision: current}, nil, nil
		}
		if received.refusal != "" {
			return sessionpkg.DraftAssetResult{Outcome: received.refusal, Revision: current, Diagnostics: changeattempt.Public(received.diagnostics)}, received.diagnostics, nil
		}
		revision, found, err := m.uploadAssetRepo.FindRevision(ctx, tx, version.ID, current)
		if err != nil {
			return sessionpkg.DraftAssetResult{}, nil, err
		}
		if !found {
			monitoring.Alert(ctx, "play draft version is missing its current revision")
			return sessionpkg.DraftAssetResult{}, nil, fmt.Errorf("version %s has no revision %d", version.UUID, current)
		}
		declared, err := declarationsOf(ctx, tx, m.uploadAssetRepo, revision)
		if err != nil {
			return sessionpkg.DraftAssetResult{}, nil, err
		}
		key := received.key
		declaration, isDeclared := declared[key]
		// The declaration may have changed while the content was received.
		if !isDeclared || !gameversion.AcceptsMIMEType(declaration.Type, received.binding.MIMEType) {
			return sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetNotDeclared, Revision: current}, nil, nil
		}
		if bound, isBound := revision.Assets[key]; isBound && bound.SHA256 == received.binding.SHA256 {
			state := draftsources.AssetState{Key: key, Status: draftsources.AssetReady, Declaration: &declaration, Binding: &bound}
			asset := publicAsset(state)
			return sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetUnchanged, Revision: current, Asset: &asset}, nil, nil
		}
		total := received.binding.Size
		for other, binding := range revision.Assets {
			if other != key {
				total += binding.Size
			}
		}
		if total > gameversion.MaxAssetsBytes {
			diagnostics := []draftsources.Diagnostic{{Path: assetPath(key), Code: codeAssetsTooLarge, Message: fmt.Sprintf("the assets would weigh %d bytes together; the limit is %d", total, gameversion.MaxAssetsBytes)}}
			return sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetInvalid, Revision: current, Diagnostics: changeattempt.Public(diagnostics)}, diagnostics, nil
		}

		advanced, err := m.uploadAssetRepo.AdvanceRevision(ctx, tx, version.ID, string(gameversion.Draft), current)
		if err != nil {
			return sessionpkg.DraftAssetResult{}, nil, err
		}
		if !advanced {
			// A change of the sources committed a new revision meanwhile; an
			// asset does not conflict with it, so decide again on top of it.
			version, err = m.uploadAssetRepo.FindVersion(ctx, tx, version.UUID)
			if err != nil {
				return sessionpkg.DraftAssetResult{}, nil, err
			}
			continue
		}
		assets := make(map[string]draftsources.AssetBinding, len(revision.Assets)+1)
		for other, binding := range revision.Assets {
			assets[other] = binding
		}
		assets[key] = received.binding
		next := current + 1
		if err := m.uploadAssetRepo.CreateRevision(ctx, tx, version.ID, next, draftsources.Revision{Files: revision.Files, Assets: assets}, assetRevisionSource); err != nil {
			return sessionpkg.DraftAssetResult{}, nil, err
		}
		binding := received.binding
		state := draftsources.AssetState{Key: key, Status: draftsources.AssetReady, Declaration: &declaration, Binding: &binding}
		asset := publicAsset(state)
		return sessionpkg.DraftAssetResult{Outcome: sessionpkg.DraftAssetAttached, Revision: next, Asset: &asset}, nil, nil
	}
	return sessionpkg.DraftAssetResult{}, nil, fmt.Errorf("version %s kept changing while attaching an asset", version.UUID)
}

// assetAt is key's asset as revision number of versionID holds it, or nil
// when the revision binds nothing to it.
func (m *Manager) assetAt(ctx context.Context, tx *gorm.DB, versionID uint, number int64, key string) (*sessionpkg.DraftAsset, error) {
	revision, found, err := m.uploadAssetRepo.FindRevision(ctx, tx, versionID, number)
	if err != nil || !found {
		return nil, err
	}
	declared, err := declarationsOf(ctx, tx, m.uploadAssetRepo, revision)
	if err != nil {
		return nil, err
	}
	for _, state := range draftsources.AssetStates(declared, revision.Assets) {
		if state.Key == key && state.Binding != nil {
			asset := publicAsset(state)
			return &asset, nil
		}
	}
	return nil, nil
}

// countAssetEdit counts an upload's or removal's outcome.
func countAssetEdit(ctx context.Context, result sessionpkg.DraftAssetResult) {
	outcome := string(result.Outcome)
	if result.Replayed {
		outcome = "replayed"
	}
	draftAssetEditsTotal.Inc(ctx, assetOutcomeLabel.Value(outcome))
	logging.LogFields(ctx, logging.Field("asset_outcome", string(result.Outcome)), logging.Field("revision", result.Revision))
}

// revisionReader is what reading a revision's asset declarations needs.
type revisionReader interface {
	FindRevision(ctx context.Context, tx *gorm.DB, versionID uint, number int64) (draftsources.Revision, bool, error)
	FindSourceContents(ctx context.Context, tx *gorm.DB, hashes []string) (map[string]string, error)
}

// currentDeclarations reads the assets the version's current revision
// declares; an unreadable project file declares none.
func currentDeclarations(ctx context.Context, tx *gorm.DB, repo revisionReader, version *internalrepo.Version) (map[string]draftsources.AssetDeclaration, error) {
	revision, found, err := repo.FindRevision(ctx, tx, version.ID, *version.CurrentRevision)
	if err != nil {
		return nil, err
	}
	if !found {
		monitoring.Alert(ctx, "play draft version is missing its current revision")
		return nil, fmt.Errorf("version %s has no revision %d", version.UUID, *version.CurrentRevision)
	}
	return declarationsOf(ctx, tx, repo, revision)
}

// declarationsOf reads the assets revision declares.
func declarationsOf(ctx context.Context, tx *gorm.DB, repo revisionReader, revision draftsources.Revision) (map[string]draftsources.AssetDeclaration, error) {
	project, found := revision.Files[draftsources.ProjectFile]
	if !found {
		return nil, nil
	}
	contents, err := repo.FindSourceContents(ctx, tx, []string{project.SHA256})
	if err != nil {
		return nil, err
	}
	declared, _ := draftsources.DeclaredAssets(contents[project.SHA256])
	return declared, nil
}

// assetPath is where a diagnostic about the asset key points.
func assetPath(key string) string {
	return "assets/" + key
}

// newUUIDString is a new random UUID.
func newUUIDString() string {
	return uuid.NewString()
}
