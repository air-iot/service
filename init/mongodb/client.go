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
		ApplyURI(cfg.ApplyURI()).
		SetMaxPoolSize(cfg.PoolSize).
		SetHeartbeatInterval(30 * time.Second).
		SetMaxConnIdleTime(30 * time.Second)

	cli, err := mongo.NewClient(opts)
	if err != nil {
		return nil, nil, err
	}

	if err := cli.Connect(context.Background()); err != nil {
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
