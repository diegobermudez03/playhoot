package compiler

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// compileUIElement compiles one program.UIElement variant, threading
// scope so a RepeatElement's item/index bindings are visible only to
// its own Body.
func (c *compiler) compileUIElement(el program.UIElement, scope exprScope, path string) engine.UIElement {
	switch e := el.(type) {
	case nil:
		c.addf(path, "missing UI element")
		return nil

	case program.EmptyElement:
		return engine.EmptyElement{}

	case program.ContainerElement:
		children := make([]engine.UIElement, len(e.Children))
		for i, child := range e.Children {
			children[i] = c.compileUIElement(child, scope, fmt.Sprintf("%s.children[%d]", path, i))
		}
		return engine.ContainerElement{
			Configuration: c.compileUIElementConfiguration(e.Configuration, scope, path+".configuration"),
			Layout:        c.compileUILayout(e.Layout, scope, path+".layout"),
			Children:      children,
		}

	case program.TextElement:
		value, valueType := c.compileExpression(e.Value, scope, path+".value")
		if valueType != nil {
			if _, ok := valueType.(engine.StringType); !ok {
				c.addf(path+".value", "text element value must be statically string, but it is %s", describeType(valueType))
			}
		}
		return engine.TextElement{Configuration: c.compileUIElementConfiguration(e.Configuration, scope, path+".configuration"), Value: value}

	case program.ImageElement:
		source, _ := c.compileExpression(e.Source, scope, path+".source")
		var altText engine.Expression
		if e.AlternativeText != nil {
			altText, _ = c.compileExpression(e.AlternativeText, scope, path+".alternative_text")
		}
		return engine.ImageElement{
			Configuration:   c.compileUIElementConfiguration(e.Configuration, scope, path+".configuration"),
			Source:          source,
			AlternativeText: altText,
		}

	case program.ButtonElement:
		children := make([]engine.UIElement, len(e.Children))
		for i, child := range e.Children {
			children[i] = c.compileUIElement(child, scope, fmt.Sprintf("%s.children[%d]", path, i))
		}
		return engine.ButtonElement{Configuration: c.compileUIElementConfiguration(e.Configuration, scope, path+".configuration"), Children: children}

	case program.RepeatElement:
		coll, collType, ok := c.compileListCollection(e.Collection, scope, path)
		if !ok {
			return engine.RepeatElement{}
		}
		bodyScope, sOk := c.bindListItem(scope, e.ItemName, e.IndexName, collType.Element, path)
		if !sOk {
			return engine.RepeatElement{}
		}
		var key engine.Expression
		if e.Key != nil {
			key, _ = c.compileExpression(e.Key, bodyScope, path+".key")
		}
		return engine.RepeatElement{
			Collection: coll,
			ItemName:   e.ItemName,
			IndexName:  e.IndexName,
			Key:        key,
			Body:       c.compileUIElement(e.Body, bodyScope, path+".body"),
		}

	case program.ConditionalElement:
		cond, condType := c.compileExpression(e.Condition, scope, path+".condition")
		if condType != nil {
			if _, ok := condType.(engine.BoolType); !ok {
				c.addf(path+".condition", "condition must be statically bool, but it is %s", describeType(condType))
			}
		}
		return engine.ConditionalElement{
			Condition: cond,
			Then:      c.compileUIElement(e.Then, scope, path+".then"),
			Else:      c.compileUIElement(e.Else, scope, path+".else"),
		}

	default:
		c.addf(path, "unsupported UI element")
		return nil
	}
}

func (c *compiler) compileUILayout(l program.UILayout, scope exprScope, path string) engine.UILayout {
	switch layout := l.(type) {
	case nil:
		c.addf(path, "missing UI layout")
		return nil

	case program.StackLayout:
		return engine.StackLayout{}

	case program.AbsoluteLayout:
		return engine.AbsoluteLayout{}

	case program.LinearLayout:
		if !layout.Direction.IsValid() {
			c.addf(path+".direction", "invalid linear layout direction %q", layout.Direction)
		}
		var gap engine.Expression
		if layout.Gap != nil {
			gap, _ = c.compileExpression(layout.Gap, scope, path+".gap")
		}
		return engine.LinearLayout{Direction: engine.LinearLayoutDirection(layout.Direction), Gap: gap}

	case program.GridLayout:
		columns, columnsType := c.compileExpression(layout.Columns, scope, path+".columns")
		if columnsType != nil && !isNumber(columnsType) {
			c.addf(path+".columns", "grid columns must be statically number, but it is %s", describeType(columnsType))
		}
		var rowGap, columnGap engine.Expression
		if layout.RowGap != nil {
			rowGap, _ = c.compileExpression(layout.RowGap, scope, path+".row_gap")
		}
		if layout.ColumnGap != nil {
			columnGap, _ = c.compileExpression(layout.ColumnGap, scope, path+".column_gap")
		}
		return engine.GridLayout{Columns: columns, RowGap: rowGap, ColumnGap: columnGap}

	default:
		c.addf(path, "unsupported UI layout")
		return nil
	}
}

func (c *compiler) compileUIElementConfiguration(cfg program.UIElementConfiguration, scope exprScope, path string) engine.UIElementConfiguration {
	props := make([]engine.UIProperty, 0, len(cfg.Properties))
	for i, p := range cfg.Properties {
		pPath := fmt.Sprintf("%s.properties[%d]", path, i)
		if p.Name == "" {
			c.addf(pPath+".name", "UI property has an empty name")
			continue
		}
		value, _ := c.compileExpression(p.Value, scope, pPath+".value")
		props = append(props, engine.UIProperty{Name: p.Name, Value: value})
	}

	events := make([]engine.UIEventHandler, 0, len(cfg.Events))
	for i, ev := range cfg.Events {
		evPath := fmt.Sprintf("%s.events[%d]", path, i)
		if !ev.Event.IsValid() {
			c.addf(evPath+".event", "invalid UI event type %q", ev.Event)
		}
		actions := make([]engine.UIAction, 0, len(ev.Actions))
		for j, a := range ev.Actions {
			actions = append(actions, c.compileUIAction(a, scope, fmt.Sprintf("%s.actions[%d]", evPath, j)))
		}
		events = append(events, engine.UIEventHandler{Event: engine.UIEventType(ev.Event), Actions: actions})
	}

	return engine.UIElementConfiguration{Properties: props, Events: events}
}

// compileUIAction compiles one program.UIAction: SetLocalStateAction's
// Target must root at "local", and EmitUserIntentAction's Intent must
// name a declared program.UserIntentDeclaration with Arguments
// matching its declared parameters exactly (see checkCallArguments).
// AnswerQuestionAction always compiles here — whether its containing
// view may be used from a non-question presentation is validated where
// a Presentation or QuestionPresentation references the view, not here
// (a View's own compilation cannot yet know how it will be used).
func (c *compiler) compileUIAction(a program.UIAction, scope exprScope, path string) engine.UIAction {
	switch action := a.(type) {
	case nil:
		c.addf(path, "missing UI action")
		return nil

	case program.SetLocalStateAction:
		target, targetType := c.compileAssignmentTarget(action.Target, scope, path+".target")
		value, valueType := c.compileExpression(action.Value, scope, path+".value")
		if target == nil || valueType == nil {
			return nil
		}
		if targetType != nil && !targetType.Equal(valueType) {
			c.addf(path+".value", "assignment target is statically %s, but the value is statically %s", describeType(targetType), describeType(valueType))
		}
		return engine.SetLocalStateAction{Target: target, Value: value}

	case program.AnswerQuestionAction:
		value, _ := c.compileExpression(action.Value, scope, path+".value")
		return engine.AnswerQuestionAction{Value: value}

	case program.EmitUserIntentAction:
		intent, ok := c.userIntentByName(action.Intent)
		if !ok {
			c.addf(path+".intent", "reference to undeclared user intent %q", action.Intent)
			return nil
		}
		args, argTypes, argsOK := c.compileCallArguments(action.Arguments, scope, path)
		schema := make([]engine.FieldType, 0, len(intent.Parameters))
		for _, p := range intent.Parameters {
			if p.Name != "" {
				schema = append(schema, engine.FieldType{Name: p.Name, Type: c.compileTypeReference(p.Type, path+".user_intent_parameters")})
			}
		}
		if !argsOK || !c.checkCallArguments(schema, args, argTypes, path) {
			return engine.EmitUserIntentAction{Intent: action.Intent, Arguments: args}
		}
		return engine.EmitUserIntentAction{Intent: action.Intent, Arguments: args}

	default:
		c.addf(path, "unsupported UI action")
		return nil
	}
}

// viewContainsAnswerQuestionAction reports whether el, or any of its
// descendants, contains an AnswerQuestionAction in any UIEventHandler —
// used to reject a View used from a non-question Presentation, per
// program.AnswerQuestionAction's documented restriction.
func viewContainsAnswerQuestionAction(el engine.UIElement) bool {
	switch e := el.(type) {
	case engine.ContainerElement:
		if configurationContainsAnswerQuestionAction(e.Configuration) {
			return true
		}
		for _, child := range e.Children {
			if viewContainsAnswerQuestionAction(child) {
				return true
			}
		}
	case engine.TextElement:
		return configurationContainsAnswerQuestionAction(e.Configuration)
	case engine.ImageElement:
		return configurationContainsAnswerQuestionAction(e.Configuration)
	case engine.ButtonElement:
		if configurationContainsAnswerQuestionAction(e.Configuration) {
			return true
		}
		for _, child := range e.Children {
			if viewContainsAnswerQuestionAction(child) {
				return true
			}
		}
	case engine.RepeatElement:
		return viewContainsAnswerQuestionAction(e.Body)
	case engine.ConditionalElement:
		return viewContainsAnswerQuestionAction(e.Then) || viewContainsAnswerQuestionAction(e.Else)
	}
	return false
}

func configurationContainsAnswerQuestionAction(cfg engine.UIElementConfiguration) bool {
	for _, ev := range cfg.Events {
		for _, a := range ev.Actions {
			if _, ok := a.(engine.AnswerQuestionAction); ok {
				return true
			}
		}
	}
	return false
}
