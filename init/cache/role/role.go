package role

import (
	"context"

	"go.mongodb.org/mongo-driver/mongo"

	"github.com/air-iot/service/init/cache"
	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/util/json"
)

// Get 根据项目ID和角色ID查询数据
func Get(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, id string, result interface{}) (err error) {
	return cache.Get(ctx, redisClient, mongoClient, project, entity.T_ROLE, id, result)
}

// GetByList 根据项目ID和资产ID查询数据
func GetByList(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project string, ids []string, result interface{}) (err error) {
	roleList := make([]map[string]interface{}, 0)
	for _, id := range ids {
		roleInfo := map[string]interface{}{}
		err := Get(ctx, redisClient, mongoClient, project, id, &roleInfo)
		if err != nil {
			return err
		}
		roleList = append(roleList, roleInfo)
	}
	return json.CopyByJson(result, &roleList)
}

// TriggerUpdate 更新redis资产数据
func TriggerUpdate(ctx context.Context, redisClient redisdb.Client, project, id string, role []byte) error {
	return cache.Update(ctx, redisClient, project, entity.T_ROLE, id, role)
}

// TriggerDelete 删除redis资产数据
func TriggerDelete(ctx context.Context, redisClient redisdb.Client, project, id string) error {
	return cache.Delete(ctx, redisClient, project, entity.T_ROLE, id)
}
