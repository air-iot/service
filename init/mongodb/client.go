package mongodb

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/air-iot/service/config"
	"github.com/air-iot/service/logger"
)

// InitMongoDB 初始化mongo存储
func InitMongoDB() (*mongo.Client, func(), error) {
	cfg := config.C.Mongo
	return NewMongoDB(cfg)
}

// NewMongoDB 创建mongo存储
func NewMongoDB(cfg config.Mongo) (*mongo.Client, func(), error) {
	opts := options.Client().
		ApplyURI(cfg.Addr).
		SetMaxPoolSize(cfg.PoolSize).
		SetHeartbeatInterval(time.Duration(cfg.HeartbeatInterval) * time.Second).
		SetMaxConnIdleTime(time.Duration(cfg.MaxConnIdleTime) * time.Second)
	cli, err := mongo.Connect(context.Background(), opts)
	if err != nil {
		return nil, nil, err
	}
	cleanFunc := func() {
		err := cli.Disconnect(context.Background())
		if err != nil {
			logger.Errorf("mongo close error: %s", err.Error())
		}
	}
	err = cli.Ping(context.Background(), nil)
	if err != nil {
		return nil, cleanFunc, err
	}
	return cli, cleanFunc, nil
}
