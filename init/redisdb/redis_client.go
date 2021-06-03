package redisdb

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
)

type RedisClient struct {
	Client *redis.Client
}

func (a *RedisClient) Get(ctx context.Context, key string) *redis.StringCmd {
	return a.Client.Get(ctx, key)
}

func (a *RedisClient) HGet(ctx context.Context, key, field string) *redis.StringCmd {
	return a.Client.HGet(ctx, key, field)
}

func (a *RedisClient) HMGet(ctx context.Context, key string, fields ...string) *redis.SliceCmd {
	return a.Client.HMGet(ctx, key, fields...)
}

func (a *RedisClient) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	return a.Client.Del(ctx, keys...)
}

func (a *RedisClient) HDel(ctx context.Context, key string, field ...string) *redis.IntCmd {
	return a.Client.HDel(ctx, key, field...)
}

func (a *RedisClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	return a.Client.Set(ctx, key, value, expiration)
}

func (a *RedisClient) HSet(ctx context.Context, key string, values ...interface{}) *redis.IntCmd {
	return a.Client.HSet(ctx, key, values...)
}

func (a *RedisClient) HMSet(ctx context.Context, key string, values ...interface{}) *redis.BoolCmd {
	return a.Client.HMSet(ctx, key, values)
}

func (a *RedisClient) HSetNX(ctx context.Context, key, field string, value interface{}) *redis.BoolCmd {
	return a.Client.HSetNX(ctx, key, field, value)
}

func (a *RedisClient) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd {
	return a.Client.SetNX(ctx, key, value, expiration)
}

func (a *RedisClient) TxPipeline() redis.Pipeliner {
	return a.Client.TxPipeline()
}
