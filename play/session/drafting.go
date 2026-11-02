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

// DraftChange is one change to a DRAFT's sources, made over BaseRevision,
// the revision its author last read. It applies whole or not at all.
// IdempotencyKey identifies it across retries; Origin is one of the draft
// change origins.
type DraftChange struct {
	BaseRevision   int64
	Files          []DraftFileContent
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
// reason is about the change as a whole.
type DraftDiagnostic struct {
	Path    string
	Code    string
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
}

// DraftRevision is one revision in a version's history. Source is who made
// it: the starter template, a change's origin, or a restore.
type DraftRevision struct {
	Number    int64
	Source    string
	Files     int
	CreatedAt time.Time
}
