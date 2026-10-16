package sandbox

import "fmt"

// buildDeterminismPrelude returns a script to evaluate before authored code,
// in the same scope, replacing Date, Math.random and other ambient time or
// randomness sources with ones derived from logicalTimeMs and randomSeed.
// Environment variables need no neutralizing: the worker has no environment.
func buildDeterminismPrelude(logicalTimeMs int64, randomSeed uint64) string {
	seed := uint32(randomSeed ^ (randomSeed >> 32))
	if seed == 0 {
		seed = 1 // an all-zero state produces an all-zero sequence in this generator.
	}
	return fmt.Sprintf(`(function() {
	var __logicalTimeMs = %d;
	var __state = %d;
	var __RealDate = Date;

	function __Date(...args) {
		if (new.target === undefined) { return new __RealDate(__logicalTimeMs).toString(); }
		if (args.length === 0) { return new __RealDate(__logicalTimeMs); }
		return new __RealDate(...args);
	}
	__Date.now = function() { return __logicalTimeMs; };
	__Date.parse = __RealDate.parse;
	__Date.UTC = __RealDate.UTC;
	__Date.prototype = __RealDate.prototype;
	globalThis.Date = __Date;

	Math.random = function() {
		__state |= 0;
		__state = (__state + 0x6D2B79F5) | 0;
		var t = Math.imul(__state ^ (__state >>> 15), 1 | __state);
		t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
		return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
	};

	globalThis.performance = undefined;
	globalThis.os = undefined;
})();`, logicalTimeMs, seed)
}
