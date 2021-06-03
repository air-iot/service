package event

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
	return cache.Get(ctx, redisClient, mongoClient, project, entity.T_EVENT, id, result)
}

// GetByType 根据项目ID和事件类型
func GetByType(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, eventType string, result interface{}) (err error) {
	event, err := GetByDBAndType(ctx, redisClient, mongoClient, project, eventType)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(event), result); err != nil {
		return err
	}
	return nil
}

func GetByDBAndType(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, eventType string) (string, error) {
	col := mongoClient.Database(project).Collection(entity.T_EVENT)
	eventTmp := make([]map[string]interface{}, 0)
	_, err := mongodb.FindFilter(ctx, col, &eventTmp, mongodb.QueryOption{Filter: map[string]interface{}{
		"type": eventType,
		"$lookups": []interface{}{
			map[string]interface{}{
				"from":         entity.T_EVENTHANDLER,
				"localField":   "_id",
				"foreignField": "event",
				"as":           "handlers",
			},
		},
	}})

	if err != nil {
		return "", err
	}
	eventBytes, err := json.Marshal(&eventTmp)
	if err != nil {
		return "", err
	}
	for _, event := range eventTmp {
		eventEleBytes, err := json.Marshal(&event)
		if err != nil {
			return "", err
		}
		idTmp, ok := event["id"].(string)
		if !ok {
			return "", fmt.Errorf("id格式错误")
		}
		if err := cache.Set(ctx, redisClient, project, entity.T_EVENT, idTmp, eventEleBytes); err != nil {
			return "", fmt.Errorf("更新缓存数据错误, %s", err.Error())
		}
	}
	event := string(eventBytes)
	return event, nil
}

// TriggerUpdate 更新redis资产数据
func TriggerUpdate(ctx context.Context, cli redisdb.Client, project, id string, event []byte) error {
	return cache.Update(ctx, cli, project, entity.T_EVENT, id, event)
}

// TriggerDelete 删除redis资产数据
func TriggerDelete(ctx context.Context, cli redisdb.Client, project, id string) error {
	return cache.Delete(ctx, cli, project, entity.T_EVENT, id)
}
