package department

import (
	"context"

	"github.com/go-redis/redis/v8"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/air-iot/service/init/cache"
	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/util/json"
)

// Get 根据项目ID和部门ID查询数据
func Get(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, id string, result interface{}) (err error) {
	return cache.Get(ctx, redisClient, mongoClient, project, entity.T_DEPT, id, result)
}

// GetByList 根据项目ID和资产ID查询数据
func GetByList(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project string, ids []string, result interface{}) (err error) {
	departmentList := make([]map[string]interface{}, 0)
	for _, id := range ids {
		deptInfo := map[string]interface{}{}
		err := Get(ctx, redisClient, mongoClient, project, id, &deptInfo)
		if err != nil {
			return err
		}
		departmentList = append(departmentList, deptInfo)
	}

	return json.CopyByJson(result, &departmentList)
}

// TriggerUpdate 更新redis资产数据
func TriggerUpdate(ctx context.Context, cli redisdb.Client, project, id string, department interface{}) error {
	return cache.Update(ctx, cli, project, entity.T_DEPT, id, department)
}

// TriggerDelete 删除redis资产数据,并发送消息通知
func TriggerDelete(ctx context.Context, cli *redis.Client, project, id string) error {
	return cache.Delete(ctx, cli, project, entity.T_DEPT, id)
}
