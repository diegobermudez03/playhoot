package compiler

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

// compileMatchExpression compiles e.Value and every case's pattern and
// result. Every case's Result must be statically the same type; that
// common type becomes the MatchExpression's own static type.
//
// This does not check that Cases is exhaustive (every enum value or
// union variant covered, or a reachable wildcard present) — see
// engine/match.go's doc comment. Evaluate reports an error, rather than
// panicking, if no case matches at runtime.
func (c *compiler) compileMatchExpression(e program.MatchExpression, scope exprScope, path string) (engine.Expression, engine.Type) {
	value, valueType := c.compileExpression(e.Value, scope, path+".value")
	if len(e.Cases) == 0 {
		c.addf(path, "match expression has no cases")
		return nil, nil
	}

	cases := make([]engine.MatchExpressionCase, 0, len(e.Cases))
	var resultType engine.Type
	ok := valueType != nil
	for i, cs := range e.Cases {
		casePath := fmt.Sprintf("%s.cases[%d]", path, i)

		pattern, caseScope, patOk := c.compileMatchPattern(cs.Pattern, valueType, scope, casePath+".pattern")
		if !patOk {
			ok = false
		}

		result, rType := c.compileExpression(cs.Result, caseScope, casePath+".result")
		if rType == nil {
			ok = false
		} else if resultType == nil {
			resultType = rType
		} else if !resultType.Equal(rType) {
			c.addf(casePath+".result", "match case result is statically %s, but an earlier case's result is %s", describeType(rType), describeType(resultType))
			ok = false
		}

		cases = append(cases, engine.MatchExpressionCase{Pattern: pattern, Result: result})
	}

	if !ok || resultType == nil {
		return nil, nil
	}
	return engine.MatchExpression{Value: value, Cases: cases}, resultType
}

// compileMatchPattern compiles pattern against matchedType, returning the
// scope in effect for the case's Result — outerScope itself, unless the
// pattern introduces lexical bindings (a UnionVariantMatchPattern's
// Bindings, or a named OptionalSomeMatchPattern.Binding), in which case
// a clone of outerScope extended with them.
func (c *compiler) compileMatchPattern(pattern program.MatchPattern, matchedType engine.Type, outerScope exprScope, path string) (engine.MatchPattern, exprScope, bool) {
	switch p := pattern.(type) {
	case nil:
		c.addf(path, "missing match pattern")
		return nil, outerScope, false

	case program.WildcardMatchPattern:
		return engine.WildcardMatchPattern{}, outerScope, true

	case program.EnumValueMatchPattern:
		et, ok := matchedType.(engine.EnumType)
		if !ok || et.Name != p.TypeName {
			c.addf(path, "enum pattern %q does not match the matched value's type %s", p.TypeName, describeType(matchedType))
			return nil, outerScope, false
		}
		if !et.HasValue(p.ValueName) {
			c.addf(path, "enum %q has no value named %q", p.TypeName, p.ValueName)
			return nil, outerScope, false
		}
		return engine.EnumValueMatchPattern{TypeName: p.TypeName, ValueName: p.ValueName}, outerScope, true

	case program.UnionVariantMatchPattern:
		return c.compileUnionVariantMatchPattern(p, matchedType, outerScope, path)

	case program.OptionalNoneMatchPattern:
		if _, ok := matchedType.(engine.OptionalType); !ok {
			c.addf(path, "optional-none pattern does not match the matched value's type %s", describeType(matchedType))
			return nil, outerScope, false
		}
		return engine.OptionalNoneMatchPattern{}, outerScope, true

	case program.OptionalSomeMatchPattern:
		ot, ok := matchedType.(engine.OptionalType)
		if !ok {
			c.addf(path, "optional-some pattern does not match the matched value's type %s", describeType(matchedType))
			return nil, outerScope, false
		}
		newScope := outerScope
		if p.Binding != "" {
			newScope = outerScope.clone()
			newScope[p.Binding] = ot.Element
		}
		return engine.OptionalSomeMatchPattern{Binding: p.Binding}, newScope, true

	default:
		c.addf(path, "unsupported match pattern")
		return nil, outerScope, false
	}
}

func (c *compiler) compileUnionVariantMatchPattern(p program.UnionVariantMatchPattern, matchedType engine.Type, outerScope exprScope, path string) (engine.MatchPattern, exprScope, bool) {
	ut, ok := matchedType.(engine.UnionType)
	if !ok || ut.Name != p.TypeName {
		c.addf(path, "union pattern %q does not match the matched value's type %s", p.TypeName, describeType(matchedType))
		return nil, outerScope, false
	}
	variant, ok := ut.VariantByName(p.VariantName)
	if !ok {
		c.addf(path, "union %q has no variant named %q", p.TypeName, p.VariantName)
		return nil, outerScope, false
	}

	newScope := outerScope.clone()
	bindings := make([]engine.MatchFieldBinding, 0, len(p.Bindings))
	seenFields := make(map[string]bool, len(p.Bindings))
	seenNames := make(map[string]bool, len(p.Bindings))
	ok = true
	for i, b := range p.Bindings {
		bPath := fmt.Sprintf("%s.bindings[%d]", path, i)
		ft, exists := variant.FieldByName(b.Field)
		if !exists {
			c.addf(bPath+".field", "variant %q has no field named %q", p.VariantName, b.Field)
			ok = false
			continue
		}
		if seenFields[b.Field] {
			c.addf(bPath+".field", "duplicate binding for field %q", b.Field)
			ok = false
			continue
		}
		if b.Name == "" {
			c.addf(bPath+".name", "binding has an empty lexical name")
			ok = false
			continue
		}
		if seenNames[b.Name] {
			c.addf(bPath+".name", "duplicate lexical binding name %q", b.Name)
			ok = false
			continue
		}
		seenFields[b.Field] = true
		seenNames[b.Name] = true
		newScope[b.Name] = ft.Type
		bindings = append(bindings, engine.MatchFieldBinding{Field: b.Field, Name: b.Name})
	}
	if !ok {
		return nil, outerScope, false
	}
	return engine.UnionVariantMatchPattern{TypeName: p.TypeName, VariantName: p.VariantName, Bindings: bindings}, newScope, true
}
