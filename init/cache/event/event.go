package event

import (
	"context"
	"fmt"

	"github.com/go-redis/redis/v8"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/mongodb"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/util/json"
)

// Get 根据项目ID和资产ID查询数据
func Get(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, id string, result interface{}) (err error) {
	eventBytes, err := GetByDB(ctx, redisClient, mongoClient, project, entity.T_EVENT, id)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(eventBytes), result); err != nil {
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
		err = Set(ctx, cli, project,tableName, id, modelTmp)
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
	eventInfo := entity.Event{}
	err = json.Unmarshal(dataBytes, &eventInfo)
	if err != nil {
		return fmt.Errorf("解序列化错误, %v", err)
	}

	//tx := cli.TxPipeline()
	locked, err := cli.HSetNX(ctx, fmt.Sprintf("%s/%s/lock", project, tableName), id, 0).Result()
	if err != nil {
		return fmt.Errorf("增加缓存锁错误, %v", err)
	}
	if locked {
		b, err := cli.HMSet(ctx, fmt.Sprintf("%s/%s", project, tableName), map[string]interface{}{id: dataBytes}).Result()
		if err != nil {
			return fmt.Errorf("更新缓存数据错误, %v", err)
		}
		if !b {
			return fmt.Errorf("保存数据未成功")
		}
	}

	// 缓存类型数据
	locked, err = cli.HSetNX(ctx, fmt.Sprintf("%s/%s/lock", project, eventInfo.Type), id, 0).Result()
	if err != nil {
		return fmt.Errorf("增加缓存数据锁错误, %v", err)
	}
	if locked {
		b, err := cli.HMSet(ctx, fmt.Sprintf("%s/%s", project, eventInfo.Type), map[string]interface{}{id: dataBytes}).Result()
		if err != nil {
			return fmt.Errorf("更新缓存事件类型数据错误, %v", err)
		}
		if !b {
			return fmt.Errorf("保存数据未成功")
		}
	}
	//if _, err := cli.Exec(ctx); err != nil {
	//	return fmt.Errorf("提交更新缓存数据事务错误, %v", err)
	//}
	return nil
}

// GetByType 根据项目ID和事件类型
func GetByType(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, eventType string, result interface{}) (err error) {
	event, err := getByDBAndType(ctx, redisClient, mongoClient, project, eventType)
	if err != nil {
		return err
	}
	return json.CopyByJson(result, event)
}

func getByDBAndType(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, eventType string) ([]map[string]interface{}, error) {
	locked, err := redisClient.SetNX(ctx, fmt.Sprintf("%s/%s/alllock", project, eventType), 0, 0).Result()
	if err != nil {
		return nil, fmt.Errorf("增加缓存类型锁错误, %v", err)
	}
	eventTmp := make([]map[string]interface{}, 0)
	if locked {
		col := mongoClient.Database(project).Collection(entity.T_EVENT)
		_, err = mongodb.FindFilter(ctx, col, &eventTmp, mongodb.QueryOption{Filter: map[string]interface{}{
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
			return nil, err
		}

		for _, event := range eventTmp {
			idTmp, ok := event["id"].(string)
			if !ok {
				return nil, fmt.Errorf("id格式错误")
			}
			if err := Set(ctx, redisClient, project, entity.T_EVENT, idTmp, event); err != nil {
				return nil, fmt.Errorf("更新缓存数据错误, %s", err.Error())
			}
		}

	} else {
		eventMaps, err := redisClient.HGetAll(ctx, fmt.Sprintf("%s/%s", project, eventType)).Result()
		if err != nil {
			return nil, fmt.Errorf("查询缓存数据错误, %v", err)
		}
		for _, e := range eventMaps {
			eventMap := make(map[string]interface{})
			if err := json.Unmarshal([]byte(e), &eventMap); err != nil {
				return nil, fmt.Errorf("姐序列化缓存数据错误, %s", err.Error())
			}
			eventTmp = append(eventTmp, eventMap)
		}
	}
	return eventTmp, nil
}

// TriggerUpdate 更新redis资产数据
func TriggerUpdate(ctx context.Context, cli redisdb.Client, project, id string, event interface{}) error {
	dataBytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("序列化数据错误, %v", err)
	}
	eventInfo := entity.Event{}
	err = json.Unmarshal(dataBytes, &eventInfo)
	if err != nil {
		return fmt.Errorf("解序列化错误, %v", err)
	}
	tx := cli.TxPipeline()
	_, err = tx.HSetNX(ctx, fmt.Sprintf("%s/%s/lock", project, entity.T_EVENT), id, 0).Result()
	if err != nil {
		return fmt.Errorf("增加缓存数据锁错误, %v", err)
	}
	_, err = tx.HMSet(ctx, fmt.Sprintf("%s/%s", project, entity.T_EVENT), map[string]interface{}{id: dataBytes}).Result()
	if err != nil {
		return fmt.Errorf("更新缓存数据错误, %v", err)
	}
	//if !b {
	//	return fmt.Errorf("更新缓存数据未成功")
	//}

	// 缓存类型数据
	_, err = tx.HSetNX(ctx, fmt.Sprintf("%s/%s/lock", project, eventInfo.Type), id, 0).Result()
	if err != nil {
		return fmt.Errorf("增加缓存数据锁错误, %v", err)
	}
	_, err = tx.HMSet(ctx, fmt.Sprintf("%s/%s", project, eventInfo.Type), map[string]interface{}{id: dataBytes}).Result()
	if err != nil {
		return fmt.Errorf("更新缓存事件类型数据错误, %v", err)
	}
	//if !b {
	//	return fmt.Errorf("更新缓存事件类型数据未成功")
	//}

	if _, err := tx.Exec(ctx); err != nil {
		return fmt.Errorf("提交更新缓存数据事务错误, %v", err)
	}

	return nil
}

// TriggerDelete 删除redis资产数据
func TriggerDelete(ctx context.Context, cli redisdb.Client, project, id string) error {

	eventStr, err := cli.HGet(ctx, fmt.Sprintf("%s/%s", project, entity.T_EVENT), id).Result()
	if err != nil {
		return fmt.Errorf("查询数据错误, %v", err)
	}
	eventInfo := entity.Event{}
	err = json.Unmarshal([]byte(eventStr), &eventInfo)
	if err != nil {
		return fmt.Errorf("解序列化错误, %v", err)
	}
	tx := cli.TxPipeline()

	if _, err := tx.HDel(ctx, fmt.Sprintf("%s/%s/lock", project, entity.T_EVENT), id).Result(); err != nil {
		return fmt.Errorf("删除缓存数据锁错误, %v", err)
	}
	_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s", project, entity.T_EVENT), id).Result()
	if err != nil {
		return fmt.Errorf("删除缓存数据错误, %v", err)
	}

	_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s/lock", project, eventInfo.Type), id).Result()
	if err != nil {
		return fmt.Errorf("删除缓存数据锁错误, %v", err)
	}
	_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s", project, eventInfo.Type), id).Result()
	if err != nil {
		return fmt.Errorf("删除缓存数据错误, %v", err)
	}

	if _, err := tx.Exec(ctx); err != nil {
		return fmt.Errorf("提交删除缓存数据事务错误, %v", err)
	}

	return nil
}
