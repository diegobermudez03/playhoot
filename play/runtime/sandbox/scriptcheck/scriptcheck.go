// Package scriptcheck statically validates a game's backend script, under its
// version's Profile, before the version is published: all persistent gameplay
// state must live in what the entrypoints return, never in module or global
// state. It is conservative: code it cannot prove harmless is rejected.
package scriptcheck

import (
	"fmt"
	"sort"

	"github.com/dop251/goja/ast"
	"github.com/dop251/goja/parser"
	"github.com/dop251/goja/token"
)

// Rules a Violation can break.
const (
	RuleSyntax       = "syntax"
	RuleEntrypoint   = "entrypoint"
	RuleModuleState  = "module_state"
	RuleGlobalAccess = "global_access"
	RuleMutation     = "mutation"
	RuleAsync        = "async"
	RuleThis         = "this"
	RuleUnsupported  = "unsupported_syntax"
)

// Violation is one place a script breaks a rule.
type Violation struct {
	Line    int
	Column  int
	Rule    string
	Message string
}

// Profile is the set of rules a game version's script is checked under; the
// version records it and the sandbox runs the script accordingly.
type Profile string

const (
	// ProfileHandwritten is a script written directly against the sandbox
	// ABI, which must not mutate its inputs.
	ProfileHandwritten Profile = "handwritten"
	// ProfileSDKV1 is a script built by the frontend's game tools from an SDK
	// project: it may change the fresh JSON each call gets, and its top-level
	// constants may be literal objects and SDK definitions, frozen at load.
	ProfileSDKV1 Profile = "sdk-v1"
)

// Valid reports whether p is a known profile.
func (p Profile) Valid() bool {
	return p == ProfileHandwritten || p == ProfileSDKV1
}

// ModuleValuesFunction is the top-level function an sdk-v1 script declares
// to list its non-primitive top-level constants, which the sandbox freezes
// once the script loads.
const ModuleValuesFunction = "__playhoot_module_values"

// entrypoints must be top-level function declarations.
var entrypoints = []string{"start", "project"}

// definitionFunctions are the SDK calls an sdk-v1 top-level constant may
// hold, all pure: they return their argument or build a plain object of it.
var definitionFunctions = map[string]bool{"defineGame": true, "defineCommands": true, "defineInputs": true}

// forbiddenGlobals reach the global object or build code at runtime.
var forbiddenGlobals = map[string]bool{
	"globalThis": true, "eval": true, "Function": true, "Reflect": true, "WebAssembly": true,
}

// forbiddenProperties reach constructors or prototypes, through which
// builtins could be changed or code built at runtime.
var forbiddenProperties = map[string]bool{
	"constructor": true, "__proto__": true, "prototype": true,
	"__defineGetter__": true, "__defineSetter__": true, "__lookupGetter__": true, "__lookupSetter__": true,
}

// mutatingMethods change the receiver they are called on.
var mutatingMethods = map[string]bool{
	"push": true, "pop": true, "shift": true, "unshift": true, "splice": true, "sort": true,
	"reverse": true, "fill": true, "copyWithin": true, "set": true, "add": true, "delete": true, "clear": true,
}

// mutatingObjectFunctions change their first argument.
var mutatingObjectFunctions = map[string]bool{
	"assign": true, "defineProperty": true, "defineProperties": true, "setPrototypeOf": true,
}

// Validate returns every violation in source under profile, sorted by
// position; none means it may be published.
func Validate(profile Profile, source []byte) []Violation {
	if !profile.Valid() {
		return []Violation{{Line: 1, Column: 1, Rule: RuleUnsupported, Message: fmt.Sprintf("unknown script profile %q", profile)}}
	}
	program, err := parser.ParseFile(nil, "script.js", string(source), 0)
	if err != nil {
		return []Violation{{Line: 1, Column: 1, Rule: RuleSyntax, Message: err.Error()}}
	}
	c := &checker{program: program, profile: profile, scopes: make(map[ast.Node]*scope)}
	c.run()
	sort.SliceStable(c.violations, func(i, j int) bool {
		if c.violations[i].Line != c.violations[j].Line {
			return c.violations[i].Line < c.violations[j].Line
		}
		return c.violations[i].Column < c.violations[j].Column
	})
	return c.violations
}

// binding is one declared name. A tainted binding may be a value given to
// authored code (a parameter) or something reachable from one; a binding that
// holds is a value the function created that stores such values inside.
type binding struct {
	topLevel bool
	tainted  bool
	holds    bool
}

type scope struct {
	parent *scope
	names  map[string]*binding
}

func (s *scope) declare(name string, b *binding) {
	if _, found := s.names[name]; !found {
		s.names[name] = b
	}
}

// resolve returns name's binding, or nil when it is not declared, that is a
// builtin or an undeclared global.
func (s *scope) resolve(name string) *binding {
	for current := s; current != nil; current = current.parent {
		if b, found := current.names[name]; found {
			return b
		}
	}
	return nil
}

type checker struct {
	program    *ast.Program
	profile    Profile
	scopes     map[ast.Node]*scope
	checking   bool
	changed    bool
	violations []Violation
	reported   map[string]bool
}

// run computes taint to a fixed point, flow-insensitively, then reports.
func (c *checker) run() {
	top := &scope{names: make(map[string]*binding)}
	c.scopes[c.program] = top
	c.checkTopLevel(top)
	for {
		c.changed = false
		c.statements(c.program.Body, top)
		if !c.changed {
			break
		}
	}
	c.checking = true
	c.reported = make(map[string]bool)
	c.statements(c.program.Body, top)
}

// checkTopLevel declares the top-level bindings and enforces that only
// functions and constants of immutable values live there.
func (c *checker) checkTopLevel(top *scope) {
	declared := make(map[string]bool)
	// frozen are the sdk-v1 constants the sandbox must freeze at load.
	var frozen []string
	for _, statement := range c.program.Body {
		switch s := statement.(type) {
		case *ast.FunctionDeclaration:
			if s.Function.Name != nil {
				top.declare(string(s.Function.Name.Name), &binding{topLevel: true})
				declared[string(s.Function.Name.Name)] = true
			}
		case *ast.LexicalDeclaration:
			if s.Token != token.CONST {
				c.report(s, RuleModuleState, "top-level state must be a const of an immutable value; let is mutable module state")
			}
			for _, b := range s.List {
				identifier, ok := b.Target.(*ast.Identifier)
				if !ok {
					c.report(b.Target, RuleModuleState, "top-level destructuring is not allowed")
					continue
				}
				top.declare(string(identifier.Name), &binding{topLevel: true})
				switch {
				case primitiveValue(b.Initializer):
				case c.profile == ProfileSDKV1 && sdkConstant(b.Initializer):
					frozen = append(frozen, string(identifier.Name))
				case c.profile == ProfileSDKV1:
					c.report(b.Initializer, RuleModuleState, fmt.Sprintf("top-level const %s must hold a literal, an object or array of them, a function or an SDK definition; computed values are module state", identifier.Name))
				case !immutableValue(b.Initializer):
					c.report(b.Initializer, RuleModuleState, fmt.Sprintf("top-level const %s must hold a primitive or a function; objects, arrays and computed values are mutable module state", identifier.Name))
				}
			}
		case *ast.ExpressionStatement:
			if literal, ok := s.Expression.(*ast.StringLiteral); ok && literal.Value == "use strict" {
				continue
			}
			c.report(s, RuleModuleState, "top-level code may only declare functions and constants")
		case *ast.VariableStatement:
			c.report(s, RuleModuleState, "top-level var is mutable module state")
		case *ast.ClassDeclaration:
			c.report(s, RuleModuleState, "top-level classes are not allowed")
		case *ast.EmptyStatement:
		default:
			c.report(statement, RuleModuleState, "top-level code may only declare functions and constants")
		}
	}
	required := entrypoints
	if c.profile == ProfileSDKV1 {
		required = append(append([]string(nil), entrypoints...), ModuleValuesFunction)
	}
	for _, name := range required {
		if !declared[name] {
			c.violations = append(c.violations, Violation{Line: 1, Column: 1, Rule: RuleEntrypoint, Message: fmt.Sprintf("the script must declare a top-level function %s", name)})
		}
	}
	if c.profile == ProfileSDKV1 && declared[ModuleValuesFunction] {
		c.checkModuleValues(frozen)
	}
}

// checkModuleValues enforces that __playhoot_module_values returns exactly
// the sdk-v1 constants that are not primitives, so the sandbox freezes them
// all.
func (c *checker) checkModuleValues(frozen []string) {
	for _, statement := range c.program.Body {
		declaration, ok := statement.(*ast.FunctionDeclaration)
		if !ok || declaration.Function.Name == nil || string(declaration.Function.Name.Name) != ModuleValuesFunction {
			continue
		}
		listed, ok := moduleValuesList(declaration.Function)
		expected := make(map[string]bool, len(frozen))
		for _, name := range frozen {
			expected[name] = true
		}
		if ok && len(listed) == len(expected) {
			for _, name := range listed {
				ok = ok && expected[name]
				delete(expected, name)
			}
		} else {
			ok = false
		}
		if !ok {
			c.report(declaration, RuleModuleState, fmt.Sprintf("%s must only return an array of exactly the top-level constants that are not primitives: %v", ModuleValuesFunction, frozen))
		}
		return
	}
}

// moduleValuesList is the names __playhoot_module_values returns, when its
// body is only `return [name, ...]`.
func moduleValuesList(function *ast.FunctionLiteral) ([]string, bool) {
	if function.Body == nil || len(function.Body.List) != 1 {
		return nil, false
	}
	returned, ok := function.Body.List[0].(*ast.ReturnStatement)
	if !ok {
		return nil, false
	}
	array, ok := returned.Argument.(*ast.ArrayLiteral)
	if !ok {
		return nil, false
	}
	names := make([]string, 0, len(array.Value))
	for _, value := range array.Value {
		identifier, ok := value.(*ast.Identifier)
		if !ok {
			return nil, false
		}
		names = append(names, string(identifier.Name))
	}
	return names, true
}

// immutableValue reports whether a handwritten top-level const may hold
// expression.
func immutableValue(expression ast.Expression) bool {
	switch expression.(type) {
	case *ast.FunctionLiteral, *ast.ArrowFunctionLiteral:
		return true
	default:
		return primitiveValue(expression)
	}
}

// primitiveValue reports whether expression is a primitive literal.
func primitiveValue(expression ast.Expression) bool {
	switch e := expression.(type) {
	case *ast.StringLiteral, *ast.NumberLiteral, *ast.BooleanLiteral, *ast.NullLiteral:
		return true
	case *ast.TemplateLiteral:
		return e.Tag == nil && len(e.Expressions) == 0
	case *ast.UnaryExpression:
		_, number := e.Operand.(*ast.NumberLiteral)
		return number && (e.Operator == token.MINUS || e.Operator == token.PLUS)
	default:
		return false
	}
}

// sdkConstant reports whether an sdk-v1 top-level const may hold expression,
// which is evaluated once at load: it can create no hidden state, since it
// calls nothing but the SDK's pure definition functions.
func sdkConstant(expression ast.Expression) bool {
	switch e := expression.(type) {
	case *ast.FunctionLiteral, *ast.ArrowFunctionLiteral, *ast.Identifier:
		return true
	case *ast.ArrayLiteral:
		for _, value := range e.Value {
			if !sdkConstant(value) {
				return false
			}
		}
		return true
	case *ast.ObjectLiteral:
		for _, property := range e.Value {
			switch p := property.(type) {
			case *ast.PropertyKeyed:
				if p.Computed || !sdkConstant(p.Value) {
					return false
				}
			case *ast.PropertyShort:
				if p.Initializer != nil {
					return false
				}
			case *ast.SpreadElement:
				if !sdkConstant(p.Expression) {
					return false
				}
			default:
				return false
			}
		}
		return true
	case *ast.SpreadElement:
		return sdkConstant(e.Expression)
	case *ast.CallExpression:
		callee, ok := e.Callee.(*ast.Identifier)
		if !ok || !definitionFunctions[string(callee.Name)] {
			return false
		}
		for _, argument := range e.ArgumentList {
			if !sdkConstant(argument) {
				return false
			}
		}
		return true
	default:
		return primitiveValue(expression)
	}
}

func (c *checker) report(node ast.Node, rule string, message string) {
	if node == nil {
		node = c.program
	}
	position := c.program.File.Position(int(node.Idx0()) - c.program.File.Base())
	key := fmt.Sprintf("%d:%d:%s", position.Line, position.Column, rule)
	if c.reported != nil {
		if c.reported[key] {
			return
		}
		c.reported[key] = true
	}
	c.violations = append(c.violations, Violation{Line: position.Line, Column: position.Column, Rule: rule, Message: message})
}

// violate reports only in the checking pass.
func (c *checker) violate(node ast.Node, rule string, message string) {
	if c.checking {
		c.report(node, rule, message)
	}
}

func (c *checker) taint(b *binding) {
	if b != nil && !b.topLevel && !b.tainted {
		b.tainted = true
		c.changed = true
	}
}

func (c *checker) hold(b *binding) {
	if b != nil && !b.topLevel && !b.holds {
		b.holds = true
		c.changed = true
	}
}

// bind records what a name bound to value may refer to.
func (c *checker) bind(b *binding, value ast.Expression, s *scope) {
	switch {
	case c.aliases(value, s):
		c.taint(b)
	case c.contains(value, s):
		c.hold(b)
	}
}

// reaches reports whether value is, or stores, an input or part of one.
func (c *checker) reaches(value ast.Expression, s *scope) bool {
	return c.aliases(value, s) || c.contains(value, s)
}

// functionScope returns the scope of a function, created once: its
// parameters are tainted, except in sdk-v1 scripts, whose inputs are fresh
// on every call and may change; and its declarations are hoisted.
func (c *checker) functionScope(node ast.Node, parent *scope, parameters *ast.ParameterList, body []ast.Statement) *scope {
	if existing, found := c.scopes[node]; found {
		return existing
	}
	s := &scope{parent: parent, names: make(map[string]*binding)}
	c.scopes[node] = s
	tainted := c.profile == ProfileHandwritten
	if parameters != nil {
		for _, parameter := range parameters.List {
			declareTarget(s, parameter.Target, tainted)
		}
		if parameters.Rest != nil {
			declareTarget(s, parameters.Rest, tainted)
		}
	}
	hoist(s, body)
	return s
}

// hoist declares every name a function body declares, without entering
// nested functions. Block scoping is merged into the function's scope, which
// only makes taint more conservative.
func hoist(s *scope, statements []ast.Statement) {
	for _, statement := range statements {
		hoistStatement(s, statement)
	}
}

func hoistStatement(s *scope, statement ast.Statement) {
	switch st := statement.(type) {
	case *ast.VariableStatement:
		for _, b := range st.List {
			declareTarget(s, b.Target, false)
		}
	case *ast.LexicalDeclaration:
		for _, b := range st.List {
			declareTarget(s, b.Target, false)
		}
	case *ast.FunctionDeclaration:
		if st.Function.Name != nil {
			s.declare(string(st.Function.Name.Name), &binding{})
		}
	case *ast.ClassDeclaration:
		if st.Class.Name != nil {
			s.declare(string(st.Class.Name.Name), &binding{})
		}
	case *ast.BlockStatement:
		hoist(s, st.List)
	case *ast.IfStatement:
		hoistStatement(s, st.Consequent)
		if st.Alternate != nil {
			hoistStatement(s, st.Alternate)
		}
	case *ast.ForStatement:
		switch initializer := st.Initializer.(type) {
		case *ast.ForLoopInitializerVarDeclList:
			for _, b := range initializer.List {
				declareTarget(s, b.Target, false)
			}
		case *ast.ForLoopInitializerLexicalDecl:
			for _, b := range initializer.LexicalDeclaration.List {
				declareTarget(s, b.Target, false)
			}
		}
		hoistStatement(s, st.Body)
	case *ast.ForInStatement:
		hoistForInto(s, st.Into)
		hoistStatement(s, st.Body)
	case *ast.ForOfStatement:
		hoistForInto(s, st.Into)
		hoistStatement(s, st.Body)
	case *ast.WhileStatement:
		hoistStatement(s, st.Body)
	case *ast.DoWhileStatement:
		hoistStatement(s, st.Body)
	case *ast.LabelledStatement:
		hoistStatement(s, st.Statement)
	case *ast.SwitchStatement:
		for _, clause := range st.Body {
			hoist(s, clause.Consequent)
		}
	case *ast.TryStatement:
		hoist(s, st.Body.List)
		if st.Catch != nil {
			if st.Catch.Parameter != nil {
				declareTarget(s, st.Catch.Parameter, false)
			}
			hoist(s, st.Catch.Body.List)
		}
		if st.Finally != nil {
			hoist(s, st.Finally.List)
		}
	}
}

func hoistForInto(s *scope, into ast.ForInto) {
	switch i := into.(type) {
	case *ast.ForIntoVar:
		declareTarget(s, i.Binding.Target, false)
	case *ast.ForDeclaration:
		declareTarget(s, i.Target, false)
	}
}

// declareTarget declares every name a binding target (an identifier or a
// destructuring pattern) introduces.
func declareTarget(s *scope, target ast.Expression, tainted bool) {
	for _, identifier := range targetNames(target) {
		s.declare(string(identifier.Name), &binding{tainted: tainted})
	}
}

// targetNames lists the identifiers a binding target binds.
func targetNames(target ast.Expression) []*ast.Identifier {
	switch t := target.(type) {
	case *ast.Identifier:
		return []*ast.Identifier{t}
	case *ast.ArrayPattern:
		var names []*ast.Identifier
		for _, element := range t.Elements {
			names = append(names, targetNames(element)...)
		}
		if t.Rest != nil {
			names = append(names, targetNames(t.Rest)...)
		}
		return names
	case *ast.ObjectPattern:
		var names []*ast.Identifier
		for _, property := range t.Properties {
			switch p := property.(type) {
			case *ast.PropertyShort:
				name := p.Name
				names = append(names, &name)
			case *ast.PropertyKeyed:
				names = append(names, targetNames(p.Value)...)
			}
		}
		if t.Rest != nil {
			names = append(names, targetNames(t.Rest)...)
		}
		return names
	case *ast.AssignExpression:
		return targetNames(t.Left)
	case *ast.Binding:
		return targetNames(t.Target)
	default:
		return nil
	}
}

func (c *checker) statements(statements []ast.Statement, s *scope) {
	for _, statement := range statements {
		c.statement(statement, s)
	}
}

func (c *checker) statement(statement ast.Statement, s *scope) {
	switch st := statement.(type) {
	case nil:
	case *ast.ExpressionStatement:
		c.expression(st.Expression, s)
	case *ast.VariableStatement:
		c.bindings(st.List, s)
	case *ast.LexicalDeclaration:
		c.bindings(st.List, s)
	case *ast.FunctionDeclaration:
		c.function(st.Function, s)
	case *ast.ClassDeclaration:
		c.class(st.Class, s)
	case *ast.BlockStatement:
		c.statements(st.List, s)
	case *ast.ReturnStatement:
		c.expression(st.Argument, s)
	case *ast.ThrowStatement:
		c.expression(st.Argument, s)
	case *ast.IfStatement:
		c.expression(st.Test, s)
		c.statement(st.Consequent, s)
		c.statement(st.Alternate, s)
	case *ast.ForStatement:
		switch initializer := st.Initializer.(type) {
		case *ast.ForLoopInitializerExpression:
			c.expression(initializer.Expression, s)
		case *ast.ForLoopInitializerVarDeclList:
			c.bindings(initializer.List, s)
		case *ast.ForLoopInitializerLexicalDecl:
			c.bindings(initializer.LexicalDeclaration.List, s)
		}
		c.expression(st.Test, s)
		c.expression(st.Update, s)
		c.statement(st.Body, s)
	case *ast.ForInStatement:
		// Keys are strings: the loop variable never aliases the source.
		c.expression(st.Source, s)
		c.forInto(st.Into, s, false)
		c.statement(st.Body, s)
	case *ast.ForOfStatement:
		if st.Await {
			c.violate(st, RuleAsync, "for await is not allowed: entrypoints are synchronous")
		}
		c.expression(st.Source, s)
		c.forInto(st.Into, s, c.reaches(st.Source, s))
		c.statement(st.Body, s)
	case *ast.WhileStatement:
		c.expression(st.Test, s)
		c.statement(st.Body, s)
	case *ast.DoWhileStatement:
		c.statement(st.Body, s)
		c.expression(st.Test, s)
	case *ast.LabelledStatement:
		c.statement(st.Statement, s)
	case *ast.SwitchStatement:
		c.expression(st.Discriminant, s)
		for _, clause := range st.Body {
			c.expression(clause.Test, s)
			c.statements(clause.Consequent, s)
		}
	case *ast.TryStatement:
		c.statements(st.Body.List, s)
		if st.Catch != nil {
			c.statements(st.Catch.Body.List, s)
		}
		if st.Finally != nil {
			c.statements(st.Finally.List, s)
		}
	case *ast.WithStatement:
		c.violate(st, RuleUnsupported, "with is not allowed")
	case *ast.BranchStatement, *ast.EmptyStatement, *ast.DebuggerStatement:
	default:
		c.violate(statement, RuleUnsupported, fmt.Sprintf("unsupported statement %T", statement))
	}
}

func (c *checker) forInto(into ast.ForInto, s *scope, tainted bool) {
	var target ast.Expression
	switch i := into.(type) {
	case *ast.ForIntoVar:
		target = i.Binding.Target
	case *ast.ForDeclaration:
		target = i.Target
	case *ast.ForIntoExpression:
		c.assignTarget(i.Expression, s, nil)
		target = i.Expression
	}
	for _, identifier := range targetNames(target) {
		if tainted {
			c.taint(s.resolve(string(identifier.Name)))
		}
	}
}

func (c *checker) bindings(list []*ast.Binding, s *scope) {
	for _, b := range list {
		c.expression(b.Initializer, s)
		c.patternDefaults(b.Target, s)
		if b.Initializer == nil {
			continue
		}
		if identifier, ok := b.Target.(*ast.Identifier); ok {
			c.bind(s.resolve(string(identifier.Name)), b.Initializer, s)
			continue
		}
		// Destructuring reads members, which may be inputs.
		if c.reaches(b.Initializer, s) {
			for _, identifier := range targetNames(b.Target) {
				c.taint(s.resolve(string(identifier.Name)))
			}
		}
	}
}

// patternDefaults visits the default values inside a destructuring pattern.
func (c *checker) patternDefaults(target ast.Expression, s *scope) {
	switch t := target.(type) {
	case *ast.ArrayPattern:
		for _, element := range t.Elements {
			c.patternDefaults(element, s)
		}
	case *ast.ObjectPattern:
		for _, property := range t.Properties {
			switch p := property.(type) {
			case *ast.PropertyShort:
				c.expression(p.Initializer, s)
			case *ast.PropertyKeyed:
				c.patternDefaults(p.Value, s)
			}
		}
	case *ast.AssignExpression:
		c.expression(t.Right, s)
	}
}

func (c *checker) function(function *ast.FunctionLiteral, parent *scope) {
	if function.Async || function.Generator {
		c.violate(function, RuleAsync, "async functions and generators are not allowed: entrypoints are synchronous")
	}
	var body []ast.Statement
	if function.Body != nil {
		body = function.Body.List
	}
	s := c.functionScope(function, parent, function.ParameterList, body)
	if function.ParameterList != nil {
		for _, parameter := range function.ParameterList.List {
			c.patternDefaults(parameter.Target, s)
			c.expression(parameter.Initializer, s)
		}
	}
	c.statements(body, s)
}

func (c *checker) arrow(arrow *ast.ArrowFunctionLiteral, parent *scope) {
	if arrow.Async {
		c.violate(arrow, RuleAsync, "async functions are not allowed: entrypoints are synchronous")
	}
	var body []ast.Statement
	if block, ok := arrow.Body.(*ast.BlockStatement); ok {
		body = block.List
	}
	s := c.functionScope(arrow, parent, arrow.ParameterList, body)
	if arrow.ParameterList != nil {
		for _, parameter := range arrow.ParameterList.List {
			c.patternDefaults(parameter.Target, s)
			c.expression(parameter.Initializer, s)
		}
	}
	switch b := arrow.Body.(type) {
	case *ast.BlockStatement:
		c.statements(b.List, s)
	case *ast.ExpressionBody:
		c.expression(b.Expression, s)
	}
}

func (c *checker) class(class *ast.ClassLiteral, s *scope) {
	c.expression(class.SuperClass, s)
	for _, element := range class.Body {
		switch e := element.(type) {
		case *ast.MethodDefinition:
			c.function(e.Body, s)
		case *ast.FieldDefinition:
			c.expression(e.Initializer, s)
		case *ast.ClassStaticBlock:
			c.violate(e, RuleModuleState, "static blocks are not allowed")
		}
	}
}

func (c *checker) expression(expression ast.Expression, s *scope) {
	switch e := expression.(type) {
	case nil:
	case *ast.Identifier:
		if forbiddenGlobals[string(e.Name)] && s.resolve(string(e.Name)) == nil {
			c.violate(e, RuleGlobalAccess, fmt.Sprintf("%s is not allowed", e.Name))
		}
	case *ast.ThisExpression:
		// In sdk-v1 scripts this only reaches objects a call created: top-level
		// classes are refused and every global is frozen.
		if c.profile == ProfileHandwritten {
			c.violate(e, RuleThis, "this is not allowed: state lives in what entrypoints return")
		}
	case *ast.MetaProperty:
		c.violate(e, RuleUnsupported, "meta properties are not allowed")
	case *ast.AwaitExpression:
		c.violate(e, RuleAsync, "await is not allowed: entrypoints are synchronous")
		c.expression(e.Argument, s)
	case *ast.YieldExpression:
		c.violate(e, RuleAsync, "yield is not allowed")
		c.expression(e.Argument, s)
	case *ast.FunctionLiteral:
		c.function(e, s)
	case *ast.ArrowFunctionLiteral:
		c.arrow(e, s)
	case *ast.ClassLiteral:
		c.class(e, s)
	case *ast.AssignExpression:
		c.expression(e.Right, s)
		c.assignTarget(e.Left, s, e.Right)
	case *ast.UnaryExpression:
		switch e.Operator {
		case token.INCREMENT, token.DECREMENT:
			c.assignTarget(e.Operand, s, nil)
		case token.DELETE:
			c.changesMemberOf(e.Operand, s, e, "delete")
			c.expression(e.Operand, s)
		default:
			c.expression(e.Operand, s)
		}
	case *ast.DotExpression:
		if forbiddenProperties[string(e.Identifier.Name)] {
			c.violate(&e.Identifier, RuleGlobalAccess, fmt.Sprintf("property %s is not allowed", e.Identifier.Name))
		}
		c.expression(e.Left, s)
	case *ast.PrivateDotExpression:
		c.expression(e.Left, s)
	case *ast.BracketExpression:
		if literal, ok := e.Member.(*ast.StringLiteral); ok && forbiddenProperties[string(literal.Value)] {
			c.violate(literal, RuleGlobalAccess, fmt.Sprintf("property %s is not allowed", literal.Value))
		}
		c.expression(e.Left, s)
		c.expression(e.Member, s)
	case *ast.CallExpression:
		c.call(e, s)
	case *ast.NewExpression:
		c.expression(e.Callee, s)
		c.expressions(e.ArgumentList, s)
	case *ast.BinaryExpression:
		c.expression(e.Left, s)
		c.expression(e.Right, s)
	case *ast.ConditionalExpression:
		c.expression(e.Test, s)
		c.expression(e.Consequent, s)
		c.expression(e.Alternate, s)
	case *ast.SequenceExpression:
		c.expressions(e.Sequence, s)
	case *ast.ArrayLiteral:
		c.expressions(e.Value, s)
	case *ast.ObjectLiteral:
		for _, property := range e.Value {
			c.expression(property, s)
		}
	case *ast.PropertyKeyed:
		if e.Computed {
			c.expression(e.Key, s)
		}
		c.expression(e.Value, s)
	case *ast.PropertyShort:
		c.expression(&e.Name, s)
		c.expression(e.Initializer, s)
	case *ast.SpreadElement:
		c.expression(e.Expression, s)
	case *ast.TemplateLiteral:
		c.expression(e.Tag, s)
		c.expressions(e.Expressions, s)
	case *ast.OptionalChain:
		c.expression(e.Expression, s)
	case *ast.Optional:
		c.expression(e.Expression, s)
	case *ast.ArrayPattern, *ast.ObjectPattern:
		// Visited through their assignment.
	case *ast.StringLiteral, *ast.NumberLiteral, *ast.BooleanLiteral, *ast.NullLiteral, *ast.RegExpLiteral:
	default:
		c.violate(expression, RuleUnsupported, fmt.Sprintf("unsupported expression %T", expression))
	}
}

func (c *checker) expressions(expressions []ast.Expression, s *scope) {
	for _, expression := range expressions {
		c.expression(expression, s)
	}
}

// call checks calls that mutate their receiver or argument, and records what
// they make tainted.
func (c *checker) call(call *ast.CallExpression, s *scope) {
	c.expression(call.Callee, s)
	c.expressions(call.ArgumentList, s)
	anyArgumentReaches := false
	for _, argument := range call.ArgumentList {
		if c.reaches(argument, s) {
			anyArgumentReaches = true
		}
	}
	dot, ok := call.Callee.(*ast.DotExpression)
	if !ok {
		return
	}
	method := string(dot.Identifier.Name)
	if receiver, isIdentifier := dot.Left.(*ast.Identifier); isIdentifier && s.resolve(string(receiver.Name)) == nil &&
		receiver.Name == "Object" && mutatingObjectFunctions[method] && len(call.ArgumentList) > 0 {
		c.mutates(call.ArgumentList[0], s, call, "Object."+method)
		if anyArgumentReaches {
			c.hold(c.rootBinding(call.ArgumentList[0], s))
		}
		return
	}
	if (method == "call" || method == "apply") && len(call.ArgumentList) > 0 && c.reaches(call.ArgumentList[0], s) {
		c.violate(call, RuleMutation, "calling a function with an input as this may mutate it")
		return
	}
	if mutatingMethods[method] {
		c.mutates(dot.Left, s, call, method+"()")
		if anyArgumentReaches {
			c.hold(c.rootBinding(dot.Left, s))
		}
	}
}

// assignTarget checks an assignment's left-hand side; value is nil for ++ and
// --. Rebinding a local name is allowed; changing anything shared is not.
func (c *checker) assignTarget(target ast.Expression, s *scope, value ast.Expression) {
	switch t := target.(type) {
	case *ast.Identifier:
		b := s.resolve(string(t.Name))
		switch {
		case b == nil:
			c.violate(t, RuleGlobalAccess, fmt.Sprintf("assigning %s writes a global", t.Name))
		case b.topLevel:
			c.violate(t, RuleModuleState, fmt.Sprintf("assigning %s changes module state", t.Name))
		case value != nil:
			c.bind(b, value, s)
		}
	case *ast.ArrayPattern, *ast.ObjectPattern:
		for _, identifier := range targetNames(t) {
			b := s.resolve(string(identifier.Name))
			switch {
			case b == nil:
				c.violate(identifier, RuleGlobalAccess, fmt.Sprintf("assigning %s writes a global", identifier.Name))
			case b.topLevel:
				c.violate(identifier, RuleModuleState, fmt.Sprintf("assigning %s changes module state", identifier.Name))
			case value != nil && c.reaches(value, s):
				c.taint(b)
			}
		}
		c.patternDefaults(t, s)
	default:
		c.changesMemberOf(target, s, target, "assignment")
		c.expression(target, s)
		if value != nil && c.reaches(value, s) {
			c.hold(c.rootBinding(target, s))
		}
	}
}

// changesMemberOf checks changing a member of the object target is a member
// of.
func (c *checker) changesMemberOf(target ast.Expression, s *scope, at ast.Node, what string) {
	switch t := target.(type) {
	case *ast.DotExpression:
		c.mutates(t.Left, s, at, what)
	case *ast.BracketExpression:
		c.mutates(t.Left, s, at, what)
	case *ast.PrivateDotExpression:
		c.mutates(t.Left, s, at, what)
	default:
		c.mutates(target, s, at, what)
	}
}

// mutates reports changing object unless it is a value the function created
// itself; a created value that stores inputs may change, its members may not.
func (c *checker) mutates(object ast.Expression, s *scope, at ast.Node, what string) {
	root := memberRoot(object)
	_, direct := object.(*ast.Identifier)
	switch r := root.(type) {
	case *ast.Identifier:
		b := s.resolve(string(r.Name))
		switch {
		case b == nil:
			c.violate(at, RuleGlobalAccess, fmt.Sprintf("%s changes the global %s", what, r.Name))
		case b.topLevel:
			c.violate(at, RuleModuleState, fmt.Sprintf("%s changes module state %s", what, r.Name))
		case b.tainted || (b.holds && !direct):
			c.violate(at, RuleMutation, fmt.Sprintf("%s changes %s, which is an input or reaches one; return a new value instead", what, r.Name))
		}
	case *ast.ThisExpression:
		c.violate(at, RuleThis, fmt.Sprintf("%s changes this", what))
	default:
		if c.aliases(object, s) {
			c.violate(at, RuleMutation, fmt.Sprintf("%s changes a value that reaches an input; return a new value instead", what))
		}
	}
}

// memberRoot is the object a member chain starts from.
func memberRoot(expression ast.Expression) ast.Expression {
	for {
		switch e := expression.(type) {
		case *ast.DotExpression:
			expression = e.Left
		case *ast.BracketExpression:
			expression = e.Left
		case *ast.PrivateDotExpression:
			expression = e.Left
		case *ast.OptionalChain:
			expression = e.Expression
		case *ast.Optional:
			expression = e.Expression
		default:
			return expression
		}
	}
}

func (c *checker) rootBinding(expression ast.Expression, s *scope) *binding {
	if identifier, ok := memberRoot(expression).(*ast.Identifier); ok {
		return s.resolve(string(identifier.Name))
	}
	return nil
}

// aliases reports whether expression may evaluate to an input, or to
// something reachable from one.
func (c *checker) aliases(expression ast.Expression, s *scope) bool {
	switch e := expression.(type) {
	case nil:
		return false
	case *ast.Identifier:
		b := s.resolve(string(e.Name))
		return b != nil && b.tainted
	case *ast.DotExpression, *ast.BracketExpression, *ast.PrivateDotExpression:
		// A member of an input, or of a value storing inputs, may be one.
		return c.reaches(memberRoot(e), s)
	case *ast.OptionalChain:
		return c.aliases(e.Expression, s)
	case *ast.Optional:
		return c.aliases(e.Expression, s)
	case *ast.ConditionalExpression:
		return c.aliases(e.Consequent, s) || c.aliases(e.Alternate, s)
	case *ast.BinaryExpression:
		if e.Operator == token.LOGICAL_AND || e.Operator == token.LOGICAL_OR || e.Operator == token.COALESCE {
			return c.aliases(e.Left, s) || c.aliases(e.Right, s)
		}
		return false
	case *ast.SequenceExpression:
		return len(e.Sequence) > 0 && c.aliases(e.Sequence[len(e.Sequence)-1], s)
	case *ast.AssignExpression:
		return c.aliases(e.Right, s)
	case *ast.SpreadElement:
		return c.aliases(e.Expression, s)
	case *ast.CallExpression:
		// A call's result may hand back its receiver's or arguments' parts.
		if dot, ok := e.Callee.(*ast.DotExpression); ok && c.reaches(dot.Left, s) {
			return true
		}
		for _, argument := range e.ArgumentList {
			if c.reaches(argument, s) {
				return true
			}
		}
		return false
	case *ast.NewExpression:
		for _, argument := range e.ArgumentList {
			if c.reaches(argument, s) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

// contains reports whether expression is a value created here that stores an
// input, or something reachable from one, inside.
func (c *checker) contains(expression ast.Expression, s *scope) bool {
	switch e := expression.(type) {
	case nil:
		return false
	case *ast.Identifier:
		b := s.resolve(string(e.Name))
		return b != nil && b.holds
	case *ast.ArrayLiteral:
		for _, value := range e.Value {
			if c.reaches(value, s) {
				return true
			}
		}
		return false
	case *ast.ObjectLiteral:
		for _, property := range e.Value {
			switch p := property.(type) {
			case *ast.PropertyKeyed:
				if c.reaches(p.Value, s) {
					return true
				}
			case *ast.PropertyShort:
				if c.reaches(&p.Name, s) {
					return true
				}
			case *ast.SpreadElement:
				if c.reaches(p.Expression, s) {
					return true
				}
			}
		}
		return false
	case *ast.SpreadElement:
		return c.reaches(e.Expression, s)
	case *ast.ConditionalExpression:
		return c.contains(e.Consequent, s) || c.contains(e.Alternate, s)
	case *ast.BinaryExpression:
		if e.Operator == token.LOGICAL_AND || e.Operator == token.LOGICAL_OR || e.Operator == token.COALESCE {
			return c.contains(e.Left, s) || c.contains(e.Right, s)
		}
		return false
	case *ast.SequenceExpression:
		return len(e.Sequence) > 0 && c.contains(e.Sequence[len(e.Sequence)-1], s)
	case *ast.AssignExpression:
		return c.contains(e.Right, s)
	default:
		return false
	}
}
