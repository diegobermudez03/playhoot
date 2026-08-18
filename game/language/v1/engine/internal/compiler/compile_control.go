package compiler

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

// compileWorkflowControl compiles control against ctx (needed for
// GotoControl's target-state check and CompleteControl's result-type
// check).
func (c *compiler) compileWorkflowControl(control program.WorkflowControl, scope exprScope, path string, ctx *workflowContext) engine.WorkflowControl {
	switch ctrl := control.(type) {
	case nil:
		c.addf(path, "missing workflow control")
		return nil

	case program.GotoControl:
		if ctrl.State == "" {
			c.addf(path+".state", "goto control has an empty state name")
		} else if !ctx.stateNames[ctrl.State] {
			c.addf(path+".state", "goto target %q is not a declared state of this workflow", ctrl.State)
		}
		return engine.GotoControl{State: ctrl.State}

	case program.StayControl:
		return engine.StayControl{}

	case program.CompleteControl:
		result, resultType := c.compileExpression(ctrl.Result, scope, path+".result")
		if resultType != nil && ctx.resultType != nil && !ctx.resultType.Equal(resultType) {
			c.addf(path+".result", "workflow declares result type %s, but this complete control's result is statically %s", describeType(ctx.resultType), describeType(resultType))
		}
		return engine.CompleteControl{Result: result}

	case program.FailControl:
		errExpr, errType := c.compileExpression(ctrl.Error, scope, path+".error")
		if errType != nil {
			if _, ok := errType.(engine.StringType); !ok {
				c.addf(path+".error", "fail control error must be statically string, but it is %s", describeType(errType))
			}
		}
		return engine.FailControl{Error: errExpr}

	case program.CancelControl:
		reason, reasonType := c.compileExpression(ctrl.Reason, scope, path+".reason")
		if reasonType != nil {
			if _, ok := reasonType.(engine.StringType); !ok {
				c.addf(path+".reason", "cancel control reason must be statically string, but it is %s", describeType(reasonType))
			}
		}
		return engine.CancelControl{Reason: reason}

	case program.ConditionalControl:
		cond, condType := c.compileExpression(ctrl.Condition, scope, path+".condition")
		if condType != nil {
			if _, ok := condType.(engine.BoolType); !ok {
				c.addf(path+".condition", "conditional control condition must be statically bool, but it is %s", describeType(condType))
			}
		}
		then := c.compileWorkflowControl(ctrl.Then, scope, path+".then", ctx)
		els := c.compileWorkflowControl(ctrl.Else, scope, path+".else", ctx)
		return engine.ConditionalControl{Condition: cond, Then: then, Else: els}

	case program.MatchControl:
		return c.compileMatchControl(ctrl, scope, path, ctx)

	default:
		c.addf(path, "unsupported workflow control")
		return nil
	}
}

// compileMatchControl mirrors compileMatchExpression's case-compilation
// shape but selects a WorkflowControl per case instead of a common-typed
// Expression result — a WorkflowControl has no "result type" to unify
// across cases, so, unlike MatchExpression, no such check applies here.
func (c *compiler) compileMatchControl(m program.MatchControl, scope exprScope, path string, ctx *workflowContext) engine.WorkflowControl {
	value, valueType := c.compileExpression(m.Value, scope, path+".value")
	if len(m.Cases) == 0 {
		c.addf(path, "match control has no cases")
	}

	cases := make([]engine.MatchControlCase, 0, len(m.Cases))
	for i, cs := range m.Cases {
		casePath := fmt.Sprintf("%s.cases[%d]", path, i)
		pattern, caseScope, _ := c.compileMatchPattern(cs.Pattern, valueType, scope, casePath+".pattern")
		control := c.compileWorkflowControl(cs.Control, caseScope, casePath+".control", ctx)
		cases = append(cases, engine.MatchControlCase{Pattern: pattern, Control: control})
	}

	return engine.MatchControl{Value: value, Cases: cases}
}
