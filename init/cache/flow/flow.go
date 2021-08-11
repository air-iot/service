package flow

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
	flowBytes, err := GetByDB(ctx, redisClient, mongoClient, project, entity.T_FLOW, id)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(flowBytes), result); err != nil {
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
		err = Set(ctx, cli, project, tableName, id, modelTmp)
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
	flowInfo := entity.Flow{}
	err = json.Unmarshal(dataBytes, &flowInfo)
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
	locked, err = cli.HSetNX(ctx, fmt.Sprintf("%s/%s/lock", project, flowInfo.Type), id, 0).Result()
	if err != nil {
		return fmt.Errorf("增加缓存数据锁错误, %v", err)
	}
	if locked {
		b, err := cli.HMSet(ctx, fmt.Sprintf("%s/%s", project, flowInfo.Type), map[string]interface{}{id: dataBytes}).Result()
		if err != nil {
			return fmt.Errorf("更新缓存流程类型数据错误, %v", err)
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

// GetByType 根据项目ID和流程类型
func GetByType(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, flowType string, result interface{}) (err error) {
	flow, err := getByDBAndType(ctx, redisClient, mongoClient, project, flowType)
	if err != nil {
		return err
	}
	return json.CopyByJson(result, flow)
}

func getByDBAndType(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, flowType string) ([]map[string]interface{}, error) {
	flowTmp := make([]map[string]interface{}, 0)
	flowMaps, err := redisClient.HGetAll(ctx, fmt.Sprintf("%s/%s", project, flowType)).Result()
	if err != nil && redis.Nil != err {
		return nil, fmt.Errorf("查询缓存数据错误, %v", err)
	} else if err == redis.Nil || len(flowTmp) == 0 {

		col := mongoClient.Database(project).Collection(entity.T_FLOW)
		_, err = mongodb.FindFilter(ctx, col, &flowTmp, mongodb.QueryOption{Filter: map[string]interface{}{
			"type": flowType,
		}})
		if err != nil {

			return nil, err
		}

		for _, flow := range flowTmp {
			idTmp, ok := flow["id"].(string)
			if !ok {
				return nil, fmt.Errorf("id格式错误")
			}
			if err := Set(ctx, redisClient, project, entity.T_FLOW, idTmp, flow); err != nil {
				return nil, fmt.Errorf("更新缓存数据错误, %s", err.Error())
			}
		}
	} else {
		for _, e := range flowMaps {
			flowMap := make(map[string]interface{})
			if err := json.Unmarshal([]byte(e), &flowMap); err != nil {
				return nil, fmt.Errorf("姐序列化缓存数据错误, %s", err.Error())
			}
			flowTmp = append(flowTmp, flowMap)
		}
	}
	return flowTmp, nil
}

// TriggerUpdate 更新redis资产数据
func TriggerUpdate(ctx context.Context, cli redisdb.Client, project, id string, flow interface{}) error {
	dataBytes, err := json.Marshal(flow)
	if err != nil {
		return fmt.Errorf("序列化数据错误, %v", err)
	}
	flowInfo := entity.Flow{}
	err = json.Unmarshal(dataBytes, &flowInfo)
	if err != nil {
		return fmt.Errorf("解序列化错误, %v", err)
	}
	tx := cli.TxPipeline()
	_, err = tx.HSetNX(ctx, fmt.Sprintf("%s/%s/lock", project, entity.T_FLOW), id, 0).Result()
	if err != nil {
		return fmt.Errorf("增加缓存数据锁错误, %v", err)
	}
	_, err = tx.HMSet(ctx, fmt.Sprintf("%s/%s", project, entity.T_FLOW), map[string]interface{}{id: dataBytes}).Result()
	if err != nil {
		return fmt.Errorf("更新缓存数据错误, %v", err)
	}
	//if !b {
	//	return fmt.Errorf("更新缓存数据未成功")
	//}

	// 缓存类型数据
	_, err = tx.HSetNX(ctx, fmt.Sprintf("%s/%s/lock", project, flowInfo.Type), id, 0).Result()
	if err != nil {
		return fmt.Errorf("增加缓存数据锁错误, %v", err)
	}
	_, err = tx.HMSet(ctx, fmt.Sprintf("%s/%s", project, flowInfo.Type), map[string]interface{}{id: dataBytes}).Result()
	if err != nil {
		return fmt.Errorf("更新缓存流程类型数据错误, %v", err)
	}
	//if !b {
	//	return fmt.Errorf("更新缓存流程类型数据未成功")
	//}

	if _, err := tx.Exec(ctx); err != nil {
		return fmt.Errorf("提交更新缓存数据事务错误, %v", err)
	}

	return nil
}

// TriggerDelete 删除redis资产数据
func TriggerDelete(ctx context.Context, cli redisdb.Client, project, id string) error {

	flowStr, err := cli.HGet(ctx, fmt.Sprintf("%s/%s", project, entity.T_FLOW), id).Result()
	if err != nil {
		return fmt.Errorf("查询数据错误, %v", err)
	}
	flowInfo := entity.Flow{}
	err = json.Unmarshal([]byte(flowStr), &flowInfo)
	if err != nil {
		return fmt.Errorf("解序列化错误, %v", err)
	}
	tx := cli.TxPipeline()

	if _, err := tx.HDel(ctx, fmt.Sprintf("%s/%s/lock", project, entity.T_FLOW), id).Result(); err != nil {
		return fmt.Errorf("删除缓存数据锁错误, %v", err)
	}
	_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s", project, entity.T_FLOW), id).Result()
	if err != nil {
		return fmt.Errorf("删除缓存数据错误, %v", err)
	}

	_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s/lock", project, flowInfo.Type), id).Result()
	if err != nil {
		return fmt.Errorf("删除缓存数据锁错误, %v", err)
	}
	_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s", project, flowInfo.Type), id).Result()
	if err != nil {
		return fmt.Errorf("删除缓存数据错误, %v", err)
	}

	if _, err := tx.Exec(ctx); err != nil {
		return fmt.Errorf("提交删除缓存数据事务错误, %v", err)
	}

	return nil
}
