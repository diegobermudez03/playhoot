// Package scriptlint statically analyzes an authored backend script's
// source text against Session Runtime's actual sandboxed JavaScript
// execution surface, before that script is accepted for publish or use.
// Frontend script is never analyzed by this package - it runs in an
// entirely different environment with its own legitimate global surface.
//
// Validate never executes the script it analyzes; it is a pure, read-only
// static check over the script's source text.
package scriptlint

import (
	"fmt"

	"github.com/dop251/goja/parser"
)

// Severity classifies a Finding.
type Severity string

const (
	// SeverityError marks a script that is guaranteed to fail, or to
	// silently produce a result other than what the sandbox itself would
	// ever return, once actually executed. A caller should reject a script
	// with any SeverityError finding rather than accept it for publish.
	SeverityError Severity = "ERROR"

	// SeverityWarning marks a script that will execute safely and
	// deterministically, but in a way its author likely does not expect. A
	// caller should surface a SeverityWarning finding to the author, but it
	// does not by itself justify rejecting the script.
	SeverityWarning Severity = "WARNING"
)

// Finding is one static-analysis result against a single script. Line and
// Column are both 1-based and refer to the analyzed source text; either may
// be zero when no meaningful position is available.
type Finding struct {
	Severity Severity
	Message  string
	Line     int
	Column   int
}

// Result is the outcome of validating one script.
type Result struct {
	Findings []Finding
}

// HasErrors reports whether Result contains any SeverityError finding - the
// caller-facing signal that this script must not be accepted as-is.
func (r Result) HasErrors() bool {
	for _, f := range r.Findings {
		if f.Severity == SeverityError {
			return true
		}
	}
	return false
}

// Validate statically analyzes source, an authored backend script's exact
// text, and returns every finding against it. A returned error means source
// failed to parse as JavaScript at all - a script in that state cannot
// produce meaningful findings and must be rejected regardless of Result.
func Validate(source string) (Result, error) {
	program, err := parser.ParseFile(nil, "script.js", source, 0)
	if err != nil {
		return Result{}, fmt.Errorf("script does not parse as JavaScript: %w", err)
	}

	bound := map[string]bool{}
	walk(program, func(name string) {
		bound[name] = true
	}, nil, nil)

	var findings []Finding
	walk(program, nil, func(name string, idx int) {
		findings = append(findings, identifierFindings(program, bound, name, idx)...)
	}, func(object, property string, idx int) {
		findings = append(findings, memberFindings(program, bound, object, property, idx)...)
	})

	return Result{Findings: findings}, nil
}
