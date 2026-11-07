package cloudweb

import (
	"sync"
	"time"
)

const dedupTTL = 60 * time.Second

type messageDedup struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func (d *messageDedup) isDuplicate(msgID string) bool {
	if msgID == "" {
		return false
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.seen == nil {
		d.seen = make(map[string]time.Time)
	}
	now := time.Now()
	for k, t := range d.seen {
		if now.Sub(t) > dedupTTL {
			delete(d.seen, k)
		}
	}
	if _, ok := d.seen[msgID]; ok {
		return true
	}
	d.seen[msgID] = now
	return false
}
