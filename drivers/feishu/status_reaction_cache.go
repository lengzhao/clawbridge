package feishu

import (
	"sync"
	"time"
)

const (
	statusReactionMaxEntries = 4096
	statusReactionTTL        = 24 * time.Hour
)

// statusReactionCache stores the last bot-added status reaction per message ID for cleanup on state transitions.
type statusReactionCache struct {
	mu         sync.Mutex
	maxEntries int
	ttl        time.Duration
	m          map[string]statusReactionEntry
}

type statusReactionEntry struct {
	reactionID string
	at         time.Time
}

func newStatusReactionCache() *statusReactionCache {
	return &statusReactionCache{
		maxEntries: statusReactionMaxEntries,
		ttl:        statusReactionTTL,
		m:          make(map[string]statusReactionEntry),
	}
}

// pop removes and returns a stored reaction ID if still valid (within TTL).
func (c *statusReactionCache) pop(messageID string) string {
	if messageID == "" {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.m[messageID]
	delete(c.m, messageID)
	if !ok {
		return ""
	}
	if time.Since(e.at) > c.ttl {
		return ""
	}
	return e.reactionID
}

func (c *statusReactionCache) put(messageID, reactionID string) {
	if messageID == "" || reactionID == "" {
		return
	}
	now := time.Now()
	c.mu.Lock()
	c.m[messageID] = statusReactionEntry{reactionID: reactionID, at: now}
	c.evictExpired(now)
	c.evictExcess()
	c.mu.Unlock()
}

func (c *statusReactionCache) evictExpired(now time.Time) {
	for k, e := range c.m {
		if now.Sub(e.at) > c.ttl {
			delete(c.m, k)
		}
	}
}

func (c *statusReactionCache) evictExcess() {
	for len(c.m) > c.maxEntries {
		var oldestKey string
		var oldestTime time.Time
		first := true
		for k, e := range c.m {
			if first || e.at.Before(oldestTime) {
				first = false
				oldestKey = k
				oldestTime = e.at
			}
		}
		if oldestKey == "" {
			break
		}
		delete(c.m, oldestKey)
	}
}
