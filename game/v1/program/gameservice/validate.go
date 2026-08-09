package gameservice

import (
	"fmt"
	"reflect"

	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// ValidationError describes one violation of the game language's own
// rules — type and operator compatibility, named-type resolution, and
// declaration-structure rules such as duplicate names — found by
// program.Definition.Validate.
//
// ValidationError never reports lexical-scope, reference-resolution, or
// execution problems (an unresolved program.ReferenceExpression, an unknown
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
// scope or references: a program.ReferenceExpression, a program.FieldExpression, an
// program.IndexExpression's target, or a program.CallExpression's target function are
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
func Validate(d program.Definition) []error {
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
	definition program.Definition
	typeNames  map[string]bool
	errors     []error
}

// --- name collection and namespace/duplicate checks ---

func (v *validator) collectNames() {
	v.typeNames = make(map[string]bool, len(v.definition.Types))
	for _, t := range v.definition.Types {
		switch tt := t.(type) {
		case program.EnumTypeDeclaration:
			v.typeNames[tt.Name] = true
		case program.RecordTypeDeclaration:
			v.typeNames[tt.Name] = true
		case program.UnionTypeDeclaration:
			v.typeNames[tt.Name] = true
		case program.NewTypeDeclaration:
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

func declarationName(t program.TypeDeclaration) string {
	switch tt := t.(type) {
	case program.EnumTypeDeclaration:
		return tt.Name
	case program.RecordTypeDeclaration:
		return tt.Name
	case program.UnionTypeDeclaration:
		return tt.Name
	case program.NewTypeDeclaration:
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
		case program.EnumTypeDeclaration:
			names := make([]string, len(tt.Values))
			for j, val := range tt.Values {
				names[j] = val.Name
			}
			checkDuplicateNames(names, path+".values", "enum value", v)
		case program.RecordTypeDeclaration:
			v.validateFieldDeclarations(tt.Fields, path+".fields")
		case program.UnionTypeDeclaration:
			variantNames := make([]string, len(tt.Variants))
			for j, variant := range tt.Variants {
				variantNames[j] = variant.Name
				v.validateFieldDeclarations(variant.Fields, fmt.Sprintf("%s.variants[%d].fields", path, j))
			}
			checkDuplicateNames(variantNames, path+".variants", "union variant", v)
		case program.NewTypeDeclaration:
			v.validateTypeReference(tt.Underlying, path+".underlying")
		}
	}
}

func (v *validator) validateFieldDeclarations(fields []program.FieldDeclaration, pathPrefix string) {
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
func (v *validator) validateTypeReference(ref program.TypeReference, path string) {
	switch t := ref.(type) {
	case nil:
		return
	case program.BuiltinTypeReference:
		if !t.Type.IsValid() {
			v.addf(path, "unknown built-in type %q", t.Type)
		}
	case program.NamedTypeReference:
		if t.Name == "" {
			v.addf(path, "named type reference has an empty name")
			return
		}
		if !v.typeNames[t.Name] {
			v.addf(path, "reference to undeclared type %q", t.Name)
		}
	case program.ListTypeReference:
		v.validateTypeReference(t.Element, path+".element")
	case program.MapTypeReference:
		v.validateTypeReference(t.Key, path+".key")
		v.validateTypeReference(t.Value, path+".value")
	case program.OptionalTypeReference:
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

func (v *validator) validateStateDeclaration(state program.StateDeclaration, path string) {
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

func (v *validator) validateUIElement(element program.UIElement, path string) {
	switch e := element.(type) {
	case nil:
		return
	case program.EmptyElement:
	case program.ContainerElement:
		v.validateUIElementConfiguration(e.Configuration, path+".configuration")
		v.validateUILayout(e.Layout, path+".layout")
		for i, child := range e.Children {
			v.validateUIElement(child, fmt.Sprintf("%s.children[%d]", path, i))
		}
	case program.TextElement:
		v.validateUIElementConfiguration(e.Configuration, path+".configuration")
		v.validateExpression(e.Value, path+".value")
	case program.ImageElement:
		v.validateUIElementConfiguration(e.Configuration, path+".configuration")
		v.validateExpression(e.Source, path+".source")
		v.validateExpression(e.AlternativeText, path+".alternative_text")
	case program.ButtonElement:
		v.validateUIElementConfiguration(e.Configuration, path+".configuration")
		for i, child := range e.Children {
			v.validateUIElement(child, fmt.Sprintf("%s.children[%d]", path, i))
		}
	case program.RepeatElement:
		v.validateExpression(e.Collection, path+".collection")
		v.validateExpression(e.Key, path+".key")
		v.validateUIElement(e.Body, path+".body")
	case program.ConditionalElement:
		v.validateExpression(e.Condition, path+".condition")
		v.validateUIElement(e.Then, path+".then")
		v.validateUIElement(e.Else, path+".else")
	}
}

func (v *validator) validateUILayout(layout program.UILayout, path string) {
	switch l := layout.(type) {
	case nil:
		return
	case program.StackLayout:
	case program.AbsoluteLayout:
	case program.LinearLayout:
		if !l.Direction.IsValid() {
			v.addf(path+".direction", "unknown linear layout direction %q", l.Direction)
		}
		v.validateExpression(l.Gap, path+".gap")
	case program.GridLayout:
		v.validateExpression(l.Columns, path+".columns")
		v.validateExpression(l.RowGap, path+".row_gap")
		v.validateExpression(l.ColumnGap, path+".column_gap")
	}
}

func (v *validator) validateUIElementConfiguration(config program.UIElementConfiguration, path string) {
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

func (v *validator) validateUIAction(action program.UIAction, path string) {
	switch a := action.(type) {
	case nil:
		return
	case program.SetLocalStateAction:
		v.validateAssignmentTarget(a.Target, path+".target")
		v.validateExpression(a.Value, path+".value")
	case program.AnswerQuestionAction:
		v.validateExpression(a.Value, path+".value")
	case program.EmitUserIntentAction:
		for i, arg := range a.Arguments {
			v.validateExpression(arg.Value, fmt.Sprintf("%s.arguments[%d].value", path, i))
		}
	}
}

func (v *validator) validateAssignmentTarget(target program.AssignmentTarget, path string) {
	switch t := target.(type) {
	case nil:
		return
	case program.NameTarget:
	case program.FieldTarget:
		v.validateAssignmentTarget(t.Target, path+".target")
	case program.IndexTarget:
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

func (v *validator) validateQuestionPresentation(presentation *program.QuestionPresentationDeclaration, path string) {
	if presentation == nil {
		return
	}
	for i, arg := range presentation.ProjectionArguments {
		v.validateExpression(arg.Value, fmt.Sprintf("%s.projection_arguments[%d].value", path, i))
	}
}

func (v *validator) validatePresentation(p program.PresentationDeclaration, path string) {
	v.validateExpression(p.Targets, path+".targets")
	for i, arg := range p.ProjectionArguments {
		v.validateExpression(arg.Value, fmt.Sprintf("%s.projection_arguments[%d].value", path, i))
	}
}

func (v *validator) validateWorkflowState(state program.WorkflowStateDeclaration, path string) {
	for i, p := range state.Presentations {
		v.validatePresentation(p, fmt.Sprintf("%s.presentations[%d]", path, i))
	}
	for i, t := range state.Transitions {
		v.validateTransition(t, fmt.Sprintf("%s.transitions[%d]", path, i))
	}
}

func (v *validator) validateTransition(t program.TransitionDeclaration, path string) {
	v.validateSignalPattern(t.Signal, path+".signal")
	v.validateExpression(t.Guard, path+".guard")
	v.validateBlock(t.Operations, path+".operations")
	v.validateWorkflowControl(t.Control, path+".control")
}

func (v *validator) validateSignalPattern(pattern program.SignalPattern, path string) {
	names := make([]string, len(pattern.Bindings))
	for i, b := range pattern.Bindings {
		names[i] = b.Name
	}
	checkDuplicateNames(names, path+".bindings", "signal binding", v)
}

// --- operations, controls ---

func (v *validator) validateBlock(block program.Block, path string) {
	for i, op := range block.Operations {
		v.validateOperation(op, fmt.Sprintf("%s.operations[%d]", path, i))
	}
}

func (v *validator) validateOperation(op program.Operation, path string) {
	switch o := op.(type) {
	case nil:
		return
	case program.LetOperation:
		v.validateTypeReference(o.Type, path+".type")
		v.validateExpression(o.Value, path+".value")
	case program.SetOperation:
		v.validateAssignmentTarget(o.Target, path+".target")
		v.validateExpression(o.Value, path+".value")
	case program.ListAppendOperation:
		v.validateAssignmentTarget(o.Target, path+".target")
		v.validateExpression(o.Value, path+".value")
	case program.ListInsertOperation:
		v.validateAssignmentTarget(o.Target, path+".target")
		v.validateExpression(o.Index, path+".index")
		v.validateExpression(o.Value, path+".value")
	case program.ListRemoveAtOperation:
		v.validateAssignmentTarget(o.Target, path+".target")
		v.validateExpression(o.Index, path+".index")
	case program.MapPutOperation:
		v.validateAssignmentTarget(o.Target, path+".target")
		v.validateExpression(o.Key, path+".key")
		v.validateExpression(o.Value, path+".value")
	case program.MapDeleteOperation:
		v.validateAssignmentTarget(o.Target, path+".target")
		v.validateExpression(o.Key, path+".key")
	case program.IfOperation:
		v.validateExpression(o.Condition, path+".condition")
		v.validateBlock(o.Then, path+".then")
		v.validateBlock(o.Else, path+".else")
	case program.ForEachOperation:
		v.validateExpression(o.Collection, path+".collection")
		v.validateBlock(o.Body, path+".body")
	case program.MatchOperation:
		v.validateExpression(o.Value, path+".value")
		for i, c := range o.Cases {
			v.validateBlock(c.Body, fmt.Sprintf("%s.cases[%d].body", path, i))
		}
	case program.OpenQuestionOperation:
		v.validateExpression(o.Recipient, path+".recipient")
		for i, arg := range o.Arguments {
			v.validateExpression(arg.Value, fmt.Sprintf("%s.arguments[%d].value", path, i))
		}
	case program.CloseQuestionOperation:
	case program.EmitEffectOperation:
		v.validateExpression(o.Recipients, path+".recipients")
		for i, arg := range o.Arguments {
			v.validateExpression(arg.Value, fmt.Sprintf("%s.arguments[%d].value", path, i))
		}
	case program.ScheduleTimerOperation:
		v.validateNumberExpression(o.DelayMilliseconds, path+".delay_milliseconds")
	case program.CancelTimerOperation:
	case program.SpawnChildWorkflowOperation:
		for i, arg := range o.Arguments {
			v.validateExpression(arg.Value, fmt.Sprintf("%s.arguments[%d].value", path, i))
		}
	case program.CancelChildWorkflowOperation:
		v.validateStringExpression(o.Reason, path+".reason")
	case program.OpenAskGroupOperation:
		v.validateExpression(o.Recipients, path+".recipients")
		for i, arg := range o.Arguments {
			v.validateExpression(arg.Value, fmt.Sprintf("%s.arguments[%d].value", path, i))
		}
		v.validateAskGroupCompletionPolicy(o.Completion, path+".completion")
	case program.FinalizeAskGroupOperation:
	case program.CancelAskGroupOperation:
	case program.BeginTaskGroupOperation:
		v.validateTaskGroupCompletionPolicy(o.Completion, path+".completion")
	case program.SpawnTaskGroupChildOperation:
		v.validateExpression(o.Key, path+".key")
		for i, arg := range o.Arguments {
			v.validateExpression(arg.Value, fmt.Sprintf("%s.arguments[%d].value", path, i))
		}
	case program.SealTaskGroupOperation:
	case program.FinalizeTaskGroupOperation:
	case program.CancelTaskGroupOperation:
		v.validateStringExpression(o.Reason, path+".reason")
	case program.DrawRandomOperation:
		v.validateRandomGenerator(o.Generator, path+".generator")
	}
}

func (v *validator) validateAskGroupCompletionPolicy(policy program.AskGroupCompletionPolicy, path string) {
	if quorum, ok := policy.(program.AskGroupQuorumPolicy); ok {
		v.validateNumberExpression(quorum.Count, path+".count")
	}
}

func (v *validator) validateTaskGroupCompletionPolicy(policy program.TaskGroupCompletionPolicy, path string) {
	if quorum, ok := policy.(program.TaskGroupQuorumTerminalPolicy); ok {
		v.validateNumberExpression(quorum.Count, path+".count")
	}
}

func (v *validator) validateRandomGenerator(generator program.RandomGenerator, path string) {
	switch g := generator.(type) {
	case nil:
		return
	case program.RandomIntegerGenerator:
		v.validateNumberExpression(g.Minimum, path+".minimum")
		v.validateNumberExpression(g.Maximum, path+".maximum")
	case program.RandomElementGenerator:
		v.validateExpression(g.Collection, path+".collection")
	case program.RandomShuffleGenerator:
		v.validateExpression(g.Collection, path+".collection")
	}
}

func (v *validator) validateWorkflowControl(control program.WorkflowControl, path string) {
	switch c := control.(type) {
	case nil:
		return
	case program.GotoControl:
	case program.StayControl:
	case program.CompleteControl:
		v.validateExpression(c.Result, path+".result")
	case program.FailControl:
		v.validateStringExpression(c.Error, path+".error")
	case program.CancelControl:
		v.validateStringExpression(c.Reason, path+".reason")
	case program.ConditionalControl:
		v.validateBoolExpression(c.Condition, path+".condition")
		v.validateWorkflowControl(c.Then, path+".then")
		v.validateWorkflowControl(c.Else, path+".else")
	case program.MatchControl:
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
func (v *validator) inferType(expr program.Expression) program.TypeReference {
	switch e := expr.(type) {
	case nil:
		return nil
	case program.UnitLiteralExpression:
		return program.BuiltinTypeReference{Type: program.BuiltinTypeUnit}
	case program.BoolLiteralExpression:
		return program.BuiltinTypeReference{Type: program.BuiltinTypeBool}
	case program.NumberLiteralExpression:
		return program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}
	case program.StringLiteralExpression:
		return program.BuiltinTypeReference{Type: program.BuiltinTypeString}
	case program.OptionalNoneExpression:
		if e.ElementType == nil {
			return nil
		}
		return program.OptionalTypeReference{Element: e.ElementType}
	case program.OptionalSomeExpression:
		if inner := v.inferType(e.Value); inner != nil {
			return program.OptionalTypeReference{Element: inner}
		}
		return nil
	case program.ListExpression:
		if e.ElementType != nil {
			return program.ListTypeReference{Element: e.ElementType}
		}
		return nil
	case program.MapExpression:
		if e.KeyType != nil && e.ValueType != nil {
			return program.MapTypeReference{Key: e.KeyType, Value: e.ValueType}
		}
		return nil
	case program.EnumValueExpression:
		if e.TypeName == "" {
			return nil
		}
		return program.NamedTypeReference{Name: e.TypeName}
	case program.RecordExpression:
		if e.TypeName == "" {
			return nil
		}
		return program.NamedTypeReference{Name: e.TypeName}
	case program.UnionExpression:
		if e.TypeName == "" {
			return nil
		}
		return program.NamedTypeReference{Name: e.TypeName}
	case program.NewTypeExpression:
		if e.TypeName == "" {
			return nil
		}
		return program.NamedTypeReference{Name: e.TypeName}
	case program.UnaryExpression:
		switch e.Operator {
		case program.UnaryOperatorNot:
			return program.BuiltinTypeReference{Type: program.BuiltinTypeBool}
		case program.UnaryOperatorNegate:
			return program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}
		}
		return nil
	case program.BinaryExpression:
		return binaryResultType(e.Operator)
	case program.ConditionalExpression:
		then := v.inferType(e.Then)
		els := v.inferType(e.Else)
		if then != nil && els != nil && typesEqual(then, els) {
			return then
		}
		return nil
	case program.MatchExpression:
		var result program.TypeReference
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
	case program.ListAnyExpression, program.ListAllExpression:
		return program.BuiltinTypeReference{Type: program.BuiltinTypeBool}
	case program.ListCountExpression:
		return program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}
	case program.ListFilterExpression:
		return v.inferType(e.Collection)
	case program.ListFirstExpression:
		if collection, ok := v.inferType(e.Collection).(program.ListTypeReference); ok {
			return program.OptionalTypeReference{Element: collection.Element}
		}
		return nil
	default:
		// program.ReferenceExpression, program.FieldExpression, program.IndexExpression,
		// program.CallExpression, program.ListMapExpression, ListFlatMapExpression: these
		// require lexical-scope or reference resolution this package does
		// not perform.
		return nil
	}
}

func binaryResultType(op program.BinaryOperator) program.TypeReference {
	switch op {
	case program.BinaryOperatorAdd, program.BinaryOperatorSubtract, program.BinaryOperatorMultiply, program.BinaryOperatorDivide, program.BinaryOperatorModulo:
		return program.BuiltinTypeReference{Type: program.BuiltinTypeNumber}
	case program.BinaryOperatorEqual, program.BinaryOperatorNotEqual,
		program.BinaryOperatorLess, program.BinaryOperatorLessOrEqual, program.BinaryOperatorGreater, program.BinaryOperatorGreaterOrEqual,
		program.BinaryOperatorAnd, program.BinaryOperatorOr,
		program.BinaryOperatorIn, program.BinaryOperatorNotIn:
		return program.BuiltinTypeReference{Type: program.BuiltinTypeBool}
	default:
		return nil
	}
}

func typesEqual(a, b program.TypeReference) bool {
	return reflect.DeepEqual(a, b)
}

func isBuiltin(t program.TypeReference, want program.BuiltinType) bool {
	b, ok := t.(program.BuiltinTypeReference)
	return ok && b.Type == want
}

func isListOrMap(t program.TypeReference) bool {
	switch t.(type) {
	case program.ListTypeReference, program.MapTypeReference:
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
func (v *validator) validateExpression(expr program.Expression, path string) {
	switch e := expr.(type) {
	case nil:
		return
	case program.UnitLiteralExpression, program.BoolLiteralExpression, program.NumberLiteralExpression, program.StringLiteralExpression:
	case program.OptionalNoneExpression:
		v.validateTypeReference(e.ElementType, path+".element_type")
	case program.OptionalSomeExpression:
		v.validateExpression(e.Value, path+".value")
	case program.ListExpression:
		v.validateTypeReference(e.ElementType, path+".element_type")
		for i, el := range e.Elements {
			v.validateExpression(el, fmt.Sprintf("%s.elements[%d]", path, i))
		}
	case program.MapExpression:
		v.validateTypeReference(e.KeyType, path+".key_type")
		v.validateTypeReference(e.ValueType, path+".value_type")
		for i, entry := range e.Entries {
			v.validateExpression(entry.Key, fmt.Sprintf("%s.entries[%d].key", path, i))
			v.validateExpression(entry.Value, fmt.Sprintf("%s.entries[%d].value", path, i))
		}
	case program.EnumValueExpression:
	case program.RecordExpression:
		for i, f := range e.Fields {
			v.validateExpression(f.Value, fmt.Sprintf("%s.fields[%d].value", path, i))
		}
	case program.UnionExpression:
		for i, f := range e.Fields {
			v.validateExpression(f.Value, fmt.Sprintf("%s.fields[%d].value", path, i))
		}
	case program.NewTypeExpression:
		v.validateExpression(e.Value, path+".value")
	case program.ReferenceExpression:
	case program.FieldExpression:
		v.validateExpression(e.Target, path+".target")
	case program.IndexExpression:
		v.validateExpression(e.Target, path+".target")
		v.validateExpression(e.Index, path+".index")
	case program.UnaryExpression:
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
		case program.UnaryOperatorNot:
			if !isBuiltin(operandType, program.BuiltinTypeBool) {
				v.addf(path, "operator %q requires a bool operand, but the operand is statically %s", e.Operator, describeType(operandType))
			}
		case program.UnaryOperatorNegate:
			if !isBuiltin(operandType, program.BuiltinTypeNumber) {
				v.addf(path, "operator %q requires a number operand, but the operand is statically %s", e.Operator, describeType(operandType))
			}
		}
	case program.BinaryExpression:
		v.validateExpression(e.Left, path+".left")
		v.validateExpression(e.Right, path+".right")
		if !e.Operator.IsValid() {
			v.addf(path+".operator", "unknown binary operator %q", e.Operator)
			return
		}
		v.validateBinaryOperandTypes(e, path)
	case program.ConditionalExpression:
		v.validateBoolExpression(e.Condition, path+".condition")
		v.validateExpression(e.Then, path+".then")
		v.validateExpression(e.Else, path+".else")
	case program.CallExpression:
		for i, arg := range e.Arguments {
			v.validateExpression(arg.Value, fmt.Sprintf("%s.arguments[%d].value", path, i))
		}
	case program.MatchExpression:
		v.validateExpression(e.Value, path+".value")
		for i, c := range e.Cases {
			v.validateExpression(c.Result, fmt.Sprintf("%s.cases[%d].result", path, i))
		}
	case program.ListMapExpression:
		v.validateExpression(e.Collection, path+".collection")
		v.validateExpression(e.Result, path+".result")
	case program.ListFilterExpression:
		v.validateExpression(e.Collection, path+".collection")
		v.validateBoolExpression(e.Predicate, path+".predicate")
	case program.ListFlatMapExpression:
		v.validateExpression(e.Collection, path+".collection")
		v.validateExpression(e.Result, path+".result")
	case program.ListAnyExpression:
		v.validateExpression(e.Collection, path+".collection")
		v.validateBoolExpression(e.Predicate, path+".predicate")
	case program.ListAllExpression:
		v.validateExpression(e.Collection, path+".collection")
		v.validateBoolExpression(e.Predicate, path+".predicate")
	case program.ListCountExpression:
		v.validateExpression(e.Collection, path+".collection")
		v.validateBoolExpression(e.Predicate, path+".predicate")
	case program.ListFirstExpression:
		v.validateExpression(e.Collection, path+".collection")
		v.validateBoolExpression(e.Predicate, path+".predicate")
	}
}

func (v *validator) validateBinaryOperandTypes(e program.BinaryExpression, path string) {
	left := v.inferType(e.Left)
	right := v.inferType(e.Right)

	switch e.Operator {
	case program.BinaryOperatorAdd, program.BinaryOperatorSubtract, program.BinaryOperatorMultiply, program.BinaryOperatorDivide, program.BinaryOperatorModulo,
		program.BinaryOperatorLess, program.BinaryOperatorLessOrEqual, program.BinaryOperatorGreater, program.BinaryOperatorGreaterOrEqual:
		if left != nil && !isBuiltin(left, program.BuiltinTypeNumber) {
			v.addf(path+".left", "operator %q requires a number operand, but the left operand is statically %s", e.Operator, describeType(left))
		}
		if right != nil && !isBuiltin(right, program.BuiltinTypeNumber) {
			v.addf(path+".right", "operator %q requires a number operand, but the right operand is statically %s", e.Operator, describeType(right))
		}
	case program.BinaryOperatorAnd, program.BinaryOperatorOr:
		if left != nil && !isBuiltin(left, program.BuiltinTypeBool) {
			v.addf(path+".left", "operator %q requires a bool operand, but the left operand is statically %s", e.Operator, describeType(left))
		}
		if right != nil && !isBuiltin(right, program.BuiltinTypeBool) {
			v.addf(path+".right", "operator %q requires a bool operand, but the right operand is statically %s", e.Operator, describeType(right))
		}
	case program.BinaryOperatorEqual, program.BinaryOperatorNotEqual:
		if left != nil && right != nil && !typesEqual(left, right) {
			v.addf(path, "operator %q compares two statically incompatible types, %s and %s", e.Operator, describeType(left), describeType(right))
		}
	case program.BinaryOperatorIn, program.BinaryOperatorNotIn:
		if right != nil && !isListOrMap(right) {
			v.addf(path+".right", "operator %q requires a list or map on the right, but the right operand is statically %s", e.Operator, describeType(right))
		}
	}
}

func (v *validator) validateBoolExpression(expr program.Expression, path string) {
	v.validateExpression(expr, path)
	if t := v.inferType(expr); t != nil && !isBuiltin(t, program.BuiltinTypeBool) {
		v.addf(path, "expected a bool expression, but it is statically %s", describeType(t))
	}
}

func (v *validator) validateNumberExpression(expr program.Expression, path string) {
	v.validateExpression(expr, path)
	if t := v.inferType(expr); t != nil && !isBuiltin(t, program.BuiltinTypeNumber) {
		v.addf(path, "expected a number expression, but it is statically %s", describeType(t))
	}
}

func (v *validator) validateStringExpression(expr program.Expression, path string) {
	v.validateExpression(expr, path)
	if t := v.inferType(expr); t != nil && !isBuiltin(t, program.BuiltinTypeString) {
		v.addf(path, "expected a string expression, but it is statically %s", describeType(t))
	}
}

func describeType(t program.TypeReference) string {
	switch tt := t.(type) {
	case nil:
		return "unknown"
	case program.BuiltinTypeReference:
		return string(tt.Type)
	case program.NamedTypeReference:
		return tt.Name
	case program.ListTypeReference:
		return "list<" + describeType(tt.Element) + ">"
	case program.MapTypeReference:
		return "map<" + describeType(tt.Key) + ", " + describeType(tt.Value) + ">"
	case program.OptionalTypeReference:
		return "optional<" + describeType(tt.Element) + ">"
	default:
		return "unknown"
	}
}
