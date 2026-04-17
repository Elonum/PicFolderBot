package telegram

import (
	"testing"
	"time"
)

func TestShouldPrefetchDedup(t *testing.T) {
	b := &Bot{prefetchLast: map[string]time.Time{}}
	if !b.shouldPrefetch("colors:f05p01") {
		t.Fatalf("expected first prefetch to be allowed")
	}
	if b.shouldPrefetch("colors:f05p01") {
		t.Fatalf("expected immediate second prefetch to be blocked by cooldown")
	}
}
