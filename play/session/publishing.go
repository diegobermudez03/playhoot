package session

// PublishBlockerCode is why a DRAFT's current revision cannot become an
// executable version yet.
type PublishBlockerCode string

const (
	// PublishBuildMissing means the current revision has no build.
	PublishBuildMissing PublishBlockerCode = "build_not_for_current_revision"
	// PublishBuildNotSucceeded means the current revision's build has not
	// succeeded (yet): PublishBlocker.BuildStatus says where it is.
	PublishBuildNotSucceeded PublishBlockerCode = "build_not_succeeded"
	// PublishMissingRequiredAssets means declared assets that are not
	// optional have no content bound: PublishBlocker.Keys names them.
	PublishMissingRequiredAssets PublishBlockerCode = "missing_required_assets"
	// PublishRevisionChanged means the DRAFT moved past the revision a
	// publication was asked for.
	PublishRevisionChanged PublishBlockerCode = "revision_changed"
)

// PublishBlocker is one reason a DRAFT cannot be published yet.
type PublishBlocker struct {
	Code        PublishBlockerCode
	BuildStatus DraftBuildStatus
	Keys        []string
}

// PublishReadinessOutcome is what CheckPublication decided.
type PublishReadinessOutcome string

const (
	// PublishReady means the DRAFT's current revision may be frozen.
	PublishReady PublishReadinessOutcome = "ready"
	// PublishNotReady means it may not yet: Blockers say why.
	PublishNotReady PublishReadinessOutcome = "not_ready"
	// PublishNotEditable means the version is not a DRAFT any more.
	PublishNotEditable PublishReadinessOutcome = "not_editable"
)

// PublishReadiness is whether a DRAFT's current revision, Revision, may
// become an executable version.
type PublishReadiness struct {
	Outcome  PublishReadinessOutcome
	Revision int64
	Blockers []PublishBlocker
}

// FreezeOutcome is what FreezeVersion decided.
type FreezeOutcome string

const (
	// FreezeFrozen means the DRAFT became an executable version, immutable
	// from now on, and its game's current version.
	FreezeFrozen FreezeOutcome = "frozen"
	// FreezeAlreadyFrozen means an earlier call froze it: the same answer
	// again.
	FreezeAlreadyFrozen FreezeOutcome = "already_frozen"
	// FreezeNotReady means the revision may not be frozen: Blockers say why.
	FreezeNotReady FreezeOutcome = "not_ready"
)

// FrozenVersion is what freezing a DRAFT at Revision decided.
type FrozenVersion struct {
	Outcome  FreezeOutcome
	Revision int64
	Blockers []PublishBlocker
	// Languages are the languages the frozen version's game is written in,
	// its main one first; set when it is frozen.
	Languages []string
}

// CleanedObjects counts the stored objects a cleanup deleted because nothing
// referenced them any more.
type CleanedObjects struct {
	Assets    int
	Artifacts int
}
