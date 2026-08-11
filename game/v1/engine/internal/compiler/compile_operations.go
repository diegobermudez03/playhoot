package compiler

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// compileBlock compiles block's operations in order, threading scope
// forward so a LetOperation's binding is visible to later operations in
// this same Block. It returns the scope in effect after the last
// operation, so a caller compiling a Transition's top-level Block can
// pass it on to Control.
//
// An operation this compiler does not support (spawning a child or
// task, or an ask-group/task-group operation — see engine.Operation's
// doc comment) is diagnosed and simply omitted from the compiled Block.
func (c *compiler) compileBlock(block program.Block, scope exprScope, path string, ctx *workflowContext) (engine.Block, exprScope) {
	ops := make([]engine.Operation, 0, len(block.Operations))
	for i, op := range block.Operations {
		opPath := fmt.Sprintf("%s.operations[%d]", path, i)
		compiled, newScope, ok := c.compileOperation(op, scope, opPath, ctx)
		scope = newScope
		if ok {
			ops = append(ops, compiled)
		}
	}
	return engine.Block{Operations: ops}, scope
}

func (c *compiler) compileOperation(op program.Operation, scope exprScope, path string, ctx *workflowContext) (engine.Operation, exprScope, bool) {
	switch o := op.(type) {
	case nil:
		c.addf(path, "missing operation")
		return nil, scope, false

	case program.LetOperation:
		if o.Name == "" {
			c.addf(path+".name", "let operation has an empty name")
			return nil, scope, false
		}
		v, vType := c.compileExpression(o.Value, scope, path+".value")
		if vType == nil {
			return nil, scope, false
		}
		newScope := scope.clone()
		newScope[o.Name] = vType
		return engine.LetOperation{Name: o.Name, Value: v}, newScope, true

	case program.SetOperation:
		target, targetType := c.compileAssignmentTarget(o.Target, scope, path+".target")
		v, vType := c.compileExpression(o.Value, scope, path+".value")
		if target == nil || vType == nil {
			return nil, scope, false
		}
		if targetType != nil && !targetType.Equal(vType) {
			c.addf(path+".value", "assignment target is statically %s, but the value is statically %s", describeType(targetType), describeType(vType))
			return nil, scope, false
		}
		return engine.SetOperation{Target: target, Value: v}, scope, true

	case program.ListAppendOperation:
		target, targetType := c.compileAssignmentTarget(o.Target, scope, path+".target")
		v, vType := c.compileExpression(o.Value, scope, path+".value")
		if target == nil || vType == nil {
			return nil, scope, false
		}
		lt, ok := targetType.(engine.ListType)
		if !ok {
			c.addf(path+".target", "list append requires a list target, but it is statically %s", describeType(targetType))
			return nil, scope, false
		}
		if !lt.Element.Equal(vType) {
			c.addf(path+".value", "list element is statically %s, but the list's element type is %s", describeType(vType), describeType(lt.Element))
			return nil, scope, false
		}
		return engine.ListAppendOperation{Target: target, Value: v}, scope, true

	case program.ListInsertOperation:
		target, targetType := c.compileAssignmentTarget(o.Target, scope, path+".target")
		idx, idxType := c.compileExpression(o.Index, scope, path+".index")
		v, vType := c.compileExpression(o.Value, scope, path+".value")
		if target == nil || idxType == nil || vType == nil {
			return nil, scope, false
		}
		if _, ok := idxType.(engine.NumberType); !ok {
			c.addf(path+".index", "list index must be statically number, but it is %s", describeType(idxType))
			return nil, scope, false
		}
		lt, ok := targetType.(engine.ListType)
		if !ok {
			c.addf(path+".target", "list insert requires a list target, but it is statically %s", describeType(targetType))
			return nil, scope, false
		}
		if !lt.Element.Equal(vType) {
			c.addf(path+".value", "list element is statically %s, but the list's element type is %s", describeType(vType), describeType(lt.Element))
			return nil, scope, false
		}
		return engine.ListInsertOperation{Target: target, Index: idx, Value: v}, scope, true

	case program.ListRemoveAtOperation:
		target, targetType := c.compileAssignmentTarget(o.Target, scope, path+".target")
		idx, idxType := c.compileExpression(o.Index, scope, path+".index")
		if target == nil || idxType == nil {
			return nil, scope, false
		}
		if _, ok := idxType.(engine.NumberType); !ok {
			c.addf(path+".index", "list index must be statically number, but it is %s", describeType(idxType))
			return nil, scope, false
		}
		if _, ok := targetType.(engine.ListType); !ok {
			c.addf(path+".target", "list remove requires a list target, but it is statically %s", describeType(targetType))
			return nil, scope, false
		}
		return engine.ListRemoveAtOperation{Target: target, Index: idx}, scope, true

	case program.MapPutOperation:
		target, targetType := c.compileAssignmentTarget(o.Target, scope, path+".target")
		key, keyType := c.compileExpression(o.Key, scope, path+".key")
		v, vType := c.compileExpression(o.Value, scope, path+".value")
		if target == nil || keyType == nil || vType == nil {
			return nil, scope, false
		}
		mt, ok := targetType.(engine.MapType)
		if !ok {
			c.addf(path+".target", "map put requires a map target, but it is statically %s", describeType(targetType))
			return nil, scope, false
		}
		ok = true
		if !mt.Key.Equal(keyType) {
			c.addf(path+".key", "map key is statically %s, but the map's key type is %s", describeType(keyType), describeType(mt.Key))
			ok = false
		}
		if !mt.Value.Equal(vType) {
			c.addf(path+".value", "map value is statically %s, but the map's value type is %s", describeType(vType), describeType(mt.Value))
			ok = false
		}
		if !ok {
			return nil, scope, false
		}
		return engine.MapPutOperation{Target: target, Key: key, Value: v}, scope, true

	case program.MapDeleteOperation:
		target, targetType := c.compileAssignmentTarget(o.Target, scope, path+".target")
		key, keyType := c.compileExpression(o.Key, scope, path+".key")
		if target == nil || keyType == nil {
			return nil, scope, false
		}
		mt, ok := targetType.(engine.MapType)
		if !ok {
			c.addf(path+".target", "map delete requires a map target, but it is statically %s", describeType(targetType))
			return nil, scope, false
		}
		if !mt.Key.Equal(keyType) {
			c.addf(path+".key", "map key is statically %s, but the map's key type is %s", describeType(keyType), describeType(mt.Key))
			return nil, scope, false
		}
		return engine.MapDeleteOperation{Target: target, Key: key}, scope, true

	case program.IfOperation:
		cond, condType := c.compileExpression(o.Condition, scope, path+".condition")
		if condType != nil {
			if _, ok := condType.(engine.BoolType); !ok {
				c.addf(path+".condition", "condition must be statically bool, but it is %s", describeType(condType))
			}
		}
		then, _ := c.compileBlock(o.Then, scope, path+".then", ctx)
		els, _ := c.compileBlock(o.Else, scope, path+".else", ctx)
		return engine.IfOperation{Condition: cond, Then: then, Else: els}, scope, true

	case program.ForEachOperation:
		coll, collType := c.compileExpression(o.Collection, scope, path+".collection")
		if collType == nil {
			return nil, scope, false
		}
		lt, ok := collType.(engine.ListType)
		if !ok {
			c.addf(path+".collection", "for-each requires a list, but the collection is statically %s", describeType(collType))
			return nil, scope, false
		}
		bodyScope, sOk := c.bindListItem(scope, o.ItemName, o.IndexName, lt.Element, path)
		if !sOk {
			return nil, scope, false
		}
		body, _ := c.compileBlock(o.Body, bodyScope, path+".body", ctx)
		return engine.ForEachOperation{Collection: coll, ItemName: o.ItemName, IndexName: o.IndexName, Body: body}, scope, true

	case program.MatchOperation:
		return c.compileMatchOperation(o, scope, path, ctx)

	case program.DrawRandomOperation:
		return c.compileDrawRandom(o, scope, path)

	case program.OpenQuestionOperation:
		return c.compileOpenQuestion(o, scope, path, ctx)

	case program.CloseQuestionOperation:
		return c.compileCloseQuestion(o, scope, path, ctx)

	case program.ScheduleTimerOperation:
		return c.compileScheduleTimer(o, scope, path, ctx)

	case program.CancelTimerOperation:
		return c.compileCancelTimer(o, scope, path, ctx)

	case program.EmitEffectOperation:
		return c.compileEmitEffect(o, scope, path)

	case program.SpawnChildWorkflowOperation:
		return c.compileSpawnChildWorkflow(o, scope, path, ctx)

	case program.CancelChildWorkflowOperation:
		return c.compileCancelChildWorkflow(o, scope, path, ctx)

	case program.OpenAskGroupOperation:
		return c.compileOpenAskGroup(o, scope, path, ctx)

	case program.FinalizeAskGroupOperation:
		return c.compileFinalizeAskGroup(o, scope, path, ctx)

	case program.CancelAskGroupOperation:
		return c.compileCancelAskGroup(o, scope, path, ctx)

	case program.BeginTaskGroupOperation:
		return c.compileBeginTaskGroup(o, scope, path, ctx)

	case program.SpawnTaskGroupChildOperation:
		return c.compileSpawnTaskGroupChild(o, scope, path, ctx)

	case program.SealTaskGroupOperation:
		return c.compileSealTaskGroup(o, scope, path, ctx)

	case program.FinalizeTaskGroupOperation:
		return c.compileFinalizeTaskGroup(o, scope, path, ctx)

	case program.CancelTaskGroupOperation:
		return c.compileCancelTaskGroup(o, scope, path, ctx)

	default:
		c.addf(path, "operation %T is not yet supported by this compiler", op)
		return nil, scope, false
	}
}

// compileDrawRandom compiles one program.DrawRandomOperation, inferring
// Name's bound type entirely from Generator, per
// program.DrawRandomOperation's documented "no explicit Type field".
func (c *compiler) compileDrawRandom(o program.DrawRandomOperation, scope exprScope, path string) (engine.Operation, exprScope, bool) {
	if o.Name == "" {
		c.addf(path+".name", "draw random operation has an empty name")
		return nil, scope, false
	}
	generator, resultType, ok := c.compileRandomGenerator(o.Generator, scope, path+".generator")
	if !ok {
		return nil, scope, false
	}
	newScope := scope.clone()
	newScope[o.Name] = resultType
	return engine.DrawRandomOperation{Name: o.Name, Generator: generator}, newScope, true
}

// compileRandomGenerator compiles generator, returning the Type of the
// value it produces.
func (c *compiler) compileRandomGenerator(g program.RandomGenerator, scope exprScope, path string) (engine.RandomGenerator, engine.Type, bool) {
	switch gen := g.(type) {
	case nil:
		c.addf(path, "missing random generator")
		return nil, nil, false

	case program.RandomIntegerGenerator:
		minExpr, minType := c.compileExpression(gen.Minimum, scope, path+".minimum")
		maxExpr, maxType := c.compileExpression(gen.Maximum, scope, path+".maximum")
		if minType == nil || maxType == nil {
			return nil, nil, false
		}
		ok := true
		if !isNumber(minType) {
			c.addf(path+".minimum", "minimum must be statically number, but it is %s", describeType(minType))
			ok = false
		}
		if !isNumber(maxType) {
			c.addf(path+".maximum", "maximum must be statically number, but it is %s", describeType(maxType))
			ok = false
		}
		if !ok {
			return nil, nil, false
		}
		return engine.RandomIntegerGenerator{Minimum: minExpr, Maximum: maxExpr}, engine.NumberType{}, true

	case program.RandomElementGenerator:
		collExpr, collType, ok := c.compileListCollection(gen.Collection, scope, path)
		if !ok {
			return nil, nil, false
		}
		return engine.RandomElementGenerator{Collection: collExpr}, collType.Element, true

	case program.RandomShuffleGenerator:
		collExpr, collType, ok := c.compileListCollection(gen.Collection, scope, path)
		if !ok {
			return nil, nil, false
		}
		return engine.RandomShuffleGenerator{Collection: collExpr}, collType, true

	default:
		c.addf(path, "unsupported random generator")
		return nil, nil, false
	}
}

// compileMatchOperation mirrors compileMatchControl's shape: each case's
// Body is compiled independently, with no result type to unify across
// cases.
func (c *compiler) compileMatchOperation(m program.MatchOperation, scope exprScope, path string, ctx *workflowContext) (engine.Operation, exprScope, bool) {
	value, valueType := c.compileExpression(m.Value, scope, path+".value")
	if len(m.Cases) == 0 {
		c.addf(path, "match operation has no cases")
	}

	cases := make([]engine.MatchOperationCase, 0, len(m.Cases))
	for i, cs := range m.Cases {
		casePath := fmt.Sprintf("%s.cases[%d]", path, i)
		pattern, caseScope, _ := c.compileMatchPattern(cs.Pattern, valueType, scope, casePath+".pattern")
		body, _ := c.compileBlock(cs.Body, caseScope, casePath+".body", ctx)
		cases = append(cases, engine.MatchOperationCase{Pattern: pattern, Body: body})
	}

	return engine.MatchOperation{Value: value, Cases: cases}, scope, valueType != nil
}

// compileOpenQuestion compiles one program.OpenQuestionOperation: Slot
// must name a question slot declared on the enclosing workflow,
// Recipient must be statically user, and Arguments must match the
// slot's question's declared parameters exactly (see
// checkCallArguments).
func (c *compiler) compileOpenQuestion(o program.OpenQuestionOperation, scope exprScope, path string, ctx *workflowContext) (engine.Operation, exprScope, bool) {
	slotDecl, slotOK := ctx.questionSlots[o.Slot]
	if !slotOK {
		c.addf(path+".slot", "reference to undeclared question slot %q", o.Slot)
	}

	recipient, recipientType := c.compileExpression(o.Recipient, scope, path+".recipient")
	ok := recipientType != nil
	if recipientType != nil && !isUser(recipientType) {
		c.addf(path+".recipient", "recipient must be statically user, but it is %s", describeType(recipientType))
		ok = false
	}

	args, argTypes, argsOK := c.compileCallArguments(o.Arguments, scope, path)
	if !argsOK {
		ok = false
	}
	if slotOK {
		if question, qOK := c.compiledQuestions[slotDecl.Question]; qOK {
			if !c.checkCallArguments(question.Parameters, args, argTypes, path) {
				ok = false
			}
		}
	}

	if !slotOK || !ok {
		return nil, scope, false
	}
	return engine.OpenQuestionOperation{Slot: o.Slot, Recipient: recipient, Arguments: args}, scope, true
}

// compileCloseQuestion compiles one program.CloseQuestionOperation: Slot
// must name a question slot declared on the enclosing workflow.
func (c *compiler) compileCloseQuestion(o program.CloseQuestionOperation, scope exprScope, path string, ctx *workflowContext) (engine.Operation, exprScope, bool) {
	if _, ok := ctx.questionSlots[o.Slot]; !ok {
		c.addf(path+".slot", "reference to undeclared question slot %q", o.Slot)
		return nil, scope, false
	}
	return engine.CloseQuestionOperation{Slot: o.Slot}, scope, true
}

// compileScheduleTimer compiles one program.ScheduleTimerOperation: Slot
// must name a timer slot declared on the enclosing workflow, and
// DelayMilliseconds must be statically number.
func (c *compiler) compileScheduleTimer(o program.ScheduleTimerOperation, scope exprScope, path string, ctx *workflowContext) (engine.Operation, exprScope, bool) {
	if !ctx.timerSlots[o.Slot] {
		c.addf(path+".slot", "reference to undeclared timer slot %q", o.Slot)
		return nil, scope, false
	}
	delay, delayType := c.compileExpression(o.DelayMilliseconds, scope, path+".delay_milliseconds")
	if delayType == nil {
		return nil, scope, false
	}
	if !isNumber(delayType) {
		c.addf(path+".delay_milliseconds", "delay must be statically number, but it is %s", describeType(delayType))
		return nil, scope, false
	}
	return engine.ScheduleTimerOperation{Slot: o.Slot, DelayMilliseconds: delay}, scope, true
}

// compileCancelTimer compiles one program.CancelTimerOperation: Slot
// must name a timer slot declared on the enclosing workflow.
func (c *compiler) compileCancelTimer(o program.CancelTimerOperation, scope exprScope, path string, ctx *workflowContext) (engine.Operation, exprScope, bool) {
	if !ctx.timerSlots[o.Slot] {
		c.addf(path+".slot", "reference to undeclared timer slot %q", o.Slot)
		return nil, scope, false
	}
	return engine.CancelTimerOperation{Slot: o.Slot}, scope, true
}

// compileEmitEffect compiles one program.EmitEffectOperation: Effect
// must name a declared effect, Recipients must be statically
// list<user>, and Arguments must match the effect's declared parameters
// exactly (see checkCallArguments).
func (c *compiler) compileEmitEffect(o program.EmitEffectOperation, scope exprScope, path string) (engine.Operation, exprScope, bool) {
	recipients, recipientsType := c.compileExpression(o.Recipients, scope, path+".recipients")
	ok := recipientsType != nil
	if recipientsType != nil {
		lt, isList := recipientsType.(engine.ListType)
		if !isList || !isUser(lt.Element) {
			c.addf(path+".recipients", "recipients must be statically list<user>, but it is %s", describeType(recipientsType))
			ok = false
		}
	}

	args, argTypes, argsOK := c.compileCallArguments(o.Arguments, scope, path)
	if !argsOK {
		ok = false
	}

	effect, effectOK := c.compiledEffects[o.Effect]
	if !effectOK {
		c.addf(path+".effect", "reference to undeclared effect %q", o.Effect)
		ok = false
	} else if !c.checkCallArguments(effect.Parameters, args, argTypes, path) {
		ok = false
	}

	if !ok {
		return nil, scope, false
	}
	return engine.EmitEffectOperation{Effect: o.Effect, Recipients: recipients, Arguments: args}, scope, true
}

// compileSpawnChildWorkflow compiles one program.SpawnChildWorkflowOperation:
// Slot must name a child slot declared on the enclosing workflow, and
// Arguments must match that slot's declared workflow's declared
// parameters exactly (see checkCallArguments).
func (c *compiler) compileSpawnChildWorkflow(o program.SpawnChildWorkflowOperation, scope exprScope, path string, ctx *workflowContext) (engine.Operation, exprScope, bool) {
	slotDecl, slotOK := ctx.childSlots[o.Slot]
	if !slotOK {
		c.addf(path+".slot", "reference to undeclared child slot %q", o.Slot)
	}

	args, argTypes, argsOK := c.compileCallArguments(o.Arguments, scope, path)
	ok := argsOK
	if slotOK {
		if params, wfOK := c.workflowParameterTypes[slotDecl.Workflow]; wfOK {
			if !c.checkCallArguments(params, args, argTypes, path) {
				ok = false
			}
		}
	}

	if !slotOK || !ok {
		return nil, scope, false
	}
	return engine.SpawnChildWorkflowOperation{Slot: o.Slot, Arguments: args}, scope, true
}

// compileCancelChildWorkflow compiles one program.CancelChildWorkflowOperation:
// Slot must name a child slot declared on the enclosing workflow, and
// Reason must be statically string.
func (c *compiler) compileCancelChildWorkflow(o program.CancelChildWorkflowOperation, scope exprScope, path string, ctx *workflowContext) (engine.Operation, exprScope, bool) {
	if _, ok := ctx.childSlots[o.Slot]; !ok {
		c.addf(path+".slot", "reference to undeclared child slot %q", o.Slot)
		return nil, scope, false
	}
	reason, reasonType := c.compileExpression(o.Reason, scope, path+".reason")
	if reasonType == nil {
		return nil, scope, false
	}
	if _, ok := reasonType.(engine.StringType); !ok {
		c.addf(path+".reason", "cancel reason must be statically string, but it is %s", describeType(reasonType))
		return nil, scope, false
	}
	return engine.CancelChildWorkflowOperation{Slot: o.Slot, Reason: reason}, scope, true
}

// compileAssignmentTarget compiles target, returning its resolved
// element Type. The compiler only accepts "global" and "local" as a
// target's root — every other name is immutable, per
// program.LetOperation's documented restriction that a lexical binding
// "can never be assigned through a SetOperation".
func (c *compiler) compileAssignmentTarget(target program.AssignmentTarget, scope exprScope, path string) (engine.AssignmentTarget, engine.Type) {
	switch t := target.(type) {
	case nil:
		c.addf(path, "missing assignment target")
		return nil, nil

	case program.NameTarget:
		if t.Name != globalScopeRootName && t.Name != "local" {
			c.addf(path, "cannot assign to %q; only \"global\" and \"local\" are mutable", t.Name)
			return nil, nil
		}
		tp, ok := scope[t.Name]
		if !ok {
			c.addf(path, "%q is not in scope here", t.Name)
			return nil, nil
		}
		return engine.NameTarget{Name: t.Name}, tp

	case program.FieldTarget:
		inner, innerType := c.compileAssignmentTarget(t.Target, scope, path+".target")
		if inner == nil || innerType == nil {
			return nil, nil
		}
		rt, ok := innerType.(engine.RecordType)
		if !ok {
			c.addf(path, "field assignment requires a record, but the target is statically %s", describeType(innerType))
			return nil, nil
		}
		ft, ok := rt.FieldByName(t.Field)
		if !ok {
			c.addf(path+".field", "record %q has no field named %q", rt.Name, t.Field)
			return nil, nil
		}
		return engine.FieldTarget{Target: inner, Field: t.Field}, ft.Type

	case program.IndexTarget:
		inner, innerType := c.compileAssignmentTarget(t.Target, scope, path+".target")
		index, indexType := c.compileExpression(t.Index, scope, path+".index")
		if inner == nil || innerType == nil || indexType == nil {
			return nil, nil
		}
		switch it := innerType.(type) {
		case engine.ListType:
			if _, ok := indexType.(engine.NumberType); !ok {
				c.addf(path+".index", "list index must be statically number, but it is %s", describeType(indexType))
				return nil, nil
			}
			return engine.IndexTarget{Target: inner, Index: index}, it.Element
		case engine.MapType:
			if !it.Key.Equal(indexType) {
				c.addf(path+".index", "map index must be statically %s, but it is %s", describeType(it.Key), describeType(indexType))
				return nil, nil
			}
			return engine.IndexTarget{Target: inner, Index: index}, it.Value
		default:
			c.addf(path, "indexing requires a list or map, but the target is statically %s", describeType(innerType))
			return nil, nil
		}

	default:
		c.addf(path, "unsupported assignment target")
		return nil, nil
	}
}
