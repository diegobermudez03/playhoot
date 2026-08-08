package program

// Block is an ordered sequence of synchronous, source-level operations.
//
// Operation order is semantically significant and is preserved with a
// slice. A Block also defines a lexical scope: bindings introduced by a
// LetOperation or by a ForEachOperation's item and index names are visible
// to later operations in the same block and to nested blocks, but bindings
// introduced by a nested block do not escape it. This package does not
// enforce scope or name-resolution rules; that is the responsibility of
// the future engine compiler.
//
// All operations in a Block are synchronous: none may suspend execution,
// wait for user input, start a timer, spawn a workflow, or emit network
// traffic. The future engine executes an entire transition's block
// atomically — if a later operation fails, no partial mutation made by
// earlier operations in the same block should become committed. This
// package only declares the operations; it does not implement atomicity or
// execution.
type Block struct {
	Operations []Operation
}

// Operation is a single synchronous, source-level unit of work inside a
// Block.
//
// Operation is a closed interface. Its marker method is unexported so that
// packages outside program cannot introduce unsupported variants; the
// future compiler can safely exhaust all cases with a type switch.
type Operation interface {
	isOperation()
}

// AssignmentTarget identifies a writable location that a SetOperation or a
// list/map mutation operation may modify.
//
// Not every Expression is assignable — AssignmentTarget is a separate,
// closed interface so that arbitrary expressions (literals, calls,
// arithmetic, and so on) cannot be used as mutation targets. Its marker
// method is unexported so that packages outside program cannot introduce
// unsupported variants.
type AssignmentTarget interface {
	isAssignmentTarget()
}

// NameTarget identifies a named writable root or lexical binding, such as
// global or local. The future compiler resolves the name and determines
// whether it refers to mutable state.
type NameTarget struct {
	Name string
}

func (NameTarget) isAssignmentTarget() {}

// FieldTarget accesses a named field below another assignment target.
type FieldTarget struct {
	Target AssignmentTarget
	Field  string
}

func (FieldTarget) isAssignmentTarget() {}

// IndexTarget accesses a list element or map entry below another
// assignment target.
type IndexTarget struct {
	Target AssignmentTarget
	Index  Expression
}

func (IndexTarget) isAssignmentTarget() {}

// LetOperation introduces an immutable lexical binding within the
// enclosing block.
//
// The binding is visible to later operations in the same block and to
// nested blocks, but it can never be assigned through a SetOperation: the
// language has no reference or alias declaration, and entity identity must
// be represented explicitly through IDs, user references, or other
// language values. Persistent mutable values belong in declared state
// (such as global state), not in a lexical binding.
//
// Type is an optional source annotation; a nil Type means the future
// compiler should infer the binding's type from Value.
type LetOperation struct {
	Name  string
	Type  TypeReference
	Value Expression
}

func (LetOperation) isOperation() {}

// SetOperation replaces the value stored at Target with the result of
// Value. The future compiler validates that Target is mutable and that
// Value is compatible with Target's type.
type SetOperation struct {
	Target AssignmentTarget
	Value  Expression
}

func (SetOperation) isOperation() {}

// ListAppendOperation appends the result of Value to the end of the list
// stored at Target. It mutates the target list and does not produce a
// value.
type ListAppendOperation struct {
	Target AssignmentTarget
	Value  Expression
}

func (ListAppendOperation) isOperation() {}

// ListInsertOperation inserts the result of Value into the list stored at
// Target at the position produced by Index. It mutates the target list and
// does not produce a value.
type ListInsertOperation struct {
	Target AssignmentTarget
	Index  Expression
	Value  Expression
}

func (ListInsertOperation) isOperation() {}

// ListRemoveAtOperation removes the element at the position produced by
// Index from the list stored at Target. It mutates the target list and
// does not produce a value.
type ListRemoveAtOperation struct {
	Target AssignmentTarget
	Index  Expression
}

func (ListRemoveAtOperation) isOperation() {}

// MapPutOperation inserts a new entry, or replaces the value of an
// existing entry, for Key in the map stored at Target. It mutates the
// target map and does not produce a value.
type MapPutOperation struct {
	Target AssignmentTarget
	Key    Expression
	Value  Expression
}

func (MapPutOperation) isOperation() {}

// MapDeleteOperation removes the entry for Key from the map stored at
// Target, if one exists. Deleting a missing key is a no-op. It mutates the
// target map and does not produce a value.
type MapDeleteOperation struct {
	Target AssignmentTarget
	Key    Expression
}

func (MapDeleteOperation) isOperation() {}

// IfOperation executes exactly one of Then or Else depending on Condition,
// which must eventually compile to a boolean value.
//
// Each branch creates its own nested lexical scope; an empty branch is
// represented by a Block with no operations. IfOperation controls only
// synchronous operations within a transition's block — it does not select
// workflow transitions or workflow states, which are modeled separately.
type IfOperation struct {
	Condition Expression
	Then      Block
	Else      Block
}

func (IfOperation) isOperation() {}

// ForEachOperation iterates, in order, over a snapshot of the finite list
// produced by evaluating Collection.
//
// Collection is evaluated exactly once when the loop begins; mutating the
// original source list while the loop runs does not change the iteration
// sequence. ItemName is an immutable lexical binding for the current
// element, scoped to the loop body. IndexName is an optional immutable
// lexical binding for the current zero-based index; an empty IndexName
// means no index binding is created. Body creates a nested lexical scope
// for each iteration.
//
// The loop cannot suspend and cannot directly control a workflow; the
// future engine enforces execution and iteration budgets.
type ForEachOperation struct {
	Collection Expression
	ItemName   string
	IndexName  string
	Body       Block
}

func (ForEachOperation) isOperation() {}
