package engineservice

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

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
