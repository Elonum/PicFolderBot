package telegram

import (
	"fmt"
	"strings"
	"time"
)

// Prefetch warms the next-step cache. With Flow tree cache enabled, these calls
// become cheap and keep UX snappy during navigation/search.
func (b *Bot) prefetchColors(product string) {
	product = strings.TrimSpace(product)
	if product == "" {
		return
	}
	if !b.shouldPrefetch("colors:" + normalizeLookup(product)) {
		return
	}
	_, _ = b.flow.ListColors(product)
}

func (b *Bot) prefetchSections(product, color string) {
	product = strings.TrimSpace(product)
	color = strings.TrimSpace(color)
	if product == "" || color == "" {
		return
	}
	key := fmt.Sprintf("sections:%s|%s", normalizeLookup(product), normalizeLookup(color))
	if !b.shouldPrefetch(key) {
		return
	}
	_, _ = b.flow.ListSections(product, color)
}

func (b *Bot) shouldPrefetch(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	now := time.Now()
	b.prefetchMu.Lock()
	defer b.prefetchMu.Unlock()
	if last, ok := b.prefetchLast[key]; ok {
		if now.Sub(last) < prefetchCooldown {
			return false
		}
	}
	b.prefetchLast[key] = now
	return true
}
