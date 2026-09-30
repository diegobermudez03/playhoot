# visibility Logical Contract

Status: PACKAGE-LOCAL IMPLEMENTATION CONTRACT

Records `WORK-0041`'s own capability-based privacy boundary for per-viewer
game state; see `docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md`
(Consequences: "a privacy-verification mechanism ... is a required
capability") and this WORK's own file (`docs/projects/active/js-runtime-migration/works/WORK-0041-per-player-view-computation-and-privacy-verification.md`)
for the accepted architecture this implements.

## The Guarantee, And What Enforces It

Privacy is primarily **capability-based, not detection-based**: the
authored `project()` script never receives data a viewer is not authorized
to see in the first place. `Filter` constructs that viewer-scoped
`ProjectionInput` from the full authoritative state and a Game's own
declared `Schema` *before* `project()` ever runs
(`session/internal/executor.Executor.Project`, `session/jsexecutor/internal/sandbox.Project`).
No later check "catches" excluded data reaching the script - it was never
handed to it.

`LeakCheck` is a separate, defense-in-depth safety net against a bug in
`Filter` itself, or a misconfigured `Schema` (an author declaring something
public/declassified that should not be). It is never relied on to
compensate for handing the script unrestricted access - see this WORK's own
Material Decision 2 for the human decision this restates.

## Schema

An author declares, per state path, one of four classes
(`schema.go`):`public` (every viewer), `private` (only the owning viewer -
determined by a Path's own wildcard-bound segment), `server_only` (no
viewer, ever), or `declassified` (not read directly - Playhoot itself
computes a derived value from a private `Source` and exposes only that
computed result, via one of a small fixed set of primitives:
`count`/`length`, `exists`, `redacted`). Any path the declared `Rules` do
not match falls back to `Schema.Default` (`server_only` if unset) - data is
exposed only because a `Rule` explicitly says so, never by omission.

**Deliberate v1 simplification**: a `Rule.Path`/`Source` template supports
at most one `"*"` wildcard segment. This covers the standard "state keyed
by owning participant" shape (`players.*.hand`) but not a game needing more
than one independent dynamic ownership dimension within a single path.
Revisit only if a real game needs it - `docs/ai/OPERATING_MODEL.md`'s
Anti-Overengineering framework.

## Filter

Walks `state`'s own JSON tree; at the first path where a `Rule` exactly
matches, the whole subtree there is decided as one unit (included
verbatim, or excluded entirely) - a container with no rule at its own exact
path is never itself decided, only reconstructed from whichever of its own
children are individually included. `declassified` rules are synthesized
in a separate pass afterward, reading from the *original* `state` (never
the partially-filtered output, and never from what a script itself
computed), since a declassified value's `Source` is commonly a path the
structural pass already excluded entirely.

## LeakCheck

Collects every subtree `Filter` would have excluded for `viewer` (at the
same decision granularity `Filter` itself uses - a whole excluded subtree,
not one finding per leaf) and checks whether its own canonical JSON
encoding appears verbatim inside `clientState`. Two documented limitations,
both deliberate, not oversights:

- **Ignores short values** (`minLeakCheckBytes`): a coincidentally shared
  small number/boolean/short string is not a meaningful signal and would
  make this check too noisy to be useful.
- **Cannot distinguish a legitimate declassified dependency from a leak by
  itself** - it never needs to, because it only flags *verbatim* excluded
  values, and a correctly-implemented `declassified` derivative is a new,
  different value (a count, a boolean, a placeholder), never the private
  value itself. A differential/causal probe (comparing outputs under a
  perturbed private input) would need to reason about this explicitly;
  this literal-containment check does not, by construction.

Neither limitation is a proof of no leak. A non-empty result is a signal a
caller should alert on and investigate, not a false-positive nuisance to
silence, and an empty result is not proof the projection is safe - `Filter`
is what makes it safe; this is its safety net.

## Explicitly Not Implemented Here

- The offline/publish-time differential probe WORK-0041's own Material
  Decision 2 also names (running `project()` under a perturbed private
  input and checking for an undeclared output dependency) - a separate
  capability, not yet built, tracked for later against this Project's own
  Anti-Overengineering framing (this package's own `Filter`+`LeakCheck` is
  the NOW-justified primary guarantee plus one cheap safety net; the
  differential probe is a stronger but costlier SOON/LATER addition).
- Any connection to *when* a `ClientState` is computed/delivered over a
  live connection - `session-runtime-v1`'s own `WORK-0015`, per WORK-0041's
  own Coordination Flag.
