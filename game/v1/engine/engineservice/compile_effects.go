package engineservice

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
)

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
