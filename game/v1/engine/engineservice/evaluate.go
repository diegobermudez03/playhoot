package engineservice

import (
	"fmt"
	"maps"
	"math"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
)

// Evaluate deterministically evaluates expr — a compiled Expression
// produced by Compile, directly or as part of a compiled
// engine.Function's Body — in scope.
//
// Evaluate always makes p.Resources available through the reserved
// lexical name "resources" (see withResources), regardless of what scope
// itself contains; a binding named "resources" in scope is shadowed, not
// merged with p.Resources. Every nested evaluation this package performs
// — a called engine.Function's body (evalCall), or, once
// engineservice.NewSnapshot's initial-state construction reaches
// invariants, "global" together with "resources" — carries this same
// guarantee forward.
//
// Evaluate performs no I/O and reads no clock, network, environment
// variable, or operating-system randomness; given the same expr, scope,
// and p, it always returns the same result. Evaluate itself never
// panics on a well-compiled expr: a Go type assertion the compiler
// already guaranteed to hold (for example, that a BinaryOperatorAdd's
// operands are both engine.NumberValue) is written as a direct
// assertion and is allowed to panic if it is ever violated, since that
// would indicate a bug in this package rather than a legitimate runtime
// condition. A condition Evaluate cannot rule out at compile time —
// scope missing a name the compiler expected the caller to provide,
// division or modulo by zero, a list index out of range, a map key with
// no entry, or a MatchExpression whose value matches none of its cases
// — is reported as an *ExecutionError instead.
func Evaluate(p engine.Program, expr engine.Expression, scope engine.Scope) (engine.Value, error) {
	e := &evaluator{program: p}
	return e.eval(expr, e.withResources(scope))
}

type evaluator struct {
	program engine.Program
}

// withResources returns scope extended with the reserved "resources"
// binding, built fresh from e.program.Resources every call so that
// evaluations running while engineservice.Compile is still resolving
// resources (see resolveResourceValue) see each dependency's value as
// soon as it is resolved, since e.program.Resources and the map
// resolveResourceValue writes into are the same map.
func (e *evaluator) withResources(scope engine.Scope) engine.Scope {
	fields := make([]engine.FieldValue, 0, len(e.program.Resources))
	for name, v := range e.program.Resources {
		fields = append(fields, engine.FieldValue{Name: name, Value: v})
	}
	resources := engine.RecordValue{TypeName: resourcesScopeRootName, Fields: fields}
	return extendScope(scope, resourcesScopeRootName, resources)
}

func (e *evaluator) eval(expr engine.Expression, scope engine.Scope) (engine.Value, error) {
	switch x := expr.(type) {
	case engine.UnitLiteralExpression:
		return engine.UnitValue{}, nil
	case engine.BoolLiteralExpression:
		return engine.BoolValue{Value: x.Value}, nil
	case engine.NumberLiteralExpression:
		return engine.NumberValue{Value: x.Value}, nil
	case engine.StringLiteralExpression:
		return engine.StringValue{Value: x.Value}, nil

	case engine.OptionalNoneExpression:
		return engine.OptionalValue{ElementType: x.ElementType}, nil
	case engine.OptionalSomeExpression:
		v, err := e.eval(x.Value, scope)
		if err != nil {
			return nil, err
		}
		return engine.OptionalValue{ElementType: x.ElementType, Value: v}, nil

	case engine.ListExpression:
		elements := make([]engine.Value, len(x.Elements))
		for i, el := range x.Elements {
			v, err := e.eval(el, scope)
			if err != nil {
				return nil, err
			}
			elements[i] = v
		}
		return engine.ListValue{ElementType: x.ElementType, Elements: elements}, nil

	case engine.MapExpression:
		entries := make([]engine.MapEntry, len(x.Entries))
		for i, entry := range x.Entries {
			k, err := e.eval(entry.Key, scope)
			if err != nil {
				return nil, err
			}
			v, err := e.eval(entry.Value, scope)
			if err != nil {
				return nil, err
			}
			entries[i] = engine.MapEntry{Key: k, Value: v}
		}
		return engine.MapValue{KeyType: x.KeyType, ValueType: x.ValueType, Entries: entries}, nil

	case engine.EnumValueExpression:
		return engine.EnumValue{TypeName: x.TypeName, ValueName: x.ValueName}, nil

	case engine.RecordExpression:
		fields, err := e.evalFieldInitializers(x.Fields, scope)
		if err != nil {
			return nil, err
		}
		return engine.RecordValue{TypeName: x.TypeName, Fields: fields}, nil

	case engine.UnionExpression:
		fields, err := e.evalFieldInitializers(x.Fields, scope)
		if err != nil {
			return nil, err
		}
		return engine.UnionValue{TypeName: x.TypeName, VariantName: x.VariantName, Fields: fields}, nil

	case engine.NewTypeExpression:
		v, err := e.eval(x.Value, scope)
		if err != nil {
			return nil, err
		}
		return engine.NewTypeValue{TypeName: x.TypeName, Underlying: v}, nil

	case engine.ReferenceExpression:
		v, ok := scope.Lookup(x.Name)
		if !ok {
			return nil, newExecutionError(ExecutionErrorUndefinedReference, "engineservice: %q is not bound in this scope", x.Name)
		}
		return v, nil

	case engine.FieldExpression:
		target, err := e.eval(x.Target, scope)
		if err != nil {
			return nil, err
		}
		rv := target.(engine.RecordValue)
		fv, ok := rv.FieldByName(x.Field)
		if !ok {
			panic(fmt.Sprintf("engineservice: compiled FieldExpression references unknown field %q on record %q — this is a compiler bug, not a runtime condition", x.Field, rv.TypeName))
		}
		return fv.Value, nil

	case engine.IndexExpression:
		return e.evalIndex(x, scope)

	case engine.UnaryExpression:
		return e.evalUnary(x, scope)

	case engine.BinaryExpression:
		return e.evalBinary(x, scope)

	case engine.ConditionalExpression:
		cond, err := e.eval(x.Condition, scope)
		if err != nil {
			return nil, err
		}
		if cond.(engine.BoolValue).Value {
			return e.eval(x.Then, scope)
		}
		return e.eval(x.Else, scope)

	case engine.CallExpression:
		return e.evalCall(x, scope)

	case engine.MatchExpression:
		return e.evalMatch(x, scope)

	case engine.ListMapExpression:
		list, err := e.evalListValue(x.Collection, scope)
		if err != nil {
			return nil, err
		}
		result := make([]engine.Value, len(list.Elements))
		for i, item := range list.Elements {
			v, err := e.eval(x.Result, bindItem(scope, x.ItemName, x.IndexName, item, i))
			if err != nil {
				return nil, err
			}
			result[i] = v
		}
		return engine.ListValue{ElementType: x.ResultElementType, Elements: result}, nil

	case engine.ListFilterExpression:
		list, err := e.evalListValue(x.Collection, scope)
		if err != nil {
			return nil, err
		}
		result := make([]engine.Value, 0, len(list.Elements))
		for i, item := range list.Elements {
			v, err := e.eval(x.Predicate, bindItem(scope, x.ItemName, x.IndexName, item, i))
			if err != nil {
				return nil, err
			}
			if v.(engine.BoolValue).Value {
				result = append(result, item)
			}
		}
		return engine.ListValue{ElementType: list.ElementType, Elements: result}, nil

	case engine.ListFlatMapExpression:
		list, err := e.evalListValue(x.Collection, scope)
		if err != nil {
			return nil, err
		}
		var result []engine.Value
		for i, item := range list.Elements {
			v, err := e.eval(x.Result, bindItem(scope, x.ItemName, x.IndexName, item, i))
			if err != nil {
				return nil, err
			}
			result = append(result, v.(engine.ListValue).Elements...)
		}
		return engine.ListValue{ElementType: x.ResultElementType, Elements: result}, nil

	case engine.ListAnyExpression:
		list, err := e.evalListValue(x.Collection, scope)
		if err != nil {
			return nil, err
		}
		for i, item := range list.Elements {
			v, err := e.eval(x.Predicate, bindItem(scope, x.ItemName, x.IndexName, item, i))
			if err != nil {
				return nil, err
			}
			if v.(engine.BoolValue).Value {
				return engine.BoolValue{Value: true}, nil
			}
		}
		return engine.BoolValue{Value: false}, nil

	case engine.ListAllExpression:
		list, err := e.evalListValue(x.Collection, scope)
		if err != nil {
			return nil, err
		}
		for i, item := range list.Elements {
			v, err := e.eval(x.Predicate, bindItem(scope, x.ItemName, x.IndexName, item, i))
			if err != nil {
				return nil, err
			}
			if !v.(engine.BoolValue).Value {
				return engine.BoolValue{Value: false}, nil
			}
		}
		return engine.BoolValue{Value: true}, nil

	case engine.ListCountExpression:
		list, err := e.evalListValue(x.Collection, scope)
		if err != nil {
			return nil, err
		}
		count := 0
		for i, item := range list.Elements {
			v, err := e.eval(x.Predicate, bindItem(scope, x.ItemName, x.IndexName, item, i))
			if err != nil {
				return nil, err
			}
			if v.(engine.BoolValue).Value {
				count++
			}
		}
		return engine.NumberValue{Value: float64(count)}, nil

	case engine.ListFirstExpression:
		list, err := e.evalListValue(x.Collection, scope)
		if err != nil {
			return nil, err
		}
		for i, item := range list.Elements {
			v, err := e.eval(x.Predicate, bindItem(scope, x.ItemName, x.IndexName, item, i))
			if err != nil {
				return nil, err
			}
			if v.(engine.BoolValue).Value {
				return engine.OptionalValue{ElementType: list.ElementType, Value: item}, nil
			}
		}
		return engine.OptionalValue{ElementType: list.ElementType}, nil

	default:
		return nil, newExecutionError(ExecutionErrorUnknown, "engineservice: cannot evaluate expression of type %T", expr)
	}
}

func (e *evaluator) evalFieldInitializers(inits []engine.FieldInitializer, scope engine.Scope) ([]engine.FieldValue, error) {
	fields := make([]engine.FieldValue, len(inits))
	for i, f := range inits {
		v, err := e.eval(f.Value, scope)
		if err != nil {
			return nil, err
		}
		fields[i] = engine.FieldValue{Name: f.Name, Value: v}
	}
	return fields, nil
}

func (e *evaluator) evalListValue(expr engine.Expression, scope engine.Scope) (engine.ListValue, error) {
	v, err := e.eval(expr, scope)
	if err != nil {
		return engine.ListValue{}, err
	}
	return v.(engine.ListValue), nil
}

// bindItem extends scope with a list query's per-element item/index
// bindings, producing a new Scope rather than mutating scope.
func bindItem(scope engine.Scope, itemName, indexName string, item engine.Value, index int) engine.Scope {
	s := extendScope(scope, itemName, item)
	if indexName != "" {
		s = extendScope(s, indexName, engine.NumberValue{Value: float64(index)})
	}
	return s
}

// extendScope returns a new Scope equal to scope plus one additional
// binding, leaving scope itself unchanged.
func extendScope(scope engine.Scope, name string, value engine.Value) engine.Scope {
	bindings := make(map[string]engine.Value, len(scope.Bindings)+1)
	maps.Copy(bindings, scope.Bindings)
	bindings[name] = value
	return engine.Scope{Bindings: bindings}
}

func (e *evaluator) evalIndex(x engine.IndexExpression, scope engine.Scope) (engine.Value, error) {
	target, err := e.eval(x.Target, scope)
	if err != nil {
		return nil, err
	}
	index, err := e.eval(x.Index, scope)
	if err != nil {
		return nil, err
	}
	switch t := target.(type) {
	case engine.ListValue:
		n := index.(engine.NumberValue).Value
		i := int(n)
		if float64(i) != n || i < 0 || i >= len(t.Elements) {
			return nil, newExecutionError(ExecutionErrorIndexOutOfRange, "engineservice: list index %v out of range for a list of length %d", n, len(t.Elements))
		}
		return t.Elements[i], nil
	case engine.MapValue:
		for _, entry := range t.Entries {
			if entry.Key.Equal(index) {
				return entry.Value, nil
			}
		}
		return nil, newExecutionError(ExecutionErrorKeyNotFound, "engineservice: map has no entry for the given key")
	default:
		return nil, newExecutionError(ExecutionErrorUnknown, "engineservice: cannot index a value of type %T", target)
	}
}

func (e *evaluator) evalUnary(x engine.UnaryExpression, scope engine.Scope) (engine.Value, error) {
	v, err := e.eval(x.Operand, scope)
	if err != nil {
		return nil, err
	}
	switch x.Operator {
	case engine.UnaryOperatorNot:
		return engine.BoolValue{Value: !v.(engine.BoolValue).Value}, nil
	case engine.UnaryOperatorNegate:
		return engine.NumberValue{Value: -v.(engine.NumberValue).Value}, nil
	default:
		return nil, newExecutionError(ExecutionErrorUnknown, "engineservice: unknown unary operator %q", x.Operator)
	}
}

// evalBinary implements short-circuit semantics for and/or: Right is
// only evaluated when Left does not already determine the result.
// Every other operator always evaluates both operands.
func (e *evaluator) evalBinary(x engine.BinaryExpression, scope engine.Scope) (engine.Value, error) {
	switch x.Operator {
	case engine.BinaryOperatorAnd:
		l, err := e.eval(x.Left, scope)
		if err != nil {
			return nil, err
		}
		if !l.(engine.BoolValue).Value {
			return engine.BoolValue{Value: false}, nil
		}
		r, err := e.eval(x.Right, scope)
		if err != nil {
			return nil, err
		}
		return engine.BoolValue{Value: r.(engine.BoolValue).Value}, nil

	case engine.BinaryOperatorOr:
		l, err := e.eval(x.Left, scope)
		if err != nil {
			return nil, err
		}
		if l.(engine.BoolValue).Value {
			return engine.BoolValue{Value: true}, nil
		}
		r, err := e.eval(x.Right, scope)
		if err != nil {
			return nil, err
		}
		return engine.BoolValue{Value: r.(engine.BoolValue).Value}, nil
	}

	l, err := e.eval(x.Left, scope)
	if err != nil {
		return nil, err
	}
	r, err := e.eval(x.Right, scope)
	if err != nil {
		return nil, err
	}

	switch x.Operator {
	case engine.BinaryOperatorAdd:
		return engine.NumberValue{Value: l.(engine.NumberValue).Value + r.(engine.NumberValue).Value}, nil
	case engine.BinaryOperatorSubtract:
		return engine.NumberValue{Value: l.(engine.NumberValue).Value - r.(engine.NumberValue).Value}, nil
	case engine.BinaryOperatorMultiply:
		return engine.NumberValue{Value: l.(engine.NumberValue).Value * r.(engine.NumberValue).Value}, nil
	case engine.BinaryOperatorDivide:
		rv := r.(engine.NumberValue).Value
		if rv == 0 {
			return nil, newExecutionError(ExecutionErrorDivisionByZero, "engineservice: division by zero")
		}
		return engine.NumberValue{Value: l.(engine.NumberValue).Value / rv}, nil
	case engine.BinaryOperatorModulo:
		rv := r.(engine.NumberValue).Value
		if rv == 0 {
			return nil, newExecutionError(ExecutionErrorDivisionByZero, "engineservice: modulo by zero")
		}
		return engine.NumberValue{Value: math.Mod(l.(engine.NumberValue).Value, rv)}, nil
	case engine.BinaryOperatorEqual:
		return engine.BoolValue{Value: l.Equal(r)}, nil
	case engine.BinaryOperatorNotEqual:
		return engine.BoolValue{Value: !l.Equal(r)}, nil
	case engine.BinaryOperatorLess:
		return engine.BoolValue{Value: l.(engine.NumberValue).Value < r.(engine.NumberValue).Value}, nil
	case engine.BinaryOperatorLessOrEqual:
		return engine.BoolValue{Value: l.(engine.NumberValue).Value <= r.(engine.NumberValue).Value}, nil
	case engine.BinaryOperatorGreater:
		return engine.BoolValue{Value: l.(engine.NumberValue).Value > r.(engine.NumberValue).Value}, nil
	case engine.BinaryOperatorGreaterOrEqual:
		return engine.BoolValue{Value: l.(engine.NumberValue).Value >= r.(engine.NumberValue).Value}, nil
	case engine.BinaryOperatorIn, engine.BinaryOperatorNotIn:
		found := false
		switch coll := r.(type) {
		case engine.ListValue:
			for _, el := range coll.Elements {
				if el.Equal(l) {
					found = true
					break
				}
			}
		case engine.MapValue:
			for _, entry := range coll.Entries {
				if entry.Key.Equal(l) {
					found = true
					break
				}
			}
		}
		if x.Operator == engine.BinaryOperatorNotIn {
			found = !found
		}
		return engine.BoolValue{Value: found}, nil
	default:
		return nil, newExecutionError(ExecutionErrorUnknown, "engineservice: unknown binary operator %q", x.Operator)
	}
}

func (e *evaluator) evalCall(x engine.CallExpression, scope engine.Scope) (engine.Value, error) {
	args := make(map[string]engine.Value, len(x.Arguments))
	for _, a := range x.Arguments {
		v, err := e.eval(a.Value, scope)
		if err != nil {
			return nil, err
		}
		args[a.Name] = v
	}

	if fn, ok := e.program.Functions[x.Function]; ok {
		return e.eval(fn.Body, e.withResources(engine.Scope{Bindings: args}))
	}
	return evalBuiltinCall(x.Function, args)
}

func evalBuiltinCall(name string, args map[string]engine.Value) (engine.Value, error) {
	switch name {
	case "length":
		switch v := args["value"].(type) {
		case engine.ListValue:
			return engine.NumberValue{Value: float64(len(v.Elements))}, nil
		case engine.MapValue:
			return engine.NumberValue{Value: float64(len(v.Entries))}, nil
		case engine.StringValue:
			return engine.NumberValue{Value: float64(len(v.Value))}, nil
		default:
			return nil, newExecutionError(ExecutionErrorUnknown, "engineservice: length given an unsupported value of type %T", v)
		}
	case "min":
		a, b := args["a"].(engine.NumberValue).Value, args["b"].(engine.NumberValue).Value
		if a < b {
			return engine.NumberValue{Value: a}, nil
		}
		return engine.NumberValue{Value: b}, nil
	case "max":
		a, b := args["a"].(engine.NumberValue).Value, args["b"].(engine.NumberValue).Value
		if a > b {
			return engine.NumberValue{Value: a}, nil
		}
		return engine.NumberValue{Value: b}, nil
	case "abs":
		v := args["value"].(engine.NumberValue).Value
		if v < 0 {
			v = -v
		}
		return engine.NumberValue{Value: v}, nil
	default:
		return nil, newExecutionError(ExecutionErrorUnknown, "engineservice: call to unknown function %q", name)
	}
}

func (e *evaluator) evalMatch(x engine.MatchExpression, scope engine.Scope) (engine.Value, error) {
	v, err := e.eval(x.Value, scope)
	if err != nil {
		return nil, err
	}
	for _, cs := range x.Cases {
		caseScope, matched := matchPattern(cs.Pattern, v, scope)
		if matched {
			return e.eval(cs.Result, caseScope)
		}
	}
	return nil, newExecutionError(ExecutionErrorNoMatchingCase, "engineservice: no match case matched the value")
}

// matchPattern reports whether v matches pattern and, if so, the scope
// extended with any lexical bindings the pattern introduces.
func matchPattern(pattern engine.MatchPattern, v engine.Value, scope engine.Scope) (engine.Scope, bool) {
	switch p := pattern.(type) {
	case engine.WildcardMatchPattern:
		return scope, true

	case engine.EnumValueMatchPattern:
		ev, ok := v.(engine.EnumValue)
		return scope, ok && ev.TypeName == p.TypeName && ev.ValueName == p.ValueName

	case engine.UnionVariantMatchPattern:
		uv, ok := v.(engine.UnionValue)
		if !ok || uv.TypeName != p.TypeName || uv.VariantName != p.VariantName {
			return scope, false
		}
		newScope := scope
		for _, b := range p.Bindings {
			fv, ok := uv.FieldByName(b.Field)
			if !ok {
				continue
			}
			newScope = extendScope(newScope, b.Name, fv.Value)
		}
		return newScope, true

	case engine.OptionalNoneMatchPattern:
		ov, ok := v.(engine.OptionalValue)
		return scope, ok && ov.Value == nil

	case engine.OptionalSomeMatchPattern:
		ov, ok := v.(engine.OptionalValue)
		if !ok || ov.Value == nil {
			return scope, false
		}
		if p.Binding == "" {
			return scope, true
		}
		return extendScope(scope, p.Binding, ov.Value), true

	default:
		return scope, false
	}
}
