package feishu

import (
	"sync"
	"time"

	"github.com/lengzhao/clawbridge/bus"
)

const (
	defaultLastSentMaxEntries = 2048
	defaultLastSentTTL        = 24 * time.Hour
)

// boundedLastSent records the latest outbound message ID per Recipient (bus.RecipientKey), with TTL and max size.
type boundedLastSent struct {
	mu         sync.Mutex
	maxEntries int
	ttl        time.Duration
	m          map[string]lastSentEntry
}

type lastSentEntry struct {
	messageID string
	at        time.Time
}

func newBoundedLastSent() *boundedLastSent {
	return &boundedLastSent{
		maxEntries: defaultLastSentMaxEntries,
		ttl:        defaultLastSentTTL,
		m:          make(map[string]lastSentEntry),
	}
}

func (s *boundedLastSent) set(to bus.Recipient, messageID string) {
	if messageID == "" {
		return
	}
	key := bus.RecipientKey(to)
	now := time.Now()
	s.mu.Lock()
	s.m[key] = lastSentEntry{messageID: messageID, at: now}
	s.evictExpired(now)
	s.evictExcess()
	s.mu.Unlock()
}

func (s *boundedLastSent) get(to bus.Recipient) (string, bool) {
	key := bus.RecipientKey(to)
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[key]
	if !ok {
		return "", false
	}
	if time.Since(e.at) > s.ttl {
		delete(s.m, key)
		return "", false
	}
	return e.messageID, true
}

func (s *boundedLastSent) evictExpired(now time.Time) {
	for k, e := range s.m {
		if now.Sub(e.at) > s.ttl {
			delete(s.m, k)
		}
	}
}

func (s *boundedLastSent) evictExcess() {
	for len(s.m) > s.maxEntries {
		var oldestKey string
		var oldestTime time.Time
		first := true
		for k, e := range s.m {
			if first || e.at.Before(oldestTime) {
				first = false
				oldestKey = k
				oldestTime = e.at
			}
		}
		if oldestKey == "" {
			break
		}
		delete(s.m, oldestKey)
	}
}
