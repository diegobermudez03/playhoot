package definition

import (
	"fmt"
)

// defines state keywords
const (
	state     string = "state"
	resources string = "resources"
	players   string = "players"
)

func (g *Game) Validate() error {
	// I think  I gotta validate only resources to confirm that for lists is the same data type for all items
	r := g.Resources.Fields

	state := map[string]ValueType{
		state:     g.RuntimeStateSchema,
		resources: g.Resources,
		players:   g.PlayersState,
	}
	statusesMap := make(map[string]Status, len(g.Statuses))
	for _, s := range g.Statuses {
		statusesMap[s.Name] = s
	}

	return g.validateStatus(statusesMap, g.InitialStatus, state)
}

func (g *Game) validateStatus(statusesMap map[string]Status, statusName string, state map[string]ValueType) error {
	s, ok := statusesMap[statusName]
	if !ok {
		return fmt.Errorf("non existent status %s", statusName)
	}

	if err := g.validateOperation(s.Op, map[string]ValueType{}); err != nil {
		return err
	}

	return nil
}

func (g *Game) validateOperation(o Operation, state map[string]ValueType) error {
	resources := g.Resources.Fields
	PerPlayerSchema := g.PerPlayerStateSchema.Fields
	State := g.RuntimeStateSchema.Fields

	switch o.GetOperationType() {
	case forEachOp:
		return g.forEachOpValidation(o.(ForEachOp), state)
	}
	return nil
}

func (g *Game) forEachOpValidation(op ForEachOp, state map[string]ValueType) error {
	expList, err := g.getRefValueType(op.List, state)
	if err != nil {
		return err
	}

	if expList.GetDataType() != listType {
		return fmt.Errorf("expected list type, got %s", expList.GetDataType())
	}

	list := expList.(ListType)
	state[op.ItemName] = list.Type
	// gotta delete when getting out of the scope
	defer func() {
		delete(state, op.ItemName)
	}()

	return g.validateOperation(op.IterationOp, state)
}

func (g *Game) getRefValueType(ref RefType, state map[string]ValueType) (ValueType, error) {
	var referenced ValueType = ObjectType{Fields: state}
	for _, key := range ref.VariableComposition {
		obj, ok := referenced.(ObjectType)
		if !ok {
			return nil, fmt.Errorf("key %s is not an object", key)
		}

		found, ok := obj.Fields[key]
		if !ok {
			return nil, fmt.Errorf("referenced not found, key %s not found in scope", key)
		}

		referenced = found
	}

	return referenced, nil
}
