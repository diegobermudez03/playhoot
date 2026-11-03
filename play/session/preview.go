package session

import (
	"errors"
	"time"
)

// A preview is the owner of a draft playing one SUCCEEDED build of a revision
// with synthetic participants, run by the live Runtime. It is not a Session:
// it is never listed, joined, limited, reconciled or replayed as one.

// ErrPreviewNotFound is returned for a preview that does not exist.
var ErrPreviewNotFound = errors.New("preview not found")

// ErrDraftBuildNotReady is returned when a revision's build did not succeed,
// so nothing of it can be served.
var ErrDraftBuildNotReady = errors.New("draft build not ready")

// PreviewOpenOutcome is the decision an OpenPreview call reached.
type PreviewOpenOutcome string

const (
	// PreviewOpened means the preview exists and runs from now on; any
	// earlier preview of the same version ended as replaced.
	PreviewOpened PreviewOpenOutcome = "opened"

	// PreviewBuildNotReady means the revision has no SUCCEEDED build;
	// BuildStatus tells the build's status when there is one.
	PreviewBuildNotReady PreviewOpenOutcome = "build_not_ready"

	// PreviewInvalidPlayers means the player count is outside what the
	// build's manifest allows; MinPlayers and MaxPlayers tell the range.
	PreviewInvalidPlayers PreviewOpenOutcome = "invalid_players"
)

// PreviewEndReason is why a preview ended.
type PreviewEndReason string

const (
	PreviewEndFinished      PreviewEndReason = "finished"
	PreviewEndIdle          PreviewEndReason = "idle"
	PreviewEndEndedByOwner  PreviewEndReason = "ended_by_owner"
	PreviewEndReplaced      PreviewEndReason = "replaced"
	PreviewEndScriptFailure PreviewEndReason = "script_failure"
	PreviewEndUnavailable   PreviewEndReason = "unavailable"
)

// PreviewOpening is what opening a preview decided. Everything after
// Participants is set only when Opened: what the Runtime starts it with.
type PreviewOpening struct {
	Outcome PreviewOpenOutcome

	// BuildStatus is the revision's build status when the build is not
	// ready; empty when the revision has no build.
	BuildStatus DraftBuildStatus

	// MinPlayers and MaxPlayers are the manifest's range when the player
	// count is invalid.
	MinPlayers int
	MaxPlayers int

	PreviewUUID  string
	Revision     int64
	Participants []string

	// Replaced is the preview of the same version this one ended, if any.
	Replaced string

	GameVersionUUID string
	Script          []byte
	ScriptProfile   string
	Manifest        RuntimeManifest
	Frontend        FrontendAccess

	// ExpiresAt is the most a preview lives, whatever its activity.
	ExpiresAt time.Time
}

// Preview is a preview as it is recorded.
type Preview struct {
	UUID          string
	OwnerUserUUID string
	Revision      int64
	Participants  []string
	CreatedAt     time.Time
	ExpiresAt     time.Time

	// Ended is set once the preview ended, with why.
	Ended     bool
	EndReason PreviewEndReason
}
