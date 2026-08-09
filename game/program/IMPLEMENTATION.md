Wrote by AI, human dev notes added as `Dev note:`

# Implementation notes: game/program

This document is for people modifying `program`, `program/internal/codec`, or `program/gameservice`. If you're just consuming the language model, read `README.md` instead.

## Package layout and dependency rules

```
game/program/                 pure AST types + Definition. Stdlib only. Imports nothing else in this repo.
game/program/internal/codec/  private wire (JSON) format. Imports program.
game/program/gameservice/     public behavior: EncodeJSON, DecodeJSON, Validate. Imports both program and internal/codec.
```

Dependency direction is one-way: `gameservice -> internal/codec -> program`. `program` never imports anything project-local — this is enforced by convention, not tooling, so don't add an import from `program` to `codec` or `gameservice` even for something that seems convenient (it would create a cycle, since `codec` already imports `program`).

`internal/codec` being under `internal/` means only code rooted at `game/program/` can import it — that's what lets `gameservice` reach it while keeping it hidden from every other consumer.

If you ever need `program` itself to expose a helper that both `codec` and `gameservice` want, it has to live in `program` and be a pure function of `program` types — it can't call into either of the other two packages.

## Why encode/decode/validate aren't methods on `Definition`

They used to be (`Definition.Validate()`, `program.EncodeJSON`), but that's incompatible with the layering above: `program` can't depend on `codec` for encoding logic, and folding all of the codec's ~2800 lines directly into `program` would defeat the point of keeping `program` a minimal, dependency-free AST. So all three are free functions in `gameservice`, taking a `program.Definition` by value (or, for `DecodeJSON`, returning a `*program.Definition`). This also means `gameservice` is free to change how validation is invoked (e.g. take options, return a richer result type) without touching `program` at all.

## File organization

`program`'s root files are organized **by entity**, not by behavior — one file per concept, holding every type, interface, and marker-method implementation for that concept together (e.g. `child_workflow.go` holds `ChildWorkflowSlotDeclaration`, its two operations, and its two signal sources). This was a deliberate consolidation from an earlier split-by-behavior layout (separate `_operation.go` / `_outcome.go` files per entity) — the earlier layout scattered a single concept across 2-3 files for no real benefit, since nothing in this package is large enough to need it. When adding a new concept, follow the same pattern: one file, holding the type(s), the interface implementation(s), and any tightly-coupled operations/signals for that concept.

`internal/codec` mirrors this same one-file-per-concept split (`declaration.go`, `expression.go`, `operation.go`, `workflow.go`, etc.), plus `wire.go` for the shared generic helpers and `error.go` for `DecodeError`.

`gameservice` is flat: `codec.go` (EncodeJSON/DecodeJSON) and `validate.go` (Validate + the whole validator).

## Closed interfaces: the `isX()` marker pattern

Every "one of a fixed set of variants" concept in `program` (26 `Expression` variants, 26 `Operation` variants, etc. — see README for the full table) is a Go interface with a single unexported marker method, e.g.:

```go
type Expression interface {
    isExpression()
}

func (UnitLiteralExpression) isExpression() {}
```

This is standard "sealed interface" Go — since the method is unexported, only types declared inside `program` can implement it, so a type switch over `Expression` inside `program` or `codec` can be treated as exhaustive. All implementations use value receivers, so these are always stored and compared as values, never pointers (the one exception is presentation types where a field is `*QuestionPresentationDeclaration`, used to represent "no presentation" via a nil pointer rather than an interface).

Nothing further needs explaining here if you already know the pattern — the only local convention worth remembering is: **new variants always get a value-receiver marker method**, and the switch statements that consume the interface (in `codec`'s encode/decode and in `gameservice.validate`) must be updated in lockstep, or you'll get a silent "encodes as null" / "always fails validation" bug instead of a compile error, since the interface itself doesn't force switches elsewhere to be exhaustive.

## internal/codec: how the wire format works

### Discriminated unions via `"kind"`

Every closed-interface value encodes as a JSON object with a `"kind"` string field (snake_case, e.g. `"list_map"`, `"binary"`, `"conditional"`) plus whatever fields that variant needs. `wireKind{Kind string}` is used to peek at just the discriminator before committing to a full strict decode.

### The `wireXxx` mirror structs

Each concrete `program` type that needs custom wire representation gets a private `wireXxx` struct in `codec` with `json:"snake_case"` tags. Fields that hold another closed-interface value (nested `Expression`, `Operation`, etc.) are typed `json.RawMessage` — they're decoded recursively through the dispatch helpers below rather than generically unmarshaled. Fields that hold an ordinary (non-interface) nested struct, like `MapEntryExpression` or `FieldInitializer`, are typed directly as their `wireXxx` struct — no need for the raw-message indirection since there's no discriminator to resolve.

### Generic dispatch helpers (`wire.go`)

- **`decodeUnion[T]`**: the shared entry point for decoding any closed-interface field.
  1. JSON-null or missing input → returns the zero value of `T` with a `nil` error. This is how "optional closed-interface field" is represented — e.g. a nil `Guard` on a transition, a nil `Else` on a conditional.
  2. Otherwise, decodes exactly one top-level JSON value (rejecting trailing bytes via `json.Decoder.More()`).
  3. Reads `"kind"`.
  4. Calls a per-family `dispatch(path, kind, raw)` function that switches on `kind`, strict-decodes into the matching `wireXxx` (via `strictDecodeInto`, which sets `DisallowUnknownFields()` and also rejects trailing bytes), then recursively resolves any nested raw-message fields and builds the real `program.X{...}` value.
  5. An unrecognized `kind` is a hard decode error (`unsupported <family> kind %q`) — unknown discriminators are never silently dropped.
- **`encodeUnion`**: the encode-side mirror.
  1. `dereferencePointer` normalizes a nil interface _or_ a typed-nil pointer (e.g. `(*QuestionPresentationDeclaration)(nil)`) into a single "is nil" signal — this is the only place in the codec that uses reflection, and only for this nil-normalization, never for generic field serialization.
  2. Nil → literal JSON `null`.
  3. Otherwise, type-switches over every concrete variant and marshals the corresponding `wireXxx{Kind: "...", ...}`, recursing into nested fields as needed.

### Ordinary objects vs. union members

Plain structs that are never themselves a closed-interface member (`program.Definition`, `program.Metadata`, `program.Block`, `program.SignalPattern`, `program.StateDeclaration`, ...) go through `decodeOrdinaryObject` instead of `decodeUnion`. The key difference: **JSON `null` is invalid** for these — it's a structural error, not a valid "zero value" encoding, matching the "Metadata and GlobalState must always be ordinary objects, never null" convention documented on `Definition`.

### Path-aware errors

`DecodeError{Path, Message, Cause}` implements `error` and `Unwrap()`. Every recursive decode helper threads a `path string` argument, built via `pathField(path, "field")` (dot-append) and `pathIndex(path, i)` (bracket-append), so a failure deep in a workflow reports something like `$.workflows[0].states[2].transitions[0].control.cases[1].control`, not just "decode error." When adding a new decode helper, always take and thread `path` the same way — don't decode a nested value without extending the path first, or errors from deep inside that value will report the wrong location.

`DecodeError` is scoped to _structural_ problems only — malformed JSON, wrong types, unknown discriminators, missing required fields. It deliberately never reports on language-semantic problems (unknown built-in type name, duplicate declaration name, bad operator/operand combination) — those are `gameservice.ValidationError`'s job. Don't blur this line by adding semantic checks into the codec.

### Normalization guarantees worth preserving

- **nil vs. empty slice**: helpers like `encodeTypeDeclarationSlice`/`decodeTypeDeclarationSlice` explicitly special-case `nil` before allocating, so a nil slice round-trips distinctly from an empty one. Any new slice-of-interface helper should follow the same `if items == nil { return nil, nil }` guard at the top.
- **Order preservation**: everything decodes from JSON arrays into Go slices index-by-index — no sorting, no map-keyed dedup. This is required, not incidental: `program`'s own doc comments call out that declaration order and duplicate names must survive so a future compiler can report them deterministically.
- **Strict unknown-field / trailing-data rejection**: applied uniformly, both at the top level (`gameservice.DecodeJSON` rejects anything other than exactly one JSON object) and at every nested node (`strictDecodeInto`). If you add a new wire struct, decode it through the existing helpers rather than a bare `json.Unmarshal`, or you'll silently lose this strictness.
- **Canonical field order**: `wireDefinition`'s field order (`metadata, types, resources, global_state, functions, invariants, projections, views, presentation_slots, user_intents, questions, effects, root_workflow, workflows`) is what `EncodeJSON`'s "identical bytes on repeated encode" guarantee relies on — Go's `encoding/json` emits struct fields in declaration order. Don't reorder `wireDefinition`'s fields without treating it as a breaking wire-format change.

## gameservice.Validate: what it actually checks and how

`Validate` builds an internal `validator{definition, typeNames map[string]bool, errors []error}` and walks the whole `Definition` top to bottom: type declarations, then duplicate-name checks across every namespace that has one (types, resources, functions, projections, views, workflows, questions, effects, user intents, presentation slots each get their own duplicate check, since names only need to be unique within their own namespace, not globally), then resources, global state, functions, invariants, projections, views (including their UI element/layout/action trees), and finally every workflow (slots, presentations, states, transitions, operations, controls, completion policies, random generators).

### Static type inference is intentionally shallow

`inferType` only figures out a type when it's directly recoverable from the expression's own shape: literals, explicit constructors (`RecordExpression`, `EnumValueExpression`, `ListExpression`, ...), and operators applied to already-inferred operands. It deliberately returns "unknown" (skips the check rather than guessing) for `ReferenceExpression`, `FieldExpression`, `IndexExpression`, `CallExpression`, `ListMapExpression`, and `ListFlatMapExpression` — all of these require resolving a name or a function/projection signature, which needs the lexical-scope and symbol-table machinery that belongs to the future engine, not to this validator. If you're tempted to extend `inferType` to handle one of these, stop and check whether you're accidentally building reference resolution into `gameservice` — that's explicitly out of scope (see `ValidationError`'s doc comment).

Because of this, `Validate` returning `nil` is a weaker guarantee than "this definition compiles." It only means: no rule _this package_ checks was violated. Don't let that guarantee get stronger by accident (e.g. by having `Validate` start reporting on something it can't fully verify) — under-reporting a real problem is fine here, over-claiming correctness is not.

### Where to add a new validation rule

If a new rule is about type/operator compatibility or duplicate names and is fully decidable from the `Definition` alone (no scope/reference resolution needed), add it as another `validate*`/`infer*` method on `validator` in `gameservice/validate.go`, following the existing methods' shape: take a `path string` for error reporting, call `v.addf(path, "...", args...)` on failure, and recurse into children the same way the sibling methods do. If the rule needs to know what a name actually resolves to, it belongs in the future engine, not here.

## Testing conventions

- `program`'s own tests (`expression_test.go`, `types_test.go`, `view_test.go`) are minimal — this package is mostly plain data structs, so tests only exist where there's actual logic (e.g. `IsValid()` methods on the string-enum types like `BuiltinType`, `BinaryOperator`). Don't add tests for struct literals that just hold fields; there's nothing to break.
- `internal/codec` has the bulk of the test coverage (`declaration_test.go`, `execution_test.go`, `foundation_test.go`, `ui_test.go`, `workflow_test.go`) since the encode/decode logic is where real behavior — and real bugs (missing kinds, wrong nil handling, path construction) — lives. New wire types should get a round-trip test (encode then decode, assert equality) plus a malformed-input test (bad kind, missing required field, wrong JSON type) following the existing files' pattern.
- `gameservice`'s tests (`gameservice_test.go`, `validate_test.go`) exercise the public API end to end and the validator's rule coverage respectively.

## Adding a new language concept, end to end

1. Add the type(s) to `program`, in a new or existing entity file, with an `isX()` marker method if it's a variant of a closed interface. Zero project imports, stdlib only.
2. Add a `wireXxx` struct and encode/decode functions to the matching file in `internal/codec`, wire it into the relevant `dispatch`/`encode` switch if it's a union member, and give it a `"kind"` string.
3. If the concept has type/operator/duplicate-name rules that are statically decidable, add a `validate*` method to `gameservice/validate.go` and call it from the right place in the existing traversal.
4. Add round-trip + malformed-input tests in `internal/codec`, and rule-coverage tests in `gameservice` if you added validation.
5. Update `program/README.md` if the concept is something callers need to know about (most are).
