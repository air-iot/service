package redisdb

import (
	"context"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/air-iot/service/config"
	"github.com/air-iot/service/logger"
)

// InitRedisDB 初始化redis存储
func InitRedisDB() (*redis.Client, func(), error) {
	cfg := config.C.Redis
	return NewRedisDB(cfg)
}

// NewRedisDB 创建redis存储
func NewRedisDB(cfg config.Redis) (*redis.Client, func(), error) {

	cli := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
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

	return cli, cleanFunc, nil
}
