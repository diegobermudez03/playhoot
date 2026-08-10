package engineservice

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

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
