package engineservice

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

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
