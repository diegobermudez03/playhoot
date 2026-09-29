package scriptlint

import (
	"reflect"

	"github.com/dop251/goja/ast"
)

// walk traverses every statement and expression reachable from n exactly
// once. onDeclare is called with every name a binding (var/let/const,
// function/class declaration, parameter, catch clause, for-loop variable)
// introduces anywhere in the script - this package deliberately treats the
// whole script as one flat scope rather than modeling JavaScript's real
// nested/block scoping, which only risks under-flagging a name shadowed
// elsewhere in the script, never flagging a script's own legitimate local
// variable as unsupported. onRead is called with every identifier read in
// expression position (never a property name, a declaration target, or a
// statement label) and its source offset. onMember is additionally called
// whenever that read is the object of a member access whose accessed
// property name is statically known - a plain "object.property" access, or
// a computed "object['property']" access with a literal string key - with
// that property name. Any callback may be nil.
func walk(n ast.Node, onDeclare func(name string), onRead func(name string, idx int), onMember func(object, property string, idx int)) {
	w := &walker{onDeclare: onDeclare, onRead: onRead, onMember: onMember}
	w.node(n)
}

type walker struct {
	onDeclare func(name string)
	onRead    func(name string, idx int)
	onMember  func(object, property string, idx int)
}

func (w *walker) declare(name string) {
	if w.onDeclare != nil && name != "" {
		w.onDeclare(name)
	}
}

func (w *walker) read(name string, idx int) {
	if w.onRead != nil && name != "" {
		w.onRead(name, idx)
	}
}

// isNilNode reports whether n is absent - either a true nil interface, or a
// nil concrete pointer boxed into one (an AST field declared as a concrete
// *ast.SomeNode type, rather than an interface, is a nil pointer of that
// exact type when its grammar production is optional and absent; passed
// through an interface-typed parameter, that is a non-nil interface whose
// value is nil, which a plain "== nil" or "case nil" check cannot detect).
func isNilNode(n ast.Node) bool {
	if n == nil {
		return true
	}
	v := reflect.ValueOf(n)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

func (w *walker) node(n ast.Node) {
	if isNilNode(n) {
		return
	}
	switch t := n.(type) {
	case *ast.Program:
		w.statements(t.Body)

	// ---- Statements ----
	case *ast.BlockStatement:
		w.statements(t.List)
	case *ast.ExpressionStatement:
		w.expr(t.Expression)
	case *ast.IfStatement:
		w.expr(t.Test)
		w.node(t.Consequent)
		w.node(t.Alternate)
	case *ast.ForStatement:
		w.forInitializer(t.Initializer)
		w.expr(t.Test)
		w.expr(t.Update)
		w.node(t.Body)
	case *ast.ForInStatement:
		w.forInto(t.Into)
		w.expr(t.Source)
		w.node(t.Body)
	case *ast.ForOfStatement:
		w.forInto(t.Into)
		w.expr(t.Source)
		w.node(t.Body)
	case *ast.WhileStatement:
		w.expr(t.Test)
		w.node(t.Body)
	case *ast.DoWhileStatement:
		w.node(t.Body)
		w.expr(t.Test)
	case *ast.ReturnStatement:
		w.expr(t.Argument)
	case *ast.ThrowStatement:
		w.expr(t.Argument)
	case *ast.SwitchStatement:
		w.expr(t.Discriminant)
		for _, c := range t.Body {
			w.expr(c.Test)
			w.statements(c.Consequent)
		}
	case *ast.TryStatement:
		w.node(t.Body)
		if t.Catch != nil {
			w.bindingTarget(t.Catch.Parameter)
			w.node(t.Catch.Body)
		}
		w.node(t.Finally)
	case *ast.VariableStatement:
		w.bindings(t.List)
	case *ast.LexicalDeclaration:
		w.bindings(t.List)
	case *ast.WithStatement:
		w.expr(t.Object)
		w.node(t.Body)
	case *ast.LabelledStatement:
		w.node(t.Statement)
	case *ast.FunctionDeclaration:
		w.declare(identifierName(t.Function.Name))
		w.functionLiteral(t.Function)
	case *ast.ClassDeclaration:
		w.declare(identifierName(t.Class.Name))
		w.classLiteral(t.Class)
	case *ast.BranchStatement, *ast.EmptyStatement, *ast.DebuggerStatement, *ast.BadStatement:
		// No expressions/bindings to visit.

	// ---- Expressions ----
	case *ast.Identifier:
		w.read(string(t.Name), int(t.Idx))
	case *ast.ThisExpression, *ast.SuperExpression, *ast.NullLiteral, *ast.BooleanLiteral,
		*ast.NumberLiteral, *ast.StringLiteral, *ast.RegExpLiteral, *ast.BadExpression:
		// Literals/keywords never reference a name.
	case *ast.ArrayLiteral:
		for _, e := range t.Value {
			w.expr(e)
		}
	case *ast.AssignExpression:
		w.expr(t.Left)
		w.expr(t.Right)
	case *ast.BinaryExpression:
		w.expr(t.Left)
		w.expr(t.Right)
	case *ast.BracketExpression:
		w.expr(t.Left)
		w.expr(t.Member)
		if id, ok := t.Left.(*ast.Identifier); ok && w.onMember != nil {
			if lit, ok2 := t.Member.(*ast.StringLiteral); ok2 {
				w.onMember(string(id.Name), string(lit.Value), int(id.Idx))
			}
		}
	case *ast.CallExpression:
		w.expr(t.Callee)
		for _, a := range t.ArgumentList {
			w.expr(a)
		}
	case *ast.ConditionalExpression:
		w.expr(t.Test)
		w.expr(t.Consequent)
		w.expr(t.Alternate)
	case *ast.DotExpression:
		w.expr(t.Left)
		if id, ok := t.Left.(*ast.Identifier); ok && w.onMember != nil {
			w.onMember(string(id.Name), string(t.Identifier.Name), int(id.Idx))
		}
	case *ast.PrivateDotExpression:
		w.expr(t.Left)
	case *ast.OptionalChain:
		w.node(t.Expression)
	case *ast.Optional:
		w.node(t.Expression)
	case *ast.FunctionLiteral:
		w.functionLiteral(t)
	case *ast.ClassLiteral:
		w.classLiteral(t)
	case *ast.ArrowFunctionLiteral:
		w.params(t.ParameterList)
		w.node(t.Body)
	case *ast.ExpressionBody:
		w.expr(t.Expression)
	case *ast.NewExpression:
		w.expr(t.Callee)
		for _, a := range t.ArgumentList {
			w.expr(a)
		}
	case *ast.ObjectLiteral:
		for _, p := range t.Value {
			w.property(p)
		}
	case *ast.SequenceExpression:
		for _, e := range t.Sequence {
			w.expr(e)
		}
	case *ast.SpreadElement:
		w.expr(t.Expression)
	case *ast.TemplateLiteral:
		w.expr(t.Tag)
		for _, e := range t.Expressions {
			w.expr(e)
		}
	case *ast.UnaryExpression:
		w.expr(t.Operand)
	case *ast.YieldExpression:
		w.expr(t.Argument)
	case *ast.AwaitExpression:
		w.expr(t.Argument)
	case *ast.MetaProperty:
		// `new.target`/`import.meta`-shaped; neither identifier is a
		// variable reference.

	// ---- Patterns (declaration targets, never reads) ----
	case *ast.ArrayPattern:
		for _, e := range t.Elements {
			w.bindingTarget(e)
		}
		w.bindingTarget(t.Rest)
	case *ast.ObjectPattern:
		for _, p := range t.Properties {
			w.patternProperty(p)
		}
		w.bindingTarget(t.Rest)

	default:
		// Node types with no children relevant to name resolution
		// (BadExpression variants, etc.) are intentionally ignored.
	}
}

func (w *walker) statements(list []ast.Statement) {
	for _, s := range list {
		w.node(s)
	}
}

func (w *walker) expr(e ast.Expression) {
	w.node(e)
}

func (w *walker) functionLiteral(f *ast.FunctionLiteral) {
	if f == nil {
		return
	}
	w.params(f.ParameterList)
	w.node(f.Body)
}

func (w *walker) classLiteral(c *ast.ClassLiteral) {
	if c == nil {
		return
	}
	w.expr(c.SuperClass)
	for _, el := range c.Body {
		w.classElement(el)
	}
}

func (w *walker) classElement(el ast.ClassElement) {
	switch t := el.(type) {
	case *ast.FieldDefinition:
		if t.Computed {
			w.expr(t.Key)
		}
		w.expr(t.Initializer)
	case *ast.MethodDefinition:
		if t.Computed {
			w.expr(t.Key)
		}
		w.functionLiteral(t.Body)
	case *ast.ClassStaticBlock:
		w.node(t.Block)
	}
}

func (w *walker) params(pl *ast.ParameterList) {
	if pl == nil {
		return
	}
	for _, b := range pl.List {
		w.binding(b)
	}
	w.bindingTarget(pl.Rest)
}

func (w *walker) bindings(list []*ast.Binding) {
	for _, b := range list {
		w.binding(b)
	}
}

func (w *walker) binding(b *ast.Binding) {
	if b == nil {
		return
	}
	w.bindingTarget(b.Target)
	w.expr(b.Initializer)
}

// bindingTarget declares every name a binding target introduces (a plain
// Identifier, or names nested inside an array/object destructuring
// pattern), and walks any default-value expressions within it as reads.
func (w *walker) bindingTarget(e ast.Expression) {
	switch t := e.(type) {
	case nil:
	case *ast.Identifier:
		w.declare(string(t.Name))
	case *ast.AssignExpression:
		w.bindingTarget(t.Left)
		w.expr(t.Right)
	default:
		// Array/object patterns and any other node type still route
		// through node(), which declares its own nested targets.
		w.node(t)
	}
}

func (w *walker) patternProperty(p ast.Property) {
	switch t := p.(type) {
	case *ast.PropertyShort:
		w.declare(string(t.Name.Name))
		w.expr(t.Initializer)
	case *ast.PropertyKeyed:
		if t.Computed {
			w.expr(t.Key)
		}
		w.bindingTarget(t.Value)
	case *ast.SpreadElement:
		w.bindingTarget(t.Expression)
	}
}

// property walks a value-position ObjectLiteral property (never a
// declaration target): a shorthand property reads its named variable, a
// keyed property's key is a property name (unless computed) and its value
// is an ordinary expression.
func (w *walker) property(p ast.Property) {
	switch t := p.(type) {
	case *ast.PropertyShort:
		w.read(string(t.Name.Name), int(t.Name.Idx))
		w.expr(t.Initializer)
	case *ast.PropertyKeyed:
		if t.Computed {
			w.expr(t.Key)
		}
		w.expr(t.Value)
	case *ast.SpreadElement:
		w.expr(t.Expression)
	}
}

func (w *walker) forInitializer(init ast.ForLoopInitializer) {
	switch t := init.(type) {
	case nil:
	case *ast.ForLoopInitializerExpression:
		w.expr(t.Expression)
	case *ast.ForLoopInitializerVarDeclList:
		w.bindings(t.List)
	case *ast.ForLoopInitializerLexicalDecl:
		w.bindings(t.LexicalDeclaration.List)
	}
}

func (w *walker) forInto(into ast.ForInto) {
	switch t := into.(type) {
	case nil:
	case *ast.ForIntoVar:
		w.binding(t.Binding)
	case *ast.ForDeclaration:
		w.bindingTarget(t.Target)
	case *ast.ForIntoExpression:
		w.expr(t.Expression)
	}
}

func identifierName(id *ast.Identifier) string {
	if id == nil {
		return ""
	}
	return string(id.Name)
}
