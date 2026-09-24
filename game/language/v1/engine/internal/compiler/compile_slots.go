package compiler

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
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
			pres = c.compileQuestionPresentation(*s.Presentation, s.Question, ctx.baseScope[globalScopeRootName].(engine.RecordType), nil, sPath+".presentation")
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
			pres = c.compileQuestionPresentation(*s.Presentation, s.Question, ctx.baseScope[globalScopeRootName].(engine.RecordType), nil, sPath+".presentation")
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

func (c *compiler) compileKeyedQuestionSlots(w program.WorkflowDeclaration, path string, ctx *workflowContext) []engine.KeyedQuestionSlot {
	prefix := path + ".keyed_question_slots"
	result := make([]engine.KeyedQuestionSlot, 0, len(w.KeyedQuestionSlots))
	seen := make(map[string]int, len(w.KeyedQuestionSlots))
	for i, s := range w.KeyedQuestionSlots {
		sPath := fmt.Sprintf("%s[%d]", prefix, i)
		if s.Name == "" {
			c.addf(sPath, "keyed question slot has an empty name")
			continue
		}
		if first, ok := seen[s.Name]; ok {
			c.addf(sPath, "duplicate keyed question slot name %q (first declared at %s[%d])", s.Name, prefix, first)
			continue
		}
		seen[s.Name] = i

		if !c.questionExists(s.Question) {
			c.addf(sPath+".question", "reference to undeclared question %q", s.Question)
		}
		keyType := c.compileTypeReference(s.KeyType, sPath+".key_type")
		ctx.keyedQuestionSlots[s.Name] = keyedQuestionSlotEntry{decl: s, keyType: keyType}

		var pres *engine.QuestionPresentation
		if s.Presentation != nil {
			pres = c.compileQuestionPresentation(*s.Presentation, s.Question, ctx.baseScope[globalScopeRootName].(engine.RecordType), keyType, sPath+".presentation")
		}
		result = append(result, engine.KeyedQuestionSlot{Name: s.Name, Question: s.Question, KeyType: keyType, Presentation: pres})
	}
	return result
}

func (c *compiler) compileKeyedAskGroupSlots(w program.WorkflowDeclaration, path string, ctx *workflowContext) []engine.KeyedAskGroupSlot {
	prefix := path + ".keyed_ask_group_slots"
	result := make([]engine.KeyedAskGroupSlot, 0, len(w.KeyedAskGroupSlots))
	seen := make(map[string]int, len(w.KeyedAskGroupSlots))
	for i, s := range w.KeyedAskGroupSlots {
		sPath := fmt.Sprintf("%s[%d]", prefix, i)
		if s.Name == "" {
			c.addf(sPath, "keyed ask-group slot has an empty name")
			continue
		}
		if first, ok := seen[s.Name]; ok {
			c.addf(sPath, "duplicate keyed ask-group slot name %q (first declared at %s[%d])", s.Name, prefix, first)
			continue
		}
		seen[s.Name] = i

		if !c.questionExists(s.Question) {
			c.addf(sPath+".question", "reference to undeclared question %q", s.Question)
		}
		keyType := c.compileTypeReference(s.KeyType, sPath+".key_type")
		ctx.keyedAskGroupSlots[s.Name] = keyedAskGroupSlotEntry{decl: s, keyType: keyType}

		var pres *engine.QuestionPresentation
		if s.Presentation != nil {
			pres = c.compileQuestionPresentation(*s.Presentation, s.Question, ctx.baseScope[globalScopeRootName].(engine.RecordType), keyType, sPath+".presentation")
		}
		result = append(result, engine.KeyedAskGroupSlot{Name: s.Name, Question: s.Question, KeyType: keyType, Presentation: pres})
	}
	return result
}

func (c *compiler) compileKeyedTimerSlots(w program.WorkflowDeclaration, path string, ctx *workflowContext) []engine.KeyedTimerSlot {
	prefix := path + ".keyed_timer_slots"
	result := make([]engine.KeyedTimerSlot, 0, len(w.KeyedTimerSlots))
	seen := make(map[string]int, len(w.KeyedTimerSlots))
	for i, s := range w.KeyedTimerSlots {
		sPath := fmt.Sprintf("%s[%d]", prefix, i)
		if s.Name == "" {
			c.addf(sPath, "keyed timer slot has an empty name")
			continue
		}
		if first, ok := seen[s.Name]; ok {
			c.addf(sPath, "duplicate keyed timer slot name %q (first declared at %s[%d])", s.Name, prefix, first)
			continue
		}
		seen[s.Name] = i
		keyType := c.compileTypeReference(s.KeyType, sPath+".key_type")
		ctx.keyedTimerSlots[s.Name] = keyedTimerSlotEntry{decl: s, keyType: keyType}
		result = append(result, engine.KeyedTimerSlot{Name: s.Name, KeyType: keyType})
	}
	return result
}

// compileQuestionPresentation compiles one
// program.QuestionPresentationDeclaration. Per its documented argument
// scope, ProjectionArguments may reference the referenced question's own
// captured parameters, the implicit "recipient" (User), "global", and
// "resources" — never workflow parameters, "local", or signal bindings.
// keyType is non-nil only when compiling against a keyed slot
// (KeyedQuestionSlotDeclaration/KeyedAskGroupSlotDeclaration), in which
// case ProjectionArguments' scope gains one additional implicit binding,
// "key" — see KeyedQuestionSlotDeclaration's doc comment. An ordinary
// (non-keyed) slot passes keyType nil, leaving the scope unchanged.
func (c *compiler) compileQuestionPresentation(pres program.QuestionPresentationDeclaration, questionName string, globalType engine.RecordType, keyType engine.Type, path string) *engine.QuestionPresentation {
	if !c.presentationSlotExists(pres.Slot) {
		c.addf(path+".slot", "reference to undeclared presentation slot %q", pres.Slot)
	}

	scope := exprScope{
		resourcesScopeRootName: c.resourcesType,
		globalScopeRootName:    globalType,
		"recipient":            engine.UserType{},
	}
	if keyType != nil {
		scope["key"] = keyType
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
