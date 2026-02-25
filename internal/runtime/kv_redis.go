package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const kvRedisPrefix = "tailflow:kv:"

type RedisKVStore struct {
	client *redis.Client
}

func NewRedisKVStore(url string) (*RedisKVStore, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("redis: invalid URL: %w", err)
	}

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = client.Ping(ctx).Err()
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("redis: ping failed: %w", err)
	}

	return &RedisKVStore{client: client}, nil
}

func (s *RedisKVStore) Get(ctx context.Context, key string) (any, bool) {
	data, err := s.client.Get(ctx, kvRedisPrefix+key).Bytes()
	if err == redis.Nil {
		return nil, false
	}
	if err != nil {
		return nil, false
	}

	var value any
	err = json.Unmarshal(data, &value)
	if err != nil {
		return nil, false
	}

	return value, true
}

func (s *RedisKVStore) Set(ctx context.Context, key string, value any, ttl time.Duration) {
	data, err := json.Marshal(value)
	if err != nil {
		return
	}

	s.client.Set(ctx, kvRedisPrefix+key, data, ttl)
}

func (s *RedisKVStore) Delete(ctx context.Context, key string) (bool, error) {
	n, err := s.client.Del(ctx, kvRedisPrefix+key).Result()
	if err != nil {
		return false, fmt.Errorf("redis: delete failed: %w", err)
	}

	return n > 0, nil
}

func (s *RedisKVStore) Close() error {
	return s.client.Close()
}
