package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type SessionStore interface {
	Get(chatID int64) *sessionState
	Set(chatID int64, state *sessionState)
	Delete(chatID int64)
}

type memorySessionStore struct {
	ttl   time.Duration
	items map[int64]sessionEntry
	mu    sync.RWMutex
}

type sessionEntry struct {
	State     *sessionState
	ExpiresAt time.Time
}

func NewMemorySessionStore(ttl time.Duration) SessionStore {
	return &memorySessionStore{ttl: ttl, items: map[int64]sessionEntry{}}
}

func (s *memorySessionStore) Get(chatID int64) *sessionState {
	s.mu.RLock()
	entry, ok := s.items[chatID]
	s.mu.RUnlock()
	if !ok || time.Now().After(entry.ExpiresAt) {
		if ok {
			s.Delete(chatID)
		}
		return nil
	}
	cp := *entry.State
	return &cp
}

func (s *memorySessionStore) Set(chatID int64, state *sessionState) {
	cp := *state
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[chatID] = sessionEntry{State: &cp, ExpiresAt: time.Now().Add(s.ttl)}
}

func (s *memorySessionStore) Delete(chatID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, chatID)
}

type redisSessionStore struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisSessionStore(client *redis.Client, ttl time.Duration) SessionStore {
	return &redisSessionStore{client: client, ttl: ttl}
}

func (s *redisSessionStore) Get(chatID int64) *sessionState {
	raw, err := s.client.Get(context.Background(), sessionKey(chatID)).Bytes()
	if err != nil {
		return nil
	}
	var state sessionState
	if json.Unmarshal(raw, &state) != nil {
		return nil
	}
	return &state
}

func (s *redisSessionStore) Set(chatID int64, state *sessionState) {
	raw, _ := json.Marshal(state)
	_ = s.client.Set(context.Background(), sessionKey(chatID), raw, s.ttl).Err()
}

func (s *redisSessionStore) Delete(chatID int64) {
	_ = s.client.Del(context.Background(), sessionKey(chatID)).Err()
}

func sessionKey(chatID int64) string {
	return "tg:session:" + fmt.Sprintf("%d", chatID)
}
