package setting

import (
	"context"
	"fmt"

	"github.com/go-redis/redis/v8"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/air-iot/service/init/cache"
	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/mongodb"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/util/json"
)

// Get 根据项目ID和资产ID查询数据
func Get(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, id string, result interface{}) (err error) {
	return cache.Get(ctx, redisClient, mongoClient, project, entity.T_SETTING, id, result)
}

// GetByWarnKindID 根据项目ID和资产ID查询数据
func GetByWarnKindID(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, warnKindID string) (string, error) {
	setting, err := GetByDBAndWarnKindID(ctx, redisClient, mongoClient, project)
	if err != nil {
		return "", err
	}
	settingInfo := entity.Setting{}
	err = json.Unmarshal([]byte(setting), &settingInfo)
	if err != nil {
		return "", err
	}
	for _, warnKind := range settingInfo.Warning.WarningKind {
		if warnKind.ID == warnKindID {
			return warnKind.Name, nil
		}
	}
	return "", fmt.Errorf("未找到数据")
}

func GetByDBAndWarnKindID(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project string) (string, error) {
	col := mongoClient.Database(project).Collection(entity.T_SETTING)
	settingTmpList := make([]map[string]interface{}, 0)
	_, err := mongodb.FindFilter(ctx, col, &settingTmpList, mongodb.QueryOption{})
	if err != nil {
		return "", err
	}
	settingTmp := make(map[string]interface{})
	if len(settingTmpList) != 0 {
		settingTmp = settingTmpList[0]
	} else {
		return "", fmt.Errorf("未查询到系统配置")
	}
	settingBytes, err := json.Marshal(&settingTmp)
	if err != nil {
		return "", err
	}
	_, err = redisClient.Set(ctx, fmt.Sprintf("%s/setting", project), string(settingBytes), 0).Result()
	if err != nil {
		return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
	}
	setting := string(settingBytes)
	return setting, nil
}

// TriggerUpdate 更新redis资产数据,并发送消息通知
func TriggerUpdate(ctx context.Context, redisClient *redis.Client, project, id string, setting []byte) error {
	return cache.Update(ctx, redisClient, project, entity.T_SETTING, id, setting)
}

// TriggerDelete 删除redis资产数据,并发送消息通知
func TriggerDelete(ctx context.Context, redisClient *redis.Client, project, id string) error {
	return cache.Delete(ctx, redisClient, project, entity.T_SETTING, id)
}
