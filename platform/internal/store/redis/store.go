package redisstore

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	redis "github.com/redis/go-redis/v9"
)

type Store struct {
	client *redis.Client
}

func NewClient(addr string) *redis.Client {
	return redis.NewClient(&redis.Options{Addr: addr})
}

func NewStore(client *redis.Client) *Store { return &Store{client: client} }

func (s *Store) Ping(ctx context.Context) error { return s.client.Ping(ctx).Err() }

func (s *Store) GetJSON(ctx context.Context, key string, dest any) (bool, error) {
	raw, err := s.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(raw), dest)
}

func (s *Store) SetJSON(ctx context.Context, key string, value any, ttl time.Duration) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, key, raw, ttl).Err()
}

func (s *Store) AcquireLock(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return s.client.SetNX(ctx, fmt.Sprintf("lock:%s", key), "1", ttl).Result()
}

func (s *Store) ReleaseLock(ctx context.Context, key string) error {
	return s.client.Del(ctx, fmt.Sprintf("lock:%s", key)).Err()
}

func (s *Store) SaveSession(ctx context.Context, sessionID string, payload any, ttl time.Duration) error {
	return s.SetJSON(ctx, fmt.Sprintf("session:%s", sessionID), payload, ttl)
}

func (s *Store) LoadSession(ctx context.Context, sessionID string, dest any) (bool, error) {
	return s.GetJSON(ctx, fmt.Sprintf("session:%s", sessionID), dest)
}

func (s *Store) DeleteSession(ctx context.Context, sessionID string) error {
	return s.client.Del(ctx, fmt.Sprintf("session:%s", sessionID)).Err()
}

func (s *Store) SaveNamespacedSession(ctx context.Context, namespace, sessionID string, payload any, ttl time.Duration) error {
	return s.SetJSON(ctx, fmt.Sprintf("session:%s:%s", namespace, sessionID), payload, ttl)
}

func (s *Store) LoadNamespacedSession(ctx context.Context, namespace, sessionID string, dest any) (bool, error) {
	return s.GetJSON(ctx, fmt.Sprintf("session:%s:%s", namespace, sessionID), dest)
}

func (s *Store) DeleteNamespacedSession(ctx context.Context, namespace, sessionID string) error {
	return s.client.Del(ctx, fmt.Sprintf("session:%s:%s", namespace, sessionID)).Err()
}
