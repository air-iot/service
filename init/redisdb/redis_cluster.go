package redisdb

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"
)

type RedisClusterClient struct {
	Client *redis.ClusterClient
}

func (a *RedisClusterClient) Get(ctx context.Context, key string) *redis.StringCmd {
	return a.Client.Get(ctx, key)
}

func (a *RedisClusterClient) HGet(ctx context.Context, key, field string) *redis.StringCmd {
	return a.Client.HGet(ctx, key, field)
}

func (a *RedisClusterClient) HMGet(ctx context.Context, key string, fields ...string) *redis.SliceCmd {
	return a.Client.HMGet(ctx, key, fields...)
}

func (a *RedisClusterClient) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	return a.Client.Del(ctx, keys...)
}

func (a *RedisClusterClient) HDel(ctx context.Context, key string, field ...string) *redis.IntCmd {
	return a.Client.HDel(ctx, key, field...)
}

func (a *RedisClusterClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	return a.Client.Set(ctx, key, value, expiration)
}

func (a *RedisClusterClient) HSet(ctx context.Context, key string, values ...interface{}) *redis.IntCmd {
	return a.Client.HSet(ctx, key, values...)
}

func (a *RedisClusterClient) HMSet(ctx context.Context, key string, values ...interface{}) *redis.BoolCmd {
	return a.Client.HMSet(ctx, key, values)
}

func (a *RedisClusterClient) HSetNX(ctx context.Context, key, field string, value interface{}) *redis.BoolCmd {
	return a.Client.HSetNX(ctx, key, field, value)
}

func (a *RedisClusterClient) SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd {
	return a.Client.SetNX(ctx, key, value, expiration)
}

func (a *RedisClusterClient) HGetAll(ctx context.Context, key string) *redis.StringStringMapCmd {
	return a.Client.HGetAll(ctx, key)
}

func (a *RedisClusterClient) TxPipeline() redis.Pipeliner {
	return a.Client.TxPipeline()
}
