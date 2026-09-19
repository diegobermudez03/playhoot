# Function Signature Standard

Status: CANONICAL ENGINEERING STANDARD

This standard owns method/function contract conventions for application, workflow, and use-case code: when to use explicit parameters, when a struct is justified, and how return values should be shaped. It does not own package placement or workflow/use-case classification (`domain-logic-placement.md`).

## Prefer Explicit Parameters Over Ceremonial Input Structs

For application/workflow/use-case methods, prefer explicit parameters over a struct that exists only to bundle arguments:

```go
func (m *Manager) Join(
    ctx context.Context,
    joinCode JoinCode,
    userUUID UserUUID,
    displayName DisplayName,
    idempotencyKey IdempotencyKey,
) error
```

not

```go
func (m *Manager) Join(ctx context.Context, input JoinInput) error
```

when `JoinInput` exists only to group method arguments.

The method signature itself should explicitly document what the operation requires, which values are mandatory, their types, and the dependencies available inside the implementation. A caller should not need to open an `Input`/`Request`/`Params`-style struct merely to discover the actual method contract. Explicit parameters also avoid encouraging partially-populated request structs whose omitted fields silently become Go zero values.

## No Argument-Count Threshold

Do not adopt a rule such as "more than 3/4 parameters requires a struct." There is no numeric threshold. Use a struct when the struct itself represents a meaningful concept, not because several arguments happen to exist.

Valid semantic structs represent a cohesive domain value or a meaningful result — for example `Session`, `Participant`, `GameDefinition`, `Money`, or `CreatedSession`. A struct is justified because the grouped fields collectively represent a concept, not merely because grouping them reduces the parameter count.

## Prefer Semantic Primitive Types Where Useful

When several parameters share the same underlying primitive and ambiguity matters, prefer semantic types over an artificial input struct introduced only to get named fields:

```go
type UserUUID string
type JoinCode string
type IdempotencyKey string
```

Do not mechanically wrap every string in a named type. Introduce a semantic type where it clarifies a domain contract or prevents accidental interchange between values of the same underlying primitive.

## Return Values

Prefer direct return values when they naturally describe the operation:

```go
func Resolve(...) (SessionUUID, GameDefinitionUUID, error)
```

Use a result struct when the result itself is a meaningful, cohesive concept:

```go
type CreatedSession struct {
    SessionUUID SessionUUID
    JoinCode    JoinCode
    ExpiresAt   time.Time
}
```

Do not introduce an `Output`/`Result` struct merely because every method is expected to have one.

## Callback-Based Helpers Return Values, Not Mutate Closures

The same preference for explicit return values over ceremony applies to callback/closure-based helpers, not only to ordinary functions. A generic callback-based helper (for example, a transaction-running helper `RunInTransaction[T](ctx, ..., func(ctx, tx) (T, error))`) should let the callback return its own result directly:

```go
result, err := RunInTransaction(ctx, repo, func(ctx context.Context, tx *gorm.DB) (T, error) {
    ...
    return value, nil
})
```

not

```go
var result T
err := RunInTransaction(ctx, repo, func(ctx context.Context, tx *gorm.DB) error {
    result = value
    return nil
})
```

Declaring an outer-scoped result (or error) variable and mutating it from inside the callback adds a layer of indirection the callback's own return value already provides for free, and invites a second, informal contract (which fields were actually set before returning) that the type signature does not describe. Prefer the generic parameterized return whenever the helper can express it. See `repositories.md -> Transaction Ownership -> Callback Contract: Generic Result, No Outer-Variable Mutation` for the concrete transaction-helper consequence of this rule.

## Enforcement

Code review and this standard. No architecture linter or other automated enforcement tool is introduced by this standard.

During code review, flag in particular:

- an `Input`/`Request`/`Params` struct introduced only to bundle method arguments, with no independent domain meaning;
- a rule that switches to a struct purely once a parameter count is exceeded;
- a callback-based helper whose caller mutates an outer-scoped result/error variable instead of using the callback's own return value (see Callback-Based Helpers Return Values, Not Mutate Closures above);
- a wrapper type introduced for every primitive regardless of whether it clarifies a contract;
- an `Output`/`Result` struct created by convention rather than because the result is a genuine cohesive concept.
