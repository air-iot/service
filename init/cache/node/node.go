package node

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
	nodeBytes, err := GetByDB(ctx, redisClient, mongoClient, project, entity.T_NODE, id)
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
	nodeInfo := entity.Node{}

	if err := json.Unmarshal(dataBytes, &nodeInfo); err != nil {
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
	locked, err = cli.HSetNX(ctx, fmt.Sprintf("%s/%s/model/lock", project, nodeInfo.Model), id, 0).Result()
	if err != nil {
		return fmt.Errorf("增加缓存数据锁错误, %v", err)
	}
	if locked {
		b, err := cli.HMSet(ctx, fmt.Sprintf("%s/%s/model", project, nodeInfo.Model), map[string]interface{}{id: dataBytes}).Result()
		if err != nil {
			return fmt.Errorf("更新缓存模型资产数据错误, %v", err)
		}
		if !b {
			return fmt.Errorf("保存数据未成功")
		}
	}

	for _, parentNode := range nodeInfo.Parent {
		locked, err := cli.HSetNX(ctx, fmt.Sprintf("%s/%s/parent/lock", project, parentNode), id, 0).Result()
		if err != nil {
			return fmt.Errorf("增加缓存数据锁错误, %v", err)
		}
		if locked {
			b, err := cli.HMSet(ctx, fmt.Sprintf("%s/%s/parent", project, parentNode), map[string]interface{}{id: dataBytes}).Result()
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

// GetByList 根据项目ID和资产ID查询数据
func GetByList(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project string, ids []string, result interface{}) (err error) {

	nodeList := make([]map[string]interface{}, 0)
	for _, id := range ids {
		deptInfo := map[string]interface{}{}
		err := Get(ctx, redisClient, mongoClient, project, id, &deptInfo)
		if err != nil {
			return err
		}
		nodeList = append(nodeList, deptInfo)
	}

	return json.CopyByJson(result, &nodeList)
}

// GetByModel 根据项目ID和资产ID查询数据
func GetByModel(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, modelID string, result interface{}) (err error) {
	nodes, err := getByDBAndModel(ctx, redisClient, mongoClient, project, modelID)
	if err != nil {
		return err
	}
	return json.CopyByJson(result, nodes)
}

func getByDBAndModel(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, modelID string) ([]map[string]interface{}, error) {
	locked, err := redisClient.SetNX(ctx, fmt.Sprintf("%s/%s/alllock", project, modelID), 0, 0).Result()
	if err != nil {
		return nil, fmt.Errorf("增加缓存类型锁错误, %v", err)
	}
	nodeTmp := make([]map[string]interface{}, 0)
	if locked {
		col := mongoClient.Database(project).Collection(entity.T_NODE)
		_, err := mongodb.FindFilter(ctx, col, &nodeTmp, mongodb.QueryOption{Filter: map[string]interface{}{
			"model": modelID,
		}})
		if err != nil {
			return nil, err
		}

		for _, nodeEle := range nodeTmp {
			idTmp, ok := nodeEle["modelID"].(string)
			if !ok {
				return nil, fmt.Errorf("资产id格式错误")
			}
			if err := Set(ctx, redisClient, project, entity.T_NODE, idTmp, nodeEle); err != nil {
				return nil, fmt.Errorf("更新redis数据错误, %s", err.Error())
			}
		}
	} else {
		nodeMaps, err := redisClient.HGetAll(ctx, fmt.Sprintf("%s/%s", project, modelID)).Result()
		if err != nil {
			return nil, fmt.Errorf("查询缓存数据错误, %v", err)
		}
		for _, e := range nodeMaps {
			modeMap := make(map[string]interface{})
			if err := json.Unmarshal([]byte(e), &modeMap); err != nil {
				return nil, fmt.Errorf("姐序列化缓存数据错误, %s", err.Error())
			}
			nodeTmp = append(nodeTmp, modeMap)
		}
	}

	return nodeTmp, nil
}

// GetByParent 根据项目ID和资产ID查询数据
func GetByParent(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, parentID string, result interface{}) (err error) {
	node, err := getByDBAndParent(ctx, redisClient, mongoClient, project, parentID)
	if err != nil {
		return err
	}
	return json.CopyByJson(result, node)
}

func getByDBAndParent(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, parentID string) ([]map[string]interface{}, error) {
	locked, err := redisClient.SetNX(ctx, fmt.Sprintf("%s/%s/parent/alllock", project, parentID), 0, 0).Result()
	if err != nil {
		return nil, fmt.Errorf("增加缓存类型锁错误, %v", err)
	}
	nodeTmp := make([]map[string]interface{}, 0)
	if locked {
		col := mongoClient.Database(project).Collection(entity.T_NODE)
		_, err := mongodb.FindFilter(ctx, col, &nodeTmp, mongodb.QueryOption{Filter: map[string]interface{}{
			"parent": parentID,
		}})
		if err != nil {
			return nil, err
		}
		for _, nodeEle := range nodeTmp {
			idTmp, ok := nodeEle["id"].(string)
			if !ok {
				return nil, fmt.Errorf("id格式错误")
			}

			if err := Set(ctx, redisClient, project, entity.T_NODE, idTmp, nodeEle); err != nil {
				return nil, fmt.Errorf("更新redis数据错误, %s", err.Error())
			}
		}
	} else {
		nodeMaps, err := redisClient.HGetAll(ctx, fmt.Sprintf("%s/%s/parent", project, parentID)).Result()
		if err != nil {
			return nil, fmt.Errorf("查询缓存数据错误, %v", err)
		}
		for _, e := range nodeMaps {
			modeMap := make(map[string]interface{})
			if err := json.Unmarshal([]byte(e), &modeMap); err != nil {
				return nil, fmt.Errorf("姐序列化缓存数据错误, %s", err.Error())
			}
			nodeTmp = append(nodeTmp, modeMap)
		}
	}
	return nodeTmp, nil
}

// TriggerUpdate 更新redis资产数据
func TriggerUpdate(ctx context.Context, cli redisdb.Client, project, id string, node interface{}) error {

	dataBytes, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("序列化数据错误, %v", err)
	}
	nodeInfo := entity.Node{}
	err = json.Unmarshal(dataBytes, &nodeInfo)
	if err != nil {
		return fmt.Errorf("解序列化错误, %v", err)
	}

	tx := cli.TxPipeline()

	_, err = tx.HSetNX(ctx, fmt.Sprintf("%s/%s/lock", project, entity.T_NODE), id, 0).Result()
	if err != nil {
		return fmt.Errorf("增加缓存数据锁错误, %v", err)
	}
	_, err = tx.HMSet(ctx, fmt.Sprintf("%s/%s", project, entity.T_NODE), map[string]interface{}{id: dataBytes}).Result()
	if err != nil {
		return fmt.Errorf("更新缓存数据错误, %v", err)
	}
	//if !b {
	//	return fmt.Errorf("更新缓存数据未成功")
	//}

	// 缓存模型下的数据
	_, err = tx.HSetNX(ctx, fmt.Sprintf("%s/%s/model/lock", project, nodeInfo.Model), id, 0).Result()
	if err != nil {
		return fmt.Errorf("增加缓存数据锁错误, %v", err)
	}
	_, err = tx.HMSet(ctx, fmt.Sprintf("%s/%s/model", project, nodeInfo.Model), map[string]interface{}{id: dataBytes}).Result()
	if err != nil {
		return fmt.Errorf("更新缓存模型资产数据错误, %v", err)
	}

	for _, parentNode := range nodeInfo.Parent {
		_, err = tx.HSetNX(ctx, fmt.Sprintf("%s/%s/parent/lock", project, parentNode), id, 0).Result()
		if err != nil {
			return fmt.Errorf("增加缓存数据锁错误, %v", err)
		}
		_, err = tx.HMSet(ctx, fmt.Sprintf("%s/%s/parent", project, parentNode), map[string]interface{}{id: dataBytes}).Result()
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

	nodeStr, err := cli.HGet(ctx, fmt.Sprintf("%s/%s", project, entity.T_NODE), id).Result()
	if err != nil {
		return fmt.Errorf("查询数据错误, %v", err)
	}
	nodeInfo := entity.Node{}
	err = json.Unmarshal([]byte(nodeStr), &nodeInfo)
	if err != nil {
		return fmt.Errorf("解序列化错误, %v", err)
	}
	tx := cli.TxPipeline()
	if _, err := tx.HDel(ctx, fmt.Sprintf("%s/%s/lock", project, entity.T_NODE), id).Result(); err != nil {
		return fmt.Errorf("删除缓存数据锁错误, %v", err)
	}
	_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s", project, entity.T_NODE), id).Result()
	if err != nil {
		return fmt.Errorf("删除缓存数据错误, %v", err)
	}

	_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s/model/lock", project, nodeInfo.Model), id).Result()
	if err != nil {
		return fmt.Errorf("删除缓存数据锁错误, %v", err)
	}
	_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s/model", project, nodeInfo.Model), id).Result()
	if err != nil {
		return fmt.Errorf("删除缓存数据错误, %v", err)
	}

	for _, parentNode := range nodeInfo.Parent {
		_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s/parent/lock", project, parentNode), id).Result()
		if err != nil {
			return fmt.Errorf("增加缓存数据锁错误, %v", err)
		}
		_, err = tx.HDel(ctx, fmt.Sprintf("%s/%s/parent", project, parentNode), id).Result()
		if err != nil {
			return fmt.Errorf("更新缓存模型资产数据错误, %v", err)
		}
	}

	if _, err := tx.Exec(ctx); err != nil {
		return fmt.Errorf("提交删除缓存数据事务错误, %v", err)
	}
	return nil
}
