package engineservice

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// workflowEntry is the registered namespace entry for one declared
// workflow name.
type workflowEntry struct {
	decl  program.WorkflowDeclaration
	path  string
	index int
}

// workflowContext holds the transient, workflow-scoped state built
// while compiling one program.WorkflowDeclaration: its slot namespaces
// (needed to resolve a signal's schema — see compile_signals.go) and
// the base scope every transition, control, and presentation compiles
// against.
type workflowContext struct {
	resultType engine.Type

	// baseScope is params + "local" + "global" + "resources" — every
	// transition's Guard and Control, and every workflow-level or
	// state-level Presentation, compiles against this, extended
	// per-transition with that transition's own signal bindings.
	baseScope exprScope

	questionSlots  map[string]program.QuestionSlotDeclaration
	askGroupSlots  map[string]program.AskGroupSlotDeclaration
	timerSlots     map[string]bool
	childSlots     map[string]program.ChildWorkflowSlotDeclaration
	taskGroupSlots map[string]taskGroupSlotInfo

	stateNames map[string]bool
}

// taskGroupSlotInfo is what compileSignalSource needs to resolve a
// TaskGroupCompletedSignalSource's schema for one registered task-group
// slot.
type taskGroupSlotInfo struct {
	workflow string
	keyType  engine.Type
}

// registerWorkflowNamespace walks c.definition.Workflows in source
// order, diagnosing an empty or duplicate name, and registers the first
// occurrence of every other name into c.workflowDeclarations.
func (c *compiler) registerWorkflowNamespace() {
	seen := make(map[string]int, len(c.definition.Workflows))
	for i, w := range c.definition.Workflows {
		path := fmt.Sprintf("$.workflows[%d]", i)
		if w.Name == "" {
			c.addf(path, "workflow declaration has an empty name")
			continue
		}
		if first, ok := seen[w.Name]; ok {
			c.addf(path, "duplicate workflow name %q (first declared at $.workflows[%d])", w.Name, first)
			continue
		}
		seen[w.Name] = i
		c.workflowDeclarations[w.Name] = workflowEntry{decl: w, path: path, index: i}
	}
}

// buildWorkflowResultTypes compiles every registered workflow's declared
// ResultType — never the rest of its body — so a child or task-group
// slot anywhere can resolve another (or its own) workflow's ResultType
// for a completion signal's schema before any workflow's full body is
// compiled. Unlike a named type or a function, this never risks
// recursion: a workflow's ResultType never depends on another
// workflow's body.
func (c *compiler) buildWorkflowResultTypes() {
	for name, entry := range c.workflowDeclarations {
		c.workflowResultTypes[name] = c.compileTypeReference(entry.decl.ResultType, entry.path+".result_type")
	}
}

// validateRootWorkflow diagnoses a program.Definition.RootWorkflow that
// is empty or does not name a registered workflow.
func (c *compiler) validateRootWorkflow() {
	if c.definition.RootWorkflow == "" {
		c.addf("$.root_workflow", "root workflow is not set")
		return
	}
	if _, ok := c.workflowDeclarations[c.definition.RootWorkflow]; !ok {
		c.addf("$.root_workflow", "root workflow %q is not a declared workflow", c.definition.RootWorkflow)
	}
}

// compileWorkflows compiles every registered workflow, in source order,
// mirroring compileTypeDeclarations.
func (c *compiler) compileWorkflows(p engine.Program) map[string]engine.Workflow {
	result := make(map[string]engine.Workflow, len(c.workflowDeclarations))
	for i, w := range c.definition.Workflows {
		if w.Name == "" {
			continue
		}
		entry, registered := c.workflowDeclarations[w.Name]
		if !registered || entry.index != i {
			continue
		}
		result[w.Name] = c.compileWorkflowDeclaration(w, entry.path, p)
	}
	return result
}

// compileWorkflowDeclaration compiles one workflow's parameters, local
// state, slots, presentations, states, and transitions, in the order
// each depends on the last: slots must exist before any signal pattern
// referencing them can resolve a schema, and state names must all be
// registered before any GotoControl can be validated against them.
func (c *compiler) compileWorkflowDeclaration(w program.WorkflowDeclaration, path string, p engine.Program) engine.Workflow {
	ctx := &workflowContext{
		resultType:     c.workflowResultTypes[w.Name],
		questionSlots:  map[string]program.QuestionSlotDeclaration{},
		askGroupSlots:  map[string]program.AskGroupSlotDeclaration{},
		timerSlots:     map[string]bool{},
		childSlots:     map[string]program.ChildWorkflowSlotDeclaration{},
		taskGroupSlots: map[string]taskGroupSlotInfo{},
		stateNames:     map[string]bool{},
	}

	// Local state initializers may see the workflow's own parameters
	// (already available at instance-creation time) and resources, but
	// not "global" or "local" itself — see compileStateFields's shared
	// use for both this and Program.GlobalState.
	paramScope := exprScope{resourcesScopeRootName: c.resourcesType}
	params := c.compileFieldDeclarationsScope(w.Parameters, path+".parameters", paramScope)
	localState := c.compileStateFields(w.LocalState.Fields, path+".local_state.fields", paramScope)
	localType := engine.RecordType{Name: "local", Fields: stateFieldTypes(localState)}

	ctx.baseScope = paramScope.clone()
	ctx.baseScope["local"] = localType
	ctx.baseScope[globalScopeRootName] = globalStateRecordType(p.GlobalState)

	questionSlots := c.compileQuestionSlots(w, path, ctx)
	askGroupSlots := c.compileAskGroupSlots(w, path, ctx)
	timerSlots := c.compileTimerSlots(w, path, ctx)
	childSlots := c.compileChildSlots(w, path, ctx)
	taskGroupSlots := c.compileTaskGroupSlots(w, path, ctx)

	presentations := c.compilePresentations(w.Presentations, path+".presentations", ctx.baseScope)

	for i, s := range w.States {
		statePath := fmt.Sprintf("%s.states[%d]", path, i)
		if s.Name == "" {
			c.addf(statePath, "workflow state has an empty name")
			continue
		}
		if ctx.stateNames[s.Name] {
			c.addf(statePath, "duplicate workflow state name %q", s.Name)
			continue
		}
		ctx.stateNames[s.Name] = true
	}

	states := make([]engine.WorkflowState, 0, len(w.States))
	seenStates := map[string]bool{}
	for i, s := range w.States {
		statePath := fmt.Sprintf("%s.states[%d]", path, i)
		if s.Name == "" || seenStates[s.Name] {
			continue
		}
		seenStates[s.Name] = true
		states = append(states, c.compileWorkflowState(s, statePath, ctx))
	}

	globalTransitions := make([]engine.Transition, 0, len(w.GlobalTransitions))
	for i, t := range w.GlobalTransitions {
		globalTransitions = append(globalTransitions, c.compileTransition(t, fmt.Sprintf("%s.global_transitions[%d]", path, i), ctx))
	}

	if w.InitialState == "" {
		c.addf(path+".initial_state", "workflow %q has no initial state", w.Name)
	} else if !ctx.stateNames[w.InitialState] {
		c.addf(path+".initial_state", "initial state %q is not a declared state of workflow %q", w.InitialState, w.Name)
	}

	return engine.Workflow{
		Name:              w.Name,
		Parameters:        params,
		ResultType:        ctx.resultType,
		LocalState:        localState,
		QuestionSlots:     questionSlots,
		AskGroupSlots:     askGroupSlots,
		TimerSlots:        timerSlots,
		ChildSlots:        childSlots,
		TaskGroupSlots:    taskGroupSlots,
		Presentations:     presentations,
		InitialState:      w.InitialState,
		GlobalTransitions: globalTransitions,
		States:            states,
	}
}

func (c *compiler) compileWorkflowState(s program.WorkflowStateDeclaration, path string, ctx *workflowContext) engine.WorkflowState {
	presentations := c.compilePresentations(s.Presentations, path+".presentations", ctx.baseScope)
	transitions := make([]engine.Transition, 0, len(s.Transitions))
	for i, t := range s.Transitions {
		transitions = append(transitions, c.compileTransition(t, fmt.Sprintf("%s.transitions[%d]", path, i), ctx))
	}
	return engine.WorkflowState{Name: s.Name, Presentations: presentations, Transitions: transitions}
}

// compileFieldDeclarationsScope compiles fields (diagnosing an empty or
// duplicate name) and writes each into scope, mirroring
// compileFieldDeclarations (type.go's field-shape compiler) but for a
// []program.FieldDeclaration used to seed an exprScope — workflow or
// function parameters — rather than a type's own field shape.
func (c *compiler) compileFieldDeclarationsScope(fields []program.FieldDeclaration, pathPrefix string, scope exprScope) []engine.FieldType {
	result := make([]engine.FieldType, 0, len(fields))
	seen := make(map[string]int, len(fields))
	for i, f := range fields {
		fPath := fmt.Sprintf("%s[%d]", pathPrefix, i)
		if f.Name == "" {
			c.addf(fPath, "parameter has an empty name")
			continue
		}
		if first, ok := seen[f.Name]; ok {
			c.addf(fPath, "duplicate parameter name %q (first declared at %s[%d])", f.Name, pathPrefix, first)
			continue
		}
		seen[f.Name] = i
		t := c.compileTypeReference(f.Type, fPath+".type")
		scope[f.Name] = t
		result = append(result, engine.FieldType{Name: f.Name, Type: t})
	}
	return result
}

// compileStateFields compiles a StateDeclaration's fields (diagnosing an
// empty or duplicate name and an initializer that does not statically
// match its declared type) in scope, mirroring compileGlobalState but
// reusable for workflow LocalState too.
func (c *compiler) compileStateFields(fields []program.StateFieldDeclaration, pathPrefix string, scope exprScope) []engine.StateField {
	result := make([]engine.StateField, 0, len(fields))
	seen := make(map[string]int, len(fields))
	for i, f := range fields {
		fPath := fmt.Sprintf("%s[%d]", pathPrefix, i)
		if f.Name == "" {
			c.addf(fPath, "state field has an empty name")
			continue
		}
		if first, ok := seen[f.Name]; ok {
			c.addf(fPath, "duplicate state field name %q (first declared at %s[%d])", f.Name, pathPrefix, first)
			continue
		}
		seen[f.Name] = i

		t := c.compileTypeReference(f.Type, fPath+".type")
		init, initType := c.compileExpression(f.Initializer, scope, fPath+".initializer")
		if t != nil && initType != nil && !t.Equal(initType) {
			c.addf(fPath+".initializer", "state field %q declares type %s, but its initializer is statically %s", f.Name, describeType(t), describeType(initType))
		}
		result = append(result, engine.StateField{Name: f.Name, Type: t, Initializer: init})
	}
	return result
}

func stateFieldTypes(fields []engine.StateField) []engine.FieldType {
	types := make([]engine.FieldType, len(fields))
	for i, f := range fields {
		types[i] = engine.FieldType{Name: f.Name, Type: f.Type}
	}
	return types
}
