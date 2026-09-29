package scriptlint

import (
	"testing"
)

func findingMessages(t *testing.T, result Result, severity Severity) []string {
	t.Helper()
	var messages []string
	for _, f := range result.Findings {
		if f.Severity == severity {
			messages = append(messages, f.Message)
			if f.Line <= 0 || f.Column <= 0 {
				t.Errorf("finding %q has no usable source position: line=%d column=%d", f.Message, f.Line, f.Column)
			}
		}
	}
	return messages
}

func TestValidate_CleanScriptProducesNoFindings(t *testing.T) {
	script := `
	function execute(previousState, event, context) {
		const counter = (previousState && previousState.counter) || 0;
		const { amount, kind } = event;
		const commands = [];
		for (let i = 0; i < amount; i++) {
			commands.push({ type: "noop", index: i });
		}
		try {
			JSON.stringify(previousState);
		} catch (err) {
			commands.push({ type: "error", message: String(err) });
		}
		class Ledger {
			constructor(total) { this.total = total; }
			add(n) { return this.total + n; }
		}
		const ledger = new Ledger(counter);
		const next = ledger.add(amount);
		const label = ` + "`turn-${next}`" + `;
		return {
			newState: { counter: next, actingActor: context.actingActor, label, kind },
			requestedCommands: commands,
		};
	}
	`
	result, err := Validate(script)
	if err != nil {
		t.Fatalf("Validate returned an error for valid JavaScript: %v", err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("expected zero findings, got %+v", result.Findings)
	}
}

func TestValidate_BannedGlobalsProduceErrorFindings(t *testing.T) {
	cases := []struct {
		name   string
		script string
	}{
		{"fetch", `function execute(p, e, c) { fetch("https://example.com"); return { newState: {}, requestedCommands: [] }; }`},
		{"require", `function execute(p, e, c) { const m = require("fs"); return { newState: {}, requestedCommands: [] }; }`},
		{"process", `function execute(p, e, c) { const v = process.env.SECRET; return { newState: {}, requestedCommands: [] }; }`},
		{"fs", `function execute(p, e, c) { fs.readFileSync("/etc/passwd"); return { newState: {}, requestedCommands: [] }; }`},
		{"XMLHttpRequest", `function execute(p, e, c) { new XMLHttpRequest(); return { newState: {}, requestedCommands: [] }; }`},
		{"WebSocket", `function execute(p, e, c) { new WebSocket("wss://example.com"); return { newState: {}, requestedCommands: [] }; }`},
		{"std", `function execute(p, e, c) { std.open("/etc/passwd", "r"); return { newState: {}, requestedCommands: [] }; }`},
		{"os", `function execute(p, e, c) { os.exec(["ls"]); return { newState: {}, requestedCommands: [] }; }`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Validate(tc.script)
			if err != nil {
				t.Fatalf("Validate returned an unexpected parse error: %v", err)
			}
			errs := findingMessages(t, result, SeverityError)
			if len(errs) != 1 {
				t.Fatalf("expected exactly one ERROR finding referencing %q, got %+v", tc.name, result.Findings)
			}
		})
	}
}

func TestValidate_MathRandomAndDateProduceWarningFindings(t *testing.T) {
	cases := []struct {
		name   string
		script string
	}{
		{"Math.random", `function execute(p, e, c) { const r = Math.random(); return { newState: { r }, requestedCommands: [] }; }`},
		{"new Date", `function execute(p, e, c) { const d = new Date(); return { newState: { d }, requestedCommands: [] }; }`},
		{"Date.now", `function execute(p, e, c) { const t = Date.now(); return { newState: { t }, requestedCommands: [] }; }`},
		{"bare Date call", `function execute(p, e, c) { const t = Date(); return { newState: { t }, requestedCommands: [] }; }`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Validate(tc.script)
			if err != nil {
				t.Fatalf("Validate returned an unexpected parse error: %v", err)
			}
			if result.HasErrors() {
				t.Fatalf("expected no ERROR findings, got %+v", result.Findings)
			}
			warnings := findingMessages(t, result, SeverityWarning)
			if len(warnings) != 1 {
				t.Fatalf("expected exactly one WARNING finding, got %+v", result.Findings)
			}
		})
	}
}

func TestValidate_MathNonRandomUsageProducesNoFinding(t *testing.T) {
	script := `function execute(p, e, c) { const v = Math.floor(Math.PI * 2); return { newState: { v }, requestedCommands: [] }; }`
	result, err := Validate(script)
	if err != nil {
		t.Fatalf("Validate returned an unexpected parse error: %v", err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("expected zero findings for non-random Math usage, got %+v", result.Findings)
	}
}

func TestValidate_SchedulingAPIsProduceWarningFindings(t *testing.T) {
	cases := []string{"setTimeout", "setInterval", "queueMicrotask"}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			script := `function execute(p, e, c) { ` + name + `(function() {}, 0); return { newState: {}, requestedCommands: [] }; }`
			result, err := Validate(script)
			if err != nil {
				t.Fatalf("Validate returned an unexpected parse error: %v", err)
			}
			if result.HasErrors() {
				t.Fatalf("expected no ERROR findings, got %+v", result.Findings)
			}
			warnings := findingMessages(t, result, SeverityWarning)
			if len(warnings) != 1 {
				t.Fatalf("expected exactly one WARNING finding, got %+v", result.Findings)
			}
		})
	}
}

func TestValidate_GlobalThisIndirectionProducesErrorFindings(t *testing.T) {
	cases := []struct {
		name   string
		script string
	}{
		{"dot access", `function execute(p, e, c) { globalThis.fetch("https://example.com"); return { newState: {}, requestedCommands: [] }; }`},
		{"bracket access", `function execute(p, e, c) { globalThis["process"].env; return { newState: {}, requestedCommands: [] }; }`},
		{"call via bracket", `function execute(p, e, c) { globalThis["require"]("fs"); return { newState: {}, requestedCommands: [] }; }`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := Validate(tc.script)
			if err != nil {
				t.Fatalf("Validate returned an unexpected parse error: %v", err)
			}
			errs := findingMessages(t, result, SeverityError)
			if len(errs) != 1 {
				t.Fatalf("expected exactly one ERROR finding, got %+v", result.Findings)
			}
		})
	}
}

func TestValidate_GlobalThisAccessToSupportedGlobalProducesNoErrorFinding(t *testing.T) {
	script := `function execute(p, e, c) { const v = globalThis.Math.floor(1.5); return { newState: { v }, requestedCommands: [] }; }`
	result, err := Validate(script)
	if err != nil {
		t.Fatalf("Validate returned an unexpected parse error: %v", err)
	}
	if result.HasErrors() {
		t.Fatalf("expected no ERROR findings for globalThis.Math, got %+v", result.Findings)
	}
}

func TestValidate_BracketNotationMathRandomProducesWarningFinding(t *testing.T) {
	script := `function execute(p, e, c) { const r = Math["random"](); return { newState: { r }, requestedCommands: [] }; }`
	result, err := Validate(script)
	if err != nil {
		t.Fatalf("Validate returned an unexpected parse error: %v", err)
	}
	if result.HasErrors() {
		t.Fatalf("expected no ERROR findings, got %+v", result.Findings)
	}
	warnings := findingMessages(t, result, SeverityWarning)
	if len(warnings) != 1 {
		t.Fatalf("expected exactly one WARNING finding, got %+v", result.Findings)
	}
}

func TestValidate_TaggedTemplateWithBannedGlobalProducesErrorFinding(t *testing.T) {
	script := "function execute(p, e, c) { const r = fetch`https://example.com`; return { newState: {}, requestedCommands: [] }; }"
	result, err := Validate(script)
	if err != nil {
		t.Fatalf("Validate returned an unexpected parse error: %v", err)
	}
	errs := findingMessages(t, result, SeverityError)
	if len(errs) != 1 {
		t.Fatalf("expected exactly one ERROR finding for a banned tagged-template tag, got %+v", result.Findings)
	}
}

func TestValidate_LocalNameShadowingABannedGlobalIsNotFlagged(t *testing.T) {
	script := `
	function execute(previousState, event, context) {
		function fetch(url) { return { url }; }
		const result = fetch("internal://noop");
		return { newState: result, requestedCommands: [] };
	}
	`
	result, err := Validate(script)
	if err != nil {
		t.Fatalf("Validate returned an unexpected parse error: %v", err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("expected zero findings when the name is locally declared, got %+v", result.Findings)
	}
}

func TestValidate_SyntacticallyInvalidScriptReturnsError(t *testing.T) {
	_, err := Validate(`function execute(p, e, c) { return { `)
	if err == nil {
		t.Fatal("expected a parse error for syntactically invalid source")
	}
}

func TestValidate_NeverExecutesTheScript(t *testing.T) {
	script := `
	throw new Error("Validate must never execute this");
	function execute(p, e, c) { return { newState: {}, requestedCommands: [] }; }
	`
	if _, err := Validate(script); err != nil {
		t.Fatalf("Validate returned an unexpected parse error: %v", err)
	}
}
