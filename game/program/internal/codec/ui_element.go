package codec

import "github.com/diegobermudez03/playhoot/game/program"

import (
	"encoding/json"
	"fmt"
)

type wireEmptyElement struct {
	Kind string `json:"kind"`
}

type wireContainerElement struct {
	Kind          string            `json:"kind"`
	Configuration json.RawMessage   `json:"configuration"`
	Layout        json.RawMessage   `json:"layout"`
	Children      []json.RawMessage `json:"children"`
}

type wireTextElement struct {
	Kind          string          `json:"kind"`
	Configuration json.RawMessage `json:"configuration"`
	Value         json.RawMessage `json:"value"`
}

type wireImageElement struct {
	Kind            string          `json:"kind"`
	Configuration   json.RawMessage `json:"configuration"`
	Source          json.RawMessage `json:"source"`
	AlternativeText json.RawMessage `json:"alternative_text"`
}

type wireButtonElement struct {
	Kind          string            `json:"kind"`
	Configuration json.RawMessage   `json:"configuration"`
	Children      []json.RawMessage `json:"children"`
}

type wireRepeatElement struct {
	Kind       string          `json:"kind"`
	Collection json.RawMessage `json:"collection"`
	ItemName   string          `json:"item_name"`
	IndexName  string          `json:"index_name"`
	Key        json.RawMessage `json:"key"`
	Body       json.RawMessage `json:"body"`
}

type wireConditionalElement struct {
	Kind      string          `json:"kind"`
	Condition json.RawMessage `json:"condition"`
	Then      json.RawMessage `json:"then"`
	Else      json.RawMessage `json:"else"`
}

func encodeUIElementSlice(path string, items []program.UIElement) ([]json.RawMessage, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]json.RawMessage, len(items))
	for i, item := range items {
		raw, err := encodeUIElement(pathIndex(path, i), item)
		if err != nil {
			return nil, err
		}
		result[i] = raw
	}
	return result, nil
}

func decodeUIElementSlice(path string, items []json.RawMessage) ([]program.UIElement, error) {
	if items == nil {
		return nil, nil
	}
	result := make([]program.UIElement, len(items))
	for i, raw := range items {
		item, err := decodeUIElement(pathIndex(path, i), raw)
		if err != nil {
			return nil, err
		}
		result[i] = item
	}
	return result, nil
}

// encodeUIElement encodes value as its JSON wire representation, or as
// JSON null when value is a nil interface or a typed nil pointer.
func encodeUIElement(path string, value program.UIElement) (json.RawMessage, error) {
	return encodeUnion(value, func(resolved any) (json.RawMessage, error) {
		switch v := resolved.(type) {
		case program.EmptyElement:
			return json.Marshal(wireEmptyElement{Kind: "empty"})
		case program.ContainerElement:
			configuration, err := encodeUIElementConfiguration(pathField(path, "configuration"), v.Configuration)
			if err != nil {
				return nil, err
			}
			layout, err := encodeUILayout(pathField(path, "layout"), v.Layout)
			if err != nil {
				return nil, err
			}
			children, err := encodeUIElementSlice(pathField(path, "children"), v.Children)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireContainerElement{Kind: "container", Configuration: configuration, Layout: layout, Children: children})
		case program.TextElement:
			configuration, err := encodeUIElementConfiguration(pathField(path, "configuration"), v.Configuration)
			if err != nil {
				return nil, err
			}
			value, err := encodeExpression(pathField(path, "value"), v.Value)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireTextElement{Kind: "text", Configuration: configuration, Value: value})
		case program.ImageElement:
			configuration, err := encodeUIElementConfiguration(pathField(path, "configuration"), v.Configuration)
			if err != nil {
				return nil, err
			}
			source, err := encodeExpression(pathField(path, "source"), v.Source)
			if err != nil {
				return nil, err
			}
			alternativeText, err := encodeExpression(pathField(path, "alternative_text"), v.AlternativeText)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireImageElement{Kind: "image", Configuration: configuration, Source: source, AlternativeText: alternativeText})
		case program.ButtonElement:
			configuration, err := encodeUIElementConfiguration(pathField(path, "configuration"), v.Configuration)
			if err != nil {
				return nil, err
			}
			children, err := encodeUIElementSlice(pathField(path, "children"), v.Children)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireButtonElement{Kind: "button", Configuration: configuration, Children: children})
		case program.RepeatElement:
			collection, err := encodeExpression(pathField(path, "collection"), v.Collection)
			if err != nil {
				return nil, err
			}
			key, err := encodeExpression(pathField(path, "key"), v.Key)
			if err != nil {
				return nil, err
			}
			body, err := encodeUIElement(pathField(path, "body"), v.Body)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireRepeatElement{Kind: "repeat", Collection: collection, ItemName: v.ItemName, IndexName: v.IndexName, Key: key, Body: body})
		case program.ConditionalElement:
			condition, err := encodeExpression(pathField(path, "condition"), v.Condition)
			if err != nil {
				return nil, err
			}
			then, err := encodeUIElement(pathField(path, "then"), v.Then)
			if err != nil {
				return nil, err
			}
			elseElement, err := encodeUIElement(pathField(path, "else"), v.Else)
			if err != nil {
				return nil, err
			}
			return json.Marshal(wireConditionalElement{Kind: "conditional", Condition: condition, Then: then, Else: elseElement})
		default:
			return nil, fmt.Errorf("%s: unsupported program.UIElement implementation %T", path, value)
		}
	})
}

// decodeUIElement decodes data as a program.UIElement, or returns a nil interface
// for JSON null or a missing value.
func decodeUIElement(path string, data json.RawMessage) (program.UIElement, error) {
	return decodeUnion(path, data, func(path, kind string, raw json.RawMessage) (program.UIElement, error) {
		switch kind {
		case "empty":
			var wire wireEmptyElement
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			return program.EmptyElement{}, nil
		case "container":
			var wire wireContainerElement
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			configuration, err := decodeUIElementConfiguration(pathField(path, "configuration"), wire.Configuration)
			if err != nil {
				return nil, err
			}
			layout, err := decodeUILayout(pathField(path, "layout"), wire.Layout)
			if err != nil {
				return nil, err
			}
			children, err := decodeUIElementSlice(pathField(path, "children"), wire.Children)
			if err != nil {
				return nil, err
			}
			return program.ContainerElement{Configuration: configuration, Layout: layout, Children: children}, nil
		case "text":
			var wire wireTextElement
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			configuration, err := decodeUIElementConfiguration(pathField(path, "configuration"), wire.Configuration)
			if err != nil {
				return nil, err
			}
			value, err := decodeExpression(pathField(path, "value"), wire.Value)
			if err != nil {
				return nil, err
			}
			return program.TextElement{Configuration: configuration, Value: value}, nil
		case "image":
			var wire wireImageElement
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			configuration, err := decodeUIElementConfiguration(pathField(path, "configuration"), wire.Configuration)
			if err != nil {
				return nil, err
			}
			source, err := decodeExpression(pathField(path, "source"), wire.Source)
			if err != nil {
				return nil, err
			}
			alternativeText, err := decodeExpression(pathField(path, "alternative_text"), wire.AlternativeText)
			if err != nil {
				return nil, err
			}
			return program.ImageElement{Configuration: configuration, Source: source, AlternativeText: alternativeText}, nil
		case "button":
			var wire wireButtonElement
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			configuration, err := decodeUIElementConfiguration(pathField(path, "configuration"), wire.Configuration)
			if err != nil {
				return nil, err
			}
			children, err := decodeUIElementSlice(pathField(path, "children"), wire.Children)
			if err != nil {
				return nil, err
			}
			return program.ButtonElement{Configuration: configuration, Children: children}, nil
		case "repeat":
			var wire wireRepeatElement
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			collection, err := decodeExpression(pathField(path, "collection"), wire.Collection)
			if err != nil {
				return nil, err
			}
			key, err := decodeExpression(pathField(path, "key"), wire.Key)
			if err != nil {
				return nil, err
			}
			body, err := decodeUIElement(pathField(path, "body"), wire.Body)
			if err != nil {
				return nil, err
			}
			return program.RepeatElement{Collection: collection, ItemName: wire.ItemName, IndexName: wire.IndexName, Key: key, Body: body}, nil
		case "conditional":
			var wire wireConditionalElement
			if err := strictDecodeInto(path, raw, &wire); err != nil {
				return nil, err
			}
			condition, err := decodeExpression(pathField(path, "condition"), wire.Condition)
			if err != nil {
				return nil, err
			}
			then, err := decodeUIElement(pathField(path, "then"), wire.Then)
			if err != nil {
				return nil, err
			}
			elseElement, err := decodeUIElement(pathField(path, "else"), wire.Else)
			if err != nil {
				return nil, err
			}
			return program.ConditionalElement{Condition: condition, Then: then, Else: elseElement}, nil
		default:
			return nil, newDecodeError(path, fmt.Sprintf("unsupported UI element kind %q", kind), nil)
		}
	})
}
