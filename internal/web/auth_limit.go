package web

import (
	"sync"
	"time"
)

const authFailureLimit = 10
const authFailureWindow = time.Minute
const authFailureAddresses = 4096

type authFailure struct {
	count   int
	expires time.Time
}

type authFailureLimiter struct {
	mu      sync.Mutex
	entries map[string]authFailure
}

func (limiter *authFailureLimiter) retryAfter(address string, now time.Time) time.Duration {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	entry := limiter.entries[address]
	if !now.Before(entry.expires) {
		delete(limiter.entries, address)
		return 0
	}
	if entry.count >= authFailureLimit {
		return entry.expires.Sub(now)
	}
	return 0
}

func (limiter *authFailureLimiter) fail(address string, now time.Time) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	if limiter.entries == nil {
		limiter.entries = map[string]authFailure{}
	}
	entry := limiter.entries[address]
	if !now.Before(entry.expires) {
		entry = authFailure{expires: now.Add(authFailureWindow)}
	}
	if len(limiter.entries) >= authFailureAddresses {
		oldestAddress := address
		oldestExpiry := entry.expires
		for candidate, failure := range limiter.entries {
			if !now.Before(failure.expires) {
				delete(limiter.entries, candidate)
			} else if failure.expires.Before(oldestExpiry) {
				oldestAddress, oldestExpiry = candidate, failure.expires
			}
		}
		if len(limiter.entries) >= authFailureAddresses {
			delete(limiter.entries, oldestAddress)
		}
	}
	entry.count++
	limiter.entries[address] = entry
}

func (limiter *authFailureLimiter) reset(address string) {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	delete(limiter.entries, address)
}
