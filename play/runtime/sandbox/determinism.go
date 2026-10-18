package sandbox

import (
	"encoding/binary"
	"fmt"
)

// determinismPrelude is evaluated before authored code. It replaces Date,
// Math.random and other ambient time or randomness sources, and evaluates to
// the object the worker controls them through; that object is never
// reachable from authored code. Math.random is mulberry32, whose whole state
// is one 32-bit integer, so it can be exported and restored exactly.
const determinismPrelude = `(function() {
	var logicalTimeMs = 0;
	var randomState = 1;
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
		randomState = (randomState + 0x6D2B79F5) | 0;
		var t = Math.imul(randomState ^ (randomState >>> 15), 1 | randomState);
		t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
		return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
	};

	globalThis.performance = undefined;
	globalThis.os = undefined;

	return {
		setTime: function(ms) { logicalTimeMs = ms; },
		getRandomState: function() { return randomState | 0; },
		setRandomState: function(state) { randomState = state | 0; }
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
