# Code Comment Standard

Status: CANONICAL ENGINEERING STANDARD

This standard owns what source-code comments are for and how they should be
written, across packages, types, struct fields, functions/methods,
interfaces, constants, repositories/storage code, and inline implementation
code. It does not own naming, package placement, or architecture; see
`domain-logic-placement.md` and `ARCHITECTURE.md` for those.

## Core Principle

Code is the primary documentation of implementation. Comments should explain
intent, meaning, contract, or non-obvious constraints — not narrate how the
implementation works. A comment should remain useful even when private
implementation details change.

A comment should normally answer:

- What does this abstraction represent?
- What responsibility does this function/type have?
- What contract or invariant matters to callers?
- Why does a non-obvious constraint exist?
- Why is something implemented in a way that may otherwise look surprising?
- What externally meaningful behavior should a caller understand?

A comment should generally NOT answer:

- Which query/table/repository method is used internally, unless that is
  part of the actual public contract.
- Which fields are later passed into which internal function, or which line
  currently consumes a field.
- A step-by-step description of the function body (loops, branches,
  transactions, helper calls the code already shows).
- Historical implementation state, or what changed in a particular task,
  WORK, ADR, or refactor ("previously X, now Y").

Historical context belongs in version control, WORK documents, or ADRs — not
in source comments.

## Exception: Comment When the Code Cannot Speak for Itself

This standard is not "avoid comments." A comment is valuable when a
maintainer could reasonably look at correct code and still ask "why are we
doing this?", "why is this necessary?", "why can this not be simplified?",
"why is this ordering/lock/transaction boundary required?", or "what subtle
correctness property would be easy to break during a future refactor?" These
cases are the exception, not the default. Examples: a non-obvious
concurrency or locking requirement, why operation ordering matters for
correctness, a subtle transaction invariant, an intentional workaround,
behavior required by an external protocol/dependency, why apparently
redundant code must remain, a surprising edge case, or a
security/compatibility/consistency/idempotency constraint.

```go
// Claim the idempotency key before performing side effects so concurrent
// retries cannot both execute the operation.
```

```go
// Treat a missing row as success because deletion is intentionally idempotent.
```

These explain *why*, not *what*. Do not turn this exception into permission
to document every internal implementation detail — if the behavior is
obvious from well-named code, no comment is needed.

## Style

Comments should be concise, stable under implementation changes, written at
the abstraction level of the symbol being documented, and focused on
purpose/contract or non-obvious reasoning rather than mechanics. Prefer
one-line comments; two or three lines are acceptable when genuinely
necessary. A long comment is justified only by a real invariant, subtle
reasoning, protocol requirement, or similar information that cannot be
clearly expressed through code.

## By Symbol Kind

**Types/structs** describe what the type represents, not who currently
constructs or consumes it:

```go
// Session represents the persisted lifecycle state of a game session.
type Session struct { ... }
```

**Struct fields** are commented only when the name/type alone leaves out
meaning that matters — units, special semantics, invariants, unusual
zero-value behavior, externally relevant distinctions. Do not comment a
field merely to say where it is later used:

```go
// ExpiresAt is the absolute time after which the lease is no longer valid.
ExpiresAt time.Time
```

**Functions/methods** describe the operation from the caller's perspective
and its contract, not the body:

```go
// StartSession starts a lobby session and initializes its runtime state.
func StartSession(...) ...
```

An inline comment inside the function body may still be appropriate when
there is a specific non-obvious reason for an operation or its ordering:

```go
// Persist the runtime snapshot before exposing RUNNING so readers never
// observe a running session without corresponding runtime state.
```

**Repositories/storage** comments describe the storage operation
semantically, not the SQL/ORM mechanics:

```go
// Claim attempts to acquire the idempotency claim for the request.
```

Database-specific behavior may be documented when it is necessary to
understand a non-obvious correctness property (e.g. "the row lock serializes
commands for the same session"), not merely to restate the query.

**Interfaces** describe the capability exposed to consumers, not which
concrete implementation currently satisfies it.

**Constants** are commented only when the meaning/purpose is not already
obvious from the name:

```go
// MaxStepsPerRuntimeTurn bounds synchronous runtime progression within one turn.
const MaxStepsPerRuntimeTurn = 20
```

**Inline comments** should be uncommon. Do not use them as subtitles for
straightforward code ("get the session", "check the error", "update the
session"). Use them to explain why something surprising is necessary, a
subtle invariant, a correctness constraint, intentional ordering, a
concurrency/transaction requirement, an intentional edge case, a workaround,
or compatibility/security/consistency requirements. Before adding one,
consider whether clearer naming or structure would make the code
self-explanatory instead; prefer that when possible.

Exported symbols still need doc comments sufficient for idiomatic Go/GoDoc,
but satisfying GoDoc does not mean writing verbose comments — a short,
contract-level sentence is normally enough.

## No Task/History Comments

Source comments must not become a changelog. Do not write things like
"Added for WORK-0001", "Changed during Slice 1", "Previously this used X",
references to the AI session or task that introduced the code, or migration
narration — unless compatibility with old behavior is an active runtime
requirement. Use git history, WORK documents, or ADRs for that information.

This includes narrating *how* a piece of reasoning was arrived at — "found
by independent review", "per explicit human direction (2026-09-19)",
"discovered via real-Postgres verification", "fixed after code review",
"per this session's instructions". A comment documents the code's contract
and behavior, not the provenance of the sentence itself. If the reasoning is
worth keeping, state it directly as a property of the code:

```go
// Bad: narrates the review/discovery process that produced the code.
// If the Session is already RUNNING, a different token's Start already won
// (found by independent review - reachable without any race, e.g. a
// client auto-retrying with a fresh key after a prior fatal failure).
// Reporting Started here too is harmless and consistent, since the
// operation is naturally idempotent.
```

```go
// Good: states the behavior and its reason, nothing about how it was found.
// If the Session is already RUNNING, a different token's Start already
// won; reporting Started here too is harmless and consistent, since the
// operation is naturally idempotent regardless of who actually caused it.
```

A reader should never be able to tell, from a comment alone, that a review
happened, when a decision was made, or which task/WORK/ADR number produced
the line.

## No Citing Internal Documents As A Stand-In For Explanation

Do not cite an ADR, WORK document, workspace file, or engineering-standard
path (`GAME-ADR-0018`, `ADR-0005`, `docs/engineering/standards/repositories.md`,
`docs/work/...`, anything under `docs/ai/...`) as the reason something is
true. Assume the reader — human or AI, inside this org or outside it —
cannot open that document and has never heard of it, even though it
technically lives in this repository. Whatever reasoning the citation
stands in for must be written out in the comment itself, in plain language,
so the comment is completely self-sufficient with zero knowledge of this
repository's internal decision-tracking or documentation structure.

```go
// Bad: the reasoning lives in a document the reader is expected to already
// know about and go open.
// Mutations against the same Session serialize with one another
// (GAME-ADR-0018).
```

```go
// Good: states the actual guarantee, understandable on its own.
// At most one mutation against a given Session executes at a time; a
// concurrent one waits until this one commits or rolls back.
```

This applies to every kind of internal citation, not only ADRs — a
standards-doc path is exactly as opaque to an outside reader as a WORK or
Slice number. Default to no citation at all. If one is included anyway (for
a maintainer who wants to trace the deeper design record), it must be
strictly supplementary: the sentence must already be complete and correct
with the citation deleted.

## Public API Comments Stay At The Public Contract

A doc comment on an exported symbol that forms part of a package's own public
boundary — a domain's public API, callable without importing that domain's
internal dependencies — must be written entirely from the *caller's*
perspective, the same way a well-written third-party API's documentation is:
you integrate with Stripe to create a charge; its docs tell you what fields
the request needs and what each field of the response means, never that a
charge is durably persisted in their database, which internal service
enqueues it, or which table stores it. You don't need to know any of that to
use the API correctly, and nothing about it changes what you should pass or
how you should read what comes back.

Concretely, an exported doc comment should be answerable from exactly two
questions:

1. What does the caller need to provide, and what constraints/shape does it
   have to satisfy?
2. What does the caller get back, and how should each part of it be
   interpreted?

Anything that does not change the answer to either question is implementation
narration, not contract, and does not belong in the comment — regardless of
whether it names a package. "This package persists X," "internally this
calls Y," "see `pkg.Helper` to build this," and "this uses a queue/cache/
retry loop under the hood" are all the same mistake: they describe how the
callee does its job, which is the callee's problem, not information the
caller needs to call it correctly. The one exception is a guarantee that
*does* change what the caller can rely on — "the response is durably
committed before this call returns, so a concurrent read is guaranteed to
observe it" is caller-relevant (it tells them what they may depend on), even
though it mentions persistence; "this package already persists the same wire
shape" is not (it tells them nothing they can act on).

This also means such a comment must never send the reader to an
`internal/...` package (literally unimportable from outside this module's
owning directory) or to a lower-layer implementation/engine/runtime package
the public API exists specifically to hide, merely to explain how the method
works underneath or how to shape a parameter — if the caller genuinely needs
a specific wire shape, describe that shape directly and self-sufficiently,
without requiring the reader to go call a helper from a different
architectural layer to produce it.

```go
// Bad: leaks a persistence detail with no effect on the caller, and sends
// the reader to a lower-layer package (not even Go-internal, just a
// different architectural layer) to understand a parameter's shape.
// answer is the caller's response payload, encoded in the same wire shape
// this package already persists (see engineservice.EncodeValue/DecodeValue)
// - callers never construct or import an engine-owned value type directly.
```

```go
// Good: says what the caller must provide, self-sufficiently.
// answer is the caller's response, encoded as plain JSON matching the
// interaction's own declared response type - a bare true/false for a
// boolean response, a bare number, a bare quoted string, or a JSON object
// keyed by field name for a record-shaped response, and so on.
```

```go
// Bad: sends the reader to a package they cannot import, and a
// lower-layer type the public API exists to hide, to understand a public
// method.
// ExpireTimer ... drives engine.SignalKindTimerExpired (or
// SignalKindKeyedTimerExpired, for a keyed timer) through the compiled
// Program ... see internal/repo.TimerObligation.UUID.
```

```go
// Good: describes the same behavior entirely in terms of the public
// contract — what the caller passes and what happens as a result.
// ExpireTimer submits the previously scheduled timer identified by
// timerObligationUUID as expired, driving whatever transition the running
// game declared for it.
```

This applies regardless of whether the referenced package is Go-`internal`
(literally inaccessible) or merely a lower architectural layer the public API
was built to abstract away (accessible, but irrelevant to a caller of the
public contract) — either way, the citation asks the reader to understand
something the public symbol was specifically designed to let them not need.
If a maintainer genuinely needs that deeper cross-reference, it belongs on
the unexported implementation this comment sits above (a private helper, or
an internal package's own doc), never on the exported symbol a caller reads.

## Enforcement: Exported Doc Comments Referencing A Lower-Layer Package

`TestExportedDocCommentsStayAtPublicContract` (root package,
`comment_standard_test.go`) mechanically catches the concrete, unambiguous
half of the rule above: an exported declaration's doc comment — on a plain
function, or a method whose receiver type is itself exported (an exported
method on an unexported type is not reachable from outside the package
either way) — in a non-internal, non-test file, referencing a qualified
symbol (`pkgname.Symbol`) from a package the file itself imports whose
import path contains `/internal/`, or that is on a small denylist of known
engine/runtime/infrastructure-layer packages (`engine`, `engineservice`,
`gorm` today — extend this list, with a one-line reason, whenever another
such layer appears, e.g. a future domain's own internal execution engine).
A reference is only flagged when that same package is not already part of
the declaration's own signature (parameters/results, or a type alias's
underlying type) — a caller who already has to see `engine.Program` because
a function returns one is not learning anything new from a comment that also
names it; the violation is a reference the signature does not already force
on the caller. It cannot catch the softer half — implementation narration
that names no package ("this package already persists...", "internally this
retries...") — which remains a code-review responsibility per the
caller-perspective test above.

## No Artificial Coupling

A comment on one symbol should not need to change merely because an
unrelated implementation elsewhere changes. A struct comment should not
enumerate every function that consumes it, how each field maps to a query,
where the value is later persisted, or the full workflow that happens after
construction. Documentation stays local to the abstraction being documented.

## Evaluation Order

1. Can the meaning be made obvious through clear names and code structure?
   If yes, prefer self-documenting code over commentary.
2. If the code is already clear but an important reason, invariant, or
   constraint remains invisible, keep or add a concise comment explaining
   that reasoning.
3. Do not encode hidden implementation history or workflow narration in
   comments.

The goal is not to minimize the absolute number of comments; it is to
eliminate redundant/fragile ones while preserving reasoning the code itself
cannot communicate.

## Enforcement

Code review and this standard are the primary enforcement mechanism; no
general linter checks comment content. Two tests in `comment_standard_test.go`
(root package) run as part of `go test ./...` and mechanically catch two
specific, common violations: `TestNoInternalDocCitationsInComments` catches a
comment citing an ADR/WORK/Blocker/Slice reference or a
`docs/work|engineering|ai/` path as the reason something is true;
`TestExportedDocCommentsStayAtPublicContract` catches an exported
declaration's doc comment referencing a lower-layer/`internal/` package (see
Public API Comments Stay At The Public Contract above). Neither catches most
of what this standard covers (narrating the function body, describing
current callers instead of what a symbol represents, historical "previously
X, now Y" phrasing, implementation narration that names no package, etc.).
Code review remains required for the rest.

During code review, flag in particular:

- a comment narrating the function body instead of its contract;
- a struct/field/type comment describing current callers or consumers
  instead of what the symbol represents;
- an inline comment restating an obvious line ("get X", "check the error");
- a comment referencing a WORK/task/PR/date or describing "previously X, now
  Y" implementation history;
- a comment citing an ADR/WORK/standards-doc path as the reason for
  something, instead of stating the reason itself in plain language;
- an exported symbol's doc comment sending the reader to an `internal/...`
  package or a lower-layer implementation type the public API exists to
  hide, instead of staying in terms of the public contract;
- an exported symbol's doc comment narrating how the implementation does its
  job (persistence, retries, internal calls, queues/caches) with no effect
  on what the caller passes or how it reads the return value - ask "does this
  sentence change either answer?" before keeping it;
- a genuine correctness/concurrency/invariant comment being deleted merely
  to reduce comment count.
