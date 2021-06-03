package handler

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

// Get 根据项目ID和事件动作ID查询数据
func Get(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, id string, result interface{}) (err error) {
	return cache.Get(ctx, redisClient, mongoClient, project, entity.T_EVENTHANDLER, id, result)
}

// GetByList 根据项目ID和资产ID查询数据
func GetByList(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project string, ids []string, result interface{}) (err error) {
	handlerList := make([]map[string]interface{}, 0)
	for _, id := range ids {
		deptInfo := map[string]interface{}{}
		err := Get(ctx, redisClient, mongoClient, project, id, &deptInfo)
		if err != nil {
			return err
		}
		handlerList = append(handlerList, deptInfo)
	}
	return json.CopyByJson(result, handlerList)
}

// GetByEvent 根据项目ID和资产ID查询数据
func GetByEvent(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, eventID string, result interface{}) (err error) {
	event, err := GetByDBAndEvent(ctx, redisClient, mongoClient, project, eventID)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(event), result); err != nil {
		return err
	}
	return nil
}

func GetByDBAndEvent(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, eventID string) (string, error) {
	col := mongoClient.Database(project).Collection(entity.T_EVENTHANDLER)
	eventHandlerTmp := make([]map[string]interface{}, 0)
	_, err := mongodb.FindFilter(ctx, col, &eventHandlerTmp, mongodb.QueryOption{Filter: map[string]interface{}{
		"event": eventID,
	}})
	if err != nil {
		return "", err
	}
	eventHandlerBytes, err := json.Marshal(&eventHandlerTmp)
	if err != nil {
		return "", err
	}
	for _, handler := range eventHandlerTmp {
		eventHandlerEleBytes, err := json.Marshal(&handler)
		if err != nil {
			return "", err
		}
		idTmp, ok := handler["id"].(string)
		if !ok {
			return "", fmt.Errorf("id格式错误")
		}
		if err := cache.Set(ctx, redisClient, project, entity.T_EVENTHANDLER, idTmp, eventHandlerEleBytes); err != nil {
			return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
		}
	}
	eventHandler := string(eventHandlerBytes)
	return eventHandler, nil
}

// TriggerUpdate 更新redis资产数据
func TriggerUpdate(ctx context.Context, redisClient *redis.Client, project, id string, handler []byte) error {
	return cache.Update(ctx, redisClient, project, entity.T_EVENTHANDLER, id, handler)
}

// TriggerDelete 删除redis资产数据
func TriggerDelete(ctx context.Context, redisClient *redis.Client, project, id string) error {
	return cache.Delete(ctx, redisClient, project, entity.T_EVENTHANDLER, id)
}
