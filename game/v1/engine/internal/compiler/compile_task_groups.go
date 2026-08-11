package compiler

import (
	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// compileBeginTaskGroup compiles one program.BeginTaskGroupOperation:
// Slot must name a task-group slot declared on the enclosing workflow,
// and Completion must compile to a valid policy.
func (c *compiler) compileBeginTaskGroup(o program.BeginTaskGroupOperation, scope exprScope, path string, ctx *workflowContext) (engine.Operation, exprScope, bool) {
	if _, ok := ctx.taskGroupSlots[o.Slot]; !ok {
		c.addf(path+".slot", "reference to undeclared task-group slot %q", o.Slot)
		return nil, scope, false
	}
	completion, ok := c.compileTaskGroupCompletionPolicy(o.Completion, scope, path+".completion")
	if !ok {
		return nil, scope, false
	}
	return engine.BeginTaskGroupOperation{Slot: o.Slot, Completion: completion}, scope, true
}

// compileTaskGroupCompletionPolicy compiles one program.TaskGroupCompletionPolicy,
// validating TaskGroupQuorumTerminalPolicy's Count is statically number.
func (c *compiler) compileTaskGroupCompletionPolicy(policy program.TaskGroupCompletionPolicy, scope exprScope, path string) (engine.TaskGroupCompletionPolicy, bool) {
	switch p := policy.(type) {
	case nil:
		c.addf(path, "missing task-group completion policy")
		return nil, false

	case program.TaskGroupAllTerminalPolicy:
		return engine.TaskGroupAllTerminalPolicy{}, true

	case program.TaskGroupFirstTerminalPolicy:
		return engine.TaskGroupFirstTerminalPolicy{}, true

	case program.TaskGroupQuorumTerminalPolicy:
		count, countType := c.compileExpression(p.Count, scope, path+".count")
		if countType == nil {
			return nil, false
		}
		if !isNumber(countType) {
			c.addf(path+".count", "quorum count must be statically number, but it is %s", describeType(countType))
			return nil, false
		}
		return engine.TaskGroupQuorumTerminalPolicy{Count: count}, true

	default:
		c.addf(path, "unsupported task-group completion policy")
		return nil, false
	}
}

// compileSpawnTaskGroupChild compiles one program.SpawnTaskGroupChildOperation:
// Slot must name a task-group slot declared on the enclosing workflow,
// Key must statically match that slot's declared KeyType, and Arguments
// must match the slot's declared workflow's declared parameters exactly
// (see checkCallArguments).
func (c *compiler) compileSpawnTaskGroupChild(o program.SpawnTaskGroupChildOperation, scope exprScope, path string, ctx *workflowContext) (engine.Operation, exprScope, bool) {
	info, slotOK := ctx.taskGroupSlots[o.Slot]
	if !slotOK {
		c.addf(path+".slot", "reference to undeclared task-group slot %q", o.Slot)
	}

	key, keyType := c.compileExpression(o.Key, scope, path+".key")
	ok := keyType != nil
	if slotOK && keyType != nil && info.keyType != nil && !info.keyType.Equal(keyType) {
		c.addf(path+".key", "task key is statically %s, but the slot's declared key type is %s", describeType(keyType), describeType(info.keyType))
		ok = false
	}

	args, argTypes, argsOK := c.compileCallArguments(o.Arguments, scope, path)
	if !argsOK {
		ok = false
	}
	if slotOK {
		if params, wfOK := c.workflowParameterTypes[info.workflow]; wfOK {
			if !c.checkCallArguments(params, args, argTypes, path) {
				ok = false
			}
		}
	}

	if !slotOK || !ok {
		return nil, scope, false
	}
	return engine.SpawnTaskGroupChildOperation{Slot: o.Slot, Key: key, Arguments: args}, scope, true
}

// compileSealTaskGroup compiles one program.SealTaskGroupOperation: Slot
// must name a task-group slot declared on the enclosing workflow.
func (c *compiler) compileSealTaskGroup(o program.SealTaskGroupOperation, scope exprScope, path string, ctx *workflowContext) (engine.Operation, exprScope, bool) {
	if _, ok := ctx.taskGroupSlots[o.Slot]; !ok {
		c.addf(path+".slot", "reference to undeclared task-group slot %q", o.Slot)
		return nil, scope, false
	}
	return engine.SealTaskGroupOperation{Slot: o.Slot}, scope, true
}

// compileFinalizeTaskGroup compiles one program.FinalizeTaskGroupOperation:
// Slot must name a task-group slot declared on the enclosing workflow.
func (c *compiler) compileFinalizeTaskGroup(o program.FinalizeTaskGroupOperation, scope exprScope, path string, ctx *workflowContext) (engine.Operation, exprScope, bool) {
	if _, ok := ctx.taskGroupSlots[o.Slot]; !ok {
		c.addf(path+".slot", "reference to undeclared task-group slot %q", o.Slot)
		return nil, scope, false
	}
	return engine.FinalizeTaskGroupOperation{Slot: o.Slot}, scope, true
}

// compileCancelTaskGroup compiles one program.CancelTaskGroupOperation:
// Slot must name a task-group slot declared on the enclosing workflow,
// and Reason must be statically string.
func (c *compiler) compileCancelTaskGroup(o program.CancelTaskGroupOperation, scope exprScope, path string, ctx *workflowContext) (engine.Operation, exprScope, bool) {
	if _, ok := ctx.taskGroupSlots[o.Slot]; !ok {
		c.addf(path+".slot", "reference to undeclared task-group slot %q", o.Slot)
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
	return engine.CancelTaskGroupOperation{Slot: o.Slot, Reason: reason}, scope, true
}
