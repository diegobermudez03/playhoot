package program

// This file adds pure, bounded query expressions over finite lists:
// ListMapExpression, ListFilterExpression, ListFlatMapExpression,
// ListAnyExpression, ListAllExpression, ListCountExpression, and
// ListFirstExpression. Each is an ordinary closed Expression variant —
// there is no separate shared query interface.
//
// # Common semantics
//
// Every list-query expression evaluates its Collection exactly once,
// requires the result to be a finite list, and iterates a stable snapshot
// of that list in source order — later mutation of the original
// authoritative state never changes an already-evaluated query's
// iteration. Each iteration introduces immutable lexical bindings visible
// only inside that expression's Result or Predicate: ItemName for the
// current element (typed as the list's element type), and, when non-empty,
// IndexName for the current zero-based index (typed number). An empty
// IndexName means no index binding is introduced. These bindings are not
// state, are never persisted, can never be assigned through SetOperation,
// and do not escape the query expression that introduced them; this
// package preserves duplicate or conflicting binding names so the future
// compiler can report deterministic diagnostics.
//
// Every list-query expression is pure: it cannot mutate global,
// workflow-local, or view-local state, open questions, schedule timers,
// spawn workflows, emit effects, draw random values, or perform external
// I/O — the future compiler is responsible for validating that every
// called function or built-in used inside a query is itself pure. Query
// expressions are deterministic: given the same compiled program, input
// values, and immutable resources, the same query always produces the
// same result, independent of Go map iteration order, goroutine
// scheduling, wall-clock time, or operating-system randomness. This step
// supports finite lists only — it adds no direct map-iteration expression;
// a deterministic map-entry expression, if ever needed, would be
// introduced as its own dedicated construct later.
//
// List-query expressions may be nested and composed freely, including
// with MatchExpression: an outer binding remains visible inside a nested
// query unless shadowed, and an inner query's bindings never escape it.
// Because these are ordinary Expression values, they may be used anywhere
// an Expression is otherwise legal (pure function bodies, invariant
// conditions, projection bodies, question validation, transition guards,
// operation values, workflow-control expressions, presentation and view
// expressions, UI properties and action values), and the surrounding
// context's existing scope and purity rules still apply unchanged — a
// pure function still cannot reference global just because it contains a
// query, and so on.
//
// # Relationship to ForEachOperation
//
// Use a list-query expression when the game needs a pure value (a
// filtered or transformed list, a boolean quantification, a count, or the
// first matching item). Use ForEachOperation when the game needs to
// execute synchronous operations per element (mutating state, emitting
// effects, opening interactions, spawning task-group children). No
// list-query expression contains an operation block, and this package
// adds no operation variant for list queries — pure collection
// transformation remains part of the expression model only.
//
// # Relationship to built-in and pattern-matching constructs
//
// Scalar and general collection built-ins (length, sum, minimum, maximum,
// format, and similar) remain ordinary CallExpression calls, to be defined
// later by the engine. map, filter, flatMap, any, all, count, and first
// are not represented as such calls because they introduce lexical
// bindings and have their own fixed evaluation semantics — that is why
// each has a dedicated, typed AST variant instead.

// ListMapExpression transforms every element of Collection into one
// Result value, producing a new list.
//
// Result is evaluated once per source element; the output list has the
// same length and order as the source list, and an empty source produces
// an empty result list. If Collection has type list<Token> and Result
// produces TokenId, the expression's result type is list<TokenId>; the
// future compiler infers this from Result, so this type has no explicit
// result-type field.
type ListMapExpression struct {
	Collection Expression
	ItemName   string
	IndexName  string
	Result     Expression
}

func (ListMapExpression) isExpression() {}

// ListFilterExpression retains the elements of Collection for which
// Predicate evaluates to true, preserving source order and the original
// element type (list<Participant> in, list<Participant> out) — a filter
// never transforms retained values; use ListMapExpression for that.
// Predicate must eventually compile to bool. An empty source, or a source
// with no element satisfying Predicate, produces an empty list.
type ListFilterExpression struct {
	Collection Expression
	ItemName   string
	IndexName  string
	Predicate  Expression
}

func (ListFilterExpression) isExpression() {}

// ListFlatMapExpression maps each element of Collection to a list via
// Result and concatenates all of those lists, in source-element order,
// into one flat result list — element order within each per-element list
// is also preserved. If Collection has type list<Team> and Result
// produces list<User>, the expression's result type is list<User>. An
// empty source produces an empty list, and an element whose Result is an
// empty list contributes nothing. This package adds no separate
// general-purpose flatten expression; ListFlatMapExpression is the only
// flattening construct.
type ListFlatMapExpression struct {
	Collection Expression
	ItemName   string
	IndexName  string
	Result     Expression
}

func (ListFlatMapExpression) isExpression() {}

// ListAnyExpression evaluates to true if Predicate is true for at least
// one element of Collection, evaluated in source order, stopping (and
// producing true) at the first element for which Predicate is true. It
// evaluates to false for an empty Collection. Because evaluation stops
// early, a later element's Predicate that would otherwise fail (for
// example an invalid index access) is never evaluated. Predicate must
// eventually compile to bool.
type ListAnyExpression struct {
	Collection Expression
	ItemName   string
	IndexName  string
	Predicate  Expression
}

func (ListAnyExpression) isExpression() {}

// ListAllExpression evaluates to true if Predicate is true for every
// element of Collection, evaluated in source order, stopping (and
// producing false) at the first element for which Predicate is false. It
// evaluates to true for an empty Collection, following standard universal
// quantification over an empty set. Predicate must eventually compile to
// bool.
type ListAllExpression struct {
	Collection Expression
	ItemName   string
	IndexName  string
	Predicate  Expression
}

func (ListAllExpression) isExpression() {}

// ListCountExpression evaluates to the built-in number of elements in
// Collection for which Predicate is true.
//
// Unlike ListAnyExpression and ListAllExpression, this expression never
// short-circuits — Predicate is evaluated for every element — and it
// preserves no elements and creates no list; it only counts. An empty
// Collection evaluates to 0. Predicate must eventually compile to bool.
type ListCountExpression struct {
	Collection Expression
	ItemName   string
	IndexName  string
	Predicate  Expression
}

func (ListCountExpression) isExpression() {}

// ListFirstExpression evaluates, in source order, to the first element of
// Collection for which Predicate is true, wrapped as a present optional
// value, stopping at that first match. If no element matches, or
// Collection is empty, it evaluates to an absent optional. If Collection
// has type list<Participant>, the expression's result type is
// optional<Participant> — the original matching element is returned
// unchanged, never a transformed value, and this type has no default-value
// field. To transform the selected value, combine ListFirstExpression with
// MatchExpression over its optional result, a pure helper function, or
// another surrounding expression. Predicate must eventually compile to
// bool.
type ListFirstExpression struct {
	Collection Expression
	ItemName   string
	IndexName  string
	Predicate  Expression
}

func (ListFirstExpression) isExpression() {}
