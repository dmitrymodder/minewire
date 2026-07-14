// Package main implements the Minewire proxy server.
// This file contains a minimal token-bucket rate limiter used to throttle
// tunnel throughput in "realistic" mode, so the connection's bandwidth
// profile resembles a real Minecraft client instead of an unlimited pipe.
package main

import (
	"sync"
	"time"
)

// tokenBucket is a simple thread-safe token bucket rate limiter.
// Tokens represent bytes. Capacity allows short bursts (e.g. the initial
// flurry of chunk packets a real client gets on join/teleport), while
// refillRate enforces the sustained steady-state throughput.
type tokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	capacity   float64
	refillRate float64 // bytes per second
	lastRefill time.Time
}

// newTokenBucket creates a bucket that sustains refillRatePerSec bytes/sec
// on average, while allowing bursts up to capacityBytes.
func newTokenBucket(refillRatePerSec, capacityBytes float64) *tokenBucket {
	if capacityBytes < refillRatePerSec {
		// Burst capacity should never be smaller than one second worth of tokens.
		capacityBytes = refillRatePerSec
	}
	return &tokenBucket{
		tokens:     capacityBytes, // start full, like a client that just joined
		capacity:   capacityBytes,
		refillRate: refillRatePerSec,
		lastRefill: time.Now(),
	}
}

// newThrottleBucket builds a tokenBucket sized according to the global config,
// or returns nil if running in fast mode (i.e. no throttling at all, matching
// pre-existing behavior for anyone not opting into realistic mode).
func newThrottleBucket() *tokenBucket {
	if cfg.Mode != ModeRealistic {
		return nil
	}
	rate := float64(cfg.RealisticBandwidthKB) * 1024
	burst := float64(cfg.RealisticBurstKB) * 1024
	return newTokenBucket(rate, burst)
}

// WaitN blocks until n bytes worth of tokens are available, then consumes them.
// This provides simple backpressure: callers writing faster than the configured
// bandwidth will be slowed down transparently.
func (tb *tokenBucket) WaitN(n int) {
	if tb == nil || n <= 0 {
		return
	}
	need := float64(n)
	for {
		tb.mu.Lock()
		now := time.Now()
		elapsed := now.Sub(tb.lastRefill).Seconds()
		tb.lastRefill = now

		tb.tokens += elapsed * tb.refillRate
		if tb.tokens > tb.capacity {
			tb.tokens = tb.capacity
		}

		if tb.tokens >= need {
			tb.tokens -= need
			tb.mu.Unlock()
			return
		}

		deficit := need - tb.tokens
		waitSec := deficit / tb.refillRate
		tb.mu.Unlock()

		// Sleep for the shortfall, then loop to re-check (handles concurrent
		// consumers and clock drift correctly instead of assuming we're alone).
		time.Sleep(time.Duration(waitSec * float64(time.Second)))
	}
}