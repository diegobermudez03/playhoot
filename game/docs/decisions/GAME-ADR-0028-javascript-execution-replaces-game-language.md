# GAME-ADR-0028: JavaScript Execution Replaces Game Language (program/engine v1)

Status: ACCEPTED
Created: 2026-09-27
Last status change: 2026-09-27
Supersedes: None
Superseded by: None

## Context

`docs/decisions/architecture/ADR-0015-javascript-rule-execution-and-iframe-frontend-contract.md` accepts, at the architecture-principle level, that Session Runtime's rule-authoring/execution mechanism moves from Game Language (`game/language/v1/program`+`engine`) to sandboxed JavaScript. This record makes that decision concrete for Session Runtime's own execution boundary: what Game Language's engine contract (`game/language/v1/engine/LOGICAL_CONTRACT.md`) currently guarantees, and what its replacement must guarantee instead.

Today's engine contract is: `Definition -> compile -> Program+Diagnostics`; `Program+InitializationInput -> StartTurn -> Outputs`; `Program+Start+priorSignals+newSignal -> AdvanceTurn -> Outputs`. Entry points are Turn-level only; the engine is stateless as a library; execution is deterministic by construction (closed operation set, seeded RNG, no wall clock/network); concurrency is entirely the caller's responsibility (`GAME-ADR-0018`); one workflow instance runs for a whole Session's lifetime (`GAME-ADR-0026`); a per-invocation step budget bounds chained internal signals (`GAME-ADR-0019`, generalized by `GAME-ADR-0027`).

## Decision

### Execution contract

Session Runtime's new execution boundary (owning package to be internal to Session Runtime, per `ADR-0014`) exposes:

```text
execute(previousState, event, context) -> { newState, requestedCommands }
```

- `previousState` is the durably persisted current state (per `GAME-ADR-0029`), passed in explicitly — the runtime never loads it itself.
- `event` is the single externally/internally driving cause for this invocation (an accepted interaction response, a fired timer, a user intent, a session lifecycle signal) — the same category of causes Session Runtime already recognizes (`session_runtime_turns.source_kind` and its satellite tables), reused rather than redesigned by this record.
- `context` carries Playhoot-controlled non-deterministic inputs the authored code needs (current logical time, a seeded random source, the acting player's identity) so authored code never reads real wall-clock/OS-randomness directly.
- `newState` is the full new authoritative state, serializable and size-bounded.
- `requestedCommands` is zero or more declarative commands (schedule/cancel a timer, emit an effect, request a specific view be computed/delivered, request a session-lifecycle transition such as completion/failure/cancellation) — data only, never a callback or host instruction.

The exact wire/serialization shape, the supported JavaScript/TypeScript authoring profile (permitted syntax subset, standard-library surface, module system if any), and the exact command vocabulary are concrete design decisions of the owning WORK (`docs/projects/active/js-runtime-migration/works/`), not fixed here. This record fixes the shape of the contract, not its encoding.

### What is preserved from the current model

- **One execution instance per Session for its whole lifetime, no nested/child instances** (`GAME-ADR-0026`'s flat model) — carried forward unchanged; nothing in the product/architecture pressure motivating this migration requires reintroducing nested execution.
- **Caller-owned concurrency serialization** (`GAME-ADR-0018`) — the new runtime does not serialize calls against the same Session any more than `engineservice` does today; Session Runtime's existing per-Session database locking is unaffected.
- **Root player-roster initialization contract** (`GAME-ADR-0006`) — `InitializationInput`'s `players` roster concept is preserved as part of the new `context`/initial-event shape; the owning WORK confirms its exact representation.
- **Timer ownership** (`GAME-ADR-0008`/`GAME-ADR-0012`) — authored code requests timer scheduling/cancellation as commands; Session Runtime persists and fires timer obligations exactly as it does today, now adapted to the command vocabulary above rather than `engine.ScheduleTimerOutput`/`CancelTimerOutput`. See the owning WORK for the concrete adaptation.
- **A per-invocation execution bound** (`GAME-ADR-0019`/`GAME-ADR-0027`'s step-chain budget) — restated as a resource limit the sandbox enforces (compute time/step count), not a DSL-specific chained-signal count, since JavaScript has no equivalent "internal signal chain" concept to bound the same way.
- **Immutable per-Session version pinning** (`GAME-ADR-0001`) — a running Session's authored script and frontend package remain pinned for its whole lifetime; unaffected in principle by the artifact's content type changing.

### What is retired

- The DSL type system, compiler, and Go-interpreter runtime (`game/language/v1/program`, `game/language/v1/engine/internal/compiler`, `internal/runtime`) are retired once the owning WORK lands; they are not kept as a parallel/fallback execution path.
- The declarative `Projection`/`View`/`Presentation`/`UIElement`/`UILayout`/`UIAction` model is retired as Playhoot's rendering contract. Per-player view computation moves into authored JavaScript's own command output (see Consequences on privacy verification); frontend rendering moves to the iframe-delivered frontend package `ADR-0015` establishes.
- `program.Metadata.LanguageVersion` as Game Language's specific versioning anchor is retired along with the language itself; the new artifact/versioning model (a required capability of the owning Project) defines how a pinned authored version is identified going forward. This is not designed by this record.

### Sandboxing is a separate, infrastructure-owned concern

This record defines the execution *contract* (inputs/outputs/preserved guarantees). It does not define the sandbox technology, isolation boundary, or resource-limit enforcement mechanism — those are `ADR-0015`'s infrastructure-ownership principle, made concrete by their own owning WORK (sandbox runtime selection, isolation boundary, resource limits), tracked separately so a security/infrastructure decision is not silently folded into a language-contract decision.

## Rationale

Keeping the preserved guarantees above at the same conceptual level (one instance per Session, caller-owned concurrency, Playhoot-owned timers, an enforced execution bound, immutable version pinning) means Session Runtime's surrounding lifecycle/persistence/concurrency architecture does not need to be redesigned from scratch merely because the language authored code is written in changes — only the internal execution boundary and the concrete shape of its inputs/outputs change. This keeps the migration's actual blast radius honest: it is a real, substantial engine replacement, not a cosmetic syntax change, but it is not a second, independent redesign of Session Runtime's already-accepted lifecycle/concurrency model.

## Alternatives Considered

### Design the full wire/serialization contract and command vocabulary in this record

Rejected for now. Per `docs/ai/protocols/CONVERSATIONAL_ORCHESTRATOR.md`'s just-in-time design guidance, freezing every field/command shape before the sandbox technology (which materially constrains what's efficient to serialize) is selected risks designing against the wrong constraints. The owning WORK designs the concrete shape once that technology decision is made.

### Keep `engine.Signal`/`engine.Output`'s exact type taxonomy and only change how the transition body is authored

Rejected. `engine.Signal`/`Output`'s current shape is intentionally coupled to the DSL's closed-workflow/transition model (`WorkflowControl`, `TransitionDeclaration` matching). A general-purpose authored function does not have that structure; forcing the new contract into the old taxonomy would preserve incidental DSL shape rather than the actual guarantees worth preserving (enumerated above).

## Consequences

- A privacy-verification mechanism for JS-computed per-player views is a required capability of the owning Project, not optional — the current compiler-enforced `Projection` purity guarantee has no equivalent once authored code computes views directly. See `docs/projects/active/js-runtime-migration/works/WORK-0041-per-player-view-computation-and-privacy-verification.md`.
- A durable command/protocol validation mechanism is required so that both the platform-wide command vocabulary (schedule timer, emit effect, request transition) and any game-specific contract (a specific game's own action/view shapes) are validated at runtime, not only relied upon via static typing — static types in authored TypeScript, if used, do not survive to the sandboxed runtime boundary as a runtime guarantee.
- Every Session Runtime call site that invokes `engineservice.Compile`/`StartTurn`/`AdvanceTurn` today (`step_create.go`, `step_join.go`, `step_start.go`, `step_answer_interaction.go`, `step_submit_user_intent.go`, `step_cancel_session.go`, `step_expire_timer.go`) must be updated to the new execution boundary; this is implementation detail for the owning WORK, not decided here.

## Canonical Knowledge Impact

- `game/README.md`, `game/CURRENT_STATE.md` — Game Language sections superseded once the owning WORK lands; not rewritten by this record directly.
- `game/language/v1/engine/LOGICAL_CONTRACT.md`, `README.md`, `IMPLEMENTATION.md`, `game/language/v1/program/README.md` — describe the engine/language being retired; retained as historical reference until the owning WORK removes/replaces them, per that WORK's own Documentation Impact.
- `game/docs/decisions/INDEX.md` — this record added; annotated relative to the DSL-specific records it restates-at-a-principle-level (see Consequences in `ADR-0015`).

## Implementation Impact

Not authorized by this record. Routed to `docs/projects/active/js-runtime-migration/` (sandbox runtime selection and isolation boundary, resource limits, command/state wire schema, command/protocol validation, timer adaptation, and the per-call-site migration).
