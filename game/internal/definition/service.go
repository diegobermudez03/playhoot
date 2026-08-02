package definition

import (
	"fmt"
	"strings"
)

// defines state keywords
const (
	state     string = "state"
	resources string = "resources"
	players   string = "players"
)

type ValidationResults struct {
	results []ValidationResult
}

func (r ValidationResults) Add(v ...ValidationResult) ValidationResults {
	r.results = append(r.results, v)
	return r
}

type ValidationResult interface {
}

type StatusValidationResult struct {
	statusName string
	problem    string
}

type Scope struct {
	// variable names defined at the scope
	variables []string
	// if scope is due to an iteration then this references the iterating variable
	// it can be empty (length == 0) if scope is a scope variable creation
	iteratingVariable variableComposition
}

func (g *Game) Validate() ValidationResult {
	stateSchema := map[string]ValueType{
		state:     g.State,
		resources: g.Resources,
		players:   g.PlayersState,
	}

	statusesMap := make(map[string]Status, len(g.Statuses))
	for _, s := range g.Statuses {
		statusesMap[s.Name] = s
	}

	usedVariables, result := g.validateStatus(statusesMap, g.InitialStatus, stateSchema)

	unusedVariables := findUnusedVariables("", state, usedVariables)
	if len(unusedVariables) > 0 {
		return fmt.Errorf("unused variables found: %v", unusedVariables)
	}

	return nil
}

func findUnusedVariables(prefix string, obj map[string]ValueType, usedVariables map[string]bool) map[string]bool {
	unusedVariables := map[string]bool{}
	for name, val := range obj {
		varName := fmt.Sprintf("%s.%s", prefix, name)
		if val.GetDataType() != objType {
			_, ok := usedVariables[varName]
			if !ok {
				unusedVariables[varName] = true
			}
		} else {
			objVal := val.(ObjectType)
			subUnusedVariables := findUnusedVariables(varName, objVal.Fields, usedVariables)
			for key := range subUnusedVariables {
				unusedVariables[key] = true
			}
		}
	}

	return unusedVariables
}

// returns map of used variables (set) and validation results
func (g *Game) validateStatus(statusesMap map[string]Status, statusName string, stateSchema map[string]ValueType) (map[string]bool, ValidationResults) {
	results := ValidationResults{}
	s, ok := statusesMap[statusName]
	if !ok {
		return nil, results.Add(StatusValidationResult{statusName: statusName, problem: "not found"})
	}

	for _, op := range s.Operations {
		usedVariables, opValidationResult := validateOperation(op, stateSchema, []string{})
		results.Add()
	}

	// CREATE PLAYER REF DATA TYPE AND WORKAROUN THE PLAYERS SLICE

	// validate interactioners

	// validate next statuses

	return nil
}

func validateOperation(o Operation, stateSchema map[string]ValueType, scopeStack []string) (usedVariables map[string]bool, res ValidationResult) {
	switch o.GetOperationType() {
	case forEachOp:
		return forEachOpValidation(o.(ForEachOp), state, listsIterations, usedVariables)
	case ifConditionOp:
		return ifConditionValidation(o.(IfConditionOp), state, listsIterations, usedVariables)
	case scopeVariableCreationOp:
		return scopeVariableValidation(o.(ScopeVariableCreationOp), state, listsIterations, usedVariables)
	case assignmentOp:
		return assignmentOpValidation(o.(AssignmentOp), state, listsIterations, usedVariables)
	}
	return nil
}

func forEachOpValidation(op ForEachOp, state map[string]ValueType, listsIterations map[string]struct{}, usedVariables map[string]bool) error {
	expList, err := getRefValueType(op.List, state, usedVariables)
	if err != nil {
		return err
	}

	if expList.GetDataType() != listType {
		return fmt.Errorf("expected list type, got %s", expList.GetDataType())
	}

	list := expList.(ListType)
	state[op.ItemName] = list.Type
	listsIterations[getCompositeVariable(op.List.VariableComposition)] = struct{}{}
	// gotta delete when getting out of the scope
	defer func() {
		delete(state, op.ItemName)
		delete(listsIterations, getCompositeVariable(op.List.VariableComposition))
	}()

	for _, op := range op.IterationOps {
		if err := validateOperation(op, state, listsIterations, usedVariables); err != nil {
			return err
		}
	}

	return nil
}

func ifConditionValidation(op IfConditionOp, state map[string]ValueType, listsIterations map[string]struct{}, usedVariables map[string]bool) error {
	for _, exp := range op.BoolExpressions {
		if err := validateBoolExpression(exp, state, listsIterations, usedVariables); err != nil {
			return err
		}
	}

	for _, conn := range op.Connectors {
		if err := validateBoolConnector(conn); err != nil {
			return err
		}
	}

	if len(op.Connectors) != len(op.BoolExpressions)-1 {
		return fmt.Errorf("wrong number of bool connectors %d for %d number of expressions", len(op.Connectors), len(op.BoolExpressions))
	}

	for _, op := range op.IfTrue {
		if err := validateOperation(op, state, listsIterations, usedVariables); err != nil {
			return err
		}
	}

	for _, op := range op.IfFalse {
		if err := validateOperation(op, state, listsIterations, usedVariables); err != nil {
			return err
		}
	}

	return nil
}

func scopeVariableValidation(op ScopeVariableCreationOp, state map[string]ValueType, listsIterations map[string]struct{}, usedVariables map[string]bool) error {
	state[op.VariableName] = op.Value
	// to remove variable outside the scope
	defer func() {
		delete(state, op.VariableName)
	}()

	for _, op := range op.Ops {
		if err := validateOperation(op, state, listsIterations, usedVariables); err != nil {
			return err
		}
	}

	return nil
}

func assignmentOpValidation(op AssignmentOp, state map[string]ValueType, listsIterations map[string]struct{}, usedVariables map[string]bool) error {
	val, err := getRefValueType(op.Field, state, usedVariables)
	if err != nil {
		return err
	}

	if isResourcesField(op.Field) {
		return fmt.Errorf("can't mutate resources field: %s", getCompositeVariable(op.Field.VariableComposition))
	}

	if op.Value.GetExpressionDataType() != val.GetDataType() {
		return fmt.Errorf("expression has different data type %s from ref type %s", op.Value.GetExpressionDataType(), val.GetDataType())
	}

	return validateExpression(op.Value, state, listsIterations, usedVariables)
}

func validateExpression(exp Expression, state map[string]ValueType, listsIterations map[string]struct{}, usedVariables map[string]bool) error {
	switch exp.GetExpressionDataType() {
	case stringType:
		return validateStringExpression(exp.(StringExpression), state, listsIterations, usedVariables)
	case numericType:
		return validateNumericExpression(exp.(NumericExpression), state, listsIterations, usedVariables)
	case boolType:
		return validateBoolExpression(exp.(BoolExpression), state, listsIterations, usedVariables)
	case listType:
		return validateListExpression(exp.(ListExpression), state, listsIterations, usedVariables)
	}
	return nil
}

func validateStringExpression(e StringExpression, state map[string]ValueType, listsIterations map[string]struct{}, usedVariables map[string]bool) error {
	val1, err := getRefValueType(e.Value1, state, usedVariables)
	if err != nil {
		return err
	}

	val2, err := getRefValueType(e.Value2, state, usedVariables)
	if err != nil {
		return err
	}

	if val1.GetDataType() != val2.GetDataType() {
		return fmt.Errorf("comparing different data types %s and %s", val1.GetDataType(), val2.GetDataType())
	}

	if val1.GetDataType() != stringType {
		return fmt.Errorf("attempted string expression on non string variables %s", val1.GetDataType())
	}

	switch e.operation {
	case joinWithOp:
		return nil
	}

	return fmt.Errorf("invalid string operation %s", e.operation)
}

func validateNumericExpression(e NumericExpression, state map[string]ValueType, listsIterations map[string]struct{}, usedVariables map[string]bool) error {
	val1, err := getRefValueType(e.Value1, state, usedVariables)
	if err != nil {
		return err
	}

	val2, err := getRefValueType(e.Value2, state, usedVariables)
	if err != nil {
		return err
	}

	if val1.GetDataType() != val2.GetDataType() {
		return fmt.Errorf("comparing different data types %s and %s", val1.GetDataType(), val2.GetDataType())
	}

	if val1.GetDataType() != numericType {
		return fmt.Errorf("attempted numeric expression on non numeric variables %s", val1.GetDataType())
	}

	switch e.operation {
	case plusOp, minusOp, timesOp, divOp:
		return nil
	}

	return fmt.Errorf("invalid numeric operation %s", e.operation)
}

func validateBoolExpression(e BoolExpression, state map[string]ValueType, listsIterations map[string]struct{}, usedVariables map[string]bool) error {
	val1, err := getRefValueType(e.Value1, state, usedVariables)
	if err != nil {
		return err
	}

	val2, err := getRefValueType(e.Value2, state, usedVariables)
	if err != nil {
		return err
	}

	if val1.GetDataType() != val2.GetDataType() {
		return fmt.Errorf("comparing different data types %s and %s", val1.GetDataType(), val2.GetDataType())
	}

	switch e.Operation {
	case greaterOp, lowerOp:
		if val1.GetDataType() != numericType {
			return fmt.Errorf("can't apply operation %s in non numeric types", string(e.Operation))
		}
	case equalOp:
		if val1.GetDataType() != numericType && val1.GetDataType() != stringType {
			return fmt.Errorf("cant apply operation %s in non numeric or strings types", string(e.Operation))
		}
	}

	return nil
}

func validateListExpression(e ListExpression, state map[string]ValueType, listsIterations map[string]struct{}, usedVariables map[string]bool) error {
	list, err := getRefValueType(e.ListRef, state, usedVariables)
	if err != nil {
		return err
	}

	if list.GetDataType() != listType {
		return fmt.Errorf("attempted list expression on non list variables %s", list.GetDataType())
	}

	if isResourcesField(e.ListRef) {
		return fmt.Errorf("can't mutate resources list %s", getCompositeVariable(e.ListRef.VariableComposition))
	}

	if e.Operation == removeOp {
		if _, ok := listsIterations[getCompositeVariable(e.ListRef.VariableComposition)]; !ok {
			return fmt.Errorf("received list remove operations for non iterating list %s", getCompositeVariable(e.ListRef.VariableComposition))
		}
	} else if e.Operation == addOp {
		if e.Value == nil {
			return fmt.Errorf("attempted adding into list a nil value")
		}

		if e.Value.GetDataType() != list.GetDataType() {
			return fmt.Errorf("attempted to add field of data type %s into list of data type %s", e.Value.GetDataType(), list.GetDataType())
		}
	}

	return nil
}

func validateBoolConnector(conn boolConnector) error {
	switch conn {
	case andConnector, orConnector:
		return nil
	}

	return fmt.Errorf("invalid bool connector %s", string(andConnector))
}

func getRefValueType(ref RefType, state map[string]ValueType, usedVariables map[string]bool) (ValueType, error) {
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

	usedVariables[getCompositeVariable(ref.VariableComposition)] = true

	return referenced, nil
}

func getCompositeVariable(variables []string) string {
	return strings.Join(variables, ".")
}

func isResourcesField(variable RefType) bool {
	return variable.VariableComposition[0] == resources
}
