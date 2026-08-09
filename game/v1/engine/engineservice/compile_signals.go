package engineservice

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// namedLifecycleSignals is engineservice's initial catalog of
// platform-defined and lifecycle signals a program.NamedSignalSource may
// name — program itself declares neither the existence nor the schema
// of named signals; that is left to "the future compiler" (see
// program.NamedSignalSource's doc comment), which is this catalog.
//
// Every entry's schema is currently empty (no payload fields): a new
// workflow instance's own parameters are already in scope directly
// through the workflow's own Parameters, not exposed as WorkflowStarted
// payload fields. SessionCancelled, UserDisconnected, and
// ParentCancelled are included because program's own documentation
// names them as examples alongside WorkflowStarted; their schemas are
// left empty until a concrete payload need is identified.
var namedLifecycleSignals = map[string]map[string]engine.Type{
	"WorkflowStarted":  {},
	"SessionCancelled": {},
	"UserDisconnected": {},
	"ParentCancelled":  {},
}

// compileTransition compiles t's signal pattern, guard, and control.
//
// It does not yet compile Operations — see engine.Transition's doc
// comment — so a transition whose only semantic content is its
// Operations block (uncommon, but not disallowed) compiles here without
// diagnosing anything about that block at all.
func (c *compiler) compileTransition(t program.TransitionDeclaration, path string, ctx *workflowContext) engine.Transition {
	signal, scope := c.compileSignalPattern(t.Signal, path+".signal", ctx)

	var guard engine.Expression
	if t.Guard != nil {
		var guardType engine.Type
		guard, guardType = c.compileExpression(t.Guard, scope, path+".guard")
		if guardType != nil {
			if _, ok := guardType.(engine.BoolType); !ok {
				c.addf(path+".guard", "guard must be statically bool, but it is %s", describeType(guardType))
			}
		}
	}

	control := c.compileWorkflowControl(t.Control, scope, path+".control", ctx)

	return engine.Transition{Name: t.Name, Signal: signal, Guard: guard, Control: control}
}

// compileSignalPattern resolves pattern.Source's schema (see
// compileSignalSource) and compiles every binding, diagnosing an empty
// or duplicate lexical name and a Field the resolved schema does not
// expose. It returns the compiled pattern together with ctx.baseScope
// extended with every successfully bound name — the scope the owning
// transition's Guard and Control compile in.
func (c *compiler) compileSignalPattern(pattern program.SignalPattern, path string, ctx *workflowContext) (engine.SignalPattern, exprScope) {
	source, schema := c.compileSignalSource(pattern.Source, path+".source", ctx)

	scope := ctx.baseScope.clone()
	bindings := make([]engine.SignalBinding, 0, len(pattern.Bindings))
	seen := make(map[string]int, len(pattern.Bindings))
	for i, b := range pattern.Bindings {
		bPath := fmt.Sprintf("%s.bindings[%d]", path, i)
		if b.Name == "" {
			c.addf(bPath+".name", "signal binding has an empty lexical name")
			continue
		}
		if first, ok := seen[b.Name]; ok {
			c.addf(bPath+".name", "duplicate signal binding name %q (first declared at %s.bindings[%d])", b.Name, path, first)
			continue
		}
		seen[b.Name] = i

		t, ok := schema[b.Field]
		if !ok {
			c.addf(bPath+".field", "signal has no field named %q", b.Field)
			continue
		}
		scope[b.Name] = t
		bindings = append(bindings, engine.SignalBinding{Field: b.Field, Name: b.Name})
	}

	return engine.SignalPattern{Source: source, Bindings: bindings}, scope
}

// compileSignalSource resolves source into its compiled form and the
// schema (field name to Type) its matched signal exposes for binding —
// see engine.SignalSource's variants for what each Slot must name.
func (c *compiler) compileSignalSource(source program.SignalSource, path string, ctx *workflowContext) (engine.SignalSource, map[string]engine.Type) {
	switch s := source.(type) {
	case nil:
		c.addf(path, "missing signal source")
		return nil, map[string]engine.Type{}

	case program.NamedSignalSource:
		schema, ok := namedLifecycleSignals[s.Name]
		if !ok {
			c.addf(path+".name", "unknown named signal %q", s.Name)
			return engine.NamedSignalSource{Name: s.Name}, map[string]engine.Type{}
		}
		return engine.NamedSignalSource{Name: s.Name}, schema

	case program.UserIntentSignalSource:
		intent, ok := c.userIntentByName(s.Intent)
		if !ok {
			c.addf(path+".intent", "reference to undeclared user intent %q", s.Intent)
			return engine.UserIntentSignalSource{Intent: s.Intent}, map[string]engine.Type{}
		}
		schema := map[string]engine.Type{"actor": engine.UserType{}}
		for _, p := range intent.Parameters {
			if p.Name != "" {
				schema[p.Name] = c.compileTypeReference(p.Type, path+".user_intent_parameters")
			}
		}
		return engine.UserIntentSignalSource{Intent: s.Intent}, schema

	case program.QuestionAnsweredSignalSource:
		slot, ok := ctx.questionSlots[s.Slot]
		if !ok {
			c.addf(path+".slot", "reference to undeclared question slot %q", s.Slot)
			return engine.QuestionAnsweredSignalSource{Slot: s.Slot}, map[string]engine.Type{}
		}
		return engine.QuestionAnsweredSignalSource{Slot: s.Slot}, map[string]engine.Type{
			"respondent": engine.UserType{},
			"answer":     c.questionResponseType(slot.Question, path),
		}

	case program.TimerExpiredSignalSource:
		if !ctx.timerSlots[s.Slot] {
			c.addf(path+".slot", "reference to undeclared timer slot %q", s.Slot)
		}
		return engine.TimerExpiredSignalSource{Slot: s.Slot}, map[string]engine.Type{}

	case program.ChildCompletedSignalSource:
		child, ok := ctx.childSlots[s.Slot]
		if !ok {
			c.addf(path+".slot", "reference to undeclared child slot %q", s.Slot)
			return engine.ChildCompletedSignalSource{Slot: s.Slot}, map[string]engine.Type{}
		}
		return engine.ChildCompletedSignalSource{Slot: s.Slot}, map[string]engine.Type{"result": c.workflowResultTypes[child.Workflow]}

	case program.ChildFailedSignalSource:
		if _, ok := ctx.childSlots[s.Slot]; !ok {
			c.addf(path+".slot", "reference to undeclared child slot %q", s.Slot)
		}
		return engine.ChildFailedSignalSource{Slot: s.Slot}, map[string]engine.Type{"error": engine.StringType{}}

	case program.ChildCancelledSignalSource:
		if _, ok := ctx.childSlots[s.Slot]; !ok {
			c.addf(path+".slot", "reference to undeclared child slot %q", s.Slot)
		}
		return engine.ChildCancelledSignalSource{Slot: s.Slot}, map[string]engine.Type{"reason": engine.StringType{}}

	case program.AskGroupCompletedSignalSource:
		slot, ok := ctx.askGroupSlots[s.Slot]
		if !ok {
			c.addf(path+".slot", "reference to undeclared ask-group slot %q", s.Slot)
			return engine.AskGroupCompletedSignalSource{Slot: s.Slot}, map[string]engine.Type{}
		}
		responseType := c.questionResponseType(slot.Question, path)
		return engine.AskGroupCompletedSignalSource{Slot: s.Slot}, map[string]engine.Type{
			"responses":   engine.MapType{Key: engine.UserType{}, Value: responseType},
			"respondents": engine.ListType{Element: engine.UserType{}},
			"missing":     engine.ListType{Element: engine.UserType{}},
		}

	case program.TaskGroupCompletedSignalSource:
		info, ok := ctx.taskGroupSlots[s.Slot]
		if !ok {
			c.addf(path+".slot", "reference to undeclared task-group slot %q", s.Slot)
			return engine.TaskGroupCompletedSignalSource{Slot: s.Slot}, map[string]engine.Type{}
		}
		resultType := c.workflowResultTypes[info.workflow]
		keyType := info.keyType
		return engine.TaskGroupCompletedSignalSource{Slot: s.Slot}, map[string]engine.Type{
			"taskKeys":      engine.ListType{Element: keyType},
			"terminalKeys":  engine.ListType{Element: keyType},
			"results":       engine.MapType{Key: keyType, Value: resultType},
			"failures":      engine.MapType{Key: keyType, Value: engine.StringType{}},
			"cancellations": engine.MapType{Key: keyType, Value: engine.StringType{}},
			"unfinished":    engine.ListType{Element: keyType},
		}

	default:
		c.addf(path, "unsupported signal source")
		return nil, map[string]engine.Type{}
	}
}
