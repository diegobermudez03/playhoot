package engine

// Effect is the compiled representation of one program.EffectDeclaration:
// a reusable, typed, client-facing presentation event's payload shape.
// Effect declares no view, transport, or rendering behavior — see
// EmitEffectOperation and EmitEffectOutput for how one is produced and
// delivered.
type Effect struct {
	Name       string
	Parameters []FieldType
}
