package compiler

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/language/v1/engine"
	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

// This file compiles program's four simplest named, reusable,
// top-level declaration catalogs — questions, effects, projections,
// and views — each compiled exactly once, keyed by declared name, with
// no cycle-detection or recursive resolution needed (unlike functions,
// which may call each other and so need compile_functions.go's own
// memoized, cycle-guarded resolution).

// compileQuestions compiles every program.QuestionDeclaration,
// diagnosing an empty or duplicate name. It must run after global state
// compiles, since a Validation expression may reference "global" — see
// program.QuestionDeclaration's documented Validation scope — and before
// any workflow compiles, since an OpenQuestionOperation validates its
// Arguments against a question's Parameters.
func (c *compiler) compileQuestions(globalType engine.RecordType) map[string]engine.Question {
	result := make(map[string]engine.Question, len(c.definition.Questions))
	seen := make(map[string]int, len(c.definition.Questions))
	for i, q := range c.definition.Questions {
		path := fmt.Sprintf("$.questions[%d]", i)
		if q.Name == "" {
			c.addf(path, "question declaration has an empty name")
			continue
		}
		if first, ok := seen[q.Name]; ok {
			c.addf(path, "duplicate question name %q (first declared at $.questions[%d])", q.Name, first)
			continue
		}
		seen[q.Name] = i
		result[q.Name] = c.compileQuestionDeclaration(q, path, globalType)
	}
	return result
}

func (c *compiler) compileQuestionDeclaration(q program.QuestionDeclaration, path string, globalType engine.RecordType) engine.Question {
	scope := exprScope{
		resourcesScopeRootName: c.resourcesType,
		globalScopeRootName:    globalType,
		"respondent":           engine.UserType{},
	}
	params := c.compileFieldDeclarationsScope(q.Parameters, path+".parameters", scope)

	responseType := c.compileTypeReference(q.ResponseType, path+".response_type")
	scope["answer"] = responseType

	var validation engine.Expression
	if q.Validation != nil {
		var validationType engine.Type
		validation, validationType = c.compileExpression(q.Validation, scope, path+".validation")
		if validationType != nil {
			if _, ok := validationType.(engine.BoolType); !ok {
				c.addf(path+".validation", "question validation must be statically bool, but it is %s", describeType(validationType))
			}
		}
	}

	return engine.Question{Name: q.Name, Parameters: params, ResponseType: responseType, Validation: validation}
}

// compileEffects compiles every program.EffectDeclaration, diagnosing
// an empty or duplicate name. An effect declares only a payload shape —
// no expression of its own to compile — so, unlike questions, ordering
// relative to global state does not matter.
func (c *compiler) compileEffects() map[string]engine.Effect {
	result := make(map[string]engine.Effect, len(c.definition.Effects))
	seen := make(map[string]int, len(c.definition.Effects))
	for i, e := range c.definition.Effects {
		path := fmt.Sprintf("$.effects[%d]", i)
		if e.Name == "" {
			c.addf(path, "effect declaration has an empty name")
			continue
		}
		if first, ok := seen[e.Name]; ok {
			c.addf(path, "duplicate effect name %q (first declared at $.effects[%d])", e.Name, first)
			continue
		}
		seen[e.Name] = i
		params := c.compileFieldDeclarationsScope(e.Parameters, path+".parameters", exprScope{})
		result[e.Name] = engine.Effect{Name: e.Name, Parameters: params}
	}
	return result
}

// compileProjections compiles every program.ProjectionDeclaration,
// diagnosing an empty or duplicate name, keyed by declared name — every
// Presentation and QuestionPresentation validates its ProjectionArguments
// and result-type assignability against these once compiled here.
func (c *compiler) compileProjections(globalType engine.RecordType) map[string]engine.Projection {
	result := make(map[string]engine.Projection, len(c.definition.Projections))
	seen := make(map[string]int, len(c.definition.Projections))
	for i, p := range c.definition.Projections {
		path := fmt.Sprintf("$.projections[%d]", i)
		if p.Name == "" {
			c.addf(path, "projection declaration has an empty name")
			continue
		}
		if first, ok := seen[p.Name]; ok {
			c.addf(path, "duplicate projection name %q (first declared at $.projections[%d])", p.Name, first)
			continue
		}
		seen[p.Name] = i
		result[p.Name] = c.compileProjectionDeclaration(p, path, globalType)
	}
	return result
}

// compileProjectionDeclaration compiles one program.ProjectionDeclaration.
// Body compiles against Parameters plus the reserved "resources" and
// "global" roots and the implicit immutable "viewer" binding (typed
// user) — never "local", a signal's fields, or any other workflow- or
// transition-specific binding; see program.ProjectionDeclaration's
// documented body scope.
func (c *compiler) compileProjectionDeclaration(p program.ProjectionDeclaration, path string, globalType engine.RecordType) engine.Projection {
	scope := exprScope{
		resourcesScopeRootName: c.resourcesType,
		globalScopeRootName:    globalType,
		"viewer":               engine.UserType{},
	}
	for _, f := range p.Parameters {
		if f.Name == "viewer" {
			c.addf(path+".parameters", "projection parameter %q collides with the implicit viewer binding", f.Name)
		}
	}
	params := c.compileFieldDeclarationsScope(p.Parameters, path+".parameters", scope)

	resultType := c.compileTypeReference(p.ResultType, path+".result_type")

	body, bodyType := c.compileExpression(p.Body, scope, path+".body")
	if resultType != nil && bodyType != nil && !resultType.Equal(bodyType) {
		c.addf(path+".body", "projection declares result type %s, but its body is statically %s", describeType(resultType), describeType(bodyType))
	}

	return engine.Projection{Name: p.Name, Parameters: params, ResultType: resultType, Body: body}
}

func (c *compiler) projectionByName(name string) (program.ProjectionDeclaration, bool) {
	for _, p := range c.definition.Projections {
		if p.Name == name {
			return p, true
		}
	}
	return program.ProjectionDeclaration{}, false
}

// compileViews compiles every program.ViewDeclaration, diagnosing an
// empty or duplicate name, keyed by declared name.
func (c *compiler) compileViews() map[string]engine.View {
	result := make(map[string]engine.View, len(c.definition.Views))
	seen := make(map[string]int, len(c.definition.Views))
	for i, v := range c.definition.Views {
		path := fmt.Sprintf("$.views[%d]", i)
		if v.Name == "" {
			c.addf(path, "view declaration has an empty name")
			continue
		}
		if first, ok := seen[v.Name]; ok {
			c.addf(path, "duplicate view name %q (first declared at $.views[%d])", v.Name, first)
			continue
		}
		seen[v.Name] = i
		result[v.Name] = c.compileViewDeclaration(v, path)
	}
	return result
}

// compileViewDeclaration compiles one program.ViewDeclaration.
// LocalState field initializers compile against only the implicit
// "model" binding; Root compiles against "model" plus the reserved
// "local" root (typed by the compiled LocalState) — never "global",
// "resources", or any other workflow- or execution-specific binding;
// see program.ViewDeclaration's documented scope restrictions.
func (c *compiler) compileViewDeclaration(v program.ViewDeclaration, path string) engine.View {
	modelType := c.compileTypeReference(v.ModelType, path+".model_type")

	modelScope := exprScope{"model": modelType}
	localState := c.compileStateFields(v.LocalState.Fields, path+".local_state.fields", modelScope)
	localType := engine.RecordType{Name: "local", Fields: stateFieldTypes(localState)}

	rootScope := exprScope{"model": modelType, "local": localType}
	root := c.compileUIElement(v.Root, rootScope, path+".root")

	return engine.View{Name: v.Name, ModelType: modelType, LocalState: localState, Root: root}
}

func (c *compiler) viewByName(name string) (engine.View, bool) {
	v, ok := c.compiledViews[name]
	return v, ok
}
