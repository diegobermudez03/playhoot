package session

import (
	"errors"
	"time"
)

// ErrDraftNotFound is returned when an operation names a game version that
// does not exist.
var ErrDraftNotFound = errors.New("draft version not found")

// ErrDraftRevisionNotFound is returned when a read names a revision the
// version does not have.
var ErrDraftRevisionNotFound = errors.New("draft revision not found")

// ErrDraftVersionConflict is returned when a DRAFT is created with a version
// UUID that already names a version of another game, or one that is no
// longer a DRAFT.
var ErrDraftVersionConflict = errors.New("version uuid already names another version")

// CreateDraftOutcome is the decision a CreateDraftVersion call reached.
type CreateDraftOutcome string

const (
	// DraftCreated means the game now has this DRAFT, at revision 1 with the
	// starter template.
	DraftCreated CreateDraftOutcome = "created"
	// DraftAlreadyCreated means this same DRAFT already existed: a retry.
	DraftAlreadyCreated CreateDraftOutcome = "already_created"
	// DraftAnotherOpen means the game already has a different DRAFT, whose
	// UUID the result names; nothing was created.
	DraftAnotherOpen CreateDraftOutcome = "another_draft_open"
)

// DraftStart is what a new DRAFT's revision 1 copies: nothing (the starter
// project), the published FromVersionUUID of the same game (a new edition),
// or, with TemplateGameUUID, the published FromVersionUUID of that template.
type DraftStart struct {
	FromVersionUUID  string
	TemplateGameUUID string
}

// DraftTemplate is a template a DRAFT switches to: a game and the Play
// version of its published edition.
type DraftTemplate struct {
	GameUUID    string
	VersionUUID string
}

// ErrDraftTemplateNotFound is returned when a template's version is not an
// executable version of that game with sources to copy.
var ErrDraftTemplateNotFound = errors.New("template version not found")

// DraftVersion is what creating a DRAFT answered: the game and the DRAFT it
// has, and that DRAFT's current revision.
type DraftVersion struct {
	Outcome     CreateDraftOutcome
	GameUUID    string
	VersionUUID string
	Revision    int64
}

// Draft change origins: who sent a change.
const (
	DraftOriginWeb = "web"
	DraftOriginAPI = "api"
	DraftOriginMCP = "mcp"
)

// DraftFileContent is one file a change writes, whole.
type DraftFileContent struct {
	Path    string
	Content string
}

// DraftFileEdit replaces Find with Replace in an existing file: its one
// occurrence, or every occurrence when All.
type DraftFileEdit struct {
	Path    string
	Find    string
	Replace string
	All     bool
}

// DraftChange is one change to a DRAFT's sources, made over BaseRevision,
// the revision its author last read. It applies whole or not at all: Edits,
// in order, on the base revision's contents, then Files written whole and
// Delete removed; a path appears in one of the three only. IdempotencyKey
// identifies it across retries; Origin is one of the draft change origins.
type DraftChange struct {
	BaseRevision   int64
	Files          []DraftFileContent
	Edits          []DraftFileEdit
	Delete         []string
	IdempotencyKey string
	Origin         string
}

// DraftChangeOutcome is the decision a change reached.
type DraftChangeOutcome string

const (
	// DraftChangeApplied means the change is the new current revision.
	DraftChangeApplied DraftChangeOutcome = "applied"
	// DraftChangeConflict means the current revision is no longer the base
	// the change was made over; nothing applied.
	DraftChangeConflict DraftChangeOutcome = "conflict"
	// DraftChangeInvalid means the change breaks the sources' rules, which
	// its diagnostics name; nothing applied.
	DraftChangeInvalid DraftChangeOutcome = "invalid"
	// DraftChangeNotEditable means the version is no longer a DRAFT.
	DraftChangeNotEditable DraftChangeOutcome = "not_editable"
	// DraftChangeUnchanged means the change leaves every file as the current
	// revision has it, so no revision was created.
	DraftChangeUnchanged DraftChangeOutcome = "unchanged"
)

// DraftDiagnostic is one reason a change was refused; Path is empty when the
// reason is about the change as a whole. Edit is the index of the edit it is
// about (edit_not_found, edit_ambiguous, ...), nil otherwise.
type DraftDiagnostic struct {
	Path    string
	Code    string
	Edit    *int
	Message string
}

// DraftChangeResult is what a change decided. Revision is the new revision
// when applied, otherwise the version's current revision when it was
// decided. Replayed means it was answered from an earlier request with the
// same idempotency key.
type DraftChangeResult struct {
	Outcome     DraftChangeOutcome
	Revision    int64
	Diagnostics []DraftDiagnostic
	Replayed    bool
	// Build is the build of Revision when the change made it, nil otherwise.
	Build *DraftBuild
	// Changes are Revision's files, and what changed from the revision
	// before it, when applied or unchanged; nil otherwise.
	Changes *DraftRevisionChanges
	// Diffs are the bounded unified diffs of the files the change edited,
	// when it applied and was not replayed.
	Diffs []DraftFileDiff
}

// What a diff between two revisions says of a file.
const (
	DraftFileAdded    = "added"
	DraftFileModified = "modified"
	DraftFileDeleted  = "deleted"
)

// MaxDraftDiffBytes bounds the diffs one comparison of two revisions
// answers in all; files past it are listed without their diff (Omitted).
const MaxDraftDiffBytes = 64 * 1024

// DraftRevisionDiff compares revision From with revision To of a version (0
// as From is the empty project): each file that differs, by path.
type DraftRevisionDiff struct {
	From  int64
	To    int64
	Files []DraftRevisionFileDiff
}

// DraftRevisionFileDiff is one file that differs between two revisions:
// Status (the DraftFile* statuses) and its unified diff, cut at 8 KiB
// (Truncated) or left out when the comparison's budget ran out (Omitted).
type DraftRevisionFileDiff struct {
	Path      string
	Status    string
	Diff      string
	Truncated bool
	Omitted   bool
}

// DraftFileDiff is a file's unified diff, cut at 8 KiB (Truncated).
type DraftFileDiff struct {
	Path      string
	Diff      string
	Truncated bool
}

// DraftRevisionChanges are a revision's files, without contents, and the
// paths that changed since an earlier revision (Since): added, modified and
// deleted, each sorted. Since 0 means the version's beginning: every file is
// added.
type DraftRevisionChanges struct {
	Since    int64
	Revision int64
	Files    []DraftFile
	Added    []string
	Modified []string
	Deleted  []string
}

// DraftFile is one source file of a revision. Content is set only when the
// read asked for contents.
type DraftFile struct {
	Path    string
	SHA256  string
	Size    int64
	Content *string
}

// DraftAttemptKind is what an attempt tried: a change of the sources, a
// restore, attaching an uploaded asset or removing one.
type DraftAttemptKind string

const (
	DraftAttemptChange       DraftAttemptKind = "change"
	DraftAttemptRestore      DraftAttemptKind = "restore"
	DraftAttemptAssetUpload  DraftAttemptKind = "asset_upload"
	DraftAttemptAssetRemoval DraftAttemptKind = "asset_removal"
	DraftAttemptTemplate     DraftAttemptKind = "template"
)

// DraftChangeAttempt is one change sent to a version, applied or not.
// Outcome is a DraftChangeOutcome or, for asset attempts, a
// DraftAssetOutcome. Revision is set only when it changed or kept the
// revision on purpose.
type DraftChangeAttempt struct {
	Kind         DraftAttemptKind
	Outcome      string
	BaseRevision int64
	Revision     *int64
	Origin       string
	Diagnostics  []DraftDiagnostic
	CreatedAt    time.Time
}

// DraftSnapshot is a version's sources and assets at one revision, with its
// most recent change attempts. A version without authored sources has
// CurrentRevision and Revision 0 and no files. AssetDiagnostics explain why
// no asset is declared when game.project.json cannot be read.
type DraftSnapshot struct {
	VersionUUID      string
	GameUUID         string
	Status           string
	CurrentRevision  int64
	Revision         int64
	Files            []DraftFile
	Assets           []DraftAsset
	AssetDiagnostics []DraftDiagnostic
	Attempts         []DraftChangeAttempt
	// Build is the build of Revision, nil when it has none.
	Build *DraftBuild
	// LastSuccessfulBuild is the version's newest succeeded build, of
	// Revision or an older one; nil when none succeeded yet.
	LastSuccessfulBuild *DraftBuild
}

// DraftAssetStatus is where a revision stands with one asset.
type DraftAssetStatus string

const (
	// DraftAssetMissing is a declared asset with no content yet.
	DraftAssetMissing DraftAssetStatus = "missing"
	// DraftAssetReady is a declared asset with its content.
	DraftAssetReady DraftAssetStatus = "ready"
	// DraftAssetUndeclared is content bound to a key the project no longer
	// declares.
	DraftAssetUndeclared DraftAssetStatus = "undeclared"
)

// DraftAsset is one asset of a revision: what the project declares and the
// content bound to it. URL is a short-lived signed link to that content,
// set when there is content and the read asked for links.
type DraftAsset struct {
	Key          string
	Type         string
	Description  string
	Optional     bool
	Status       DraftAssetStatus
	SHA256       string
	Size         int64
	MIMEType     string
	URL          string
	URLExpiresAt time.Time
}

// MaxDraftAssetBytes is the most one asset upload may weigh, whatever its
// type: callers bound a request body by it; the asset's type sets the real,
// smaller limit.
const MaxDraftAssetBytes = 50 << 20

// ErrUploadsBusy means this instance is already receiving as many asset
// uploads as it holds in memory at once; the caller may try again shortly.
var ErrUploadsBusy = errors.New("too many asset uploads in progress")

// DraftAssetOutcome is the decision attaching or removing an asset reached.
type DraftAssetOutcome string

const (
	// DraftAssetAttached means the uploaded content is bound to its key in
	// the new current revision.
	DraftAssetAttached DraftAssetOutcome = "attached"
	// DraftAssetRemoved means the key has no content in the new current
	// revision.
	DraftAssetRemoved DraftAssetOutcome = "removed"
	// DraftAssetUnchanged means the revision already held exactly that, so
	// no revision was created.
	DraftAssetUnchanged DraftAssetOutcome = "unchanged"
	// DraftAssetInvalid means the content breaks the asset rules, which the
	// diagnostics name.
	DraftAssetInvalid DraftAssetOutcome = "invalid"
	// DraftAssetTooLarge means the upload weighs more than its declared type
	// allows; it was cut there and nothing was stored.
	DraftAssetTooLarge DraftAssetOutcome = "too_large"
	// DraftAssetNotEditable means the version is no longer a DRAFT.
	DraftAssetNotEditable DraftAssetOutcome = "not_editable"
	// DraftAssetNotDeclared means the project does not declare the key, or
	// declares it with another type.
	DraftAssetNotDeclared DraftAssetOutcome = "not_declared"
)

// DraftAssetResult is what attaching or removing an asset decided. Revision
// is the new revision when it changed, otherwise the current one. Asset is
// the bound asset when attached or unchanged by an upload.
type DraftAssetResult struct {
	Outcome     DraftAssetOutcome
	Revision    int64
	Asset       *DraftAsset
	Diagnostics []DraftDiagnostic
	Replayed    bool
	// Build is the build of Revision when the attachment or removal made it,
	// nil otherwise.
	Build *DraftBuild
	// Changes are Revision's files, and what changed from the revision
	// before it, when attached, removed or unchanged; nil otherwise.
	Changes *DraftRevisionChanges
}

// Kinds of revision: what made it.
const (
	DraftRevisionTemplate = "template"
	DraftRevisionEdition  = "edition"
	DraftRevisionChange   = "change"
	DraftRevisionRestore  = "restore"
	DraftRevisionAsset    = "asset"
)

// Kinds of actor behind a revision.
const (
	DraftActorPerson   = "person"
	DraftActorAI       = "ai"
	DraftActorPlatform = "platform"
)

// DraftRevision is one revision in a version's history with its provenance.
type DraftRevision struct {
	Number int64
	// Kind is one of the DraftRevision* kinds; the list may grow.
	Kind string
	// Origin is one of the draft change origins, empty when the platform
	// made the revision.
	Origin string
	Actor  string
	// Parent is nil for revision 1; RestoredFrom is set on restores only.
	Parent       *int64
	RestoredFrom *int64
	// From is the version it was copied from: a template's or, for a new
	// edition, the published one.
	From *DraftRevisionFrom
	// SDK is the version's script profile.
	SDK       string
	Files     int
	CreatedAt time.Time
}

// DraftRevisionFrom is the version a revision was copied from, at Revision
// when known.
type DraftRevisionFrom struct {
	GameUUID    string
	VersionUUID string
	Revision    *int64
}

// MaxDraftRevisionsPage is the most revisions one page of a version's
// history holds.
const MaxDraftRevisionsPage = 100

// ErrDraftBuildNotFound is returned when a revision has no build: it was
// made before builds existed.
var ErrDraftBuildNotFound = errors.New("draft build not found")

// DraftBuildStatus is where a revision's build is: QUEUED, RUNNING, then
// SUCCEEDED, FAILED or SUPERSEDED (a newer revision was queued before it
// ran). Unknown values may appear.
type DraftBuildStatus string

const (
	DraftBuildQueued     DraftBuildStatus = "QUEUED"
	DraftBuildRunning    DraftBuildStatus = "RUNNING"
	DraftBuildSucceeded  DraftBuildStatus = "SUCCEEDED"
	DraftBuildFailed     DraftBuildStatus = "FAILED"
	DraftBuildSuperseded DraftBuildStatus = "SUPERSEDED"
)

// DraftBuildDiagnostic is one problem a build found: Stage infrastructure is
// the platform failing, never the game.
type DraftBuildDiagnostic struct {
	Stage   string
	Code    string
	File    string
	Line    int
	Column  int
	Message string
}

// DraftBuildArtifact identifies one built file.
type DraftBuildArtifact struct {
	SHA256 string
	Size   int64
}

// DraftBuild is one revision's build. Server, Client and Manifest are set
// only when it succeeded; Reused means it took the artifacts of an earlier
// build with the same inputs instead of building again.
type DraftBuild struct {
	Revision    int64
	Status      DraftBuildStatus
	Diagnostics []DraftBuildDiagnostic
	Server      *DraftBuildArtifact
	Client      *DraftBuildArtifact
	Manifest    []byte
	AspectRatio string
	Reused      bool
	QueuedAt    time.Time
	StartedAt   *time.Time
	FinishedAt  *time.Time
}

// BuiltRevision is a revision's build as running it needs: Ready only when
// the build SUCCEEDED, and then with its backend script, the profile the
// script runs under and its manifest.
type BuiltRevision struct {
	Revision      int64
	BuildStatus   DraftBuildStatus
	Ready         bool
	Script        []byte
	ScriptProfile string
	Manifest      RuntimeManifest
	// ToolchainVersion identifies the toolchain that built it, when known.
	ToolchainVersion string
}
