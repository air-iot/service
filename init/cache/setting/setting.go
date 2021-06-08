package setting

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/mongo"

	"github.com/air-iot/service/init/cache"
	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/redisdb"
)

const id = "setting"

// Get 根据项目ID和资产ID查询数据
func Get(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project string, result interface{}) (err error) {
	return cache.Get(ctx, redisClient, mongoClient, project, entity.T_SETTING, id, result)
}

// GetByWarnKindID 根据项目ID和资产ID查询数据
func GetByWarnKindID(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, warnKindID string) (string, error) {
	settingInfo := entity.Setting{}
	if err := Get(ctx, redisClient, mongoClient, project, &settingInfo); err != nil {
		return "", err
	}
	for _, warnKind := range settingInfo.Warning.WarningKind {
		if warnKind.ID == warnKindID {
			return warnKind.Name, nil
		}
	}
	return "", fmt.Errorf("未找到数据")
}

// TriggerUpdate 更新redis资产数据,并发送消息通知
func TriggerUpdate(ctx context.Context, redisClient redisdb.Client, project string, setting interface{}) error {
	return cache.Update(ctx, redisClient, project, entity.T_SETTING, id, setting)
}

// TriggerDelete 删除redis资产数据,并发送消息通知
func TriggerDelete(ctx context.Context, redisClient redisdb.Client, project string) error {
	return cache.Delete(ctx, redisClient, project, entity.T_SETTING, id)
}
