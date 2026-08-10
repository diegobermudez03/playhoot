package engineservice

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// compilePresentations compiles decls (diagnosing an empty or duplicate
// name within decls itself), shared by a Workflow's workflow-level
// Presentations and a WorkflowState's state-level ones — each call gets
// its own fresh dedup state, matching program's documentation that a
// presentation's Name is unique only within its own owning scope.
func (c *compiler) compilePresentations(decls []program.PresentationDeclaration, pathPrefix string, scope exprScope) []engine.Presentation {
	result := make([]engine.Presentation, 0, len(decls))
	seen := make(map[string]int, len(decls))
	for i, p := range decls {
		pPath := fmt.Sprintf("%s[%d]", pathPrefix, i)
		if p.Name == "" {
			c.addf(pPath, "presentation has an empty name")
			continue
		}
		if first, ok := seen[p.Name]; ok {
			c.addf(pPath, "duplicate presentation name %q (first declared at %s[%d])", p.Name, pathPrefix, first)
			continue
		}
		seen[p.Name] = i
		result = append(result, c.compilePresentation(p, pPath, scope))
	}
	return result
}

// compilePresentation compiles one program.PresentationDeclaration.
// Targets, and ProjectionArguments, are compiled in scope — the same
// base scope (parameters, "local", "global", "resources") the enclosing
// Workflow or WorkflowState uses for its transitions, since a
// presentation is not tied to any one signal. Unlike a QuestionPresentation,
// the referenced View must not contain an AnswerQuestionAction anywhere
// in its element tree — per program.AnswerQuestionAction's documented
// restriction, that action is only valid in a view mounted through a
// pending question.
func (c *compiler) compilePresentation(p program.PresentationDeclaration, path string, scope exprScope) engine.Presentation {
	if !c.presentationSlotExists(p.Slot) {
		c.addf(path+".slot", "reference to undeclared presentation slot %q", p.Slot)
	}

	targets, targetsType := c.compileExpression(p.Targets, scope, path+".targets")
	if targetsType != nil {
		lt, ok := targetsType.(engine.ListType)
		if !ok || !isUser(lt.Element) {
			c.addf(path+".targets", "presentation targets must be statically list<user>, but it is %s", describeType(targetsType))
		}
	}

	args, argTypes, argsOK := c.compileCallArguments(p.ProjectionArguments, scope, path)

	projection, projOK := c.compiledProjections[p.Projection]
	if !projOK {
		c.addf(path+".projection", "reference to undeclared projection %q", p.Projection)
	} else if argsOK {
		c.checkCallArguments(projection.Parameters, args, argTypes, path)
	}

	view, viewOK := c.compiledViews[p.View]
	if !viewOK {
		c.addf(path+".view", "reference to undeclared view %q", p.View)
	}
	if projOK && viewOK {
		if projection.ResultType != nil && view.ModelType != nil && !projection.ResultType.Equal(view.ModelType) {
			c.addf(path+".view", "projection %q result type %s is not assignable to view %q's model type %s",
				p.Projection, describeType(projection.ResultType), p.View, describeType(view.ModelType))
		}
		if viewContainsAnswerQuestionAction(view.Root) {
			c.addf(path+".view", "view %q uses AnswerQuestionAction, which is only valid in a view mounted through a pending question", p.View)
		}
	}

	return engine.Presentation{
		Name:                p.Name,
		Slot:                p.Slot,
		Targets:             targets,
		Projection:          p.Projection,
		ProjectionArguments: args,
		View:                p.View,
	}
}

func isUser(t engine.Type) bool {
	_, ok := t.(engine.UserType)
	return ok
}
