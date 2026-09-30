package visibility

import (
	"bytes"
	"encoding/json"
	"testing"
)

// This file substantially hardens Filter's own verification beyond
// visibility_test.go's original coverage, per explicit human direction
// after independent review: Filter is the load-bearing privacy guarantee
// (unauthorized data is structurally unavailable to project() because it
// was never included in the first place, not merely hidden by the
// untrusted script's own choices) - its own test coverage must be
// exhaustive across nested structures, array-based collections, dynamic
// participant ownership with more than two participants, and every
// declassification primitive, and must verify structural absence at the
// raw encoded-bytes level, not only via Go map lookups.

// TestFilter_DeeplyNestedPrivateSubtreeAtomicallyDecided verifies a Rule
// matched several levels deep decides its entire subtree as one unit -
// nested containers/scalars several levels below the matched Path are
// included or excluded together, never individually re-decided.
func TestFilter_DeeplyNestedPrivateSubtreeAtomicallyDecided(t *testing.T) {
	schema := Schema{Rules: []Rule{
		{Path: "table.round.players.*.hand", Class: ClassPrivate},
	}}
	state := json.RawMessage(`{
		"table": {
			"round": {
				"players": {
					"p1": {"hand": {"cards": ["A", "K"], "meta": {"drawnAt": 3, "source": {"deck": "main"}}}},
					"p2": {"hand": {"cards": ["7", "2", "9"], "meta": {"drawnAt": 5, "source": {"deck": "main"}}}}
				}
			}
		}
	}`)

	owner, err := Filter(state, "p1", schema)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	var ownerParsed map[string]interface{}
	if err := json.Unmarshal(owner, &ownerParsed); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	hand := digMap(t, ownerParsed, "table", "round", "players", "p1", "hand")
	meta := digMap(t, hand, "meta")
	if meta["drawnAt"] != float64(3) {
		t.Fatalf("expected owner to see the whole nested subtree (meta.drawnAt), got %v", hand)
	}
	source := digMap(t, meta, "source")
	if source["deck"] != "main" {
		t.Fatalf("expected owner to see the deepest nested field (meta.source.deck), got %v", meta)
	}

	nonOwner, err := Filter(state, "p2", schema)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	var nonOwnerParsed map[string]interface{}
	if err := json.Unmarshal(nonOwner, &nonOwnerParsed); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	players := digMap(t, nonOwnerParsed, "table", "round", "players")
	if _, present := players["p1"]; present {
		t.Fatalf("expected p1's entire nested subtree absent for viewer p2, got %v", players)
	}
}

// TestFilter_ArrayIndexWildcardOwnership exercises the array-container code
// path (enumerateInstantiations/filterValue's []interface{} branches),
// which visibility_test.go's original map-keyed fixtures never touch: a
// participant collection addressed by array index rather than object key.
func TestFilter_ArrayIndexWildcardOwnership(t *testing.T) {
	schema := Schema{Rules: []Rule{
		{Path: "participants.*.hand", Class: ClassPrivate},
		{Path: "participants.*.name", Class: ClassPublic},
	}}
	state := json.RawMessage(`{
		"participants": [
			{"name": "Alice", "hand": ["A", "K"]},
			{"name": "Bob", "hand": ["7", "2", "9"]}
		]
	}`)

	// viewer "0" owns participants[0] - the array index is the wildcard's
	// own bound segment, the same identity concept an object key would be.
	out, err := Filter(state, "0", schema)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	participants, ok := parsed["participants"].([]interface{})
	if !ok {
		t.Fatalf("expected a participants array, got %v", parsed["participants"])
	}
	foundOwnHand := false
	for _, p := range participants {
		entry := p.(map[string]interface{})
		if entry["name"] == "Alice" {
			if entry["hand"] == nil {
				t.Fatalf("expected viewer 0 (Alice) to see her own hand, got %v", entry)
			}
			foundOwnHand = true
		}
		if entry["name"] == "Bob" && entry["hand"] != nil {
			t.Fatalf("expected Bob's hand absent for viewer 0, got %v", entry)
		}
	}
	if !foundOwnHand {
		t.Fatalf("expected to find viewer 0's own public name+hand in the output, got %v", participants)
	}
}

// TestFilter_MultiParticipantStrictCrossOwnerIsolation uses four
// participants and confirms every non-owning pairing is isolated, plus
// that a viewer identity that owns nothing at all (a spectator/unrelated
// caller, never a key in state) still gets a well-formed projection input
// containing only public data - never an error, never someone else's
// private data merely because it "found no owner to match."
func TestFilter_MultiParticipantStrictCrossOwnerIsolation(t *testing.T) {
	schema := Schema{Rules: []Rule{
		{Path: "players.*.hand", Class: ClassPrivate},
		{Path: "table", Class: ClassPublic},
	}}
	state := json.RawMessage(`{
		"players": {
			"p1": {"hand": ["A"]},
			"p2": {"hand": ["B"]},
			"p3": {"hand": ["C"]},
			"p4": {"hand": ["D"]}
		},
		"table": "public-table-state"
	}`)

	owners := []string{"p1", "p2", "p3", "p4"}
	for _, viewer := range owners {
		out, err := Filter(state, viewer, schema)
		if err != nil {
			t.Fatalf("Filter(%s): %v", viewer, err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(out, &parsed); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		players, _ := parsed["players"].(map[string]interface{})
		if players == nil || players[viewer] == nil {
			t.Fatalf("viewer %s must see its own hand, got %s", viewer, out)
		}
		if len(players) != 1 {
			t.Fatalf("viewer %s must see exactly its own entry, no other participant's, got %s", viewer, out)
		}
		for _, other := range owners {
			if other == viewer {
				continue
			}
			if players[other] != nil {
				t.Fatalf("viewer %s must never see %s's hand, got %s", viewer, other, out)
			}
		}
	}

	// A viewer who is nobody's owner at all - never a key anywhere in
	// state - still gets a valid projection input, with only public data,
	// never an error and never any participant's private hand.
	spectator, err := Filter(state, "spectator-not-in-state", schema)
	if err != nil {
		t.Fatalf("Filter(spectator): %v", err)
	}
	var spectatorParsed map[string]interface{}
	if err := json.Unmarshal(spectator, &spectatorParsed); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if spectatorParsed["table"] != "public-table-state" {
		t.Fatalf("expected spectator to see public data, got %s", spectator)
	}
	if _, present := spectatorParsed["players"]; present {
		t.Fatalf("expected spectator to see no players data at all, got %s", spectator)
	}
}

// TestFilter_ExcludedDataStructurallyAbsentFromRawBytes checks structural
// absence at the encoded-JSON-bytes level, not merely that a Go map lookup
// returns nil - the guarantee this package actually makes is that the
// value never reaches project() at all, so its own distinctive byte
// sequence must not appear anywhere in Filter's raw output, under any key
// name a bug might place it under.
func TestFilter_ExcludedDataStructurallyAbsentFromRawBytes(t *testing.T) {
	const distinctiveMarker = "zzz-unique-private-marker-9f3c2a-zzz"
	schema := Schema{Rules: []Rule{
		{Path: "players.*.secret", Class: ClassPrivate},
	}}
	state := json.RawMessage(`{"players": {"p1": {"secret": "mine"}, "p2": {"secret": "` + distinctiveMarker + `"}}}`)

	out, err := Filter(state, "p1", schema)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	if bytes.Contains(out, []byte(distinctiveMarker)) {
		t.Fatalf("p2's private value must be structurally absent from the raw encoded output, found it in %s", out)
	}
}

// TestFilter_NestedRulesAreRejected verifies a Schema declaring one Rule's
// Path nested inside another Rule's own Path is rejected outright, rather
// than accepted and left to degrade at Filter/LeakCheck time.
//
// filterValue decides a matched path's whole subtree atomically and never
// re-descends into it once a shallower Rule already matched. Left
// unvalidated, a nested Rule would be either silently unreachable (the
// "safe" direction: a more-restrictive nested Rule under a less-
// restrictive parent, still only over-exposing to the parent's own
// legitimate audience) or, in the dangerous direction this test also
// covers, silently override its own intended restriction across *every*
// viewer - a shallower Public/Private Rule matches first and returns its
// whole subtree verbatim without ever consulting a nested ServerOnly Rule
// meant to carve out part of it. An author needing finer control must
// declare Rules at the exact leaf/subtree granularity they want
// controlled, never nested inside another Rule's own Path.
func TestFilter_NestedRulesAreRejected(t *testing.T) {
	t.Run("safe_direction_private_parent_server_only_child", func(t *testing.T) {
		schema := Schema{Rules: []Rule{
			{Path: "players.*.hand", Class: ClassPrivate},
			{Path: "players.*.hand.secretSeed", Class: ClassServerOnly},
		}}
		if _, err := Filter(json.RawMessage(`{}`), "p1", schema); err == nil {
			t.Fatalf("expected Filter to reject a Schema with a Rule nested inside another Rule's own Path")
		}
	})

	t.Run("dangerous_direction_public_parent_server_only_child_would_leak_to_everyone", func(t *testing.T) {
		// If this were accepted, the shallower "table" Public Rule would
		// match first at path=["table"] and return the whole subtree
		// verbatim - including secretDeck - to every viewer, since
		// filterValue never re-descends to consult the nested, more
		// restrictive Rule at all.
		schema := Schema{Rules: []Rule{
			{Path: "table", Class: ClassPublic},
			{Path: "table.secretDeck", Class: ClassServerOnly},
		}}
		if _, err := Filter(json.RawMessage(`{}`), "anyone", schema); err == nil {
			t.Fatalf("expected Filter to reject a Schema where a broader Public Rule sits above a more-restrictive nested Rule")
		}
	})
}

// TestParseSchema_RejectsNestedRules is the same regression at the
// ParseSchema boundary, where an author's declared schema is first loaded.
func TestParseSchema_RejectsNestedRules(t *testing.T) {
	raw := json.RawMessage(`{"rules":[{"path":"table","class":"public"},{"path":"table.secretDeck","class":"server_only"}]}`)
	if _, err := ParseSchema(raw); err == nil {
		t.Fatalf("expected ParseSchema to reject nested Rule paths, got no error")
	}
}

// TestFilter_DeclassifiedRuleExemptFromNestingCheck verifies a
// ClassDeclassified Rule's own Path may coexist with a structural Rule at
// a nesting relationship that would otherwise be rejected - declassified
// Rules are synthesized separately (by direct path/value assignment, never
// matched during Filter's own structural walk) and so cannot participate
// in the shadowing hazard the nesting check above exists to catch.
func TestFilter_DeclassifiedRuleExemptFromNestingCheck(t *testing.T) {
	schema := Schema{Rules: []Rule{
		{Path: "players.*.hand", Class: ClassPrivate},
		{Path: "players.*.hand.count", Class: ClassDeclassified, Derive: DeriveCount, Source: "players.*.hand.cards"},
	}}
	state := json.RawMessage(`{"players": {"p1": {"hand": {"cards": ["A", "K"]}}}}`)

	if _, err := Filter(state, "someone-else", schema); err != nil {
		t.Fatalf("expected a declassified Rule nested under a structural Rule's own Path to be accepted, got: %v", err)
	}
}

// TestFilter_DeclassifiedFromNestedSource verifies a declassification Rule
// can derive from a Source several levels deep, not only a top-level path.
func TestFilter_DeclassifiedFromNestedSource(t *testing.T) {
	schema := Schema{Rules: []Rule{
		{Path: "players.*.hand", Class: ClassPrivate},
		{Path: "players.*.handCount", Class: ClassDeclassified, Derive: DeriveCount, Source: "players.*.hand.cards"},
	}}
	state := json.RawMessage(`{"players": {"p1": {"hand": {"cards": ["A","K"]}}, "p2": {"hand": {"cards": ["7","2","9"]}}}}`)

	out, err := Filter(state, "p1", schema)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	p2 := digMap(t, parsed, "players", "p2")
	if p2["handCount"] != float64(3) {
		t.Fatalf("expected p2.handCount == 3 derived from a nested Source, got %v", p2["handCount"])
	}
	if p2["hand"] != nil {
		t.Fatalf("expected p2's raw nested hand absent despite the declassified derivative, got %v", p2)
	}
}

// TestFilter_DeriveExistsPrimitive and TestFilter_DeriveRedactedPrimitive
// cover the two declassification primitives cardGameSchema's own tests
// never exercise (only DeriveCount is covered there).
//
// A declassification Rule is only synthesized for instantiations that
// actually resolve against Source in state (enumerateInstantiations' own
// documented behavior) - if the Source path is entirely absent (not merely
// falsy/empty), no derived field is added at all, rather than defaulting to
// false/0. This is a real, deliberate characteristic worth covering
// explicitly: it never leaks anything (an omitted field reveals nothing an
// author didn't already choose to omit), but an author relying on a
// declassified field always being present should know a not-yet-set
// private value produces no field at all, not a default value.
func TestFilter_DeriveExistsPrimitive(t *testing.T) {
	schema := Schema{Rules: []Rule{
		{Path: "players.*.hiddenCard", Class: ClassPrivate},
		{Path: "players.*.hasHiddenCard", Class: ClassDeclassified, Derive: DeriveExists, Source: "players.*.hiddenCard"},
	}}

	t.Run("present", func(t *testing.T) {
		out, err := Filter(json.RawMessage(`{"players": {"p1": {"hiddenCard": "A"}}}`), "someone-else", schema)
		if err != nil {
			t.Fatalf("Filter: %v", err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(out, &parsed); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		p1 := digMap(t, parsed, "players", "p1")
		if p1["hasHiddenCard"] != true {
			t.Fatalf("expected hasHiddenCard == true, got %v (output: %s)", p1["hasHiddenCard"], out)
		}
		if p1["hiddenCard"] != nil {
			t.Fatalf("expected the raw hiddenCard absent regardless, got %v", p1)
		}
	})

	t.Run("empty_string", func(t *testing.T) {
		out, err := Filter(json.RawMessage(`{"players": {"p1": {"hiddenCard": ""}}}`), "someone-else", schema)
		if err != nil {
			t.Fatalf("Filter: %v", err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(out, &parsed); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		p1 := digMap(t, parsed, "players", "p1")
		if p1["hasHiddenCard"] != false {
			t.Fatalf("expected hasHiddenCard == false for a present-but-empty source, got %v (output: %s)", p1["hasHiddenCard"], out)
		}
	})

	t.Run("source_entirely_absent_produces_no_field_at_all", func(t *testing.T) {
		out, err := Filter(json.RawMessage(`{"players": {"p1": {}}}`), "someone-else", schema)
		if err != nil {
			t.Fatalf("Filter: %v", err)
		}
		var parsed map[string]interface{}
		if err := json.Unmarshal(out, &parsed); err != nil {
			t.Fatalf("decoding: %v", err)
		}
		// p1 has no hiddenCard key and no other visible field, so it
		// never appears in the base filtered tree, and no instantiation
		// exists for the declassification pass to derive from either -
		// "players" itself is absent, not an object with a false-valued
		// hasHiddenCard. Safe (nothing leaked), just not a defaulted field.
		if _, present := parsed["players"]; present {
			t.Fatalf("expected no players entry at all when the source is entirely absent, got %s", out)
		}
	})
}

func TestFilter_DeriveRedactedPrimitive(t *testing.T) {
	schema := Schema{Rules: []Rule{
		{Path: "players.*.secretMessage", Class: ClassPrivate},
		{Path: "players.*.hasMessage", Class: ClassDeclassified, Derive: DeriveRedacted, Source: "players.*.secretMessage"},
	}}
	state := json.RawMessage(`{"players": {"p1": {"secretMessage": "meet me at dawn"}}}`)

	out, err := Filter(state, "someone-else", schema)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	p1 := digMap(t, parsed, "players", "p1")
	if p1["hasMessage"] != redactedPlaceholder {
		t.Fatalf("expected the fixed redacted placeholder, got %v", p1["hasMessage"])
	}
	if bytes.Contains(out, []byte("meet me at dawn")) {
		t.Fatalf("the redacted primitive must never leak the underlying content, got %s", out)
	}
}

// TestParseSchema_RejectsDeclassifiedSourceNotDeclaredPrivate verifies a
// ClassDeclassified Rule's own Source must resolve to data explicitly
// declared ClassPrivate - never server_only data, and never a path no Rule
// declares at all (which falls back to Default and carries no per-viewer
// ownership concept regardless of what Default itself is set to).
// applyDeclassifiedRule reads Source directly against the original,
// unfiltered state, entirely bypassing whatever class governs that path
// during the structural walk - found during independent review, not
// anticipated upfront.
func TestParseSchema_RejectsDeclassifiedSourceNotDeclaredPrivate(t *testing.T) {
	t.Run("source_is_server_only", func(t *testing.T) {
		raw := json.RawMessage(`{"rules":[
			{"path":"adminSecret","class":"server_only"},
			{"path":"adminSecretExists","class":"declassified","derive":"exists","source":"adminSecret"}
		]}`)
		if _, err := ParseSchema(raw); err == nil {
			t.Fatalf("expected ParseSchema to reject a declassified Rule deriving from server_only data")
		}
	})

	t.Run("source_is_undeclared_falls_to_default", func(t *testing.T) {
		raw := json.RawMessage(`{"rules":[
			{"path":"somethingExists","class":"declassified","derive":"exists","source":"undeclaredField"}
		]}`)
		if _, err := ParseSchema(raw); err == nil {
			t.Fatalf("expected ParseSchema to reject a declassified Rule deriving from an undeclared path")
		}
	})
}

// TestFilter_DeclassifiedSourceNotDeclaredPrivateFailsClosed mirrors the
// ParseSchema-level regression above, but proves Filter/LeakCheck
// themselves independently re-validate too (the same defense-in-depth
// pattern already established for the earlier fixes), against the exact
// reproduction a fresh reviewer constructed: a schema using only
// recognized Class values that would otherwise expose server_only data to
// every viewer through a declassified derivative.
func TestFilter_DeclassifiedSourceNotDeclaredPrivateFailsClosed(t *testing.T) {
	schema := Schema{Rules: []Rule{
		{Path: "adminSecret", Class: ClassServerOnly},
		{Path: "adminSecretExists", Class: ClassDeclassified, Derive: DeriveExists, Source: "adminSecret"},
	}}
	state := json.RawMessage(`{"adminSecret": "top-secret"}`)

	if _, err := Filter(state, "anyone", schema); err == nil {
		t.Fatalf("expected Filter to reject a declassified Rule deriving from server_only data")
	}
}

// TestParseSchema_RejectsDuplicatePath verifies two structural Rules
// declaring the exact same Path are rejected, rather than silently
// resolved by declaration order (matchRule always returns the first
// match) - the same class of ambiguous-schema hazard as the nesting
// rejection above, just at zero distance instead of a proper prefix.
func TestParseSchema_RejectsDuplicatePath(t *testing.T) {
	raw := json.RawMessage(`{"rules":[
		{"path":"secret","class":"public"},
		{"path":"secret","class":"server_only"}
	]}`)
	if _, err := ParseSchema(raw); err == nil {
		t.Fatalf("expected ParseSchema to reject two Rules declaring the exact same Path")
	}
}

// TestFilter_DeclassifiedThroughArrayDoesNotDestroySiblingData verifies a
// declassified Rule whose Path traverses a container the structural pass
// already built as a JSON array does not destroy that array's own
// previously-included contents. setByPath only ever descends into
// existing map containers; encountering the array instead, it must skip
// writing the declassified value for that instantiation rather than
// overwrite the array with a fresh empty map - found during independent
// review, not anticipated upfront.
func TestFilter_DeclassifiedThroughArrayDoesNotDestroySiblingData(t *testing.T) {
	schema := Schema{Rules: []Rule{
		{Path: "participants.*.hand", Class: ClassPrivate},
		{Path: "participants.*.name", Class: ClassPublic},
		{Path: "participants.*.handCount", Class: ClassDeclassified, Derive: DeriveCount, Source: "participants.*.hand"},
	}}
	state := json.RawMessage(`{
		"participants": [
			{"name": "Alice", "hand": ["A", "K"]},
			{"name": "Bob", "hand": ["7", "2", "9"]}
		]
	}`)

	out, err := Filter(state, "0", schema)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	participants, ok := parsed["participants"].([]interface{})
	if !ok {
		t.Fatalf("expected the participants array to survive the declassification pass intact, got %v", parsed["participants"])
	}
	if len(participants) != 2 {
		t.Fatalf("expected both participants' own public data to survive, got %d entries: %v", len(participants), participants)
	}
	foundAlice, foundBob := false, false
	for _, p := range participants {
		entry := p.(map[string]interface{})
		// The declassified handCount is never added here - setByPath
		// cannot address into an existing array (a documented v1
		// restriction), so it safely skips the write rather than
		// destroying the array. This is the accepted trade-off, not an
		// oversight: verified explicitly rather than merely by omission.
		if entry["handCount"] != nil {
			t.Fatalf("expected handCount to be safely skipped (unsupported array traversal), not silently added, got %v", entry)
		}
		switch entry["name"] {
		case "Alice":
			foundAlice = true
			if entry["hand"] == nil {
				t.Fatalf("expected viewer 0 (Alice) to still see her own hand, got %v", entry)
			}
		case "Bob":
			foundBob = true
			if entry["hand"] != nil {
				t.Fatalf("expected Bob's hand to remain excluded, got %v", entry)
			}
		}
	}
	if !foundAlice || !foundBob {
		t.Fatalf("expected both Alice's and Bob's public entries to survive, got %v", participants)
	}
}

// TestParseSchema_RejectsMultipleWildcardsInPathOrSource verifies a Path
// or Source with more than one "*" is rejected outright, rather than
// accepted and left to matchTemplate to silently bind ownership to only
// the last wildcard's own segment - found during independent review, not
// anticipated upfront, and directly relevant to this codebase's own real
// viewer identity shape (a small decimal actor id, indistinguishable in
// form from a JSON array index - see
// TestFilter_MultipleWildcardsFailClosedRatherThanMisattributeOwnership
// below for the concrete ownership-inversion this guards against).
func TestParseSchema_RejectsMultipleWildcardsInPathOrSource(t *testing.T) {
	t.Run("path", func(t *testing.T) {
		raw := json.RawMessage(`{"rules":[{"path":"players.*.privateLog.*.text","class":"private"}]}`)
		if _, err := ParseSchema(raw); err == nil {
			t.Fatalf("expected ParseSchema to reject a Path with more than one wildcard")
		}
	})

	t.Run("source", func(t *testing.T) {
		raw := json.RawMessage(`{"rules":[
			{"path":"players.*.hand","class":"private"},
			{"path":"players.*.handCount","class":"declassified","derive":"count","source":"players.*.hand.*.nested"}
		]}`)
		if _, err := ParseSchema(raw); err == nil {
			t.Fatalf("expected ParseSchema to reject a Source with more than one wildcard")
		}
	})
}

// TestFilter_MultipleWildcardsFailsClosed proves Filter itself
// independently rejects a 2+-wildcard Rule (the same defense-in-depth
// layering as every other fix in this file), against a Schema built
// directly in Go rather than through ParseSchema.
func TestFilter_MultipleWildcardsFailsClosed(t *testing.T) {
	schema := Schema{Rules: []Rule{
		{Path: "players.*.privateLog.*", Class: ClassPrivate},
	}}
	state := json.RawMessage(`{"players": {"7": {"privateLog": ["first entry", "second entry"]}}}`)

	if _, err := Filter(state, "7", schema); err == nil {
		t.Fatalf("expected Filter to reject a Rule with more than one wildcard")
	}
}

// TestMatchTemplate_MultipleWildcardsFailsClosed unit-tests the deeper
// defense-in-depth backstop directly: Schema.validate rejects a 2+-
// wildcard Rule before Filter/LeakCheck ever reach matchTemplate at all,
// so this can only be exercised by calling matchTemplate itself (an
// internal caller bypassing Filter's own entry-point validation, the same
// hypothetical this package's other fail-closed backstops guard against).
// Proves the concrete ownership-inversion a naive "last wildcard wins"
// implementation would cause: player "7" owns a privateLog array whose
// second entry (index "1") shares its own array-index shape with a real,
// distinct viewer identity ("1") - the exact hazard this codebase's own
// real viewer format (a small decimal actor id) makes concrete. Rather
// than silently binding ownership to that trailing array index,
// matchTemplate must refuse to match a 2+-wildcard template at all.
func TestMatchTemplate_MultipleWildcardsFailsClosed(t *testing.T) {
	bound, ok := matchTemplate(
		[]string{"players", wildcard, "privateLog", wildcard},
		[]string{"players", "7", "privateLog", "1"},
	)
	if ok {
		t.Fatalf("expected matchTemplate to refuse to match a template with more than one wildcard, got bound=%q ok=%v", bound, ok)
	}
}

// TestParseSchema_RejectsWildcardSourceCoveredOnlyByConcretePrivateRule
// verifies a declassified Rule's own Source with a wildcard is not
// wrongly treated as "covered" by a ClassPrivate Rule whose own Path is
// concrete (no wildcard) at that same position - found during independent
// review, not anticipated upfront. A concrete private.Path only ever
// governs a single instantiation; a wildcarded Source ranges over every
// matching key/index enumerateInstantiations finds in real state, so a
// concrete Rule can never verifiably cover it.
func TestParseSchema_RejectsWildcardSourceCoveredOnlyByConcretePrivateRule(t *testing.T) {
	raw := json.RawMessage(`{"rules":[
		{"path":"players.0.hand","class":"private"},
		{"path":"players.*.handCount","class":"declassified","derive":"count","source":"players.*.hand"}
	], "default":"server_only"}`)
	if _, err := ParseSchema(raw); err == nil {
		t.Fatalf("expected ParseSchema to reject a wildcarded Source only covered by a concrete private Rule")
	}
}

// TestFilter_WildcardSourceCoveredOnlyByConcretePrivateRuleFailsClosed
// mirrors the ParseSchema-level regression above, but proves Filter
// itself independently rejects it too, against the reviewer's own exact
// reproduction: without this fix, viewer "1" (who owns nothing declared
// private at all - player 1's own hand falls to Default: server_only)
// would still receive a declassified handCount derived from data no Rule
// ever verifiably declared private for that instantiation.
func TestFilter_WildcardSourceCoveredOnlyByConcretePrivateRuleFailsClosed(t *testing.T) {
	schema := Schema{
		Rules: []Rule{
			{Path: "players.0.hand", Class: ClassPrivate},
			{Path: "players.*.handCount", Class: ClassDeclassified, Derive: DeriveCount, Source: "players.*.hand"},
		},
		Default: ClassServerOnly,
	}
	state := json.RawMessage(`{"players": [{"hand": ["A","K"]}, {"hand": ["Q","J","10"]}]}`)

	if _, err := Filter(state, "1", schema); err == nil {
		t.Fatalf("expected Filter to reject a wildcarded Source only covered by a concrete private Rule")
	}
}

// TestFilter_ConcreteSourceMatchingConcretePrivatePathIsAccepted closes a
// coverage gap independent review flagged as NON_BLOCKING: a fully
// concrete (wildcard-free) private Rule Path and an identically concrete
// declassified Source naming the exact same path is a legitimate,
// accepted case - no wildcard is required on either side for
// privateRuleCoversEverySourceInstantiation to recognize coverage.
func TestFilter_ConcreteSourceMatchingConcretePrivatePathIsAccepted(t *testing.T) {
	schema := Schema{Rules: []Rule{
		{Path: "players.7.hand", Class: ClassPrivate},
		{Path: "players.7.handCount", Class: ClassDeclassified, Derive: DeriveCount, Source: "players.7.hand"},
	}}
	state := json.RawMessage(`{"players": {"7": {"hand": ["A", "K"]}}}`)

	out, err := Filter(state, "someone-else", schema)
	if err != nil {
		t.Fatalf("Filter: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	p7 := digMap(t, parsed, "players", "7")
	if p7["handCount"] != float64(2) {
		t.Fatalf("expected players.7.handCount == 2, got %v", p7["handCount"])
	}
	if p7["hand"] != nil {
		t.Fatalf("expected the raw hand absent for a non-owning viewer, got %v", p7)
	}
}

// digMap walks a chain of map keys, failing the test immediately with a
// clear path if any segment is missing or not itself a map - avoids
// repetitive nil-checking boilerplate across the tests above.
func digMap(t *testing.T, root map[string]interface{}, path ...string) map[string]interface{} {
	t.Helper()
	cur := root
	for i, seg := range path {
		next, ok := cur[seg].(map[string]interface{})
		if !ok {
			t.Fatalf("expected path %v to resolve to a map at segment %q (index %d), got %v", path, seg, i, cur[seg])
		}
		cur = next
	}
	return cur
}
