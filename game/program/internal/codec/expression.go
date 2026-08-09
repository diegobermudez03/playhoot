package codec

import "github.com/diegobermudez03/playhoot/game/program"

import (
	"encoding/json"
	"fmt"
)

// --- literal and optional wire structs ---

type wireUnitLiteralExpression struct {
	Kind string `json:"kind"`
}

type wireBoolLiteralExpression struct {
	Kind  string `json:"kind"`
	Value bool   `json:"value"`
}

type wireNumberLiteralExpression struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type wireStringLiteralExpression struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

type wireOptionalNoneExpression struct {
	Kind        string          `json:"kind"`
	ElementType json.RawMessage `json:"element_type"`
}

type wireOptionalSomeExpression struct {
	Kind  string          `json:"kind"`
	Value json.RawMessage `json:"value"`
}

// --- collection wire structs ---

type wireListExpression struct {
	Kind        string            `json:"kind"`
	ElementType json.RawMessage   `json:"element_type"`
	Elements    []json.RawMessage `json:"elements"`
}

type wireMapEntryExpression struct {
	Key   json.RawMessage `json:"key"`
	Value json.RawMessage `json:"value"`
}

type wireMapExpression struct {
	Kind      string                   `json:"kind"`
	KeyType   json.RawMessage          `json:"key_type"`
	ValueType json.RawMessage          `json:"value_type"`
	Entries   []wireMapEntryExpression `json:"entries"`
}

// --- named-type construction wire structs ---

type wireEnumValueExpression struct {
	Kind      string `json:"kind"`
	TypeName  string `json:"type_name"`
	ValueName string `json:"value_name"`
}

type wireFieldInitializer struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

type wireRecordExpression struct {
	Kind     string                 `json:"kind"`
	TypeName string                 `json:"type_name"`
	Fields   []wireFieldInitializer `json:"fields"`
}

type wireUnionExpression struct {
	Kind        string                 `json:"kind"`
	TypeName    string                 `json:"type_name"`
	VariantName string                 `json:"variant_name"`
	Fields      []wireFieldInitializer `json:"fields"`
}

type wireNewTypeExpression struct {
	Kind     string          `json:"kind"`
	TypeName string          `json:"type_name"`
	Value    json.RawMessage `json:"value"`
}

// --- reference and access wire structs ---

type wireReferenceExpression struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type wireFieldExpression struct {
	Kind   string          `json:"kind"`
	Target json.RawMessage `json:"target"`
	Field  string          `json:"field"`
}

type wireIndexExpression struct {
	Kind   string          `json:"kind"`
	Target json.RawMessage `json:"target"`
	Index  json.RawMessage `json:"index"`
}

// --- operator wire structs ---

type wireUnaryExpression struct {
	Kind     string                `json:"kind"`
	Operator program.UnaryOperator `json:"operator"`
	Operand  json.RawMessage       `json:"operand"`
}

type wireBinaryExpression struct {
	Kind     string                 `json:"kind"`
	Operator program.BinaryOperator `json:"operator"`
	Left     json.RawMessage        `json:"left"`
	Right    json.RawMessage        `json:"right"`
}

type wireConditionalExpression struct {
	Kind      string          `json:"kind"`
	Condition json.RawMessage `json:"condition"`
	Then      json.RawMessage `json:"then"`
	Else      json.RawMessage `json:"else"`
}

// --- call wire structs ---

type wireCallArgument struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

type wireCallExpression struct {
	Kind      string             `json:"kind"`
	Function  string             `json:"function"`
	Arguments []wireCallArgument `json:"arguments"`
}

// --- match wire structs ---

type wireMatchExpressionCase struct {
	Pattern json.RawMessage `json:"pattern"`
	Result  json.RawMessage `json:"result"`
}

type wireMatchExpression struct {
	Kind  string                    `json:"kind"`
	Value json.RawMessage           `json:"value"`
	Cases []wireMatchExpressionCase `json:"cases"`
}

// --- list-query wire structs ---

type wireListMapExpression struct {
	Kind       string          `json:"kind"`
	Collection json.RawMessage `json:"collection"`
	ItemName   string          `json:"item_name"`
	IndexName  string          `json:"index_name"`
	Result     json.RawMessage `json:"result"`
}

type wireListFilterExpression struct {
	Kind       string          `json:"kind"`
	Collection json.RawMessage `json:"collection"`
	ItemName   string          `json:"item_name"`
	IndexName  string          `json:"index_name"`
	Predicate  json.RawMessage `json:"predicate"`
}

type wireListFlatMapExpression struct {
	Kind       string          `json:"kind"`
	Collection json.RawMessage `json:"collection"`
	ItemName   string          `json:"item_name"`
	IndexName  string          `json:"index_name"`
	Result     json.RawMessage `json:"result"`
}

type wireListAnyExpression struct {
	Kind       string          `json:"kind"`
	Collection json.RawMessage `json:"collection"`
	ItemName   string          `json:"item_name"`
	IndexName  string          `json:"index_name"`
	Predicate  json.RawMessage `json:"predicate"`
}

type wireListAllExpression struct {
	Kind       string          `json:"kind"`
	Collection json.RawMessage `json:"collection"`
	ItemName   string          `json:"item_name"`
	IndexName  string          `json:"index_name"`
	Predicate  json.RawMessage `json:"predicate"`
}

type wireListCountExpression struct {
	Kind       string          `json:"kind"`
	Collection json.RawMessage `json:"collection"`
	ItemName   string          `json:"item_name"`
	IndexName  string          `json:"index_name"`
	Predicate  json.RawMessage `json:"predicate"`
}

type wireListFirstExpression struct {
	Kind       string          `json:"kind"`
	Collection json.RawMessage `json:"collection"`
	ItemName   string          `json:"item_name"`
	IndexName  string          `json:"index_name"`
	Predicate  json.RawMessage `json:"predicate"`
}

// --- slice helpers shared across expression variants ---

func encodeExpressionSlice(path string, items []program.Expression) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeExpression(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeExpressionSlice(path string, items []json.RawMessage) ([]program.Expression, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.Expression, len(items))
	for i, raw := range items {
		item, err := decodeExpression(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

func encodeMapEntries(path string, entries []program.MapEntryExpression) ([]wireMapEntryExpression, error) {
	if entries == nil {
		return nil, nil
	}
	result := make([]wireMapEntryExpression, len(entries))
	for i, entry := range entries {
		itemPath := pathIndex(path, i)
		key, err := encodeExpression(pathField(itemPath, "key"), entry.Key)
		if err != nil {
			return nil, err
		}
		value, err := encodeExpression(pathField(itemPath, "value"), entry.Value)
		if err != nil {
			return nil, err
		}
		result[i] = wireMapEntryExpression{Key: key, Value: value}
	}
	return result, nil
}

func decodeMapEntries(path string, entries []wireMapEntryExpression) ([]program.MapEntryExpression, error) {
	if entries == nil {
		return nil, nil
	}
	result := make([]program.MapEntryExpression, len(entries))
	for i, entry := range entries {
		itemPath := pathIndex(path, i)
		key, err := decodeExpression(pathField(itemPath, "key"), entry.Key)
		if err != nil {
			return nil, err
		}
		value, err := decodeExpression(pathField(itemPath, "value"), entry.Value)
		if err != nil {
			return nil, err
		}
		result[i] = program.MapEntryExpression{Key: key, Value: value}
	}
	return result, nil
}

func encodeFieldInitializers(path string, fields []program.FieldInitializer) ([]wireFieldInitializer, error) {
	if fields == nil {
		return nil, nil
	}
	result := make([]wireFieldInitializer, len(fields))
	for i, field := range fields {
		itemPath := pathIndex(path, i)
		value, err := encodeExpression(pathField(itemPath, "value"), field.Value)
		if err != nil {
			return nil, err
		}
		result[i] = wireFieldInitializer{Name: field.Name, Value: value}
	}
	return result, nil
}

func decodeFieldInitializers(path string, fields []wireFieldInitializer) ([]program.FieldInitializer, error) {
	if fields == nil {
		return nil, nil
	}
	result := make([]program.FieldInitializer, len(fields))
	for i, field := range fields {
		itemPath := pathIndex(path, i)
		value, err := decodeExpression(pathField(itemPath, "value"), field.Value)
		if err != nil {
			return nil, err
		}
		result[i] = program.FieldInitializer{Name: field.Name, Value: value}
	}
	return result, nil
}

func encodeCallArguments(path string, arguments []program.CallArgument) ([]wireCallArgument, error) {
	if arguments == nil {
		return nil, nil
	}
	result := make([]wireCallArgument, len(arguments))
	for i, argument := range arguments {
		itemPath := pathIndex(path, i)
		value, err := encodeExpression(pathField(itemPath, "value"), argument.Value)
		if err != nil {
			return nil, err
		}
		result[i] = wireCallArgument{Name: argument.Name, Value: value}
	}
	return result, nil
}

func decodeCallArguments(path string, arguments []wireCallArgument) ([]program.CallArgument, error) {
	if arguments == nil {
		return nil, nil
	}
	result := make([]program.CallArgument, len(arguments))
	for i, argument := range arguments {
		itemPath := pathIndex(path, i)
		value, err := decodeExpression(pathField(itemPath, "value"), argument.Value)
		if err != nil {
			return nil, err
		}
		result[i] = program.CallArgument{Name: argument.Name, Value: value}
	}
	return result, nil
}

func encodeMatchExpressionCases(path string, cases []program.MatchExpressionCase) ([]wireMatchExpressionCase, error) {
	if cases == nil {
		return nil, nil
	}
	result := make([]wireMatchExpressionCase, len(cases))
	for i, c := range cases {
		itemPath := pathIndex(path, i)
		pattern, err := encodeMatchPattern(pathField(itemPath, "pattern"), c.Pattern)
		if err != nil {
			return nil, err
		}
		resultExpr, err := encodeExpression(pathField(itemPath, "result"), c.Result)
		if err != nil {
			return nil, err
		}
		result[i] = wireMatchExpressionCase{Pattern: pattern, Result: resultExpr}
	}
	return result, nil
}

func decodeMatchExpressionCases(path string, cases []wireMatchExpressionCase) ([]program.MatchExpressionCase, error) {
	if cases == nil {
		return nil, nil
	}
	result := make([]program.MatchExpressionCase, len(cases))
	for i, c := range cases {
		itemPath := pathIndex(path, i)
		pattern, err := decodeMatchPattern(pathField(itemPath, "pattern"), c.Pattern)
		if err != nil {
			return nil, err
		}
		resultExpr, err := decodeExpression(pathField(itemPath, "result"), c.Result)
		if err != nil {
			return nil, err
		}
		result[i] = program.MatchExpressionCase{Pattern: pattern, Result: resultExpr}
	}
	return result, nil
}

// encodeExpression encodes value as its JSON wire representation, or as
// JSON null when value is a nil interface or a typed nil pointer.
func encodeExpression(path string, value program.Expression) (json.RawMessage, error) {
	return encodeUnion(value, func(resolved any) (json.RawMessage, error) {
		switch v := resolved.(type) {
		case program.UnitLiteralExpression:
			return json.Marshal(wireUnitLiteralExpression{Kind: "unit_literal"})
		case program.BoolLiteralExpression:
			return json.Marshal(wireBoolLiteralExpression{Kind: "bool_literal", Value: v.Value})
		case program.NumberLiteralExpression:
			return json.Marshal(wireNumberLiteralExpression{Kind: "number_literal", Value: v.Value})
		case program.StringLiteralExpression:
			return json.Marshal(wireStringLiteralExpression{Kind: "string_literal", Value: v.Value})
		case program.OptionalNoneExpression:
			elementType, err := encodeTypeReference(pathField(path, "element_type"), v.ElementType)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireOptionalNoneExpression{Kind: "optional_none", ElementType: elementType})
		case program.OptionalSomeExpression:
			value, err := encodeExpression(pathField(path, "value"), v.Value)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireOptionalSomeExpression{Kind: "optional_some", Value: value})
		case program.ListExpression:
			elementType, err := encodeTypeReference(pathField(path, "element_type"), v.ElementType)
			if err != nil {
				return nil, err
			}
			elements, err := encodeExpressionSlice(pathField(path, "elements"), v.Elements)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireListExpression{Kind: "list", ElementType: elementType, Elements: elements})
		case program.MapExpression:
			keyType, err := encodeTypeReference(pathField(path, "key_type"), v.KeyType)
			if err != nil {
				return nil, err
			}
			valueType, err := encodeTypeReference(pathField(path, "value_type"), v.ValueType)
			if err != nil {
				return nil, err
			}
			entries, err := encodeMapEntries(pathField(path, "entries"), v.Entries)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireMapExpression{Kind: "map", KeyType: keyType, ValueType: valueType, Entries: entries})
		case program.EnumValueExpression:
			return json.Marshal(wireEnumValueExpression{Kind: "enum_value", TypeName: v.TypeName, ValueName: v.ValueName})
		case program.RecordExpression:
			fields, err := encodeFieldInitializers(pathField(path, "fields"), v.Fields)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireRecordExpression{Kind: "record", TypeName: v.TypeName, Fields: fields})
		case program.UnionExpression:
			fields, err := encodeFieldInitializers(pathField(path, "fields"), v.Fields)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireUnionExpression{Kind: "union", TypeName: v.TypeName, VariantName: v.VariantName, Fields: fields})
		case program.NewTypeExpression:
			value, err := encodeExpression(pathField(path, "value"), v.Value)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireNewTypeExpression{Kind: "new_type", TypeName: v.TypeName, Value: value})
		case program.ReferenceExpression:
			return json.Marshal(wireReferenceExpression{Kind: "reference", Name: v.Name})
		case program.FieldExpression:
			target, err := encodeExpression(pathField(path, "target"), v.Target)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireFieldExpression{Kind: "field", Target: target, Field: v.Field})
		case program.IndexExpression:
			target, err := encodeExpression(pathField(path, "target"), v.Target)
			if err != nil {
				return nil, err
			}
			index, err := encodeExpression(pathField(path, "index"), v.Index)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireIndexExpression{Kind: "index", Target: target, Index: index})
		case program.UnaryExpression:
			operand, err := encodeExpression(pathField(path, "operand"), v.Operand)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireUnaryExpression{Kind: "unary", Operator: v.Operator, Operand: operand})
		case program.BinaryExpression:
			left, err := encodeExpression(pathField(path, "left"), v.Left)
			if err != nil {
				return nil, err
			}
			right, err := encodeExpression(pathField(path, "right"), v.Right)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireBinaryExpression{Kind: "binary", Operator: v.Operator, Left: left, Right: right})
		case program.ConditionalExpression:
			condition, err := encodeExpression(pathField(path, "condition"), v.Condition)
			if err != nil {
				return nil, err
			}
			then, err := encodeExpression(pathField(path, "then"), v.Then)
			if err != nil {
				return nil, err
			}
			elseExpr, err := encodeExpression(pathField(path, "else"), v.Else)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireConditionalExpression{Kind: "conditional", Condition: condition, Then: then, Else: elseExpr})
		case program.CallExpression:
			arguments, err := encodeCallArguments(pathField(path, "arguments"), v.Arguments)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireCallExpression{Kind: "call", Function: v.Function, Arguments: arguments})
		case program.MatchExpression:
			matchValue, err := encodeExpression(pathField(path, "value"), v.Value)
			if err != nil {
				return nil, err
			}
			cases, err := encodeMatchExpressionCases(pathField(path, "cases"), v.Cases)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireMatchExpression{Kind: "match", Value: matchValue, Cases: cases})
		case program.ListMapExpression:
			collection, err := encodeExpression(pathField(path, "collection"), v.Collection)
			if err != nil {
				return nil, err
			}
			result, err := encodeExpression(pathField(path, "result"), v.Result)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireListMapExpression{Kind: "list_map", Collection: collection, ItemName: v.ItemName, IndexName: v.IndexName, Result: result})
		case program.ListFilterExpression:
			collection, err := encodeExpression(pathField(path, "collection"), v.Collection)
			if err != nil {
				return nil, err
			}
			predicate, err := encodeExpression(pathField(path, "predicate"), v.Predicate)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireListFilterExpression{Kind: "list_filter", Collection: collection, ItemName: v.ItemName, IndexName: v.IndexName, Predicate: predicate})
		case program.ListFlatMapExpression:
			collection, err := encodeExpression(pathField(path, "collection"), v.Collection)
			if err != nil {
				return nil, err
			}
			result, err := encodeExpression(pathField(path, "result"), v.Result)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireListFlatMapExpression{Kind: "list_flat_map", Collection: collection, ItemName: v.ItemName, IndexName: v.IndexName, Result: result})
		case program.ListAnyExpression:
			collection, err := encodeExpression(pathField(path, "collection"), v.Collection)
			if err != nil {
				return nil, err
			}
			predicate, err := encodeExpression(pathField(path, "predicate"), v.Predicate)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireListAnyExpression{Kind: "list_any", Collection: collection, ItemName: v.ItemName, IndexName: v.IndexName, Predicate: predicate})
		case program.ListAllExpression:
			collection, err := encodeExpression(pathField(path, "collection"), v.Collection)
			if err != nil {
				return nil, err
			}
			predicate, err := encodeExpression(pathField(path, "predicate"), v.Predicate)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireListAllExpression{Kind: "list_all", Collection: collection, ItemName: v.ItemName, IndexName: v.IndexName, Predicate: predicate})
		case program.ListCountExpression:
			collection, err := encodeExpression(pathField(path, "collection"), v.Collection)
			if err != nil {
				return nil, err
			}
			predicate, err := encodeExpression(pathField(path, "predicate"), v.Predicate)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireListCountExpression{Kind: "list_count", Collection: collection, ItemName: v.ItemName, IndexName: v.IndexName, Predicate: predicate})
		case program.ListFirstExpression:
			collection, err := encodeExpression(pathField(path, "collection"), v.Collection)
			if err != nil {
				return nil, err
			}
			predicate, err := encodeExpression(pathField(path, "predicate"), v.Predicate)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireListFirstExpression{Kind: "list_first", Collection: collection, ItemName: v.ItemName, IndexName: v.IndexName, Predicate: predicate})
		default:
			return nil, fmt.Errorf("%s: unsupported program.Expression implementation %T", path, value)
		}
	})
}

// decodeExpression decodes data as an program.Expression, or returns a nil
// interface for JSON null or a missing value.
func decodeExpression(path string, data json.RawMessage) (program.Expression, error) {
	return decodeUnion(path, data, func(path, kind string, raw json.RawMessage) (program.Expression, error) {
		switch kind {
		case "unit_literal":
			var wire wireUnitLiteralExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.UnitLiteralExpression{}, nil
		case "bool_literal":
			var wire wireBoolLiteralExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.BoolLiteralExpression{Value: wire.Value}, nil
		case "number_literal":
			var wire wireNumberLiteralExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.NumberLiteralExpression{Value: wire.Value}, nil
		case "string_literal":
			var wire wireStringLiteralExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.StringLiteralExpression{Value: wire.Value}, nil
		case "optional_none":
			var wire wireOptionalNoneExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			elementType, err := decodeTypeReference(pathField(path, "element_type"), wire.ElementType)
			if err != nil {
				return nil, err
			}
			return program.OptionalNoneExpression{ElementType: elementType}, nil
		case "optional_some":
			var wire wireOptionalSomeExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			value, err := decodeExpression(pathField(path, "value"), wire.Value)
			if err != nil {
				return nil, err
			}
			return program.OptionalSomeExpression{Value: value}, nil
		case "list":
			var wire wireListExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			elementType, err := decodeTypeReference(pathField(path, "element_type"), wire.ElementType)
			if err != nil {
				return nil, err
			}
			elements, err := decodeExpressionSlice(pathField(path, "elements"), wire.Elements)
			if err != nil {
				return nil, err
			}
			return program.ListExpression{ElementType: elementType, Elements: elements}, nil
		case "map":
			var wire wireMapExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			keyType, err := decodeTypeReference(pathField(path, "key_type"), wire.KeyType)
			if err != nil {
				return nil, err
			}
			valueType, err := decodeTypeReference(pathField(path, "value_type"), wire.ValueType)
			if err != nil {
				return nil, err
			}
			entries, err := decodeMapEntries(pathField(path, "entries"), wire.Entries)
			if err != nil {
				return nil, err
			}
			return program.MapExpression{KeyType: keyType, ValueType: valueType, Entries: entries}, nil
		case "enum_value":
			var wire wireEnumValueExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.EnumValueExpression{TypeName: wire.TypeName, ValueName: wire.ValueName}, nil
		case "record":
			var wire wireRecordExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			fields, err := decodeFieldInitializers(pathField(path, "fields"), wire.Fields)
			if err != nil {
				return nil, err
			}
			return program.RecordExpression{TypeName: wire.TypeName, Fields: fields}, nil
		case "union":
			var wire wireUnionExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			fields, err := decodeFieldInitializers(pathField(path, "fields"), wire.Fields)
			if err != nil {
				return nil, err
			}
			return program.UnionExpression{TypeName: wire.TypeName, VariantName: wire.VariantName, Fields: fields}, nil
		case "new_type":
			var wire wireNewTypeExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			value, err := decodeExpression(pathField(path, "value"), wire.Value)
			if err != nil {
				return nil, err
			}
			return program.NewTypeExpression{TypeName: wire.TypeName, Value: value}, nil
		case "reference":
			var wire wireReferenceExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.ReferenceExpression{Name: wire.Name}, nil
		case "field":
			var wire wireFieldExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			target, err := decodeExpression(pathField(path, "target"), wire.Target)
			if err != nil {
				return nil, err
			}
			return program.FieldExpression{Target: target, Field: wire.Field}, nil
		case "index":
			var wire wireIndexExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			target, err := decodeExpression(pathField(path, "target"), wire.Target)
			if err != nil {
				return nil, err
			}
			index, err := decodeExpression(pathField(path, "index"), wire.Index)
			if err != nil {
				return nil, err
			}
			return program.IndexExpression{Target: target, Index: index}, nil
		case "unary":
			var wire wireUnaryExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			operand, err := decodeExpression(pathField(path, "operand"), wire.Operand)
			if err != nil {
				return nil, err
			}
			return program.UnaryExpression{Operator: wire.Operator, Operand: operand}, nil
		case "binary":
			var wire wireBinaryExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			left, err := decodeExpression(pathField(path, "left"), wire.Left)
			if err != nil {
				return nil, err
			}
			right, err := decodeExpression(pathField(path, "right"), wire.Right)
			if err != nil {
				return nil, err
			}
			return program.BinaryExpression{Operator: wire.Operator, Left: left, Right: right}, nil
		case "conditional":
			var wire wireConditionalExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			condition, err := decodeExpression(pathField(path, "condition"), wire.Condition)
			if err != nil {
				return nil, err
			}
			then, err := decodeExpression(pathField(path, "then"), wire.Then)
			if err != nil {
				return nil, err
			}
			elseExpr, err := decodeExpression(pathField(path, "else"), wire.Else)
			if err != nil {
				return nil, err
			}
			return program.ConditionalExpression{Condition: condition, Then: then, Else: elseExpr}, nil
		case "call":
			var wire wireCallExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			arguments, err := decodeCallArguments(pathField(path, "arguments"), wire.Arguments)
			if err != nil {
				return nil, err
			}
			return program.CallExpression{Function: wire.Function, Arguments: arguments}, nil
		case "match":
			var wire wireMatchExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			matchValue, err := decodeExpression(pathField(path, "value"), wire.Value)
			if err != nil {
				return nil, err
			}
			cases, err := decodeMatchExpressionCases(pathField(path, "cases"), wire.Cases)
			if err != nil {
				return nil, err
			}
			return program.MatchExpression{Value: matchValue, Cases: cases}, nil
		case "list_map":
			var wire wireListMapExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			collection, err := decodeExpression(pathField(path, "collection"), wire.Collection)
			if err != nil {
				return nil, err
			}
			result, err := decodeExpression(pathField(path, "result"), wire.Result)
			if err != nil {
				return nil, err
			}
			return program.ListMapExpression{Collection: collection, ItemName: wire.ItemName, IndexName: wire.IndexName, Result: result}, nil
		case "list_filter":
			var wire wireListFilterExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			collection, err := decodeExpression(pathField(path, "collection"), wire.Collection)
			if err != nil {
				return nil, err
			}
			predicate, err := decodeExpression(pathField(path, "predicate"), wire.Predicate)
			if err != nil {
				return nil, err
			}
			return program.ListFilterExpression{Collection: collection, ItemName: wire.ItemName, IndexName: wire.IndexName, Predicate: predicate}, nil
		case "list_flat_map":
			var wire wireListFlatMapExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			collection, err := decodeExpression(pathField(path, "collection"), wire.Collection)
			if err != nil {
				return nil, err
			}
			result, err := decodeExpression(pathField(path, "result"), wire.Result)
			if err != nil {
				return nil, err
			}
			return program.ListFlatMapExpression{Collection: collection, ItemName: wire.ItemName, IndexName: wire.IndexName, Result: result}, nil
		case "list_any":
			var wire wireListAnyExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			collection, err := decodeExpression(pathField(path, "collection"), wire.Collection)
			if err != nil {
				return nil, err
			}
			predicate, err := decodeExpression(pathField(path, "predicate"), wire.Predicate)
			if err != nil {
				return nil, err
			}
			return program.ListAnyExpression{Collection: collection, ItemName: wire.ItemName, IndexName: wire.IndexName, Predicate: predicate}, nil
		case "list_all":
			var wire wireListAllExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			collection, err := decodeExpression(pathField(path, "collection"), wire.Collection)
			if err != nil {
				return nil, err
			}
			predicate, err := decodeExpression(pathField(path, "predicate"), wire.Predicate)
			if err != nil {
				return nil, err
			}
			return program.ListAllExpression{Collection: collection, ItemName: wire.ItemName, IndexName: wire.IndexName, Predicate: predicate}, nil
		case "list_count":
			var wire wireListCountExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			collection, err := decodeExpression(pathField(path, "collection"), wire.Collection)
			if err != nil {
				return nil, err
			}
			predicate, err := decodeExpression(pathField(path, "predicate"), wire.Predicate)
			if err != nil {
				return nil, err
			}
			return program.ListCountExpression{Collection: collection, ItemName: wire.ItemName, IndexName: wire.IndexName, Predicate: predicate}, nil
		case "list_first":
			var wire wireListFirstExpression
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			collection, err := decodeExpression(pathField(path, "collection"), wire.Collection)
			if err != nil {
				return nil, err
			}
			predicate, err := decodeExpression(pathField(path, "predicate"), wire.Predicate)
			if err != nil {
				return nil, err
			}
			return program.ListFirstExpression{Collection: collection, ItemName: wire.ItemName, IndexName: wire.IndexName, Predicate: predicate}, nil
		default:
			return nil, newDecodeError(path, fmt.Sprintf("unsupported expression kind %q", kind), nil)
		}
	})
}
