package engine

// AssignmentTarget is the compiled representation of one
// program.AssignmentTarget: a writable location a SetOperation or a
// list/map mutation operation may modify. The compiler guarantees the
// root of every AssignmentTarget is a NameTarget naming "global" or
// "local" — the language's only two mutable roots.
//
// AssignmentTarget is a closed interface, mirroring program's own
// closed-interface pattern.
type AssignmentTarget interface {
	isAssignmentTarget()
}

// NameTarget identifies the mutable root "global" or "local".
type NameTarget struct {
	Name string
}

func (NameTarget) isAssignmentTarget() {}

// FieldTarget accesses the named field Field below Target. The compiler
// guarantees Target's resolved type is a record declaring Field.
type FieldTarget struct {
	Target AssignmentTarget
	Field  string
}

func (FieldTarget) isAssignmentTarget() {}

// IndexTarget accesses the list element or map entry below Target at
// the position or key produced by Index.
type IndexTarget struct {
	Target AssignmentTarget
	Index  Expression
}

func (IndexTarget) isAssignmentTarget() {}

// Block is the compiled representation of one program.Block: an ordered
// sequence of synchronous operations.
type Block struct {
	Operations []Operation
}

// Operation is the compiled representation of one program.Operation.
//
// This version compiles the operations a purely sequential workflow
// needs — lexical bindings, mutation, conditionals, iteration, and
// pattern-matched branching — and does not yet compile an operation
// that opens an interaction, schedules a timer, spawns a child or task,
// or draws a random value; a Block containing one of those is
// diagnosed, at compile time, as using an unsupported operation, and
// does not appear in the compiled Block at all — see engineservice's
// compile_operations.go.
//
// Operation is a closed interface, mirroring program's own
// closed-interface pattern.
type Operation interface {
	isOperation()
}

// LetOperation introduces the immutable lexical binding Name, visible to
// later operations in the same Block (and this Block's owning
// Transition's Control, if this is the transition's top-level Block),
// bound to Value.
type LetOperation struct {
	Name  string
	Value Expression
}

func (LetOperation) isOperation() {}

// SetOperation replaces the value stored at Target with the result of
// Value.
type SetOperation struct {
	Target AssignmentTarget
	Value  Expression
}

func (SetOperation) isOperation() {}

// ListAppendOperation appends the result of Value to the end of the
// list stored at Target.
type ListAppendOperation struct {
	Target AssignmentTarget
	Value  Expression
}

func (ListAppendOperation) isOperation() {}

// ListInsertOperation inserts the result of Value into the list stored
// at Target at the position produced by Index.
type ListInsertOperation struct {
	Target AssignmentTarget
	Index  Expression
	Value  Expression
}

func (ListInsertOperation) isOperation() {}

// ListRemoveAtOperation removes the element at the position produced by
// Index from the list stored at Target.
type ListRemoveAtOperation struct {
	Target AssignmentTarget
	Index  Expression
}

func (ListRemoveAtOperation) isOperation() {}

// MapPutOperation inserts a new entry, or replaces the value of an
// existing entry, for Key in the map stored at Target.
type MapPutOperation struct {
	Target AssignmentTarget
	Key    Expression
	Value  Expression
}

func (MapPutOperation) isOperation() {}

// MapDeleteOperation removes the entry for Key from the map stored at
// Target, if one exists.
type MapDeleteOperation struct {
	Target AssignmentTarget
	Key    Expression
}

func (MapDeleteOperation) isOperation() {}

// IfOperation executes exactly one of Then or Else depending on
// Condition.
type IfOperation struct {
	Condition Expression
	Then      Block
	Else      Block
}

func (IfOperation) isOperation() {}

// ForEachOperation iterates, in order, over a snapshot of the finite
// list produced by evaluating Collection, executing Body once per
// element with ItemName (and, if non-empty, IndexName) bound.
type ForEachOperation struct {
	Collection Expression
	ItemName   string
	IndexName  string
	Body       Block
}

func (ForEachOperation) isOperation() {}

// MatchOperation executes the Body of the first case in Cases whose
// Pattern matches Value — see MatchPattern.
type MatchOperation struct {
	Value Expression
	Cases []MatchOperationCase
}

func (MatchOperation) isOperation() {}

// MatchOperationCase pairs one MatchPattern with the Body executed when
// that pattern is selected. Pattern's lexical bindings, if any, are in
// scope only within Body.
type MatchOperationCase struct {
	Pattern MatchPattern
	Body    Block
}
