package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type AlbumStore interface {
	Get(key string) *albumBuffer
	Set(key string, album *albumBuffer)
	Delete(key string)
}

type memoryAlbumStore struct {
	ttl   time.Duration
	items map[string]albumEntry
	mu    sync.RWMutex
}

type albumEntry struct {
	Album     *albumBuffer
	ExpiresAt time.Time
}

func NewMemoryAlbumStore(ttl time.Duration) AlbumStore {
	return &memoryAlbumStore{ttl: ttl, items: map[string]albumEntry{}}
}

func (s *memoryAlbumStore) Get(key string) *albumBuffer {
	s.mu.RLock()
	entry, ok := s.items[key]
	s.mu.RUnlock()
	if !ok || time.Now().After(entry.ExpiresAt) {
		if ok {
			s.Delete(key)
		}
		return nil
	}
	cp := *entry.Album
	cp.Items = append([]albumItem(nil), entry.Album.Items...)
	cp.Timer = nil
	return &cp
}

func (s *memoryAlbumStore) Set(key string, album *albumBuffer) {
	cp := *album
	cp.Items = append([]albumItem(nil), album.Items...)
	cp.Timer = nil
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = albumEntry{Album: &cp, ExpiresAt: time.Now().Add(s.ttl)}
}

func (s *memoryAlbumStore) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, key)
}

type redisAlbumStore struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisAlbumStore(client *redis.Client, ttl time.Duration) AlbumStore {
	return &redisAlbumStore{client: client, ttl: ttl}
}

func (s *redisAlbumStore) Get(key string) *albumBuffer {
	raw, err := s.client.Get(context.Background(), albumKey(key)).Bytes()
	if err != nil {
		return nil
	}
	var album albumBuffer
	if json.Unmarshal(raw, &album) != nil {
		return nil
	}
	return &album
}

func (s *redisAlbumStore) Set(key string, album *albumBuffer) {
	cp := *album
	cp.Timer = nil
	raw, _ := json.Marshal(&cp)
	_ = s.client.Set(context.Background(), albumKey(key), raw, s.ttl).Err()
}

func (s *redisAlbumStore) Delete(key string) {
	_ = s.client.Del(context.Background(), albumKey(key)).Err()
}

func albumKey(key string) string {
	return "tg:album:" + fmt.Sprintf("%s", key)
}
