package program

import (
	"fmt"
	"reflect"
)

// ValidationError describes one violation of the game language's own
// rules — type and operator compatibility, named-type resolution, and
// declaration-structure rules such as duplicate names — found by
// Definition.Validate.
//
// ValidationError never reports lexical-scope, reference-resolution, or
// execution problems (an unresolved ReferenceExpression, an unknown
// function call, an unreachable match case, and so on); those require the
// runtime and compiled-symbol context a future engine provides, not
// anything program itself declares.
type ValidationError struct {
	Path    string
	Message string
}

// Error implements the error interface.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

func (v *validator) addf(path, format string, args ...any) {
	v.errors = append(v.errors, &ValidationError{Path: path, Message: fmt.Sprintf(format, args...)})
}

// Validate checks d against the rules the game language itself defines —
// operator/operand type compatibility, named-type resolution, and
// declaration-structure rules such as duplicate names within one
// namespace — and returns every violation found, or nil if none.
//
// Validate is intentionally narrow in scope. It never resolves lexical
// scope or references: a ReferenceExpression, a FieldExpression, an
// IndexExpression's target, or a CallExpression's target function are
// never inspected, because resolving them requires the symbol tables and
// runtime context a future engine's compiler builds, not information the
// source model owns by itself. Concretely, Validate only flags a type or
// operator mismatch when the operand's type can be determined directly
// from the expression itself (a literal, a named-type construction such
// as an enum value or record, a nested operator's known result type, and
// so on) — an expression whose type depends on resolving a name is simply
// left unchecked, not assumed correct.
//
// A nil result does not mean d is fully compilable: it only means d does
// not violate any rule this package itself owns. Reference resolution,
// exhaustiveness, reachability, structured-concurrency lifecycle rules,
// and every other diagnostic described as "the future engine compiler
// owns this" throughout this package's documentation remain entirely the
// responsibility of that future compiler.
func (d Definition) Validate() []error {
	v := &validator{definition: d}
	v.collectNames()

	v.validateTypeDeclarations()
	v.validateNamespaces()
	v.validateResources()
	v.validateGlobalState()
	v.validateFunctions()
	v.validateInvariants()
	v.validateProjections()
	v.validateViews()
	v.validateWorkflows()

	return v.errors
}

type validator struct {
	definition Definition
	typeNames  map[string]bool
	errors     []error
}

// --- name collection and namespace/duplicate checks ---

func (v *validator) collectNames() {
	v.typeNames = make(map[string]bool, len(v.definition.Types))
	for _, t := range v.definition.Types {
		switch tt := t.(type) {
		case EnumTypeDeclaration:
			v.typeNames[tt.Name] = true
		case RecordTypeDeclaration:
			v.typeNames[tt.Name] = true
		case UnionTypeDeclaration:
			v.typeNames[tt.Name] = true
		case NewTypeDeclaration:
			v.typeNames[tt.Name] = true
		}
	}
}

func checkDuplicateNames(names []string, pathPrefix, kind string, v *validator) {
	seen := make(map[string]int, len(names))
	for i, name := range names {
		if first, ok := seen[name]; ok {
			v.addf(fmt.Sprintf("%s[%d]", pathPrefix, i), "duplicate %s name %q (first declared at %s[%d])", kind, name, pathPrefix, first)
			continue
		}
		seen[name] = i
	}
}

func (v *validator) validateNamespaces() {
	typeNames := make([]string, len(v.definition.Types))
	for i, t := range v.definition.Types {
		typeNames[i] = declarationName(t)
	}
	checkDuplicateNames(typeNames, "$.types", "type", v)

	resourceNames := make([]string, len(v.definition.Resources))
	for i, r := range v.definition.Resources {
		resourceNames[i] = r.Name
	}
	checkDuplicateNames(resourceNames, "$.resources", "resource", v)

	functionNames := make([]string, len(v.definition.Functions))
	for i, f := range v.definition.Functions {
		functionNames[i] = f.Name
	}
	checkDuplicateNames(functionNames, "$.functions", "function", v)

	projectionNames := make([]string, len(v.definition.Projections))
	for i, p := range v.definition.Projections {
		projectionNames[i] = p.Name
	}
	checkDuplicateNames(projectionNames, "$.projections", "projection", v)

	viewNames := make([]string, len(v.definition.Views))
	for i, view := range v.definition.Views {
		viewNames[i] = view.Name
	}
	checkDuplicateNames(viewNames, "$.views", "view", v)

	workflowNames := make([]string, len(v.definition.Workflows))
	for i, w := range v.definition.Workflows {
		workflowNames[i] = w.Name
	}
	checkDuplicateNames(workflowNames, "$.workflows", "workflow", v)

	questionNames := make([]string, len(v.definition.Questions))
	for i, q := range v.definition.Questions {
		questionNames[i] = q.Name
	}
	checkDuplicateNames(questionNames, "$.questions", "question", v)

	effectNames := make([]string, len(v.definition.Effects))
	for i, e := range v.definition.Effects {
		effectNames[i] = e.Name
	}
	checkDuplicateNames(effectNames, "$.effects", "effect", v)

	intentNames := make([]string, len(v.definition.UserIntents))
	for i, ui := range v.definition.UserIntents {
		intentNames[i] = ui.Name
	}
	checkDuplicateNames(intentNames, "$.user_intents", "user intent", v)

	slotNames := make([]string, len(v.definition.PresentationSlots))
	for i, s := range v.definition.PresentationSlots {
		slotNames[i] = s.Name
	}
	checkDuplicateNames(slotNames, "$.presentation_slots", "presentation slot", v)
}

func declarationName(t TypeDeclaration) string {
	switch tt := t.(type) {
	case EnumTypeDeclaration:
		return tt.Name
	case RecordTypeDeclaration:
		return tt.Name
	case UnionTypeDeclaration:
		return tt.Name
	case NewTypeDeclaration:
		return tt.Name
	default:
		return ""
	}
}

// --- type declarations ---

func (v *validator) validateTypeDeclarations() {
	for i, t := range v.definition.Types {
		path := fmt.Sprintf("$.types[%d]", i)
		switch tt := t.(type) {
		case EnumTypeDeclaration:
			names := make([]string, len(tt.Values))
			for j, val := range tt.Values {
				names[j] = val.Name
			}
			checkDuplicateNames(names, path+".values", "enum value", v)
		case RecordTypeDeclaration:
			v.validateFieldDeclarations(tt.Fields, path+".fields")
		case UnionTypeDeclaration:
			variantNames := make([]string, len(tt.Variants))
			for j, variant := range tt.Variants {
				variantNames[j] = variant.Name
				v.validateFieldDeclarations(variant.Fields, fmt.Sprintf("%s.variants[%d].fields", path, j))
			}
			checkDuplicateNames(variantNames, path+".variants", "union variant", v)
		case NewTypeDeclaration:
			v.validateTypeReference(tt.Underlying, path+".underlying")
		}
	}
}

func (v *validator) validateFieldDeclarations(fields []FieldDeclaration, pathPrefix string) {
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = f.Name
		v.validateTypeReference(f.Type, fmt.Sprintf("%s[%d].type", pathPrefix, i))
	}
	checkDuplicateNames(names, pathPrefix, "field", v)
}

// validateTypeReference checks that ref, if present, uses a valid
// built-in type or refers to a declared type by name. It does not require
// ref to be non-nil: an absent type reference is a separate, already
// representable concern this package does not treat as invalid on its
// own.
func (v *validator) validateTypeReference(ref TypeReference, path string) {
	switch t := ref.(type) {
	case nil:
		return
	case BuiltinTypeReference:
		if !t.Type.IsValid() {
			v.addf(path, "unknown built-in type %q", t.Type)
		}
	case NamedTypeReference:
		if t.Name == "" {
			v.addf(path, "named type reference has an empty name")
			return
		}
		if !v.typeNames[t.Name] {
			v.addf(path, "reference to undeclared type %q", t.Name)
		}
	case ListTypeReference:
		v.validateTypeReference(t.Element, path+".element")
	case MapTypeReference:
		v.validateTypeReference(t.Key, path+".key")
		v.validateTypeReference(t.Value, path+".value")
	case OptionalTypeReference:
		v.validateTypeReference(t.Element, path+".element")
	}
}

// --- resources, global state ---

func (v *validator) validateResources() {
	for i, r := range v.definition.Resources {
		path := fmt.Sprintf("$.resources[%d]", i)
		v.validateTypeReference(r.Type, path+".type")
		v.validateExpression(r.Value, path+".value")
	}
}

func (v *validator) validateGlobalState() {
	v.validateStateDeclaration(v.definition.GlobalState, "$.global_state")
}

func (v *validator) validateStateDeclaration(state StateDeclaration, path string) {
	names := make([]string, len(state.Fields))
	for i, field := range state.Fields {
		names[i] = field.Name
		fieldPath := fmt.Sprintf("%s.fields[%d]", path, i)
		v.validateTypeReference(field.Type, fieldPath+".type")
		v.validateExpression(field.Initializer, fieldPath+".initializer")
	}
	checkDuplicateNames(names, path+".fields", "state field", v)
}

// --- functions, invariants, projections ---

func (v *validator) validateFunctions() {
	for i, f := range v.definition.Functions {
		path := fmt.Sprintf("$.functions[%d]", i)
		v.validateFieldDeclarations(f.Parameters, path+".parameters")
		v.validateTypeReference(f.ResultType, path+".result_type")
		v.validateExpression(f.Body, path+".body")
	}
}

func (v *validator) validateInvariants() {
	for i, inv := range v.definition.Invariants {
		v.validateExpression(inv.Condition, fmt.Sprintf("$.invariants[%d].condition", i))
	}
}

func (v *validator) validateProjections() {
	for i, p := range v.definition.Projections {
		path := fmt.Sprintf("$.projections[%d]", i)
		v.validateFieldDeclarations(p.Parameters, path+".parameters")
		v.validateTypeReference(p.ResultType, path+".result_type")
		v.validateExpression(p.Body, path+".body")
	}
}

// --- views ---

func (v *validator) validateViews() {
	for i, view := range v.definition.Views {
		path := fmt.Sprintf("$.views[%d]", i)
		v.validateTypeReference(view.ModelType, path+".model_type")
		v.validateStateDeclaration(view.LocalState, path+".local_state")
		v.validateUIElement(view.Root, path+".root")
	}
}

func (v *validator) validateUIElement(element UIElement, path string) {
	switch e := element.(type) {
	case nil:
		return
	case EmptyElement:
	case ContainerElement:
		v.validateUIElementConfiguration(e.Configuration, path+".configuration")
		v.validateUILayout(e.Layout, path+".layout")
		for i, child := range e.Children {
			v.validateUIElement(child, fmt.Sprintf("%s.children[%d]", path, i))
		}
	case TextElement:
		v.validateUIElementConfiguration(e.Configuration, path+".configuration")
		v.validateExpression(e.Value, path+".value")
	case ImageElement:
		v.validateUIElementConfiguration(e.Configuration, path+".configuration")
		v.validateExpression(e.Source, path+".source")
		v.validateExpression(e.AlternativeText, path+".alternative_text")
	case ButtonElement:
		v.validateUIElementConfiguration(e.Configuration, path+".configuration")
		for i, child := range e.Children {
			v.validateUIElement(child, fmt.Sprintf("%s.children[%d]", path, i))
		}
	case RepeatElement:
		v.validateExpression(e.Collection, path+".collection")
		v.validateExpression(e.Key, path+".key")
		v.validateUIElement(e.Body, path+".body")
	case ConditionalElement:
		v.validateExpression(e.Condition, path+".condition")
		v.validateUIElement(e.Then, path+".then")
		v.validateUIElement(e.Else, path+".else")
	}
}

func (v *validator) validateUILayout(layout UILayout, path string) {
	switch l := layout.(type) {
	case nil:
		return
	case StackLayout:
	case AbsoluteLayout:
	case LinearLayout:
		if !l.Direction.IsValid() {
			v.addf(path+".direction", "unknown linear layout direction %q", l.Direction)
		}
		v.validateExpression(l.Gap, path+".gap")
	case GridLayout:
		v.validateExpression(l.Columns, path+".columns")
		v.validateExpression(l.RowGap, path+".row_gap")
		v.validateExpression(l.ColumnGap, path+".column_gap")
	}
}

func (v *validator) validateUIElementConfiguration(config UIElementConfiguration, path string) {
	for i, prop := range config.Properties {
		v.validateExpression(prop.Value, fmt.Sprintf("%s.properties[%d].value", path, i))
	}
	for i, handler := range config.Events {
		handlerPath := fmt.Sprintf("%s.events[%d]", path, i)
		if !handler.Event.IsValid() {
			v.addf(handlerPath+".event", "unknown UI event type %q", handler.Event)
		}
		for j, action := range handler.Actions {
			v.validateUIAction(action, fmt.Sprintf("%s.actions[%d]", handlerPath, j))
		}
	}
}

func (v *validator) validateUIAction(action UIAction, path string) {
	switch a := action.(type) {
	case nil:
		return
	case SetLocalStateAction:
		v.validateAssignmentTarget(a.Target, path+".target")
		v.validateExpression(a.Value, path+".value")
	case AnswerQuestionAction:
		v.validateExpression(a.Value, path+".value")
	case EmitUserIntentAction:
		for i, arg := range a.Arguments {
			v.validateExpression(arg.Value, fmt.Sprintf("%s.arguments[%d].value", path, i))
		}
	}
}

func (v *validator) validateAssignmentTarget(target AssignmentTarget, path string) {
	switch t := target.(type) {
	case nil:
		return
	case NameTarget:
	case FieldTarget:
		v.validateAssignmentTarget(t.Target, path+".target")
	case IndexTarget:
		v.validateAssignmentTarget(t.Target, path+".target")
		v.validateExpression(t.Index, path+".index")
	}
}

// --- workflows ---

func (v *validator) validateWorkflows() {
	for i, w := range v.definition.Workflows {
		path := fmt.Sprintf("$.workflows[%d]", i)
		v.validateFieldDeclarations(w.Parameters, path+".parameters")
		v.validateTypeReference(w.ResultType, path+".result_type")
		v.validateStateDeclaration(w.LocalState, path+".local_state")

		for j, slot := range w.TaskGroupSlots {
			v.validateTypeReference(slot.KeyType, fmt.Sprintf("%s.task_group_slots[%d].key_type", path, j))
		}
		for j, slot := range w.QuestionSlots {
			v.validateQuestionPresentation(slot.Presentation, fmt.Sprintf("%s.question_slots[%d].presentation", path, j))
		}
		for j, slot := range w.AskGroupSlots {
			v.validateQuestionPresentation(slot.Presentation, fmt.Sprintf("%s.ask_group_slots[%d].presentation", path, j))
		}

		for j, p := range w.Presentations {
			v.validatePresentation(p, fmt.Sprintf("%s.presentations[%d]", path, j))
		}

		stateNames := make([]string, len(w.States))
		for j, state := range w.States {
			stateNames[j] = state.Name
			v.validateWorkflowState(state, fmt.Sprintf("%s.states[%d]", path, j))
		}
		checkDuplicateNames(stateNames, path+".states", "workflow state", v)

		for j, t := range w.GlobalTransitions {
			v.validateTransition(t, fmt.Sprintf("%s.global_transitions[%d]", path, j))
		}
	}
}

func (v *validator) validateQuestionPresentation(presentation *QuestionPresentationDeclaration, path string) {
	if presentation == nil {
		return
	}
	for i, arg := range presentation.ProjectionArguments {
		v.validateExpression(arg.Value, fmt.Sprintf("%s.projection_arguments[%d].value", path, i))
	}
}

func (v *validator) validatePresentation(p PresentationDeclaration, path string) {
	v.validateExpression(p.Targets, path+".targets")
	for i, arg := range p.ProjectionArguments {
		v.validateExpression(arg.Value, fmt.Sprintf("%s.projection_arguments[%d].value", path, i))
	}
}

func (v *validator) validateWorkflowState(state WorkflowStateDeclaration, path string) {
	for i, p := range state.Presentations {
		v.validatePresentation(p, fmt.Sprintf("%s.presentations[%d]", path, i))
	}
	for i, t := range state.Transitions {
		v.validateTransition(t, fmt.Sprintf("%s.transitions[%d]", path, i))
	}
}

func (v *validator) validateTransition(t TransitionDeclaration, path string) {
	v.validateSignalPattern(t.Signal, path+".signal")
	v.validateExpression(t.Guard, path+".guard")
	v.validateBlock(t.Operations, path+".operations")
	v.validateWorkflowControl(t.Control, path+".control")
}

func (v *validator) validateSignalPattern(pattern SignalPattern, path string) {
	names := make([]string, len(pattern.Bindings))
	for i, b := range pattern.Bindings {
		names[i] = b.Name
	}
	checkDuplicateNames(names, path+".bindings", "signal binding", v)
}

// --- operations, controls ---

func (v *validator) validateBlock(block Block, path string) {
	for i, op := range block.Operations {
		v.validateOperation(op, fmt.Sprintf("%s.operations[%d]", path, i))
	}
}

func (v *validator) validateOperation(op Operation, path string) {
	switch o := op.(type) {
	case nil:
		return
	case LetOperation:
		v.validateTypeReference(o.Type, path+".type")
		v.validateExpression(o.Value, path+".value")
	case SetOperation:
		v.validateAssignmentTarget(o.Target, path+".target")
		v.validateExpression(o.Value, path+".value")
	case ListAppendOperation:
		v.validateAssignmentTarget(o.Target, path+".target")
		v.validateExpression(o.Value, path+".value")
	case ListInsertOperation:
		v.validateAssignmentTarget(o.Target, path+".target")
		v.validateExpression(o.Index, path+".index")
		v.validateExpression(o.Value, path+".value")
	case ListRemoveAtOperation:
		v.validateAssignmentTarget(o.Target, path+".target")
		v.validateExpression(o.Index, path+".index")
	case MapPutOperation:
		v.validateAssignmentTarget(o.Target, path+".target")
		v.validateExpression(o.Key, path+".key")
		v.validateExpression(o.Value, path+".value")
	case MapDeleteOperation:
		v.validateAssignmentTarget(o.Target, path+".target")
		v.validateExpression(o.Key, path+".key")
	case IfOperation:
		v.validateExpression(o.Condition, path+".condition")
		v.validateBlock(o.Then, path+".then")
		v.validateBlock(o.Else, path+".else")
	case ForEachOperation:
		v.validateExpression(o.Collection, path+".collection")
		v.validateBlock(o.Body, path+".body")
	case MatchOperation:
		v.validateExpression(o.Value, path+".value")
		for i, c := range o.Cases {
			v.validateBlock(c.Body, fmt.Sprintf("%s.cases[%d].body", path, i))
		}
	case OpenQuestionOperation:
		v.validateExpression(o.Recipient, path+".recipient")
		for i, arg := range o.Arguments {
			v.validateExpression(arg.Value, fmt.Sprintf("%s.arguments[%d].value", path, i))
		}
	case CloseQuestionOperation:
	case EmitEffectOperation:
		v.validateExpression(o.Recipients, path+".recipients")
		for i, arg := range o.Arguments {
			v.validateExpression(arg.Value, fmt.Sprintf("%s.arguments[%d].value", path, i))
		}
	case ScheduleTimerOperation:
		v.validateNumberExpression(o.DelayMilliseconds, path+".delay_milliseconds")
	case CancelTimerOperation:
	case SpawnChildWorkflowOperation:
		for i, arg := range o.Arguments {
			v.validateExpression(arg.Value, fmt.Sprintf("%s.arguments[%d].value", path, i))
		}
	case CancelChildWorkflowOperation:
		v.validateStringExpression(o.Reason, path+".reason")
	case OpenAskGroupOperation:
		v.validateExpression(o.Recipients, path+".recipients")
		for i, arg := range o.Arguments {
			v.validateExpression(arg.Value, fmt.Sprintf("%s.arguments[%d].value", path, i))
		}
		v.validateAskGroupCompletionPolicy(o.Completion, path+".completion")
	case FinalizeAskGroupOperation:
	case CancelAskGroupOperation:
	case BeginTaskGroupOperation:
		v.validateTaskGroupCompletionPolicy(o.Completion, path+".completion")
	case SpawnTaskGroupChildOperation:
		v.validateExpression(o.Key, path+".key")
		for i, arg := range o.Arguments {
			v.validateExpression(arg.Value, fmt.Sprintf("%s.arguments[%d].value", path, i))
		}
	case SealTaskGroupOperation:
	case FinalizeTaskGroupOperation:
	case CancelTaskGroupOperation:
		v.validateStringExpression(o.Reason, path+".reason")
	case DrawRandomOperation:
		v.validateRandomGenerator(o.Generator, path+".generator")
	}
}

func (v *validator) validateAskGroupCompletionPolicy(policy AskGroupCompletionPolicy, path string) {
	if quorum, ok := policy.(AskGroupQuorumPolicy); ok {
		v.validateNumberExpression(quorum.Count, path+".count")
	}
}

func (v *validator) validateTaskGroupCompletionPolicy(policy TaskGroupCompletionPolicy, path string) {
	if quorum, ok := policy.(TaskGroupQuorumTerminalPolicy); ok {
		v.validateNumberExpression(quorum.Count, path+".count")
	}
}

func (v *validator) validateRandomGenerator(generator RandomGenerator, path string) {
	switch g := generator.(type) {
	case nil:
		return
	case RandomIntegerGenerator:
		v.validateNumberExpression(g.Minimum, path+".minimum")
		v.validateNumberExpression(g.Maximum, path+".maximum")
	case RandomElementGenerator:
		v.validateExpression(g.Collection, path+".collection")
	case RandomShuffleGenerator:
		v.validateExpression(g.Collection, path+".collection")
	}
}

func (v *validator) validateWorkflowControl(control WorkflowControl, path string) {
	switch c := control.(type) {
	case nil:
		return
	case GotoControl:
	case StayControl:
	case CompleteControl:
		v.validateExpression(c.Result, path+".result")
	case FailControl:
		v.validateStringExpression(c.Error, path+".error")
	case CancelControl:
		v.validateStringExpression(c.Reason, path+".reason")
	case ConditionalControl:
		v.validateBoolExpression(c.Condition, path+".condition")
		v.validateWorkflowControl(c.Then, path+".then")
		v.validateWorkflowControl(c.Else, path+".else")
	case MatchControl:
		v.validateExpression(c.Value, path+".value")
		for i, cs := range c.Cases {
			v.validateWorkflowControl(cs.Control, fmt.Sprintf("%s.cases[%d].control", path, i))
		}
	}
}

// --- expression validation and static type inference ---

// staticType represents an expression's statically determinable type, or
// nil if this package cannot determine it without lexical-scope or
// reference resolution.
func (v *validator) inferType(expr Expression) TypeReference {
	switch e := expr.(type) {
	case nil:
		return nil
	case UnitLiteralExpression:
		return BuiltinTypeReference{Type: BuiltinTypeUnit}
	case BoolLiteralExpression:
		return BuiltinTypeReference{Type: BuiltinTypeBool}
	case NumberLiteralExpression:
		return BuiltinTypeReference{Type: BuiltinTypeNumber}
	case StringLiteralExpression:
		return BuiltinTypeReference{Type: BuiltinTypeString}
	case OptionalNoneExpression:
		if e.ElementType == nil {
			return nil
		}
		return OptionalTypeReference{Element: e.ElementType}
	case OptionalSomeExpression:
		if inner := v.inferType(e.Value); inner != nil {
			return OptionalTypeReference{Element: inner}
		}
		return nil
	case ListExpression:
		if e.ElementType != nil {
			return ListTypeReference{Element: e.ElementType}
		}
		return nil
	case MapExpression:
		if e.KeyType != nil && e.ValueType != nil {
			return MapTypeReference{Key: e.KeyType, Value: e.ValueType}
		}
		return nil
	case EnumValueExpression:
		if e.TypeName == "" {
			return nil
		}
		return NamedTypeReference{Name: e.TypeName}
	case RecordExpression:
		if e.TypeName == "" {
			return nil
		}
		return NamedTypeReference{Name: e.TypeName}
	case UnionExpression:
		if e.TypeName == "" {
			return nil
		}
		return NamedTypeReference{Name: e.TypeName}
	case NewTypeExpression:
		if e.TypeName == "" {
			return nil
		}
		return NamedTypeReference{Name: e.TypeName}
	case UnaryExpression:
		switch e.Operator {
		case UnaryOperatorNot:
			return BuiltinTypeReference{Type: BuiltinTypeBool}
		case UnaryOperatorNegate:
			return BuiltinTypeReference{Type: BuiltinTypeNumber}
		}
		return nil
	case BinaryExpression:
		return binaryResultType(e.Operator)
	case ConditionalExpression:
		then := v.inferType(e.Then)
		els := v.inferType(e.Else)
		if then != nil && els != nil && typesEqual(then, els) {
			return then
		}
		return nil
	case MatchExpression:
		var result TypeReference
		for _, c := range e.Cases {
			t := v.inferType(c.Result)
			if t == nil {
				return nil
			}
			if result == nil {
				result = t
			} else if !typesEqual(result, t) {
				return nil
			}
		}
		return result
	case ListAnyExpression, ListAllExpression:
		return BuiltinTypeReference{Type: BuiltinTypeBool}
	case ListCountExpression:
		return BuiltinTypeReference{Type: BuiltinTypeNumber}
	case ListFilterExpression:
		return v.inferType(e.Collection)
	case ListFirstExpression:
		if collection, ok := v.inferType(e.Collection).(ListTypeReference); ok {
			return OptionalTypeReference{Element: collection.Element}
		}
		return nil
	default:
		// ReferenceExpression, FieldExpression, IndexExpression,
		// CallExpression, ListMapExpression, ListFlatMapExpression: these
		// require lexical-scope or reference resolution this package does
		// not perform.
		return nil
	}
}

func binaryResultType(op BinaryOperator) TypeReference {
	switch op {
	case BinaryOperatorAdd, BinaryOperatorSubtract, BinaryOperatorMultiply, BinaryOperatorDivide, BinaryOperatorModulo:
		return BuiltinTypeReference{Type: BuiltinTypeNumber}
	case BinaryOperatorEqual, BinaryOperatorNotEqual,
		BinaryOperatorLess, BinaryOperatorLessOrEqual, BinaryOperatorGreater, BinaryOperatorGreaterOrEqual,
		BinaryOperatorAnd, BinaryOperatorOr,
		BinaryOperatorIn, BinaryOperatorNotIn:
		return BuiltinTypeReference{Type: BuiltinTypeBool}
	default:
		return nil
	}
}

func typesEqual(a, b TypeReference) bool {
	return reflect.DeepEqual(a, b)
}

func isBuiltin(t TypeReference, want BuiltinType) bool {
	b, ok := t.(BuiltinTypeReference)
	return ok && b.Type == want
}

func isListOrMap(t TypeReference) bool {
	switch t.(type) {
	case ListTypeReference, MapTypeReference:
		return true
	default:
		return false
	}
}

// validateExpression recursively validates expr: it reports invalid
// operator strings, operand/operator type mismatches (only when an
// operand's type is statically inferable without reference resolution),
// and recurses into every nested expression, target, block, or control it
// contains.
func (v *validator) validateExpression(expr Expression, path string) {
	switch e := expr.(type) {
	case nil:
		return
	case UnitLiteralExpression, BoolLiteralExpression, NumberLiteralExpression, StringLiteralExpression:
	case OptionalNoneExpression:
		v.validateTypeReference(e.ElementType, path+".element_type")
	case OptionalSomeExpression:
		v.validateExpression(e.Value, path+".value")
	case ListExpression:
		v.validateTypeReference(e.ElementType, path+".element_type")
		for i, el := range e.Elements {
			v.validateExpression(el, fmt.Sprintf("%s.elements[%d]", path, i))
		}
	case MapExpression:
		v.validateTypeReference(e.KeyType, path+".key_type")
		v.validateTypeReference(e.ValueType, path+".value_type")
		for i, entry := range e.Entries {
			v.validateExpression(entry.Key, fmt.Sprintf("%s.entries[%d].key", path, i))
			v.validateExpression(entry.Value, fmt.Sprintf("%s.entries[%d].value", path, i))
		}
	case EnumValueExpression:
	case RecordExpression:
		for i, f := range e.Fields {
			v.validateExpression(f.Value, fmt.Sprintf("%s.fields[%d].value", path, i))
		}
	case UnionExpression:
		for i, f := range e.Fields {
			v.validateExpression(f.Value, fmt.Sprintf("%s.fields[%d].value", path, i))
		}
	case NewTypeExpression:
		v.validateExpression(e.Value, path+".value")
	case ReferenceExpression:
	case FieldExpression:
		v.validateExpression(e.Target, path+".target")
	case IndexExpression:
		v.validateExpression(e.Target, path+".target")
		v.validateExpression(e.Index, path+".index")
	case UnaryExpression:
		v.validateExpression(e.Operand, path+".operand")
		if !e.Operator.IsValid() {
			v.addf(path+".operator", "unknown unary operator %q", e.Operator)
			return
		}
		operandType := v.inferType(e.Operand)
		if operandType == nil {
			return
		}
		switch e.Operator {
		case UnaryOperatorNot:
			if !isBuiltin(operandType, BuiltinTypeBool) {
				v.addf(path, "operator %q requires a bool operand, but the operand is statically %s", e.Operator, describeType(operandType))
			}
		case UnaryOperatorNegate:
			if !isBuiltin(operandType, BuiltinTypeNumber) {
				v.addf(path, "operator %q requires a number operand, but the operand is statically %s", e.Operator, describeType(operandType))
			}
		}
	case BinaryExpression:
		v.validateExpression(e.Left, path+".left")
		v.validateExpression(e.Right, path+".right")
		if !e.Operator.IsValid() {
			v.addf(path+".operator", "unknown binary operator %q", e.Operator)
			return
		}
		v.validateBinaryOperandTypes(e, path)
	case ConditionalExpression:
		v.validateBoolExpression(e.Condition, path+".condition")
		v.validateExpression(e.Then, path+".then")
		v.validateExpression(e.Else, path+".else")
	case CallExpression:
		for i, arg := range e.Arguments {
			v.validateExpression(arg.Value, fmt.Sprintf("%s.arguments[%d].value", path, i))
		}
	case MatchExpression:
		v.validateExpression(e.Value, path+".value")
		for i, c := range e.Cases {
			v.validateExpression(c.Result, fmt.Sprintf("%s.cases[%d].result", path, i))
		}
	case ListMapExpression:
		v.validateExpression(e.Collection, path+".collection")
		v.validateExpression(e.Result, path+".result")
	case ListFilterExpression:
		v.validateExpression(e.Collection, path+".collection")
		v.validateBoolExpression(e.Predicate, path+".predicate")
	case ListFlatMapExpression:
		v.validateExpression(e.Collection, path+".collection")
		v.validateExpression(e.Result, path+".result")
	case ListAnyExpression:
		v.validateExpression(e.Collection, path+".collection")
		v.validateBoolExpression(e.Predicate, path+".predicate")
	case ListAllExpression:
		v.validateExpression(e.Collection, path+".collection")
		v.validateBoolExpression(e.Predicate, path+".predicate")
	case ListCountExpression:
		v.validateExpression(e.Collection, path+".collection")
		v.validateBoolExpression(e.Predicate, path+".predicate")
	case ListFirstExpression:
		v.validateExpression(e.Collection, path+".collection")
		v.validateBoolExpression(e.Predicate, path+".predicate")
	}
}

func (v *validator) validateBinaryOperandTypes(e BinaryExpression, path string) {
	left := v.inferType(e.Left)
	right := v.inferType(e.Right)

	switch e.Operator {
	case BinaryOperatorAdd, BinaryOperatorSubtract, BinaryOperatorMultiply, BinaryOperatorDivide, BinaryOperatorModulo,
		BinaryOperatorLess, BinaryOperatorLessOrEqual, BinaryOperatorGreater, BinaryOperatorGreaterOrEqual:
		if left != nil && !isBuiltin(left, BuiltinTypeNumber) {
			v.addf(path+".left", "operator %q requires a number operand, but the left operand is statically %s", e.Operator, describeType(left))
		}
		if right != nil && !isBuiltin(right, BuiltinTypeNumber) {
			v.addf(path+".right", "operator %q requires a number operand, but the right operand is statically %s", e.Operator, describeType(right))
		}
	case BinaryOperatorAnd, BinaryOperatorOr:
		if left != nil && !isBuiltin(left, BuiltinTypeBool) {
			v.addf(path+".left", "operator %q requires a bool operand, but the left operand is statically %s", e.Operator, describeType(left))
		}
		if right != nil && !isBuiltin(right, BuiltinTypeBool) {
			v.addf(path+".right", "operator %q requires a bool operand, but the right operand is statically %s", e.Operator, describeType(right))
		}
	case BinaryOperatorEqual, BinaryOperatorNotEqual:
		if left != nil && right != nil && !typesEqual(left, right) {
			v.addf(path, "operator %q compares two statically incompatible types, %s and %s", e.Operator, describeType(left), describeType(right))
		}
	case BinaryOperatorIn, BinaryOperatorNotIn:
		if right != nil && !isListOrMap(right) {
			v.addf(path+".right", "operator %q requires a list or map on the right, but the right operand is statically %s", e.Operator, describeType(right))
		}
	}
}

func (v *validator) validateBoolExpression(expr Expression, path string) {
	v.validateExpression(expr, path)
	if t := v.inferType(expr); t != nil && !isBuiltin(t, BuiltinTypeBool) {
		v.addf(path, "expected a bool expression, but it is statically %s", describeType(t))
	}
}

func (v *validator) validateNumberExpression(expr Expression, path string) {
	v.validateExpression(expr, path)
	if t := v.inferType(expr); t != nil && !isBuiltin(t, BuiltinTypeNumber) {
		v.addf(path, "expected a number expression, but it is statically %s", describeType(t))
	}
}

func (v *validator) validateStringExpression(expr Expression, path string) {
	v.validateExpression(expr, path)
	if t := v.inferType(expr); t != nil && !isBuiltin(t, BuiltinTypeString) {
		v.addf(path, "expected a string expression, but it is statically %s", describeType(t))
	}
}

func describeType(t TypeReference) string {
	switch tt := t.(type) {
	case nil:
		return "unknown"
	case BuiltinTypeReference:
		return string(tt.Type)
	case NamedTypeReference:
		return tt.Name
	case ListTypeReference:
		return "list<" + describeType(tt.Element) + ">"
	case MapTypeReference:
		return "map<" + describeType(tt.Key) + ", " + describeType(tt.Value) + ">"
	case OptionalTypeReference:
		return "optional<" + describeType(tt.Element) + ">"
	default:
		return "unknown"
	}
}
