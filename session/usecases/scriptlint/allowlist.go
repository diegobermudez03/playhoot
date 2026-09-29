package scriptlint

import (
	"fmt"

	"github.com/dop251/goja/ast"
)

// allowedGlobals is every identifier this package treats as part of the
// sandbox's actual supported global surface for a backend script: every
// standard ECMAScript intrinsic the sandbox exposes unmodified or safely
// substitutes, plus the small set of de facto conveniences (console) that
// execute harmlessly even though they have no observable effect. Anything
// not in this set is either genuinely absent (any reference throws at
// runtime) or a residual sandbox-implementation-specific global this
// package deliberately treats the same way (std, os, performance, and
// similar), since a backend script has no legitimate reason to depend on
// any of them.
//
// This list must be kept consistent with the sandbox's own actual
// behavior, recorded at session/jsexecutor/internal/sandbox/LOGICAL_CONTRACT.md
// (its Isolation Guarantees section) - that document, not this one, is the
// source of truth for what the sandbox itself does; this list only encodes
// a policy decision over that surface.
var allowedGlobals = map[string]bool{
	"AggregateError": true, "Array": true, "ArrayBuffer": true,
	"BigInt": true, "BigInt64Array": true, "BigUint64Array": true, "Boolean": true,
	"DataView": true, "Date": true,
	"Error": true, "EvalError": true,
	"FinalizationRegistry": true, "Float16Array": true, "Float32Array": true, "Float64Array": true, "Function": true,
	"Infinity": true, "Int16Array": true, "Int32Array": true, "Int8Array": true, "InternalError": true, "Iterator": true,
	"JSON": true,
	"Map": true, "Math": true,
	"NaN": true, "Number": true,
	"Object": true,
	"Promise": true, "Proxy": true,
	"RangeError": true, "ReferenceError": true, "Reflect": true, "RegExp": true,
	"Set": true, "SharedArrayBuffer": true, "String": true, "Symbol": true, "SyntaxError": true,
	"TypeError": true,
	"URIError": true, "Uint16Array": true, "Uint32Array": true, "Uint8Array": true, "Uint8ClampedArray": true,
	"WeakMap": true, "WeakRef": true, "WeakSet": true,
	"clearInterval": true, "clearTimeout": true, "console": true,
	"decodeURI": true, "decodeURIComponent": true,
	"encodeURI": true, "encodeURIComponent": true, "escape": true, "eval": true,
	"globalThis": true,
	"isFinite": true, "isNaN": true,
	"parseFloat": true, "parseInt": true,
	"queueMicrotask": true,
	"setInterval": true, "setTimeout": true,
	"undefined": true, "unescape": true,
}

// misleadingIdentifiers maps a bare identifier reference (not a property
// name) to a finding message: safe to execute, but the sandbox's behavior
// for it differs from what an author unfamiliar with the sandbox's
// single-shot, synchronous execution model would expect.
var misleadingIdentifiers = map[string]string{
	"Date": "Date is not the real wall clock: the sandbox replaces it with a value derived from this execution's own logical time, so authored code cannot observe real elapsed time even indirectly.",
	"setTimeout":     "setTimeout is accepted by the sandbox's JavaScript environment but its callback never runs: execute()'s return value is captured synchronously and the sandbox's runtime closes immediately afterward.",
	"setInterval":    "setInterval is accepted by the sandbox's JavaScript environment but its callback never runs: execute()'s return value is captured synchronously and the sandbox's runtime closes immediately afterward.",
	"queueMicrotask": "queueMicrotask is accepted by the sandbox's JavaScript environment but its callback never runs: execute()'s return value is captured synchronously and the sandbox's runtime closes immediately afterward.",
}

// misleadingMember maps a "object.property" member access to a finding
// message, for cases where only one specific property of an otherwise
// unremarkable global is actually misleading.
var misleadingMember = map[[2]string]string{
	{"Math", "random"}: "Math.random() does not return real randomness: the sandbox replaces it with a value deterministically derived from this execution's own random seed.",
}

// identifierFindings classifies one free (unbound, non-property-name)
// identifier reference found at source position idx.
func identifierFindings(program *ast.Program, bound map[string]bool, name string, idx int) []Finding {
	if bound[name] {
		return nil
	}

	line, column := position(program, idx)
	var findings []Finding

	if !allowedGlobals[name] {
		findings = append(findings, Finding{
			Severity: SeverityError,
			Message:  fmt.Sprintf("reference to %q, which is not part of the sandbox's supported API surface for backend scripts", name),
			Line:     line,
			Column:   column,
		})
		return findings
	}

	if msg, ok := misleadingIdentifiers[name]; ok {
		findings = append(findings, Finding{Severity: SeverityWarning, Message: msg, Line: line, Column: column})
	}
	return findings
}

// memberFindings classifies a "object.property" or "object['property']"
// member access, where object is a free (unbound) identifier reference.
// object == "globalThis" is treated as exactly equivalent to a direct
// reference to property, since that is the only way this package's
// allow-list check could otherwise be defeated by a completely static,
// directly-visible expression - unlike eval or another genuinely dynamic
// indirection, "globalThis.<name>" resolves to a fixed name with no
// execution required to know it.
func memberFindings(program *ast.Program, bound map[string]bool, object, property string, idx int) []Finding {
	if bound[object] {
		return nil
	}
	if object == "globalThis" {
		return identifierFindings(program, bound, property, idx)
	}
	msg, ok := misleadingMember[[2]string{object, property}]
	if !ok {
		return nil
	}
	line, column := position(program, idx)
	return []Finding{{Severity: SeverityWarning, Message: msg, Line: line, Column: column}}
}

func position(program *ast.Program, idx int) (line, column int) {
	if program.File == nil {
		return 0, 0
	}
	pos := program.File.Position(idx)
	return pos.Line, pos.Column
}
