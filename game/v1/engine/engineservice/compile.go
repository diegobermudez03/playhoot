package engineservice

import (
	"fmt"

	"github.com/diegobermudez03/playhoot/game/v1/engine"
	"github.com/diegobermudez03/playhoot/game/v1/program"
)

// Severity classifies a Diagnostic.
type Severity int

const (
	// SeverityError marks a diagnostic that makes the definition
	// uncompilable: Compile does not produce an engine.Program safe to
	// execute when any Diagnostic in its result has this severity.
	SeverityError Severity = iota

	// SeverityWarning marks a diagnostic that does not by itself
	// prevent compilation but flags behavior a creator likely did not
	// intend.
	SeverityWarning

	// SeverityInfo marks a purely informational diagnostic.
	SeverityInfo
)

func (s Severity) String() string {
	switch s {
	case SeverityError:
		return "error"
	case SeverityWarning:
		return "warning"
	case SeverityInfo:
		return "info"
	default:
		return "unknown"
	}
}

// Diagnostic reports one compile-time observation about a
// program.Definition.
//
// Path identifies where in the definition the diagnostic applies, in the
// same dotted/bracketed style used by program/gameservice's
// ValidationError and DecodeError (for example
// "$.workflows[0].states[2].transitions[0]"), so tooling built on top of
// program, gameservice, and engineservice can present a consistent
// location format regardless of which layer produced a given diagnostic.
type Diagnostic struct {
	Severity Severity
	Path     string
	Message  string
}

func (d Diagnostic) String() string {
	if d.Path == "" {
		return fmt.Sprintf("%s: %s", d.Severity, d.Message)
	}
	return fmt.Sprintf("%s: %s: %s", d.Severity, d.Path, d.Message)
}

// Diagnostics is an ordered collection of Diagnostic values produced by
// Compile.
//
// Compile collects every diagnostic it can find rather than stopping at
// the first one. A nil or empty Diagnostics means this package's
// compiler found nothing wrong; it is not, by itself, a guarantee that
// the resulting engine.Program is safe to execute in every future sense
// — only that no rule this package's compiler currently checks was
// violated.
type Diagnostics []Diagnostic

// HasErrors reports whether any Diagnostic in ds has SeverityError.
func (ds Diagnostics) HasErrors() bool {
	for _, d := range ds {
		if d.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Compile validates def and produces its immutable, executable
// representation as an engine.Program.
//
// Compile never panics on an invalid definition and never stops at the
// first problem it finds: it collects every Diagnostic it can and
// returns them alongside the result. When the returned Diagnostics has
// any SeverityError entry, the returned engine.Program must not be
// executed.
//
// This version does not yet perform symbol resolution, type checking,
// control-flow validation, or any other semantic check described in
// LOGICAL_CONTRACT.md and engine/README.md's "Compiler responsibilities"
// section — those are added in later steps. For now, Compile always
// succeeds with no diagnostics, and its result must not be treated as a
// guarantee that def is semantically valid.
func Compile(def program.Definition) (engine.Program, Diagnostics) {
	return engine.Program{}, nil
}
