package visibility

import (
	"encoding/json"
	"testing"
)

// cardGameSchema mirrors a typical two-player card game's own declared
// shape: each player's own hand is private to them; a handCount derivative
// is declassified so an opponent's card count is visible without their
// actual cards; pile is public; secretDeck is server_only (never exposed
// to any viewer, including a script computing its own project()); every
// other path defaults to ClassServerOnly (the safe default).
var cardGameSchema = Schema{
	Rules: []Rule{
		{Path: "players.*.hand", Class: ClassPrivate},
		{Path: "players.*.handCount", Class: ClassDeclassified, Derive: DeriveCount, Source: "players.*.hand"},
		{Path: "pile", Class: ClassPublic},
		{Path: "secretDeck", Class: ClassServerOnly},
	},
}

const cardGameState = `{
	"players": {
		"p1": {"hand": ["A", "K"]},
		"p2": {"hand": ["7", "2", "9"]}
	},
	"pile": ["3", "4"],
	"secretDeck": ["Q", "J", "10"]
}`

func TestFilter_PrivateOnlyVisibleToOwner(t *testing.T) {
	out, err := Filter(json.RawMessage(cardGameState), "p1", cardGameSchema)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("decoding filtered output: %v", err)
	}

	players, _ := parsed["players"].(map[string]interface{})
	if players == nil {
		t.Fatalf("expected players in output, got %s", out)
	}
	p1, _ := players["p1"].(map[string]interface{})
	if p1 == nil || p1["hand"] == nil {
		t.Fatalf("expected p1's own hand present, got %s", out)
	}
	p2, _ := players["p2"].(map[string]interface{})
	if p2 != nil && p2["hand"] != nil {
		t.Fatalf("p2's hand must never appear in p1's projection input, got %s", out)
	}
}

func TestFilter_DeclassifiedDerivativeVisibleWithoutRawPrivateValue(t *testing.T) {
	out, err := Filter(json.RawMessage(cardGameState), "p1", cardGameSchema)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("decoding filtered output: %v", err)
	}

	players, _ := parsed["players"].(map[string]interface{})
	p2, _ := players["p2"].(map[string]interface{})
	if p2 == nil {
		t.Fatalf("expected p2 present (for its declassified handCount), got %s", out)
	}
	count, ok := p2["handCount"].(float64)
	if !ok || count != 3 {
		t.Fatalf("expected p2.handCount == 3, got %v (output: %s)", p2["handCount"], out)
	}
	if p2["hand"] != nil {
		t.Fatalf("p2's raw hand must never accompany its declassified handCount, got %s", out)
	}
}

func TestFilter_PublicAlwaysVisible(t *testing.T) {
	out, err := Filter(json.RawMessage(cardGameState), "p2", cardGameSchema)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("decoding filtered output: %v", err)
	}
	pile, ok := parsed["pile"].([]interface{})
	if !ok || len(pile) != 2 {
		t.Fatalf("expected public pile with 2 elements, got %v", parsed["pile"])
	}
}

func TestFilter_ServerOnlyNeverVisibleToAnyViewer(t *testing.T) {
	for _, viewer := range []string{"p1", "p2", "someone-else"} {
		out, err := Filter(json.RawMessage(cardGameState), viewer, cardGameSchema)
		if err != nil {
			t.Fatalf("Filter(%s): %v", viewer, err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(out, &parsed); err != nil {
			t.Fatalf("decoding filtered output: %v", err)
		}
		if _, present := parsed["secretDeck"]; present {
			t.Fatalf("secretDeck must never appear for viewer %s, got %s", viewer, out)
		}
	}
}

func TestFilter_UndeclaredPathDefaultsToExcluded(t *testing.T) {
	state := `{"players": {"p1": {"hand": ["A"]}}, "undeclaredDebugField": "should never leak"}`
	out, err := Filter(json.RawMessage(state), "p1", cardGameSchema)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("decoding filtered output: %v", err)
	}
	if _, present := parsed["undeclaredDebugField"]; present {
		t.Fatalf("an undeclared path must default to excluded (ClassServerOnly), got %s", out)
	}
}

func TestLeakCheck_FlagsVerbatimPrivateLeak(t *testing.T) {
	// A buggy project() that copies an opponent's whole hand verbatim into
	// its own output - the common "forgot to filter" mistake this check
	// exists to catch.
	buggyClientState := `{"myHand": ["A", "K"], "opponentHand": ["7", "2", "9"]}`

	findings, err := LeakCheck(json.RawMessage(cardGameState), "p1", json.RawMessage(buggyClientState), cardGameSchema)
	if err != nil {
		t.Fatalf("LeakCheck: %v", err)
	}
	if len(findings) == 0 {
		t.Fatalf("expected LeakCheck to flag p2's verbatim hand leaking into p1's client state")
	}
}

func TestLeakCheck_DoesNotFlagLegitimateDeclassifiedDerivative(t *testing.T) {
	// A correct project() that only exposes the declared handCount
	// derivative - LeakCheck must not treat this legitimate dependency on
	// private data as a leak merely because it varies with the private
	// hand's own size.
	correctClientState := `{"myHand": ["A", "K"], "opponentHandCount": 3}`

	findings, err := LeakCheck(json.RawMessage(cardGameState), "p1", json.RawMessage(correctClientState), cardGameSchema)
	if err != nil {
		t.Fatalf("LeakCheck: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected no findings for a legitimate declassified derivative, got %v", findings)
	}
}

func TestLeakCheck_IgnoresShortScalarCoincidence(t *testing.T) {
	// A trivially small shared value (a lone digit-like short string) must
	// not be flagged - see minLeakCheckBytes's own doc comment.
	state := `{"players": {"p1": {"hand": ["A"]}, "p2": {"secret": "9"}}}`
	schema := Schema{Rules: []Rule{
		{Path: "players.*.hand", Class: ClassPrivate},
		{Path: "players.*.secret", Class: ClassPrivate},
	}}
	clientState := `{"myHand": ["A"], "unrelatedPublicNumber": "9"}`

	findings, err := LeakCheck(json.RawMessage(state), "p1", json.RawMessage(clientState), schema)
	if err != nil {
		t.Fatalf("LeakCheck: %v", err)
	}
	if len(findings) != 0 {
		t.Fatalf("expected short scalar coincidence to be ignored, got %v", findings)
	}
}

func TestParseSchema_EmptyIsMaximallyRestrictive(t *testing.T) {
	schema, err := ParseSchema(nil)
	if err != nil {
		t.Fatalf("ParseSchema(nil): %v", err)
	}
	out, err := Filter(json.RawMessage(cardGameState), "p1", schema)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("decoding filtered output: %v", err)
	}
	if len(parsed) != 0 {
		t.Fatalf("expected an empty schema to exclude everything, got %s", out)
	}
}
