package systemVariable

import (
	"context"
	"fmt"

	"github.com/go-redis/redis/v8"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/mongodb"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/util/json"
)

// Get 根据项目ID和资产ID查询数据
func Get(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, id string, result interface{}) (err error) {
	nodeBytes, err := GetByDB(ctx, redisClient, mongoClient, project, entity.T_SYSTEMVARIABLE, id)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(nodeBytes, result); err != nil {
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
	systemVariableInfo := entity.SystemVariable{}
	err = json.Unmarshal(dataBytes, &systemVariableInfo)
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
	locked, err = cli.HSetNX(ctx, fmt.Sprintf("%s/%s/uid/lock", project, tableName), systemVariableInfo.Uid, 0).Result()
	if err != nil {
		return fmt.Errorf("增加缓存数据锁错误, %v", err)
	}
	if locked {
		b, err := cli.HMSet(ctx, fmt.Sprintf("%s/%s/uid", project, tableName), map[string]interface{}{systemVariableInfo.Uid: dataBytes}).Result()
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

// GetByUid 根据项目ID和资产ID查询数据
func GetByUid(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, eventType string, result interface{}) (err error) {
	res, err := getByDBAndUid(ctx, redisClient, mongoClient, project, eventType)
	if err != nil {
		return err
	}
	return json.Unmarshal(res, result)
}

func getByDBAndUid(ctx context.Context, cli redisdb.Client, mongoClient *mongo.Client, project, uid string) ([]byte, error) {
	model, err := cli.HGet(ctx, fmt.Sprintf("%s/%s/uid", project, entity.T_SYSTEMVARIABLE), uid).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	} else if err == redis.Nil {
		col := mongoClient.Database(project).Collection(entity.T_SYSTEMVARIABLE)
		modelTmp := make(map[string]interface{})
		err := mongodb.FindOne(ctx, col, &modelTmp, bson.M{"uid": uid})
		if err != nil {
			return nil, err
		}
		modelBytes, err := json.Marshal(&modelTmp)
		if err != nil {
			return nil, err
		}
		idTmp, ok := modelTmp["id"].(string)
		if !ok {
			return nil, fmt.Errorf("id格式错误")
		}
		err = Set(ctx, cli, entity.T_SYSTEMVARIABLE, project, idTmp, modelTmp)
		if err != nil {
			return nil, fmt.Errorf("更新缓存数据错误, %v", err)
		}
		return modelBytes, nil
	}
	return []byte(model), nil
}

// TriggerUpdate 更新redis资产数据,并发送消息通知
func TriggerUpdate(ctx context.Context, cli redisdb.Client, project, id string, systemVariable interface{}) error {
	dataBytes, err := json.Marshal(systemVariable)
	if err != nil {
		return fmt.Errorf("序列化数据错误, %v", err)
	}
	systemVariableInfo := entity.SystemVariable{}
	err = json.Unmarshal(dataBytes, &systemVariableInfo)
	if err != nil {
		return fmt.Errorf("解序列化错误, %v", err)
	}
	tx := cli.TxPipeline()
	_, err = tx.HSetNX(ctx, fmt.Sprintf("%s/%s/lock", project, entity.T_SYSTEMVARIABLE), id, 0).Result()
	if err != nil {
		return fmt.Errorf("增加缓存数据锁错误, %v", err)
	}
	_, err = tx.HMSet(ctx, fmt.Sprintf("%s/%s", project, entity.T_SYSTEMVARIABLE), map[string]interface{}{id: dataBytes}).Result()
	if err != nil {
		return fmt.Errorf("更新缓存数据错误, %v", err)
	}
	//if !b {
	//	return fmt.Errorf("更新缓存数据未成功")
	//}

	// 缓存类型数据
	_, err = tx.HSetNX(ctx, fmt.Sprintf("%s/%s/uid/lock", project, entity.T_SYSTEMVARIABLE), systemVariableInfo.Uid, 0).Result()
	if err != nil {
		return fmt.Errorf("增加缓存数据锁错误, %v", err)
	}
	_, err = tx.HMSet(ctx, fmt.Sprintf("%s/%s/uid", project, entity.T_SYSTEMVARIABLE), map[string]interface{}{systemVariableInfo.Uid: dataBytes}).Result()
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

// TriggerDelete 删除redis资产数据,并发送消息通知
func TriggerDelete(ctx context.Context, cli redisdb.Client, project, id string) error {
	tx := cli.TxPipeline()

	eventStr, err := tx.HGet(ctx, fmt.Sprintf("%s/%s", project, entity.T_SYSTEMVARIABLE), id).Result()
	if err != nil {
		return fmt.Errorf("查询数据错误, %v", err)
	}
	systemVariableInfo := entity.SystemVariable{}
	err = json.Unmarshal([]byte(eventStr), &systemVariableInfo)
	if err != nil {
		return fmt.Errorf("解序列化错误, %v", err)
	}
	if _, err := tx.HDel(ctx, fmt.Sprintf("%s/%s/lock", project, entity.T_SYSTEMVARIABLE), id).Result(); err != nil {
		return fmt.Errorf("删除缓存数据锁错误, %v", err)
	}
	_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s", project, entity.T_SYSTEMVARIABLE), id).Result()
	if err != nil {
		return fmt.Errorf("删除缓存数据错误, %v", err)
	}

	_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s/uid/lock", project, entity.T_SYSTEMVARIABLE), systemVariableInfo.Uid).Result()
	if err != nil {
		return fmt.Errorf("删除缓存数据锁错误, %v", err)
	}
	_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s/uid", project, entity.T_SYSTEMVARIABLE), systemVariableInfo.Uid).Result()
	if err != nil {
		return fmt.Errorf("删除缓存数据错误, %v", err)
	}

	if _, err := tx.Exec(ctx); err != nil {
		return fmt.Errorf("提交删除缓存数据事务错误, %v", err)
	}

	return nil
}
