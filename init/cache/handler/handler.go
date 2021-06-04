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

// Get 事件动作ID查询数据
func Get(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, id string, result interface{}) (err error) {
	eventHandlerBytes, err := GetByDB(ctx, redisClient, mongoClient, project, entity.T_EVENTHANDLER, id)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(eventHandlerBytes), result); err != nil {
		return err
	}
	return nil
}

func GetByDB(ctx context.Context, cli redisdb.Client, mongoClient *mongo.Client, project, tableName, id string) ([]byte, error) {
	model, err := cli.HGet(ctx, fmt.Sprintf("%s/%s", project, tableName), id).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	} else if err == redis.Nil {
		col := mongoClient.Database(project).Collection(tableName)
		modelTmp := make(map[string]interface{})
		err := mongodb.FindByID(ctx, col, &modelTmp, id)
		if err != nil {
			return nil, err
		}
		modelBytes, err := json.Marshal(&modelTmp)
		if err != nil {
			return nil, err
		}
		err = Set(ctx, cli, tableName, project, id, modelTmp)
		if err != nil {
			return nil, fmt.Errorf("更新缓存数据错误, %v", err)
		}
		return modelBytes, nil
	}
	return []byte(model), nil
}

func Set(ctx context.Context, cli redisdb.Client, project, tableName, id string, data interface{}) error {
	dataBytes, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化数据错误, %v", err)
	}
	eventHandlerInfo := entity.EventHandler{}
	err = json.Unmarshal(dataBytes, &eventHandlerInfo)
	if err != nil {
		return fmt.Errorf("解序列化错误, %v", err)
	}

	tx := cli.TxPipeline()
	locked, err := tx.HSetNX(ctx, fmt.Sprintf("%s/%s/lock", project, tableName), id, 0).Result()
	if err != nil {
		return fmt.Errorf("增加缓存锁错误, %v", err)
	}
	if locked {
		b, err := tx.HMSet(ctx, fmt.Sprintf("%s/%s", project, tableName), map[string]interface{}{id: dataBytes}).Result()
		if err != nil {
			return fmt.Errorf("更新缓存数据错误, %v", err)
		}
		if !b {
			return fmt.Errorf("保存数据未成功")
		}
	}

	// 缓存类型数据
	locked, err = tx.HSetNX(ctx, fmt.Sprintf("%s/%s/lock", project, eventHandlerInfo.Event), id, 0).Result()
	if err != nil {
		return fmt.Errorf("增加缓存数据锁错误, %v", err)
	}
	if locked {
		b, err := tx.HMSet(ctx, fmt.Sprintf("%s/%s", project, eventHandlerInfo.Event), map[string]interface{}{id: dataBytes}).Result()
		if err != nil {
			return fmt.Errorf("更新缓存事件的动作数据错误, %v", err)
		}
		if !b {
			return fmt.Errorf("保存事件的动作数据未成功")
		}
	}
	if _, err := tx.Exec(ctx); err != nil {
		return fmt.Errorf("提交更新缓存数据事务错误, %v", err)
	}
	return nil
}

// GetByList 事件动作ID数组查询数据
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
	event, err := getByDBAndEvent(ctx, redisClient, mongoClient, project, eventID)
	if err != nil {
		return err
	}
	if err := json.CopyByJson(result, event); err != nil {
		return err
	}
	return nil
}

func getByDBAndEvent(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, eventID string) ([]map[string]interface{}, error) {
	locked, err := redisClient.SetNX(ctx, fmt.Sprintf("%s/%s/alllock", project, eventID), 0, 0).Result()
	if err != nil {
		return nil, fmt.Errorf("增加缓存类型锁错误, %v", err)
	}
	eventHandlerTmp := make([]map[string]interface{}, 0)
	if locked {
		col := mongoClient.Database(project).Collection(entity.T_EVENTHANDLER)
		_, err := mongodb.FindFilter(ctx, col, &eventHandlerTmp, mongodb.QueryOption{Filter: map[string]interface{}{
			"event": eventID,
		}})
		if err != nil {
			return nil, err
		}
		for _, handler := range eventHandlerTmp {
			idTmp, ok := handler["id"].(string)
			if !ok {
				return nil, fmt.Errorf("id格式错误")
			}
			if err := cache.Set(ctx, redisClient, project, entity.T_EVENTHANDLER, idTmp, handler); err != nil {
				return nil, fmt.Errorf("更新缓存数据错误, %s", err.Error())
			}
		}
	} else {
		eventMaps, err := redisClient.HGetAll(ctx, fmt.Sprintf("%s/%s", project, eventID)).Result()
		if err != nil {
			return nil, fmt.Errorf("查询缓存数据错误, %v", err)
		}
		for _, e := range eventMaps {
			eventMap := make(map[string]interface{})
			if err := json.Unmarshal([]byte(e), &eventMap); err != nil {
				return nil, fmt.Errorf("姐序列化缓存数据错误, %s", err.Error())
			}
			eventHandlerTmp = append(eventHandlerTmp, eventMap)
		}
	}
	return eventHandlerTmp, nil
}

// TriggerUpdate 更新redis资产数据
func TriggerUpdate(ctx context.Context, cli *redis.Client, project, id string, handler interface{}) error {
	dataBytes, err := json.Marshal(handler)
	if err != nil {
		return fmt.Errorf("序列化数据错误, %v", err)
	}
	eventHandlerInfo := entity.EventHandler{}
	if err := json.Unmarshal(dataBytes, &eventHandlerInfo); err != nil {
		return fmt.Errorf("解序列化错误, %v", err)
	}
	tx := cli.TxPipeline()
	_, err = tx.HSetNX(ctx, fmt.Sprintf("%s/%s/lock", project, entity.T_EVENTHANDLER), id, 0).Result()
	if err != nil {
		return fmt.Errorf("增加缓存数据锁错误, %v", err)
	}
	_, err = tx.HMSet(ctx, fmt.Sprintf("%s/%s", project, entity.T_EVENTHANDLER), map[string]interface{}{id: dataBytes}).Result()
	if err != nil {
		return fmt.Errorf("更新缓存数据错误, %v", err)
	}
	//if !b {
	//	return fmt.Errorf("更新缓存数据未成功")
	//}

	// 缓存类型数据
	_, err = tx.HSetNX(ctx, fmt.Sprintf("%s/%s/lock", project, eventHandlerInfo.Event), id, 0).Result()
	if err != nil {
		return fmt.Errorf("增加缓存数据锁错误, %v", err)
	}
	_, err = tx.HMSet(ctx, fmt.Sprintf("%s/%s", project, eventHandlerInfo.Event), map[string]interface{}{id: dataBytes}).Result()
	if err != nil {
		return fmt.Errorf("更新缓存事件的动作数据错误, %v", err)
	}
	//if !b {
	//	return fmt.Errorf("更新缓存事件的动作数据未成功")
	//}

	if _, err := tx.Exec(ctx); err != nil {
		return fmt.Errorf("提交更新缓存数据事务错误, %v", err)
	}

	return nil
}

// TriggerDelete 删除redis资产数据
func TriggerDelete(ctx context.Context, cli *redis.Client, project, id string) error {

	tx := cli.TxPipeline()

	eventStr, err := tx.HGet(ctx, fmt.Sprintf("%s/%s", project, entity.T_EVENTHANDLER), id).Result()
	if err != nil {
		return fmt.Errorf("查询数据错误, %v", err)
	}
	eventHandlerInfo := entity.EventHandler{}
	if err := json.Unmarshal([]byte(eventStr), &eventHandlerInfo); err != nil {
		return fmt.Errorf("解序列化错误, %v", err)
	}
	if _, err := tx.HDel(ctx, fmt.Sprintf("%s/%s/lock", project, entity.T_EVENTHANDLER), id).Result(); err != nil {
		return fmt.Errorf("删除缓存数据锁错误, %v", err)
	}
	_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s", project, entity.T_EVENTHANDLER), id).Result()
	if err != nil {
		return fmt.Errorf("删除缓存数据错误, %v", err)
	}

	_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s/lock", project, eventHandlerInfo.Event), id).Result()
	if err != nil {
		return fmt.Errorf("删除缓存数据锁错误, %v", err)
	}
	_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s", project, eventHandlerInfo.Event), id).Result()
	if err != nil {
		return fmt.Errorf("删除缓存数据错误, %v", err)
	}

	if _, err := tx.Exec(ctx); err != nil {
		return fmt.Errorf("提交删除缓存数据事务错误, %v", err)
	}
	return nil
}
