package redisdb

import (
	"context"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/air-iot/service/config"
	"github.com/air-iot/service/logger"
)

const (
	TYPE = "CLUSTER"
)

// Client redis客户端
type Client interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	HGet(ctx context.Context, key, field string) *redis.StringCmd
	HMGet(ctx context.Context, key string, fields ...string) *redis.SliceCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	HDel(ctx context.Context, key string, field ...string) *redis.IntCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	HSet(ctx context.Context, key string, values ...interface{}) *redis.IntCmd
	HMSet(ctx context.Context, key string, values ...interface{}) *redis.BoolCmd
	HSetNX(ctx context.Context, key, field string, value interface{}) *redis.BoolCmd
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd
	TxPipeline() redis.Pipeliner
}

// InitRedisDB 初始化redis存储
func InitRedisDB() (Client, func(), error) {
	cfg := config.C.Redis
	return NewRedisDB(cfg)
}

// NewRedisDB 创建redis存储
func NewRedisDB(cfg config.Redis) (Client, func(), error) {

	if strings.ToUpper(cfg.Type) == TYPE {
		cli := redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:        cfg.Addrs,
			Password:     cfg.Password, // no password set
			PoolSize:     cfg.PoolSize,
			DialTimeout:  10 * time.Second,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		})
		cleanFunc := func() {
			err := cli.Close()
			if err != nil {
				logger.Errorf("redis close error: %s", err.Error())
			}
		}
		p := cli.Ping(context.Background())
		if p.Err() != nil {
			return nil, cleanFunc, p.Err()
		}
		return &RedisClusterClient{cli}, cleanFunc, nil
	} else {
		cli := redis.NewClient(&redis.Options{
			Addr:         cfg.Addrs[0],
			Password:     cfg.Password, // no password set
			PoolSize:     cfg.PoolSize,
			DialTimeout:  10 * time.Second,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		})
		cleanFunc := func() {
			err := cli.Close()
			if err != nil {
				logger.Errorf("redis close error: %s", err.Error())
			}
		}
		p := cli.Ping(context.Background())
		if p.Err() != nil {
			return nil, cleanFunc, p.Err()
		}

		return &RedisClient{cli}, cleanFunc, nil
	}
}
