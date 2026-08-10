package engineservice

import (
	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// compileOpenAskGroup compiles one program.OpenAskGroupOperation: Slot
// must name an ask-group slot declared on the enclosing workflow,
// Recipients must be statically list<user>, Arguments must match the
// slot's question's declared parameters exactly (see checkCallArguments),
// and Completion must compile to a valid policy.
func (c *compiler) compileOpenAskGroup(o program.OpenAskGroupOperation, scope exprScope, path string, ctx *workflowContext) (engine.Operation, exprScope, bool) {
	slotDecl, slotOK := ctx.askGroupSlots[o.Slot]
	if !slotOK {
		c.addf(path+".slot", "reference to undeclared ask-group slot %q", o.Slot)
	}

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
	if slotOK {
		if question, qOK := c.compiledQuestions[slotDecl.Question]; qOK {
			if !c.checkCallArguments(question.Parameters, args, argTypes, path) {
				ok = false
			}
		}
	}

	completion, compOK := c.compileAskGroupCompletionPolicy(o.Completion, scope, path+".completion")
	if !compOK {
		ok = false
	}

	if !slotOK || !ok {
		return nil, scope, false
	}
	return engine.OpenAskGroupOperation{Slot: o.Slot, Recipients: recipients, Arguments: args, Completion: completion}, scope, true
}

// compileAskGroupCompletionPolicy compiles one program.AskGroupCompletionPolicy,
// validating AskGroupQuorumPolicy's Count is statically number.
func (c *compiler) compileAskGroupCompletionPolicy(policy program.AskGroupCompletionPolicy, scope exprScope, path string) (engine.AskGroupCompletionPolicy, bool) {
	switch p := policy.(type) {
	case nil:
		c.addf(path, "missing ask-group completion policy")
		return nil, false

	case program.AskGroupAllResponsesPolicy:
		return engine.AskGroupAllResponsesPolicy{}, true

	case program.AskGroupFirstResponsePolicy:
		return engine.AskGroupFirstResponsePolicy{}, true

	case program.AskGroupQuorumPolicy:
		count, countType := c.compileExpression(p.Count, scope, path+".count")
		if countType == nil {
			return nil, false
		}
		if !isNumber(countType) {
			c.addf(path+".count", "quorum count must be statically number, but it is %s", describeType(countType))
			return nil, false
		}
		return engine.AskGroupQuorumPolicy{Count: count}, true

	default:
		c.addf(path, "unsupported ask-group completion policy")
		return nil, false
	}
}

// compileFinalizeAskGroup compiles one program.FinalizeAskGroupOperation:
// Slot must name an ask-group slot declared on the enclosing workflow.
func (c *compiler) compileFinalizeAskGroup(o program.FinalizeAskGroupOperation, scope exprScope, path string, ctx *workflowContext) (engine.Operation, exprScope, bool) {
	if _, ok := ctx.askGroupSlots[o.Slot]; !ok {
		c.addf(path+".slot", "reference to undeclared ask-group slot %q", o.Slot)
		return nil, scope, false
	}
	return engine.FinalizeAskGroupOperation{Slot: o.Slot}, scope, true
}

// compileCancelAskGroup compiles one program.CancelAskGroupOperation:
// Slot must name an ask-group slot declared on the enclosing workflow.
func (c *compiler) compileCancelAskGroup(o program.CancelAskGroupOperation, scope exprScope, path string, ctx *workflowContext) (engine.Operation, exprScope, bool) {
	if _, ok := ctx.askGroupSlots[o.Slot]; !ok {
		c.addf(path+".slot", "reference to undeclared ask-group slot %q", o.Slot)
		return nil, scope, false
	}
	return engine.CancelAskGroupOperation{Slot: o.Slot}, scope, true
}
