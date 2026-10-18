package sandbox

import (
	"encoding/binary"
	"fmt"
)

// determinismPrelude is evaluated before authored code. It removes the host
// helpers QuickJS installs, replaces Date and Math.random with deterministic
// ones, and evaluates to the object the worker controls them through, which
// authored code can never reach. Math.random is mulberry32, whose whole state
// is one 32-bit integer, so it can be exported and restored exactly.
const determinismPrelude = `(function() {
	var hostHelpers = ["std", "os", "bjson", "print", "console", "scriptArgs", "navigator", "performance",
		"setTimeout", "clearTimeout", "setInterval", "clearInterval", "queueMicrotask", "Worker"];
	for (var i = 0; i < hostHelpers.length; i++) {
		delete globalThis[hostHelpers[i]];
		if (typeof globalThis[hostHelpers[i]] !== "undefined") { globalThis[hostHelpers[i]] = undefined; }
	}

	var logicalTimeMs = 0;
	var randomState = 1;
	var randomAllowed = true;
	var RealDate = Date;

	function PlayhootDate(...args) {
		if (new.target === undefined) { return new RealDate(logicalTimeMs).toString(); }
		if (args.length === 0) { return new RealDate(logicalTimeMs); }
		return new RealDate(...args);
	}
	PlayhootDate.now = function() { return logicalTimeMs; };
	PlayhootDate.parse = RealDate.parse;
	PlayhootDate.UTC = RealDate.UTC;
	PlayhootDate.prototype = RealDate.prototype;
	globalThis.Date = PlayhootDate;

	Math.random = function() {
		if (!randomAllowed) { throw new Error("Math.random is not available here"); }
		randomState = (randomState + 0x6D2B79F5) | 0;
		var t = Math.imul(randomState ^ (randomState >>> 15), 1 | randomState);
		t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
		return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
	};

	var freeze = Object.freeze;
	var ownKeys = Reflect.ownKeys;
	var describe = Object.getOwnPropertyDescriptor;
	var prototypeOf = Object.getPrototypeOf;

	// deepFreeze freezes value and everything reachable from it, without
	// running getters.
	function deepFreeze(value, seen) {
		if ((typeof value !== "object" && typeof value !== "function") || value === null || seen.has(value)) { return value; }
		seen.add(value);
		try { freeze(value); } catch (e) {}
		var keys = ownKeys(value);
		for (var i = 0; i < keys.length; i++) {
			var descriptor = describe(value, keys[i]);
			if (!descriptor) { continue; }
			if ("value" in descriptor) { deepFreeze(descriptor.value, seen); }
			else { deepFreeze(descriptor.get, seen); deepFreeze(descriptor.set, seen); }
		}
		deepFreeze(prototypeOf(value), seen);
		return value;
	}

	return {
		setTime: function(ms) { logicalTimeMs = ms; },
		getRandomState: function() { return randomState | 0; },
		setRandomState: function(state) { randomState = state | 0; },
		allowRandom: function(allowed) { randomAllowed = allowed; },
		// freezeValue makes an argument read-only before authored code gets it.
		freezeValue: function(value) { return deepFreeze(value, new Set()); },
		// lockdown freezes every global and builtin once the script is loaded,
		// so no state can live outside what executions return.
		lockdown: function() { deepFreeze(globalThis, new Set()); }
	};
})()`

// randomStateFromSeed is the generator state a game's seed starts it at.
func randomStateFromSeed(seed uint64) uint32 {
	state := uint32(seed ^ (seed >> 32))
	if state == 0 {
		// An all-zero state produces an all-zero sequence in this generator.
		state = 1
	}
	return state
}

// encodeRandomState is the generator state as the opaque PRNG state the
// Runtime keeps and checkpoints.
func encodeRandomState(state uint32) []byte {
	encoded := make([]byte, 4)
	binary.LittleEndian.PutUint32(encoded, state)
	return encoded
}

// decodeRandomState reads a PRNG state encodeRandomState produced.
func decodeRandomState(encoded []byte) (uint32, error) {
	if len(encoded) != 4 {
		return 0, fmt.Errorf("prng state must be 4 bytes, got %d", len(encoded))
	}
	return binary.LittleEndian.Uint32(encoded), nil
}
