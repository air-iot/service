package user

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/air-iot/service/errors"
	"github.com/air-iot/service/init/cache"
	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/mongodb"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/util/formatx"
	"github.com/air-iot/service/util/json"
)

// Get 根据项目ID和资产ID查询数据
func Get(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, id string, result interface{}) (err error) {
	return cache.Get(ctx, redisClient, mongoClient, project, entity.T_USER, id, result)
}

// GetByDept 根据项目ID和资产ID查询数据
func GetByDept(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project string, ids []string, result interface{}) (err error) {
	var user string
	userList := make([]map[string]interface{}, 0)
	for _, id := range ids {
		user, err = GetByDBAndDept(ctx, redisClient, mongoClient, project, id)
		if err != nil {
			return err
		}
		userInfo := map[string]interface{}{}
		if err := json.Unmarshal([]byte(user), &userInfo); err != nil {
			return err
		}
		userList = formatx.AddNonRepMapByLoop(userList, userInfo)
	}

	return json.CopyByJson(result, &userList)
}

func GetByDBAndDept(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, id string) (string, error) {
	col := mongoClient.Database(project).Collection(entity.T_USER)
	userTmp := make([]map[string]interface{}, 0)
	pipeline := mongo.Pipeline{bson.D{bson.E{Key: "$match", Value: bson.M{"department": id}}}}
	err := mongodb.FindPipeline(ctx, col, &userTmp, pipeline)
	if err != nil {
		return "", err
	}
	userBytes, err := json.Marshal(&userTmp)
	if err != nil {
		return "", err
	}
	for _, user := range userTmp {
		userEleBytes, err := json.Marshal(&user)
		if err != nil {
			return "", err
		}
		idTmp, ok := user["id"].(string)
		if !ok {
			return "", fmt.Errorf("id格式错误")
		}

		if err := cache.Set(ctx, redisClient, project, entity.T_USER, idTmp, userEleBytes); err != nil {
			return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
		}
	}
	user := string(userBytes)
	return user, nil
}

// GetByRole 根据项目ID和资产ID查询数据
func GetByRole(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project string, ids []string, result interface{}) (err error) {
	var user string
	userList := make([]map[string]interface{}, 0)
	for _, id := range ids {
		user, err = GetByDBAndRole(ctx, redisClient, mongoClient, project, id)
		if err != nil {
			return errors.WithStack(err)
		}
		userInfo := map[string]interface{}{}
		if err := json.Unmarshal([]byte(user), &userInfo); err != nil {
			return errors.WithStack(err)
		}
		userList = formatx.AddNonRepMapByLoop(userList, userInfo)
	}

	resultByte, err := json.Marshal(userList)
	if err != nil {
		return errors.WithStack(err)
	}
	if err := json.Unmarshal(resultByte, result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func GetByDBAndRole(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, project, id string) (string, error) {
	//user, err := redisClient.HGet(ctx, fmt.Sprintf("%s/user", project), id).Result()
	//if err != nil && err != redis.Nil {
	//	return "", err
	//} else if err == redis.Nil {
	col := mongoClient.Database(project).Collection("user")
	userTmp := make([]entity.UserMongo, 0)
	paramMatch := bson.D{bson.E{Key: "$match", Value: bson.M{"roles": id}}}
	pipeline := mongo.Pipeline{}
	pipeline = append(pipeline, paramMatch)
	err := mongodb.FindPipeline(ctx, col, &userTmp, pipeline)
	if err != nil {
		return "", err
	}
	userBytes, err := json.Marshal(&userTmp)
	if err != nil {
		return "", err
	}
	for _, user := range userTmp {
		userEleBytes, err := json.Marshal(&user)
		if err != nil {
			return "", err
		}
		_, err = redisClient.HMSet(ctx, fmt.Sprintf("%s/user", project), map[string]interface{}{user.ID: userEleBytes}).Result()
		if err != nil {
			return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
		}
	}
	user := string(userBytes)
	//}
	return user, nil
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

// TriggerUpdate 更新redis资产数据,并发送消息通知
func TriggerUpdate(ctx context.Context, redisClient redisdb.Client, project, id string, user []byte) error {
	return cache.Update(ctx, redisClient, project, entity.T_USER, id, user)
}

// TriggerDelete 删除redis资产数据,并发送消息通知
func TriggerDelete(ctx context.Context, redisClient redisdb.Client, project, id string) error {
	return cache.Delete(ctx, redisClient, project, entity.T_USER, id)
}
