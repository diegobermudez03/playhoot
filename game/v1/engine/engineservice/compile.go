package engineservice

import (
	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/engine/internal/compiler"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// Severity classifies a Diagnostic. See the underlying
// engine/internal/compiler.Severity for its documented meaning.
type Severity = compiler.Severity

const (
	SeverityError   = compiler.SeverityError
	SeverityWarning = compiler.SeverityWarning
	SeverityInfo    = compiler.SeverityInfo
)

// Diagnostic reports one compile-time observation about a
// program.Definition. See the underlying engine/internal/compiler.Diagnostic
// for field documentation.
type Diagnostic = compiler.Diagnostic

// Diagnostics is an ordered collection of Diagnostic values produced by
// Compile. See the underlying engine/internal/compiler.Diagnostics.
type Diagnostics = compiler.Diagnostics

// Compile validates def and produces its immutable, executable
// representation as an engine.Program.
//
// Compile never panics on an invalid definition and never stops at the
// first problem it finds: it collects every Diagnostic it can and
// returns them alongside the result. When the returned Diagnostics has
// any SeverityError entry, the returned engine.Program must not be
// executed — see Diagnostics.HasErrors.
//
// The actual compilation pipeline — symbol resolution, type checking,
// structured-concurrency and UI validation, and so on — lives in
// engine/internal/compiler; this is a thin, deliberately stable entry
// point onto it. See LOGICAL_CONTRACT.md for Compile's place in the
// engine's overall contract (Definition -> Program + diagnostics).
func Compile(def program.Definition) (engine.Program, Diagnostics) {
	return compiler.Compile(def)
}
