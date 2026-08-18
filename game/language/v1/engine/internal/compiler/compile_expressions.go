package compiler

import (
	"fmt"
	"strconv"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

// compileExpression compiles expr in scope, returning the compiled
// engine.Expression together with its statically determined engine.Type.
//
// Unlike gameservice.Validate's shallow inferType, this always resolves
// a name — a ReferenceExpression, a FieldExpression, an IndexExpression,
// a CallExpression — because scope, the compile-time symbol table this
// package builds, gives it the lexical information gameservice never
// had. A nil returned Type means expr is invalid and a Diagnostic was
// already recorded; callers should stop relying on that branch rather
// than re-diagnosing it.
func (c *compiler) compileExpression(expr program.Expression, scope exprScope, path string) (engine.Expression, engine.Type) {
	switch e := expr.(type) {
	case nil:
		c.addf(path, "missing expression")
		return nil, nil

	case program.UnitLiteralExpression:
		return engine.UnitLiteralExpression{}, engine.UnitType{}

	case program.BoolLiteralExpression:
		return engine.BoolLiteralExpression{Value: e.Value}, engine.BoolType{}

	case program.NumberLiteralExpression:
		n, err := strconv.ParseFloat(e.Value, 64)
		if err != nil {
			c.addf(path, "invalid number literal %q: %s", e.Value, err)
			return nil, nil
		}
		return engine.NumberLiteralExpression{Value: n}, engine.NumberType{}

	case program.StringLiteralExpression:
		return engine.StringLiteralExpression{Value: e.Value}, engine.StringType{}

	case program.OptionalNoneExpression:
		t := c.compileTypeReference(e.ElementType, path+".element_type")
		if t == nil {
			return nil, nil
		}
		return engine.OptionalNoneExpression{ElementType: t}, engine.OptionalType{Element: t}

	case program.OptionalSomeExpression:
		v, t := c.compileExpression(e.Value, scope, path+".value")
		if t == nil {
			return nil, nil
		}
		return engine.OptionalSomeExpression{ElementType: t, Value: v}, engine.OptionalType{Element: t}

	case program.ListExpression:
		return c.compileListExpression(e, scope, path)

	case program.MapExpression:
		return c.compileMapExpression(e, scope, path)

	case program.EnumValueExpression:
		return c.compileEnumValueExpression(e, path)

	case program.RecordExpression:
		return c.compileRecordExpression(e, scope, path)

	case program.UnionExpression:
		return c.compileUnionExpression(e, scope, path)

	case program.NewTypeExpression:
		return c.compileNewTypeExpression(e, scope, path)

	case program.ReferenceExpression:
		return c.compileReferenceExpression(e, scope, path)

	case program.FieldExpression:
		return c.compileFieldExpression(e, scope, path)

	case program.IndexExpression:
		return c.compileIndexExpression(e, scope, path)

	case program.UnaryExpression:
		return c.compileUnaryExpression(e, scope, path)

	case program.BinaryExpression:
		return c.compileBinaryExpression(e, scope, path)

	case program.ConditionalExpression:
		return c.compileConditionalExpression(e, scope, path)

	case program.CallExpression:
		return c.compileCallExpression(e, scope, path)

	case program.MatchExpression:
		return c.compileMatchExpression(e, scope, path)

	case program.ListMapExpression:
		return c.compileListMapExpression(e, scope, path)

	case program.ListFilterExpression:
		return c.compileListFilterExpression(e, scope, path)

	case program.ListFlatMapExpression:
		return c.compileListFlatMapExpression(e, scope, path)

	case program.ListAnyExpression:
		coll, pred, ok := c.compileListPredicateParts(e.Collection, e.ItemName, e.IndexName, e.Predicate, scope, path)
		if !ok {
			return nil, nil
		}
		return engine.ListAnyExpression{Collection: coll, ItemName: e.ItemName, IndexName: e.IndexName, Predicate: pred}, engine.BoolType{}

	case program.ListAllExpression:
		coll, pred, ok := c.compileListPredicateParts(e.Collection, e.ItemName, e.IndexName, e.Predicate, scope, path)
		if !ok {
			return nil, nil
		}
		return engine.ListAllExpression{Collection: coll, ItemName: e.ItemName, IndexName: e.IndexName, Predicate: pred}, engine.BoolType{}

	case program.ListCountExpression:
		coll, pred, ok := c.compileListPredicateParts(e.Collection, e.ItemName, e.IndexName, e.Predicate, scope, path)
		if !ok {
			return nil, nil
		}
		return engine.ListCountExpression{Collection: coll, ItemName: e.ItemName, IndexName: e.IndexName, Predicate: pred}, engine.NumberType{}

	case program.ListFirstExpression:
		return c.compileListFirstExpression(e, scope, path)

	default:
		c.addf(path, "unsupported expression")
		return nil, nil
	}
}

// compileListPredicateParts compiles the shared shape of
// ListAnyExpression, ListAllExpression, and ListCountExpression: a
// Collection that must be statically a list, and a Predicate compiled in
// a scope extended with the per-element item/index bindings that must be
// statically bool.
func (c *compiler) compileListPredicateParts(collExpr program.Expression, itemName, indexName string, predicate program.Expression, scope exprScope, path string) (engine.Expression, engine.Expression, bool) {
	coll, listType, ok := c.compileListCollection(collExpr, scope, path)
	if !ok {
		return coll, nil, false
	}
	predScope, sOk := c.bindListItem(scope, itemName, indexName, listType.Element, path)
	if !sOk {
		return coll, nil, false
	}
	pred, predType := c.compileExpression(predicate, predScope, path+".predicate")
	if predType == nil {
		return coll, pred, false
	}
	if _, ok := predType.(engine.BoolType); !ok {
		c.addf(path+".predicate", "predicate must be statically bool, but it is %s", describeType(predType))
		return coll, pred, false
	}
	return coll, pred, true
}

// compileListCollection compiles collExpr and requires its static type
// to be a list, reporting a Diagnostic and ok == false otherwise.
func (c *compiler) compileListCollection(collExpr program.Expression, scope exprScope, path string) (engine.Expression, engine.ListType, bool) {
	coll, collType := c.compileExpression(collExpr, scope, path+".collection")
	if collType == nil {
		return coll, engine.ListType{}, false
	}
	listType, ok := collType.(engine.ListType)
	if !ok {
		c.addf(path+".collection", "expected a list, but the collection is statically %s", describeType(collType))
		return coll, engine.ListType{}, false
	}
	return coll, listType, true
}

// bindListItem extends scope with itemName bound to elementType and, if
// indexName is non-empty, indexName bound to number — the per-element
// lexical bindings every list-query expression introduces.
func (c *compiler) bindListItem(scope exprScope, itemName, indexName string, elementType engine.Type, path string) (exprScope, bool) {
	if itemName == "" {
		c.addf(path, "list query is missing its item binding name")
		return scope, false
	}
	newScope := scope.clone()
	newScope[itemName] = elementType
	if indexName != "" {
		newScope[indexName] = engine.NumberType{}
	}
	return newScope, true
}

func (c *compiler) compileListMapExpression(e program.ListMapExpression, scope exprScope, path string) (engine.Expression, engine.Type) {
	coll, listType, ok := c.compileListCollection(e.Collection, scope, path)
	if !ok {
		return nil, nil
	}
	resultScope, sOk := c.bindListItem(scope, e.ItemName, e.IndexName, listType.Element, path)
	if !sOk {
		return nil, nil
	}
	result, resultType := c.compileExpression(e.Result, resultScope, path+".result")
	if resultType == nil {
		return nil, nil
	}
	return engine.ListMapExpression{Collection: coll, ItemName: e.ItemName, IndexName: e.IndexName, Result: result, ResultElementType: resultType},
		engine.ListType{Element: resultType}
}

func (c *compiler) compileListFilterExpression(e program.ListFilterExpression, scope exprScope, path string) (engine.Expression, engine.Type) {
	coll, listType, ok := c.compileListCollection(e.Collection, scope, path)
	if !ok {
		return nil, nil
	}
	predScope, sOk := c.bindListItem(scope, e.ItemName, e.IndexName, listType.Element, path)
	if !sOk {
		return nil, nil
	}
	pred, predType := c.compileExpression(e.Predicate, predScope, path+".predicate")
	if predType == nil {
		return nil, nil
	}
	if _, ok := predType.(engine.BoolType); !ok {
		c.addf(path+".predicate", "predicate must be statically bool, but it is %s", describeType(predType))
		return nil, nil
	}
	return engine.ListFilterExpression{Collection: coll, ItemName: e.ItemName, IndexName: e.IndexName, Predicate: pred}, listType
}

func (c *compiler) compileListFlatMapExpression(e program.ListFlatMapExpression, scope exprScope, path string) (engine.Expression, engine.Type) {
	coll, listType, ok := c.compileListCollection(e.Collection, scope, path)
	if !ok {
		return nil, nil
	}
	resultScope, sOk := c.bindListItem(scope, e.ItemName, e.IndexName, listType.Element, path)
	if !sOk {
		return nil, nil
	}
	result, resultType := c.compileExpression(e.Result, resultScope, path+".result")
	if resultType == nil {
		return nil, nil
	}
	resultListType, ok := resultType.(engine.ListType)
	if !ok {
		c.addf(path+".result", "flat-map result must be statically a list, but it is %s", describeType(resultType))
		return nil, nil
	}
	return engine.ListFlatMapExpression{Collection: coll, ItemName: e.ItemName, IndexName: e.IndexName, Result: result, ResultElementType: resultListType.Element},
		engine.ListType{Element: resultListType.Element}
}

func (c *compiler) compileListFirstExpression(e program.ListFirstExpression, scope exprScope, path string) (engine.Expression, engine.Type) {
	coll, listType, ok := c.compileListCollection(e.Collection, scope, path)
	if !ok {
		return nil, nil
	}
	predScope, sOk := c.bindListItem(scope, e.ItemName, e.IndexName, listType.Element, path)
	if !sOk {
		return nil, nil
	}
	pred, predType := c.compileExpression(e.Predicate, predScope, path+".predicate")
	if predType == nil {
		return nil, nil
	}
	if _, ok := predType.(engine.BoolType); !ok {
		c.addf(path+".predicate", "predicate must be statically bool, but it is %s", describeType(predType))
		return nil, nil
	}
	return engine.ListFirstExpression{Collection: coll, ItemName: e.ItemName, IndexName: e.IndexName, Predicate: pred},
		engine.OptionalType{Element: listType.Element}
}

func (c *compiler) compileListExpression(e program.ListExpression, scope exprScope, path string) (engine.Expression, engine.Type) {
	var elementType engine.Type
	if e.ElementType != nil {
		elementType = c.compileTypeReference(e.ElementType, path+".element_type")
	}

	elements := make([]engine.Expression, 0, len(e.Elements))
	ok := true
	for i, el := range e.Elements {
		v, t := c.compileExpression(el, scope, fmt.Sprintf("%s.elements[%d]", path, i))
		if t == nil {
			ok = false
			continue
		}
		if elementType == nil {
			elementType = t
		} else if !elementType.Equal(t) {
			c.addf(fmt.Sprintf("%s.elements[%d]", path, i), "list element is statically %s, but the list's element type is %s", describeType(t), describeType(elementType))
			ok = false
		}
		elements = append(elements, v)
	}

	if elementType == nil {
		c.addf(path, "cannot infer the element type of an empty list without an explicit annotation")
		return nil, nil
	}
	if !ok {
		return nil, nil
	}
	return engine.ListExpression{ElementType: elementType, Elements: elements}, engine.ListType{Element: elementType}
}

func (c *compiler) compileMapExpression(e program.MapExpression, scope exprScope, path string) (engine.Expression, engine.Type) {
	var keyType, valueType engine.Type
	if e.KeyType != nil {
		keyType = c.compileTypeReference(e.KeyType, path+".key_type")
	}
	if e.ValueType != nil {
		valueType = c.compileTypeReference(e.ValueType, path+".value_type")
	}

	entries := make([]engine.MapEntryExpression, 0, len(e.Entries))
	ok := true
	for i, entry := range e.Entries {
		entryPath := fmt.Sprintf("%s.entries[%d]", path, i)
		k, kt := c.compileExpression(entry.Key, scope, entryPath+".key")
		v, vt := c.compileExpression(entry.Value, scope, entryPath+".value")
		if kt == nil || vt == nil {
			ok = false
			continue
		}
		if keyType == nil {
			keyType = kt
		} else if !keyType.Equal(kt) {
			c.addf(entryPath+".key", "map key is statically %s, but the map's key type is %s", describeType(kt), describeType(keyType))
			ok = false
		}
		if valueType == nil {
			valueType = vt
		} else if !valueType.Equal(vt) {
			c.addf(entryPath+".value", "map value is statically %s, but the map's value type is %s", describeType(vt), describeType(valueType))
			ok = false
		}
		entries = append(entries, engine.MapEntryExpression{Key: k, Value: v})
	}

	if keyType == nil || valueType == nil {
		c.addf(path, "cannot infer the key/value type of an empty map without an explicit annotation")
		return nil, nil
	}
	if !ok {
		return nil, nil
	}
	return engine.MapExpression{KeyType: keyType, ValueType: valueType, Entries: entries}, engine.MapType{Key: keyType, Value: valueType}
}

func (c *compiler) compileEnumValueExpression(e program.EnumValueExpression, path string) (engine.Expression, engine.Type) {
	if e.TypeName == "" {
		c.addf(path+".type_name", "enum value expression has an empty type name")
		return nil, nil
	}
	t, ok := c.compileNamedTypeUse(e.TypeName, path+".type_name")
	if !ok {
		return nil, nil
	}
	et, ok := t.(engine.EnumType)
	if !ok {
		c.addf(path+".type_name", "type %q is not an enum", e.TypeName)
		return nil, nil
	}
	if !et.HasValue(e.ValueName) {
		c.addf(path+".value_name", "enum %q has no value named %q", e.TypeName, e.ValueName)
		return nil, nil
	}
	return engine.EnumValueExpression{TypeName: e.TypeName, ValueName: e.ValueName}, et
}

func (c *compiler) compileRecordExpression(e program.RecordExpression, scope exprScope, path string) (engine.Expression, engine.Type) {
	if e.TypeName == "" {
		c.addf(path+".type_name", "record expression has an empty type name")
		return nil, nil
	}
	t, ok := c.compileNamedTypeUse(e.TypeName, path+".type_name")
	if !ok {
		return nil, nil
	}
	rt, ok := t.(engine.RecordType)
	if !ok {
		c.addf(path+".type_name", "type %q is not a record", e.TypeName)
		return nil, nil
	}
	fields, ok := c.compileFieldInitializers(e.Fields, rt.Fields, scope, path+".fields")
	if !ok {
		return nil, nil
	}
	return engine.RecordExpression{TypeName: e.TypeName, Fields: fields}, rt
}

func (c *compiler) compileUnionExpression(e program.UnionExpression, scope exprScope, path string) (engine.Expression, engine.Type) {
	if e.TypeName == "" {
		c.addf(path+".type_name", "union expression has an empty type name")
		return nil, nil
	}
	t, ok := c.compileNamedTypeUse(e.TypeName, path+".type_name")
	if !ok {
		return nil, nil
	}
	ut, ok := t.(engine.UnionType)
	if !ok {
		c.addf(path+".type_name", "type %q is not a union", e.TypeName)
		return nil, nil
	}
	variant, ok := ut.VariantByName(e.VariantName)
	if !ok {
		c.addf(path+".variant_name", "union %q has no variant named %q", e.TypeName, e.VariantName)
		return nil, nil
	}
	fields, ok := c.compileFieldInitializers(e.Fields, variant.Fields, scope, path+".fields")
	if !ok {
		return nil, nil
	}
	return engine.UnionExpression{TypeName: e.TypeName, VariantName: e.VariantName, Fields: fields}, ut
}

func (c *compiler) compileNewTypeExpression(e program.NewTypeExpression, scope exprScope, path string) (engine.Expression, engine.Type) {
	if e.TypeName == "" {
		c.addf(path+".type_name", "new type expression has an empty type name")
		return nil, nil
	}
	t, ok := c.compileNamedTypeUse(e.TypeName, path+".type_name")
	if !ok {
		return nil, nil
	}
	nt, ok := t.(engine.NewType)
	if !ok {
		c.addf(path+".type_name", "type %q is not a new type", e.TypeName)
		return nil, nil
	}
	v, vt := c.compileExpression(e.Value, scope, path+".value")
	if vt == nil {
		return nil, nil
	}
	if !nt.Underlying.Equal(vt) {
		c.addf(path+".value", "new type %q requires an underlying value of %s, but the value is statically %s", e.TypeName, describeType(nt.Underlying), describeType(vt))
		return nil, nil
	}
	return engine.NewTypeExpression{TypeName: e.TypeName, Value: v}, nt
}

// compileNamedTypeUse resolves a use of a named type from within the
// expression language — a type constructor's TypeName, a match
// pattern's TypeName — the same way compileTypeReference resolves a
// program.NamedTypeReference: it checks existence and rejects a
// reference to a type this compiler is still in the middle of compiling.
func (c *compiler) compileNamedTypeUse(name, path string) (engine.Type, bool) {
	if _, ok := c.typeDeclarations[name]; !ok {
		c.addf(path, "reference to undeclared type %q", name)
		return nil, false
	}
	if c.resolvingTypes[name] {
		c.addf(path, "type %q is defined recursively (directly or through a list, map, or optional), which this compiler does not yet support", name)
		return nil, false
	}
	return c.resolveNamedType(name), true
}

// compileFieldInitializers compiles inits against declared, the field
// shape of the record or union variant being constructed: every
// declared field must be initialized exactly once, with no unknown or
// duplicate initializer, and every value must statically match its
// field's declared type.
func (c *compiler) compileFieldInitializers(inits []program.FieldInitializer, declared []engine.FieldType, scope exprScope, path string) ([]engine.FieldInitializer, bool) {
	result := make([]engine.FieldInitializer, 0, len(inits))
	seen := make(map[string]int, len(inits))
	ok := true
	for i, f := range inits {
		fPath := fmt.Sprintf("%s[%d]", path, i)
		if f.Name == "" {
			c.addf(fPath+".name", "field initializer has an empty name")
			ok = false
			continue
		}
		if first, dup := seen[f.Name]; dup {
			c.addf(fPath, "duplicate field initializer %q (first provided at %s[%d])", f.Name, path, first)
			ok = false
			continue
		}
		seen[f.Name] = i

		v, vt := c.compileExpression(f.Value, scope, fPath+".value")
		ft, exists := fieldTypeIndex(declared, f.Name)
		if !exists {
			c.addf(fPath+".name", "unknown field %q", f.Name)
			ok = false
		} else if vt != nil && !ft.Type.Equal(vt) {
			c.addf(fPath+".value", "field %q is statically %s, but %s is required", f.Name, describeType(vt), describeType(ft.Type))
			ok = false
		}
		if vt == nil {
			ok = false
		}
		result = append(result, engine.FieldInitializer{Name: f.Name, Value: v})
	}
	for _, d := range declared {
		if _, provided := seen[d.Name]; !provided {
			c.addf(path, "missing required field %q", d.Name)
			ok = false
		}
	}
	return result, ok
}

func fieldTypeIndex(fields []engine.FieldType, name string) (engine.FieldType, bool) {
	for _, f := range fields {
		if f.Name == name {
			return f, true
		}
	}
	return engine.FieldType{}, false
}

func (c *compiler) compileReferenceExpression(e program.ReferenceExpression, scope exprScope, path string) (engine.Expression, engine.Type) {
	if e.Name == "" {
		c.addf(path, "reference expression has an empty name")
		return nil, nil
	}
	t, ok := scope[e.Name]
	if !ok {
		c.addf(path, "reference to undeclared name %q", e.Name)
		return nil, nil
	}
	return engine.ReferenceExpression{Name: e.Name}, t
}

func (c *compiler) compileFieldExpression(e program.FieldExpression, scope exprScope, path string) (engine.Expression, engine.Type) {
	target, targetType := c.compileExpression(e.Target, scope, path+".target")
	if targetType == nil {
		return nil, nil
	}
	rt, ok := targetType.(engine.RecordType)
	if !ok {
		c.addf(path, "field access requires a record, but the target is statically %s", describeType(targetType))
		return nil, nil
	}
	ft, ok := rt.FieldByName(e.Field)
	if !ok {
		c.addf(path+".field", "record %q has no field named %q", rt.Name, e.Field)
		return nil, nil
	}
	return engine.FieldExpression{Target: target, Field: e.Field}, ft.Type
}

func (c *compiler) compileIndexExpression(e program.IndexExpression, scope exprScope, path string) (engine.Expression, engine.Type) {
	target, targetType := c.compileExpression(e.Target, scope, path+".target")
	index, indexType := c.compileExpression(e.Index, scope, path+".index")
	if targetType == nil || indexType == nil {
		return nil, nil
	}
	switch t := targetType.(type) {
	case engine.ListType:
		if _, ok := indexType.(engine.NumberType); !ok {
			c.addf(path+".index", "list index must be statically number, but it is %s", describeType(indexType))
			return nil, nil
		}
		return engine.IndexExpression{Target: target, Index: index}, t.Element
	case engine.MapType:
		if !t.Key.Equal(indexType) {
			c.addf(path+".index", "map index must be statically %s, but it is %s", describeType(t.Key), describeType(indexType))
			return nil, nil
		}
		return engine.IndexExpression{Target: target, Index: index}, t.Value
	default:
		c.addf(path, "indexing requires a list or map, but the target is statically %s", describeType(targetType))
		return nil, nil
	}
}

func (c *compiler) compileUnaryExpression(e program.UnaryExpression, scope exprScope, path string) (engine.Expression, engine.Type) {
	operand, operandType := c.compileExpression(e.Operand, scope, path+".operand")
	op, ok := compileUnaryOperator(e.Operator)
	if !ok {
		c.addf(path+".operator", "unknown unary operator %q", e.Operator)
		return nil, nil
	}
	if operandType == nil {
		return nil, nil
	}
	switch op {
	case engine.UnaryOperatorNot:
		if !isBool(operandType) {
			c.addf(path, "operator %q requires a bool operand, but the operand is statically %s", op, describeType(operandType))
			return nil, nil
		}
		return engine.UnaryExpression{Operator: op, Operand: operand}, engine.BoolType{}
	case engine.UnaryOperatorNegate:
		if !isNumber(operandType) {
			c.addf(path, "operator %q requires a number operand, but the operand is statically %s", op, describeType(operandType))
			return nil, nil
		}
		return engine.UnaryExpression{Operator: op, Operand: operand}, engine.NumberType{}
	default:
		return nil, nil
	}
}

func compileUnaryOperator(op program.UnaryOperator) (engine.UnaryOperator, bool) {
	switch op {
	case program.UnaryOperatorNot:
		return engine.UnaryOperatorNot, true
	case program.UnaryOperatorNegate:
		return engine.UnaryOperatorNegate, true
	default:
		return "", false
	}
}

func (c *compiler) compileBinaryExpression(e program.BinaryExpression, scope exprScope, path string) (engine.Expression, engine.Type) {
	left, leftType := c.compileExpression(e.Left, scope, path+".left")
	right, rightType := c.compileExpression(e.Right, scope, path+".right")
	op, ok := compileBinaryOperator(e.Operator)
	if !ok {
		c.addf(path+".operator", "unknown binary operator %q", e.Operator)
		return nil, nil
	}
	if leftType == nil || rightType == nil {
		return nil, nil
	}

	switch op {
	case engine.BinaryOperatorAdd, engine.BinaryOperatorSubtract, engine.BinaryOperatorMultiply, engine.BinaryOperatorDivide, engine.BinaryOperatorModulo:
		if !isNumber(leftType) || !isNumber(rightType) {
			c.addf(path, "operator %q requires number operands, but the operands are statically %s and %s", op, describeType(leftType), describeType(rightType))
			return nil, nil
		}
		return engine.BinaryExpression{Operator: op, Left: left, Right: right}, engine.NumberType{}

	case engine.BinaryOperatorLess, engine.BinaryOperatorLessOrEqual, engine.BinaryOperatorGreater, engine.BinaryOperatorGreaterOrEqual:
		if !isNumber(leftType) || !isNumber(rightType) {
			c.addf(path, "operator %q requires number operands, but the operands are statically %s and %s", op, describeType(leftType), describeType(rightType))
			return nil, nil
		}
		return engine.BinaryExpression{Operator: op, Left: left, Right: right}, engine.BoolType{}

	case engine.BinaryOperatorAnd, engine.BinaryOperatorOr:
		if !isBool(leftType) || !isBool(rightType) {
			c.addf(path, "operator %q requires bool operands, but the operands are statically %s and %s", op, describeType(leftType), describeType(rightType))
			return nil, nil
		}
		return engine.BinaryExpression{Operator: op, Left: left, Right: right}, engine.BoolType{}

	case engine.BinaryOperatorEqual, engine.BinaryOperatorNotEqual:
		if !leftType.Equal(rightType) {
			c.addf(path, "operator %q compares statically incompatible types %s and %s", op, describeType(leftType), describeType(rightType))
			return nil, nil
		}
		return engine.BinaryExpression{Operator: op, Left: left, Right: right}, engine.BoolType{}

	case engine.BinaryOperatorIn, engine.BinaryOperatorNotIn:
		switch rt := rightType.(type) {
		case engine.ListType:
			if !rt.Element.Equal(leftType) {
				c.addf(path, "operator %q requires the left operand's type %s to match the list's element type %s", op, describeType(leftType), describeType(rt.Element))
				return nil, nil
			}
		case engine.MapType:
			if !rt.Key.Equal(leftType) {
				c.addf(path, "operator %q requires the left operand's type %s to match the map's key type %s", op, describeType(leftType), describeType(rt.Key))
				return nil, nil
			}
		default:
			c.addf(path+".right", "operator %q requires a list or map on the right, but the right operand is statically %s", op, describeType(rightType))
			return nil, nil
		}
		return engine.BinaryExpression{Operator: op, Left: left, Right: right}, engine.BoolType{}

	default:
		return nil, nil
	}
}

func compileBinaryOperator(op program.BinaryOperator) (engine.BinaryOperator, bool) {
	switch op {
	case program.BinaryOperatorAdd:
		return engine.BinaryOperatorAdd, true
	case program.BinaryOperatorSubtract:
		return engine.BinaryOperatorSubtract, true
	case program.BinaryOperatorMultiply:
		return engine.BinaryOperatorMultiply, true
	case program.BinaryOperatorDivide:
		return engine.BinaryOperatorDivide, true
	case program.BinaryOperatorModulo:
		return engine.BinaryOperatorModulo, true
	case program.BinaryOperatorEqual:
		return engine.BinaryOperatorEqual, true
	case program.BinaryOperatorNotEqual:
		return engine.BinaryOperatorNotEqual, true
	case program.BinaryOperatorLess:
		return engine.BinaryOperatorLess, true
	case program.BinaryOperatorLessOrEqual:
		return engine.BinaryOperatorLessOrEqual, true
	case program.BinaryOperatorGreater:
		return engine.BinaryOperatorGreater, true
	case program.BinaryOperatorGreaterOrEqual:
		return engine.BinaryOperatorGreaterOrEqual, true
	case program.BinaryOperatorAnd:
		return engine.BinaryOperatorAnd, true
	case program.BinaryOperatorOr:
		return engine.BinaryOperatorOr, true
	case program.BinaryOperatorIn:
		return engine.BinaryOperatorIn, true
	case program.BinaryOperatorNotIn:
		return engine.BinaryOperatorNotIn, true
	default:
		return "", false
	}
}

func (c *compiler) compileConditionalExpression(e program.ConditionalExpression, scope exprScope, path string) (engine.Expression, engine.Type) {
	cond, condType := c.compileExpression(e.Condition, scope, path+".condition")
	then, thenType := c.compileExpression(e.Then, scope, path+".then")
	els, elseType := c.compileExpression(e.Else, scope, path+".else")

	if condType != nil {
		if !isBool(condType) {
			c.addf(path+".condition", "condition must be statically bool, but it is %s", describeType(condType))
			condType = nil
		}
	}
	if condType == nil || thenType == nil || elseType == nil {
		return nil, nil
	}
	if !thenType.Equal(elseType) {
		c.addf(path, "conditional branches are statically incompatible: then is %s, else is %s", describeType(thenType), describeType(elseType))
		return nil, nil
	}
	return engine.ConditionalExpression{Condition: cond, Then: then, Else: els}, thenType
}

func isNumber(t engine.Type) bool {
	_, ok := t.(engine.NumberType)
	return ok
}

func isBool(t engine.Type) bool {
	_, ok := t.(engine.BoolType)
	return ok
}

// describeType renders t for diagnostic messages, mirroring
// program/gameservice's own describeType but over engine.Type.
func describeType(t engine.Type) string {
	switch tt := t.(type) {
	case nil:
		return "unknown"
	case engine.UnitType:
		return "unit"
	case engine.BoolType:
		return "bool"
	case engine.NumberType:
		return "number"
	case engine.StringType:
		return "string"
	case engine.UserType:
		return "user"
	case engine.EnumType:
		return tt.Name
	case engine.RecordType:
		return tt.Name
	case engine.UnionType:
		return tt.Name
	case engine.NewType:
		return tt.Name
	case engine.ListType:
		return "list<" + describeType(tt.Element) + ">"
	case engine.MapType:
		return "map<" + describeType(tt.Key) + ", " + describeType(tt.Value) + ">"
	case engine.OptionalType:
		return "optional<" + describeType(tt.Element) + ">"
	default:
		return "unknown"
	}
}
