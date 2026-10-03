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

**A `Rule`'s own `Path` must never be a prefix of another `Rule`'s own
`Path`** (excluding `declassified` `Rule`s, which are exempt - see below):
`ParseSchema`/`Filter`/`LeakCheck` all reject such a `Schema` outright,
rather than let it degrade silently. Since a matched `Path` decides its
whole subtree atomically and is never re-descended into, a nested `Rule`
would either be unreachable (a less-restrictive parent already decided a
narrower, more-restrictive child's own subtree first) or, in the dangerous
direction, would silently expose the nested `Rule`'s own intended
restriction to every viewer - a shallower `public`/`private` `Rule` matches
first and returns its whole subtree verbatim without ever consulting a
`server_only` (or other-owner `private`) `Rule` nested inside it. This was
found during independent review of `WORK-0041`'s own implementation, not
anticipated upfront - the fix is schema-level rejection, the same
mechanism already used for an unrecognized `Class`.

`declassified` `Rule`s are exempt from this check: `matchRule` never
considers them during `Filter`'s own structural walk (they are synthesized
separately, by direct path/value assignment against the *original* state -
see above), so a `declassified` `Rule`'s own `Path` nesting under a
structural `Rule`'s `Path` cannot participate in the same shadowing hazard.

**Two `Rule`s must never declare the exact same `Path`** (also excluding
`declassified` `Rule`s): `matchRule` always resolves to whichever `Rule`
appears first in `Schema.Rules`, so a duplicate would silently depend on
declaration order rather than fail loudly - the same class of ambiguous-
schema hazard as the nesting rejection above, at zero distance instead of
a proper prefix.

**A `declassified` `Rule`'s own `Source` must resolve to data explicitly
declared `private`** - never `server_only` data, and never an undeclared
path (which falls back to `Schema.Default` and carries no per-viewer
ownership concept regardless of what `Default` itself is set to).
`applyDeclassifiedRule` reads `Source` directly against the *original*,
unfiltered `state` - entirely independent of whatever class governs that
same path during the structural walk - so nothing else in this package
would otherwise stop a `Source` pointing at data the `Schema` itself says
must never reach any viewer. `Source` is accepted when it is exactly, or
nested inside, some `Rule`'s own `private` `Path` (a `private` `Rule`
already decides its entire subtree atomically as owned by its own bound
viewer, and the nesting rejection above guarantees no other `Rule` can
intercept a path nested inside it - so a `Source` reaching into that same
subtree is exactly as private as the field the `Rule` itself governs).
`ParseSchema`/`Filter`/`LeakCheck` all reject a `Schema` violating this,
the same defense-in-depth layering as every other rejection here. Found
during independent review, not anticipated upfront.

**This coverage check is deliberately asymmetric, and must never reuse
`isPrefixOrEqual`/`pathsNest`'s own symmetric wildcard tolerance** - a
distinct bug independent review also found, in the very check the
previous paragraph describes. `isPrefixOrEqual` treats a `"*"` on
*either* side as compatible with anything, which is exactly right for
`pathsNest` (a Schema is declared once and applied across arbitrarily
many states, so any structural possibility of nesting for *some* state is
a hazard worth rejecting). It is wrong for Source-coverage: a `"*"` on a
`declassified` Rule's own `Source` is walked by `enumerateInstantiations`
against *every* matching key/index actually present in state at runtime,
entirely independent of which single instantiation a concrete `private`
Rule's own `Path` happens to name - so a concrete `private.Path` (for
example `players.0.hand`) must never be treated as "covering" a
wildcarded `Source` (for example `players.*.hand`), even though
`isPrefixOrEqual` itself would accept that pairing. `sourceIsExplicitlyPrivate`
therefore uses its own separate, asymmetric
`privateRuleCoversEverySourceInstantiation`: a wildcard on the `private`
Rule's own `Path` covers anything, but a wildcard on `Source`'s own side
is never forgiven by a concrete `private.Path` segment.

**A `declassified` `Rule` whose `Path` traverses a container the
structural pass already built as a JSON array does not destroy that
array's own contents.** `setByPath` only ever descends into existing map
containers (per its own "no array intermediate containers" restriction
above); encountering an array instead, it now skips the write entirely
(the declassified field is simply absent for that instantiation) rather
than replacing the array with a fresh empty map, which would silently
discard every sibling entry the structural pass had already legitimately
included. Found during independent review, not anticipated upfront.

**A `declassified` `Rule`'s own destination `Path` may coincide with a
value some other Rule (or `Schema.Default`'s own leaf-level fallback)
already placed there - this is traced safe, never a leak, and left
unguarded on purpose.** `setByPath`'s own final-segment assignment
unconditionally overwrites whatever value already occupied that exact
key, replacing it with the computed declassified value. Because a
declassified value is by construction never more revealing than the raw
value it could silently replace (it is always a Playhoot-computed
primitive derived from data the Schema itself already permits exposing in
some form), this can only ever be an author-facing data-availability
foot-gun (a raw field silently becomes its own derivative instead), never
a privacy violation - true whether the pre-existing value came from a
matched structural Rule or from `Default`'s own fallback inclusion.

**At most one `"*"` per `Path`/`Source`, enforced, not only documented.**
A second wildcard has no defined owner-binding meaning: `matchTemplate`
can only ever report one bound segment, so a 2+-wildcard template would
silently bind ownership to whichever wildcard is evaluated last - an
entirely different segment than the template's own intended owner, with
no schema-authoring typo or malicious intent required to trigger it (this
codebase's own real viewer identity, a small decimal actor id, has the
identical shape as a JSON array index, making the hazard concrete rather
than theoretical). `Schema.validate` rejects any Rule/Source with more
than one wildcard outright; `matchTemplate` itself additionally refuses to
match a 2+-wildcard template at all, as a defense-in-depth backstop for a
caller that builds a `Rule` without going through `validate`. Found during
independent review, not anticipated upfront.

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

- The offline/publish-time differential probe (running `project()` under a
  perturbed private input and checking for an undeclared output
  dependency) - explicitly descoped from `WORK-0041` by human decision and
  tracked as its own standalone follow-up,
  `docs/projects/active/js-runtime-migration/works/WORK-0054-projection-visibility-differential-publish-validation.md`
  (this package's own `Filter`+`LeakCheck` is the NOW-justified primary
  guarantee plus one cheap safety net; the differential probe is a
  stronger but costlier SOON/LATER addition, and - given `Filter`'s own
  capability-based guarantee - primarily defends against a regression in
  `Filter`/the declassification model's own implementation, not against
  arbitrary authored-script behavior).
- Any connection to *when* a `ClientState` is computed/delivered over a
  live connection - `session-runtime-v1`'s own `WORK-0015`, per WORK-0041's
  own Coordination Flag.
