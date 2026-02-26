package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const kvRedisPrefix = "tailflow:kv:"

// jsonMarshalKV and jsonUnmarshalKV wrap json functions for testing.
var (
	jsonMarshalKV   = json.Marshal
	jsonUnmarshalKV = json.Unmarshal
)

// redisClient is the subset of *redis.Client methods used by RedisKVStore.
type redisClient interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	Ping(ctx context.Context) *redis.StatusCmd
	Close() error
}

// newRedisClientFn wraps redis.NewClient for testing.
var newRedisClientFn = func(opts *redis.Options) redisClient {
	return redis.NewClient(opts)
}

type RedisKVStore struct {
	client redisClient
}

func NewRedisKVStore(url string) (*RedisKVStore, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("redis: invalid URL: %w", err)
	}

	client := newRedisClientFn(opts)

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
	if errors.Is(err, redis.Nil) {
		return nil, false
	}

	if err != nil {
		return nil, false
	}

	var value any

	err = jsonUnmarshalKV(data, &value)
	if err != nil {
		return nil, false
	}

	return value, true
}

func (s *RedisKVStore) Set(ctx context.Context, key string, value any, ttl time.Duration) {
	data, err := jsonMarshalKV(value)
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
