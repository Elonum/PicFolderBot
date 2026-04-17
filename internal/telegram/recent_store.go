package telegram

import (
	"strings"
	"sync"
	"time"
)

type RecentPath struct {
	Product string
	Color   string
	Section string
	Level   string
	At      time.Time
}

type RecentStore interface {
	List(chatID int64) []RecentPath
	Push(chatID int64, p RecentPath)
	Clear(chatID int64)
}

type memoryRecentStore struct {
	limit int
	mu    sync.RWMutex
	items map[int64][]RecentPath
}

func NewMemoryRecentStore(limit int) RecentStore {
	if limit <= 0 {
		limit = 8
	}
	return &memoryRecentStore{
		limit: limit,
		items: map[int64][]RecentPath{},
	}
}

func (s *memoryRecentStore) List(chatID int64) []RecentPath {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.items[chatID]
	out := make([]RecentPath, 0, len(src))
	out = append(out, src...)
	return out
}

func (s *memoryRecentStore) Clear(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, chatID)
}

func (s *memoryRecentStore) Push(chatID int64, p RecentPath) {
	p.Product = strings.TrimSpace(p.Product)
	p.Color = strings.TrimSpace(p.Color)
	p.Section = strings.TrimSpace(p.Section)
	p.Level = strings.TrimSpace(p.Level)
	if p.Product == "" {
		return
	}
	if p.Level == "" {
		p.Level = "section"
	}
	if p.At.IsZero() {
		p.At = time.Now()
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.items[chatID]
	// De-dup by (level, product, color, section)
	out := make([]RecentPath, 0, s.limit)
	out = append(out, p)
	for _, it := range cur {
		if sameRecent(it, p) {
			continue
		}
		out = append(out, it)
		if len(out) == s.limit {
			break
		}
	}
	s.items[chatID] = out
}

func sameRecent(a, b RecentPath) bool {
	return strings.EqualFold(a.Level, b.Level) &&
		strings.EqualFold(a.Product, b.Product) &&
		strings.EqualFold(a.Color, b.Color) &&
		strings.EqualFold(a.Section, b.Section)
}
