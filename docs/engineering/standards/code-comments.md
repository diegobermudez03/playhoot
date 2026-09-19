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

Code review and this standard. No automated linter enforces comment content.
During code review, flag in particular:

- a comment narrating the function body instead of its contract;
- a struct/field/type comment describing current callers or consumers
  instead of what the symbol represents;
- an inline comment restating an obvious line ("get X", "check the error");
- a comment referencing a WORK/task/PR/date or describing "previously X, now
  Y" implementation history;
- a genuine correctness/concurrency/invariant comment being deleted merely
  to reduce comment count.
