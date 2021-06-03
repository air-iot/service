package node

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
	return cache.Get(ctx, redisClient, mongoClient, project, entity.T_NODE, id, result)
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
	node, err := GetByDBAndModel(ctx, redisClient, mongoClient, project, modelID)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(node), result); err != nil {
		return err
	}
	return nil
}

func GetByDBAndModel(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, id string) (string, error) {
	col := mongoClient.Database(project).Collection(entity.T_NODE)
	nodeTmp := make([]map[string]interface{}, 0)
	_, err := mongodb.FindFilter(ctx, col, &nodeTmp, mongodb.QueryOption{Filter: map[string]interface{}{
		"model": id,
	}})
	if err != nil {
		return "", err
	}
	nodeBytes, err := json.Marshal(&nodeTmp)
	if err != nil {
		return "", err
	}
	for _, nodeEle := range nodeTmp {
		idTmp, ok := nodeEle["id"].(string)
		if !ok {
			return "", fmt.Errorf("资产id格式错误")
		}
		nodeEleBytes, err := json.Marshal(&nodeEle)
		if err != nil {
			return "", err
		}
		if err := cache.Set(ctx, redisClient, project, entity.T_NODE, idTmp, nodeEleBytes); err != nil {
			return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
		}
	}
	node := string(nodeBytes)
	return node, nil
}

// GetByParent 根据项目ID和资产ID查询数据
func GetByParent(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, parentID string, result interface{}) (err error) {
	node, err := GetByDBAndParent(ctx, redisClient, mongoClient, project, parentID)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(node), result); err != nil {
		return err
	}
	return nil
}

func GetByDBAndParent(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, id string) (string, error) {
	col := mongoClient.Database(project).Collection(entity.T_NODE)
	nodeTmp := make([]map[string]interface{}, 0)
	_, err := mongodb.FindFilter(ctx, col, &nodeTmp, mongodb.QueryOption{Filter: map[string]interface{}{
		"parent": id,
	}})
	if err != nil {
		return "", err
	}
	nodeBytes, err := json.Marshal(&nodeTmp)
	if err != nil {
		return "", err
	}
	for _, nodeEle := range nodeTmp {
		nodeEleBytes, err := json.Marshal(&nodeEle)
		if err != nil {
			return "", err
		}
		idTmp, ok := nodeEle["id"].(string)
		if !ok {
			return "", fmt.Errorf("id格式错误")
		}

		if err := cache.Set(ctx, redisClient, project, entity.T_NODE, idTmp, nodeEleBytes); err != nil {
			return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
		}
	}
	node := string(nodeBytes)
	return node, nil
}

// TriggerUpdate 更新redis资产数据
func TriggerUpdate(ctx context.Context, cli redisdb.Client, project, id string, node []byte) error {
	return cache.Update(ctx, cli, project, entity.T_NODE, id, node)
}

// TriggerDelete 删除redis资产数据
func TriggerDelete(ctx context.Context, cli redisdb.Client, project, id string) error {
	return cache.Delete(ctx, cli, project, entity.T_NODE, id)
}
