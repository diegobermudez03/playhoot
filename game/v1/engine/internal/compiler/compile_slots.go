package compiler

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

func (c *compiler) compileQuestionSlots(w program.WorkflowDeclaration, path string, ctx *workflowContext) []engine.QuestionSlot {
	prefix := path + ".question_slots"
	result := make([]engine.QuestionSlot, 0, len(w.QuestionSlots))
	seen := make(map[string]int, len(w.QuestionSlots))
	for i, s := range w.QuestionSlots {
		sPath := fmt.Sprintf("%s[%d]", prefix, i)
		if s.Name == "" {
			c.addf(sPath, "question slot has an empty name")
			continue
		}
		if first, ok := seen[s.Name]; ok {
			c.addf(sPath, "duplicate question slot name %q (first declared at %s[%d])", s.Name, prefix, first)
			continue
		}
		seen[s.Name] = i
		ctx.questionSlots[s.Name] = s

		if !c.questionExists(s.Question) {
			c.addf(sPath+".question", "reference to undeclared question %q", s.Question)
		}
		var pres *engine.QuestionPresentation
		if s.Presentation != nil {
			pres = c.compileQuestionPresentation(*s.Presentation, s.Question, ctx.baseScope[globalScopeRootName].(engine.RecordType), sPath+".presentation")
		}
		result = append(result, engine.QuestionSlot{Name: s.Name, Question: s.Question, Presentation: pres})
	}
	return result
}

func (c *compiler) compileAskGroupSlots(w program.WorkflowDeclaration, path string, ctx *workflowContext) []engine.AskGroupSlot {
	prefix := path + ".ask_group_slots"
	result := make([]engine.AskGroupSlot, 0, len(w.AskGroupSlots))
	seen := make(map[string]int, len(w.AskGroupSlots))
	for i, s := range w.AskGroupSlots {
		sPath := fmt.Sprintf("%s[%d]", prefix, i)
		if s.Name == "" {
			c.addf(sPath, "ask-group slot has an empty name")
			continue
		}
		if first, ok := seen[s.Name]; ok {
			c.addf(sPath, "duplicate ask-group slot name %q (first declared at %s[%d])", s.Name, prefix, first)
			continue
		}
		seen[s.Name] = i
		ctx.askGroupSlots[s.Name] = s

		if !c.questionExists(s.Question) {
			c.addf(sPath+".question", "reference to undeclared question %q", s.Question)
		}
		var pres *engine.QuestionPresentation
		if s.Presentation != nil {
			pres = c.compileQuestionPresentation(*s.Presentation, s.Question, ctx.baseScope[globalScopeRootName].(engine.RecordType), sPath+".presentation")
		}
		result = append(result, engine.AskGroupSlot{Name: s.Name, Question: s.Question, Presentation: pres})
	}
	return result
}

func (c *compiler) compileTimerSlots(w program.WorkflowDeclaration, path string, ctx *workflowContext) []string {
	prefix := path + ".timer_slots"
	result := make([]string, 0, len(w.TimerSlots))
	seen := make(map[string]int, len(w.TimerSlots))
	for i, s := range w.TimerSlots {
		sPath := fmt.Sprintf("%s[%d]", prefix, i)
		if s.Name == "" {
			c.addf(sPath, "timer slot has an empty name")
			continue
		}
		if first, ok := seen[s.Name]; ok {
			c.addf(sPath, "duplicate timer slot name %q (first declared at %s[%d])", s.Name, prefix, first)
			continue
		}
		seen[s.Name] = i
		ctx.timerSlots[s.Name] = true
		result = append(result, s.Name)
	}
	return result
}

func (c *compiler) compileChildSlots(w program.WorkflowDeclaration, path string, ctx *workflowContext) []engine.ChildWorkflowSlot {
	prefix := path + ".child_slots"
	result := make([]engine.ChildWorkflowSlot, 0, len(w.ChildSlots))
	seen := make(map[string]int, len(w.ChildSlots))
	for i, s := range w.ChildSlots {
		sPath := fmt.Sprintf("%s[%d]", prefix, i)
		if s.Name == "" {
			c.addf(sPath, "child slot has an empty name")
			continue
		}
		if first, ok := seen[s.Name]; ok {
			c.addf(sPath, "duplicate child slot name %q (first declared at %s[%d])", s.Name, prefix, first)
			continue
		}
		seen[s.Name] = i
		ctx.childSlots[s.Name] = s

		if _, ok := c.workflowDeclarations[s.Workflow]; !ok {
			c.addf(sPath+".workflow", "reference to undeclared workflow %q", s.Workflow)
		}
		result = append(result, engine.ChildWorkflowSlot{Name: s.Name, Workflow: s.Workflow})
	}
	return result
}

func (c *compiler) compileTaskGroupSlots(w program.WorkflowDeclaration, path string, ctx *workflowContext) []engine.TaskGroupSlot {
	prefix := path + ".task_group_slots"
	result := make([]engine.TaskGroupSlot, 0, len(w.TaskGroupSlots))
	seen := make(map[string]int, len(w.TaskGroupSlots))
	for i, s := range w.TaskGroupSlots {
		sPath := fmt.Sprintf("%s[%d]", prefix, i)
		if s.Name == "" {
			c.addf(sPath, "task-group slot has an empty name")
			continue
		}
		if first, ok := seen[s.Name]; ok {
			c.addf(sPath, "duplicate task-group slot name %q (first declared at %s[%d])", s.Name, prefix, first)
			continue
		}
		seen[s.Name] = i

		if _, ok := c.workflowDeclarations[s.Workflow]; !ok {
			c.addf(sPath+".workflow", "reference to undeclared workflow %q", s.Workflow)
		}
		keyType := c.compileTypeReference(s.KeyType, sPath+".key_type")
		ctx.taskGroupSlots[s.Name] = taskGroupSlotInfo{workflow: s.Workflow, keyType: keyType}
		result = append(result, engine.TaskGroupSlot{Name: s.Name, Workflow: s.Workflow, KeyType: keyType})
	}
	return result
}

// compileQuestionPresentation compiles one
// program.QuestionPresentationDeclaration. Per its documented argument
// scope, ProjectionArguments may reference the referenced question's own
// captured parameters, the implicit "recipient" (User), "global", and
// "resources" — never workflow parameters, "local", or signal bindings.
func (c *compiler) compileQuestionPresentation(pres program.QuestionPresentationDeclaration, questionName string, globalType engine.RecordType, path string) *engine.QuestionPresentation {
	if !c.presentationSlotExists(pres.Slot) {
		c.addf(path+".slot", "reference to undeclared presentation slot %q", pres.Slot)
	}

	scope := exprScope{
		resourcesScopeRootName: c.resourcesType,
		globalScopeRootName:    globalType,
		"recipient":            engine.UserType{},
	}
	if q, ok := c.questionByName(questionName); ok {
		for _, p := range q.Parameters {
			if p.Name != "" {
				scope[p.Name] = c.compileTypeReference(p.Type, path+".question_parameters")
			}
		}
	}

	args, argTypes, argsOK := c.compileCallArguments(pres.ProjectionArguments, scope, path)

	projection, projOK := c.compiledProjections[pres.Projection]
	if !projOK {
		c.addf(path+".projection", "reference to undeclared projection %q", pres.Projection)
	} else if argsOK {
		c.checkCallArguments(projection.Parameters, args, argTypes, path)
	}

	if view, viewOK := c.compiledViews[pres.View]; !viewOK {
		c.addf(path+".view", "reference to undeclared view %q", pres.View)
	} else if projOK && projection.ResultType != nil && view.ModelType != nil && !projection.ResultType.Equal(view.ModelType) {
		c.addf(path+".view", "projection %q result type %s is not assignable to view %q's model type %s",
			pres.Projection, describeType(projection.ResultType), pres.View, describeType(view.ModelType))
	}

	return &engine.QuestionPresentation{Slot: pres.Slot, Projection: pres.Projection, ProjectionArguments: args, View: pres.View}
}

func (c *compiler) questionByName(name string) (program.QuestionDeclaration, bool) {
	for _, q := range c.definition.Questions {
		if q.Name == name {
			return q, true
		}
	}
	return program.QuestionDeclaration{}, false
}

func (c *compiler) questionExists(name string) bool {
	_, ok := c.questionByName(name)
	return ok
}

func (c *compiler) questionResponseType(name, path string) engine.Type {
	q, ok := c.questionByName(name)
	if !ok {
		return nil
	}
	return c.compileTypeReference(q.ResponseType, path+".question_response_type")
}

func (c *compiler) userIntentByName(name string) (program.UserIntentDeclaration, bool) {
	for _, u := range c.definition.UserIntents {
		if u.Name == name {
			return u, true
		}
	}
	return program.UserIntentDeclaration{}, false
}

func (c *compiler) presentationSlotExists(name string) bool {
	for _, s := range c.definition.PresentationSlots {
		if s.Name == name {
			return true
		}
	}
	return false
}

func (c *compiler) projectionExists(name string) bool {
	for _, p := range c.definition.Projections {
		if p.Name == name {
			return true
		}
	}
	return false
}

func (c *compiler) viewExists(name string) bool {
	for _, v := range c.definition.Views {
		if v.Name == name {
			return true
		}
	}
	return false
}
