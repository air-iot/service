package systemVariable

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/mongo"

	"github.com/air-iot/service/init/cache"
	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/mongodb"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/util/json"
)

// Get 根据项目ID和资产ID查询数据
func Get(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, id string, result interface{}) (err error) {
	return cache.Get(ctx, redisClient, mongoClient, project, entity.T_SYSTEMVARIABLE, id, result)
}

// GetByUid 根据项目ID和资产ID查询数据
func GetByUid(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, eventType string, result interface{}) (err error) {
	res, err := GetByDBAndUid(ctx, redisClient, mongoClient, project, eventType)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(res), result); err != nil {
		return err
	}
	return nil
}

func GetByDBAndUid(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, eventType string) (string, error) {
	col := mongoClient.Database(project).Collection(entity.T_SYSTEMVARIABLE)
	tmp := make([]map[string]interface{}, 0)
	_, err := mongodb.FindFilter(ctx, col, &tmp, mongodb.QueryOption{Filter: map[string]interface{}{
		"uid": eventType,
	}})

	if err != nil {
		return "", err
	}
	eventBytes := make([]byte, 0)
	if len(tmp) != 0 {
		eventBytes, err = json.Marshal(tmp[0])
		if err != nil {
			return "", err
		}
	}
	for _, tmp1 := range tmp {
		eventEleBytes, err := json.Marshal(&tmp1)
		if err != nil {
			return "", err
		}
		idTmp, ok := tmp1["id"].(string)
		if !ok {
			return "", fmt.Errorf("资产id格式错误")
		}

		if err := cache.Set(ctx, redisClient, project, entity.T_SYSTEMVARIABLE, idTmp, eventEleBytes); err != nil {
			return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
		}
	}
	event := string(eventBytes)
	return event, nil
}

// TriggerUpdate 更新redis资产数据,并发送消息通知
func TriggerUpdate(ctx context.Context, redisClient redisdb.Client, project, id string, systemVariable []byte) error {
	return cache.Update(ctx, redisClient, project, entity.T_SYSTEMVARIABLE, id, systemVariable)
}

// TriggerDelete 删除redis资产数据,并发送消息通知
func TriggerDelete(ctx context.Context, redisClient redisdb.Client, project, id string) error {
	return cache.Delete(ctx, redisClient, project, entity.T_SYSTEMVARIABLE, id)
}
