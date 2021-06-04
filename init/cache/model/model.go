package model

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"

	"github.com/air-iot/service/init/cache"
	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/redisdb"
)

// Get 根据项目ID和ID查询数据
func Get(ctx context.Context, cli redisdb.Client, mongoClient *mongo.Client, project, id string, result interface{}) (err error) {
	return cache.Get(ctx, cli, mongoClient, project, entity.T_MODEL, id, result)
}

// TriggerUpdate 更新redis数据
func TriggerUpdate(ctx context.Context, cli redisdb.Client, project, id string, model interface{}) error {
	return cache.Update(ctx, cli, project, entity.T_MODEL, id, model)
}

// TriggerDelete 删除redis数据
func TriggerDelete(ctx context.Context, cli redisdb.Client, project, id string) error {
	return cache.Delete(ctx, cli, project, entity.T_MODEL, id)
}
