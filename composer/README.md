# Composer

Composer is the coordination layer for cross-domain reads.

- Coordinates composite read operations across business domains.
- Is stateless with respect to business/domain state.
- Invokes domain read capabilities and composes the result.
- May contain composition/read-coordination logic.
- Owns no business entities.
- Owns no domain business rules.

Illustrative example only: a composite read may need to consider whether data read from different domains represents a consistent enough view. Some future composition might use version/snapshot information if participating domains support it. This is not a global requirement that every domain support historical or versioned reads.

See `../ARCHITECTURE.md` for global architecture rules.

## Current state

Groups are organized like `orchestrator`'s: one subpackage per behavior, a `Manager` per group, narrow interfaces on the domains it calls.

`gameauthoring` (2026-11-01, game creation project): `ReadDraft` and `ListDraftRevisions` ask Game Management whether the caller may edit, and so read, the game (`FindEditableVersion`), and only when found read Play's version being built. A version Game Management found that Play does not have yet (a game whose creation stopped between the domains) is reported `Missing`, not an error: the caller asks the orchestrator to create it (`EnsureDraft`) and reads again. Since 2026-11-02 the draft read carries the version's assets with their signed preview links.

`gameauthoring.ReadDraftBuild` (2026-11-02): the same authorization, then Play's `ReadBuild` of one revision's build, waiting up to 20 s for it to end; a version Play lacks is reported `Missing` for the API to repair, like the other reads.

`gameauthoring.AILinkStatus` (2026-11-02): the same Game Management decision as the AI link writes (the owner, found or `not_editable`), then Identity's `ResourceLinkStatus` for the game: whether a live link exists, never the link itself.
