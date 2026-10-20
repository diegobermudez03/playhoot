// Package ratelimit bounds how often something may happen. A Limiter combines
// several limits, such as at most 5 per second and at most 30 per minute, and
// lets an event through only when every one of them has room. It knows no
// domain: callers decide what an event is and what to do with a refusal.
package ratelimit

import (
	"fmt"
	"sync"
	"time"
)

// Limit allows at most Count events within any Per. It refills continuously,
// so after a burst room comes back at Count/Per; "one every 5 minutes" is
// Limit{Count: 1, Per: 5 * time.Minute}.
type Limit struct {
	Count int
	Per   time.Duration
}

// Valid reports whether the limit allows anything at all.
func (l Limit) Valid() bool {
	return l.Count > 0 && l.Per > 0
}

// bucket is one Limit's room, kept as the time it would be full again: each
// event pushes it Per/Count further, and an event is refused when that would
// put it more than Per ahead of now. Integer time keeps it exact.
type bucket struct {
	interval time.Duration
	per      time.Duration
	// fullAt is when every event taken so far will have been refilled.
	fullAt time.Time
	// latest is the latest now seen, so a clock going backwards adds no room.
	latest time.Time
}

func newBucket(limit Limit) bucket {
	return bucket{interval: limit.Per / time.Duration(limit.Count), per: limit.Per}
}

// at is now, never before a time the bucket already saw.
func (b *bucket) at(now time.Time) time.Time {
	if now.Before(b.latest) {
		return b.latest
	}
	b.latest = now
	return now
}

// wait is how long until n events fit; 0 when they already do.
func (b *bucket) wait(now time.Time, n int) time.Duration {
	return max(0, b.after(now, n).Sub(now)-b.per)
}

// after is when the bucket would be full again had it taken n events at now.
func (b *bucket) after(now time.Time, n int) time.Time {
	from := b.fullAt
	if from.Before(now) {
		from = now
	}
	return from.Add(time.Duration(n) * b.interval)
}

// full reports whether every event taken was refilled by now.
func (b *bucket) full(now time.Time) bool {
	return !b.fullAt.After(now)
}

// Limiter enforces every one of its limits on one subject. It is safe for
// concurrent use.
type Limiter struct {
	mu      sync.Mutex
	buckets []bucket
}

// New constructs a Limiter allowing an event only within every limit. It
// panics on a limit that allows nothing, which is a programming error.
func New(limits ...Limit) *Limiter {
	buckets := make([]bucket, len(limits))
	for i, limit := range limits {
		if !limit.Valid() {
			panic(fmt.Sprintf("ratelimit: invalid limit %+v", limit))
		}
		buckets[i] = newBucket(limit)
	}
	return &Limiter{buckets: buckets}
}

// Allow takes one event at now when every limit has room for it. Otherwise it
// takes nothing and returns how long until it would have room.
func (l *Limiter) Allow(now time.Time) (bool, time.Duration) {
	return l.AllowN(now, 1)
}

// AllowN is Allow for n events at once, such as n bytes.
func (l *Limiter) AllowN(now time.Time, n int) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return allow(l.buckets, now, n)
}

// Record takes one event at now whether or not there is room, for events that
// cannot be refused but must count against the ones that can.
func (l *Limiter) Record(now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.buckets {
		b := &l.buckets[i]
		at := b.at(now)
		// An empty bucket stays empty: it never owes more than Per.
		b.fullAt = b.after(at, 1)
		if limit := at.Add(b.per); b.fullAt.After(limit) {
			b.fullAt = limit
		}
	}
}

func allow(buckets []bucket, now time.Time, n int) (bool, time.Duration) {
	var retryAfter time.Duration
	for i := range buckets {
		retryAfter = max(retryAfter, buckets[i].wait(buckets[i].at(now), n))
	}
	if retryAfter > 0 {
		return false, retryAfter
	}
	for i := range buckets {
		buckets[i].fullAt = buckets[i].after(buckets[i].at(now), n)
	}
	return true, 0
}

// Keyed enforces the same limits on each key, such as each user or each
// client address, independently. Keys whose room fully came back are
// forgotten, so memory follows the keys active within the longest limit. It
// is safe for concurrent use.
type Keyed struct {
	limits []Limit
	// sweepEvery is the longest limit's Per: a key unused that long is full.
	sweepEvery time.Duration

	mu        sync.Mutex
	subjects  map[string][]bucket
	lastSweep time.Time
}

// NewKeyed constructs a Keyed allowing each key's events within every limit.
// It panics on a limit that allows nothing, which is a programming error.
func NewKeyed(limits ...Limit) *Keyed {
	var sweepEvery time.Duration
	for _, limit := range limits {
		if !limit.Valid() {
			panic(fmt.Sprintf("ratelimit: invalid limit %+v", limit))
		}
		sweepEvery = max(sweepEvery, limit.Per)
	}
	return &Keyed{limits: limits, sweepEvery: sweepEvery, subjects: make(map[string][]bucket)}
}

// Allow takes one of key's events at now when every limit has room for it.
// Otherwise it takes nothing and returns how long until it would have room.
func (k *Keyed) Allow(key string, now time.Time) (bool, time.Duration) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.sweep(now)
	buckets, found := k.subjects[key]
	if !found {
		buckets = make([]bucket, len(k.limits))
		for i, limit := range k.limits {
			buckets[i] = newBucket(limit)
		}
		k.subjects[key] = buckets
	}
	return allow(buckets, now, 1)
}

// Len returns how many keys are remembered.
func (k *Keyed) Len() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.subjects)
}

// sweep forgets, at most once per sweepEvery, every key whose room fully came
// back: it behaves exactly as a key never seen.
func (k *Keyed) sweep(now time.Time) {
	if now.Sub(k.lastSweep) < k.sweepEvery {
		return
	}
	k.lastSweep = now
	for key, buckets := range k.subjects {
		full := true
		for i := range buckets {
			if !buckets[i].full(now) {
				full = false
				break
			}
		}
		if full {
			delete(k.subjects, key)
		}
	}
}
