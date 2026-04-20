package redisstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
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

func (s *Store) GetString(ctx context.Context, key string) (string, error) {
	raw, err := s.client.Get(ctx, key).Result()
	if err != nil {
		return "", err
	}
	return raw, nil
}

func (s *Store) SetString(ctx context.Context, key, value string, ttl time.Duration) error {
	return s.client.Set(ctx, key, value, ttl).Err()
}

func (s *Store) Del(ctx context.Context, key string) error {
	return s.client.Del(ctx, key).Err()
}

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

func (s *Store) AddUserNamespacedSession(ctx context.Context, namespace string, userID uint64, sessionID string, ttl time.Duration) error {
	if userID == 0 || namespace == "" || sessionID == "" {
		return nil
	}
	key := namespacedUserSessionIndexKey(namespace, userID)
	pipe := s.client.TxPipeline()
	pipe.SAdd(ctx, key, sessionID)
	if ttl > 0 {
		pipe.Expire(ctx, key, ttl)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (s *Store) RemoveUserNamespacedSession(ctx context.Context, namespace string, userID uint64, sessionID string) error {
	if userID == 0 || namespace == "" || sessionID == "" {
		return nil
	}
	key := namespacedUserSessionIndexKey(namespace, userID)
	pipe := s.client.TxPipeline()
	pipe.SRem(ctx, key, sessionID)
	pipe.SCard(ctx, key)
	cmds, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return err
	}
	if len(cmds) > 1 {
		if cardCmd, ok := cmds[1].(*redis.IntCmd); ok && cardCmd.Val() == 0 {
			return s.client.Del(ctx, key).Err()
		}
	}
	return nil
}

func (s *Store) DeleteUserNamespacedSessions(ctx context.Context, namespace string, userID uint64) error {
	if userID == 0 || namespace == "" {
		return nil
	}
	indexKey := namespacedUserSessionIndexKey(namespace, userID)
	sessionIDs, err := s.client.SMembers(ctx, indexKey).Result()
	if err != nil && err != redis.Nil {
		return err
	}
	keys := make([]string, 0, len(sessionIDs)+1)
	for _, sessionID := range sessionIDs {
		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" {
			continue
		}
		keys = append(keys, fmt.Sprintf("session:%s:%s", namespace, sessionID))
	}
	keys = append(keys, indexKey)
	return s.client.Del(ctx, keys...).Err()
}

func namespacedUserSessionIndexKey(namespace string, userID uint64) string {
	return "session_index:" + namespace + ":user:" + strconv.FormatUint(userID, 10)
}
