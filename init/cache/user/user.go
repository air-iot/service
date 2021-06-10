package user

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/air-iot/service/errors"
	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/mongodb"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/util/formatx"
	"github.com/air-iot/service/util/json"
)

// Get 根据项目ID和资产ID查询数据
func Get(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, id string, result interface{}) (err error) {
	userBytes, err := GetByDB(ctx, redisClient, mongoClient, project, entity.T_USER, id)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(userBytes, result); err != nil {
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
	userInfo := entity.User{}

	if err := json.Unmarshal(dataBytes, &userInfo); err != nil {
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

	for _, dept := range userInfo.Department {
		// 缓存类型数据
		locked, err := cli.HSetNX(ctx, fmt.Sprintf("%s/%s/dept/lock", project, dept), id, 0).Result()
		if err != nil {
			return fmt.Errorf("增加缓存数据锁错误, %v", err)
		}
		if locked {
			b, err := cli.HMSet(ctx, fmt.Sprintf("%s/%s/dept", project, dept), map[string]interface{}{id: dataBytes}).Result()
			if err != nil {
				return fmt.Errorf("更新缓存模型资产数据错误, %v", err)
			}
			if !b {
				return fmt.Errorf("保存数据未成功")
			}
		}
	}

	for _, role := range userInfo.Roles {
		locked, err := cli.HSetNX(ctx, fmt.Sprintf("%s/%s/role/lock", project, role), id, 0).Result()
		if err != nil {
			return fmt.Errorf("增加缓存数据锁错误, %v", err)
		}
		if locked {
			b, err := cli.HMSet(ctx, fmt.Sprintf("%s/%s/role", project, role), map[string]interface{}{id: dataBytes}).Result()
			if err != nil {
				return fmt.Errorf("更新缓存模型资产数据错误, %v", err)
			}
			if !b {
				return fmt.Errorf("保存数据未成功")
			}
		}
	}
	//if _, err := cli.Exec(ctx); err != nil {
	//	return fmt.Errorf("提交更新缓存数据事务错误, %v", err)
	//}
	return nil
}

// GetByDept 根据项目ID和资产ID查询数据
func GetByDept(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project string, ids []string, result interface{}) (err error) {
	userList := make([]map[string]interface{}, 0)
	for _, id := range ids {
		users, err := getByDBAndDept(ctx, redisClient, mongoClient, project, id)
		if err != nil {
			return err
		}
		for _, user := range users {
			userList = formatx.AddNonRepMapByLoop(userList, user)
		}
	}
	return json.CopyByJson(result, &userList)
}

func getByDBAndDept(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, deptID string) ([]map[string]interface{}, error) {
	locked, err := redisClient.SetNX(ctx, fmt.Sprintf("%s/%s/dept/alllock", project, deptID), 0, 0).Result()
	if err != nil {
		return nil, fmt.Errorf("增加缓存类型锁错误, %v", err)
	}
	userTmp := make([]map[string]interface{}, 0)
	if locked {
		col := mongoClient.Database(project).Collection(entity.T_USER)
		pipeline := mongo.Pipeline{bson.D{bson.E{Key: "$match", Value: bson.M{"department": deptID}}}}
		err := mongodb.FindPipeline(ctx, col, &userTmp, pipeline)
		if err != nil {
			return nil, err
		}
		for _, user := range userTmp {
			idTmp, ok := user["id"].(string)
			if !ok {
				return nil, fmt.Errorf("id格式错误")
			}

			if err := Set(ctx, redisClient, project, entity.T_USER, idTmp, user); err != nil {
				return nil, fmt.Errorf("更新redis数据错误, %s", err.Error())
			}
		}
	} else {
		nodeMaps, err := redisClient.HGetAll(ctx, fmt.Sprintf("%s/%s/dept", project, deptID)).Result()
		if err != nil {
			return nil, fmt.Errorf("查询缓存数据错误, %v", err)
		}
		for _, e := range nodeMaps {
			modeMap := make(map[string]interface{})
			if err := json.Unmarshal([]byte(e), &modeMap); err != nil {
				return nil, fmt.Errorf("姐序列化缓存数据错误, %s", err.Error())
			}
			userTmp = append(userTmp, modeMap)
		}
	}
	return userTmp, nil
}

// GetByRole 根据项目ID和资产ID查询数据
func GetByRole(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project string, ids []string, result interface{}) (err error) {
	userList := make([]map[string]interface{}, 0)
	for _, id := range ids {
		users, err := getByDBAndRole(ctx, redisClient, mongoClient, project, id)
		if err != nil {
			return errors.WithStack(err)
		}
		for _, user := range users {
			userList = formatx.AddNonRepMapByLoop(userList, user)
		}
	}

	return json.CopyByJson(result, &userList)
}

func getByDBAndRole(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, roleID string) ([]map[string]interface{}, error) {
	locked, err := redisClient.SetNX(ctx, fmt.Sprintf("%s/%s/role/alllock", project, roleID), 0, 0).Result()
	if err != nil {
		return nil, fmt.Errorf("增加缓存类型锁错误, %v", err)
	}
	userTmp := make([]map[string]interface{}, 0)

	if locked {
		col := mongoClient.Database(project).Collection(entity.T_USER)
		paramMatch := bson.D{bson.E{Key: "$match", Value: bson.M{"roles": roleID}}}
		pipeline := mongo.Pipeline{}
		pipeline = append(pipeline, paramMatch)
		err := mongodb.FindPipeline(ctx, col, &userTmp, pipeline)
		if err != nil {
			return nil, err
		}

		for _, user := range userTmp {
			idTmp, ok := user["id"].(string)
			if !ok {
				return nil, fmt.Errorf("id格式错误")
			}
			if err := Set(ctx, redisClient, project, entity.T_USER, idTmp, user); err != nil {
				return nil, fmt.Errorf("更新redis数据错误, %s", err.Error())
			}
		}
	} else {
		nodeMaps, err := redisClient.HGetAll(ctx, fmt.Sprintf("%s/%s/role", project, roleID)).Result()
		if err != nil {
			return nil, fmt.Errorf("查询缓存数据错误, %v", err)
		}
		for _, e := range nodeMaps {
			modeMap := make(map[string]interface{})
			if err := json.Unmarshal([]byte(e), &modeMap); err != nil {
				return nil, fmt.Errorf("姐序列化缓存数据错误, %s", err.Error())
			}
			userTmp = append(userTmp, modeMap)
		}
	}
	return userTmp, nil
}

// GetByList 根据项目ID和资产ID查询数据
func GetByList(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project string, ids []string, result interface{}) (err error) {
	userList := make([]map[string]interface{}, 0)
	for _, id := range ids {
		userInfo := map[string]interface{}{}
		err := Get(ctx, redisClient, mongoClient, project, id, &userInfo)
		if err != nil {
			return err
		}
		userList = append(userList, userInfo)
	}
	return json.CopyByJson(result, &userList)
}

// TriggerUpdate 更新redis资产数据
func TriggerUpdate(ctx context.Context, cli redisdb.Client, project, id string, user interface{}) error {
	dataBytes, err := json.Marshal(user)
	if err != nil {
		return fmt.Errorf("序列化数据错误, %v", err)
	}
	userInfo := entity.User{}
	err = json.Unmarshal(dataBytes, &userInfo)
	if err != nil {
		return fmt.Errorf("解序列化错误, %v", err)
	}

	tx := cli.TxPipeline()

	_, err = tx.HSetNX(ctx, fmt.Sprintf("%s/%s/lock", project, entity.T_USER), id, 0).Result()
	if err != nil {
		return fmt.Errorf("增加缓存数据锁错误, %v", err)
	}
	_, err = tx.HMSet(ctx, fmt.Sprintf("%s/%s", project, entity.T_USER), map[string]interface{}{id: dataBytes}).Result()
	if err != nil {
		return fmt.Errorf("更新缓存数据错误, %v", err)
	}
	//if !b {
	//	return fmt.Errorf("更新缓存数据未成功")
	//}

	for _, dept := range userInfo.Department {
		// 缓存模型下的数据
		_, err = tx.HSetNX(ctx, fmt.Sprintf("%s/%s/dept/lock", project, dept), id, 0).Result()
		if err != nil {
			return fmt.Errorf("增加缓存数据锁错误, %v", err)
		}
		_, err = tx.HMSet(ctx, fmt.Sprintf("%s/%s/dept", project, dept), map[string]interface{}{id: dataBytes}).Result()
		if err != nil {
			return fmt.Errorf("更新缓存模型资产数据错误, %v", err)
		}
	}

	for _, role := range userInfo.Roles {
		_, err = tx.HSetNX(ctx, fmt.Sprintf("%s/%s/role/lock", project, role), id, 0).Result()
		if err != nil {
			return fmt.Errorf("增加缓存数据锁错误, %v", err)
		}
		_, err = tx.HMSet(ctx, fmt.Sprintf("%s/%s/role", project, role), map[string]interface{}{id: dataBytes}).Result()
		if err != nil {
			return fmt.Errorf("更新缓存模型资产数据错误, %v", err)
		}
	}
	//if !b {
	//	return fmt.Errorf("更新缓存模型资产数据未成功")
	//}

	if _, err := tx.Exec(ctx); err != nil {
		return fmt.Errorf("提交更新缓存数据事务错误, %v", err)
	}

	return nil
}

// TriggerDelete 删除redis资产数据
func TriggerDelete(ctx context.Context, cli redisdb.Client, project, id string) error {

	nodeStr, err := cli.HGet(ctx, fmt.Sprintf("%s/%s", project, entity.T_USER), id).Result()
	if err != nil {
		return fmt.Errorf("查询数据错误, %v", err)
	}
	userInfo := entity.User{}
	err = json.Unmarshal([]byte(nodeStr), &userInfo)
	if err != nil {
		return fmt.Errorf("解序列化错误, %v", err)
	}
	tx := cli.TxPipeline()
	if _, err := tx.HDel(ctx, fmt.Sprintf("%s/%s/lock", project, entity.T_USER), id).Result(); err != nil {
		return fmt.Errorf("删除缓存数据锁错误, %v", err)
	}
	_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s", project, entity.T_USER), id).Result()
	if err != nil {
		return fmt.Errorf("删除缓存数据错误, %v", err)
	}
	for _, dept := range userInfo.Department {
		_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s/dept/lock", project, dept), id).Result()
		if err != nil {
			return fmt.Errorf("删除缓存数据锁错误, %v", err)
		}
		_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s/dept", project, dept), id).Result()
		if err != nil {
			return fmt.Errorf("删除缓存数据错误, %v", err)
		}
	}

	for _, role := range userInfo.Roles {
		_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s/role/lock", project, role), id).Result()
		if err != nil {
			return fmt.Errorf("增加缓存数据锁错误, %v", err)
		}
		_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s/role", project, role), id).Result()
		if err != nil {
			return fmt.Errorf("更新缓存模型资产数据错误, %v", err)
		}
	}

	if _, err := tx.Exec(ctx); err != nil {
		return fmt.Errorf("提交删除缓存数据事务错误, %v", err)
	}
	return nil
}
