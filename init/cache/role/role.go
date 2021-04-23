package role

import (
	"context"
	"fmt"
	"github.com/air-iot/service/init/mongodb"
	"go.mongodb.org/mongo-driver/mongo"
	"math/rand"
	"sync"

	"github.com/go-redis/redis/v8"

	"github.com/air-iot/service/errors"
	"github.com/air-iot/service/init/cache"
	"github.com/air-iot/service/init/mq"
	"github.com/air-iot/service/logger"
	"github.com/air-iot/service/util/json"
)

const CacheRolePrefix = "cacheRole"

var MemoryRoleData = struct {
	sync.Mutex
	Cache       map[string]map[string]string
}{
	Cache:       map[string]map[string]string{},
}

// CacheHandler 初始化role缓存
func CacheHandler(ctx context.Context, redisClient *redis.Client, mqCli mq.MQ) (func(), error) {
	topic := []string{CacheRolePrefix, "#"}

	ctx = context.WithValue(ctx, "exchange", CacheRolePrefix)
	ctx = context.WithValue(ctx, "queue", fmt.Sprintf("role%d", rand.Int()))

	// 0: 前缀 1: 处理方式 2: 项目id 3: 模型id
	if err := mqCli.Consume(ctx, topic, 4, func(topic1 string, topics []string, payload []byte) {
		if len(topics) != 4 || topics[0] != CacheRolePrefix {
			logger.Errorln("缓存模型数据,消息主题(topic)格式错误")
			return
		}
		project, id := topics[2], topics[3]
		switch cache.CacheMethod(topics[1]) {
		case cache.Update:
			result, err := redisClient.HGet(ctx, fmt.Sprintf("%s/role", project), id).Result()
			if err != nil {
				logger.Errorf("缓存模型数据,查询redis缓存[ %s %s ]错误, %s", project, id, err.Error())
				return
			}
			MemoryRoleData.Lock()
			roleMap, ok := MemoryRoleData.Cache[project]
			if ok {
				roleMap[id] = result
			} else {
				roleMap = make(map[string]string)
				roleMap[id] = result
			}
			MemoryRoleData.Cache[project] = roleMap
			MemoryRoleData.Unlock()
		case cache.Delete:
			MemoryRoleData.Lock()
			roleMap, ok := MemoryRoleData.Cache[project]
			if ok {
				delete(roleMap, id)
				MemoryRoleData.Cache[project] = roleMap
			}
			MemoryRoleData.Unlock()
		default:
			return
		}

	}); err != nil {
		return nil, errors.WithStack(err)
	}

	cleanFunc := func() {
		if err := mqCli.UnSubscription(ctx, topic); err != nil {
			logger.Errorln("取消订阅role缓存错误,", err.Error())
		}
	}

	return cleanFunc, nil
}

// Get 根据项目ID和资产ID查询数据
func Get(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, id string, result interface{}) (err error) {
	MemoryRoleData.Lock()
	defer MemoryRoleData.Unlock()
	var role string
	roleMap, ok := MemoryRoleData.Cache[project]
	if ok {
		role, ok = roleMap[id]
		if !ok {
			role, err = getByDB(ctx, redisClient, mongoClient, project, id)
			if err != nil {
				return errors.WithStack(err)
			}
			roleMap[id] = role
			MemoryRoleData.Cache[project] = roleMap
		}
	} else {
		role, err = getByDB(ctx, redisClient, mongoClient, project, id)
		if err != nil {
			return errors.WithStack(err)
		}
		roleMap = make(map[string]string)
		roleMap[id] = role
		MemoryRoleData.Cache[project] = roleMap
	}

	if err := json.Unmarshal([]byte(role), result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Get 根据项目ID和资产ID查询数据
func GetByList(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project string, ids []string, result interface{}) (err error) {
	MemoryRoleData.Lock()
	defer MemoryRoleData.Unlock()
	roleList := make([]map[string]interface{},0)
	for _, id := range ids {
		roleInfo := map[string]interface{}{}
		err := Get(ctx,redisClient,mongoClient,project,id,&roleInfo)
		if err != nil {
			return errors.WithStack(err)
		}
		roleList = append(roleList,roleInfo)
	}

	resultByte,err := json.Marshal(roleList)
	if err != nil {
		return errors.WithStack(err)
	}
	if err := json.Unmarshal(resultByte, result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}


func getByDB(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, id string) (string, error) {
	role, err := redisClient.HGet(ctx, fmt.Sprintf("%s/role", project), id).Result()
	if err != nil && err != redis.Nil {
		return "", err
	} else if err == redis.Nil {
		col := mongoClient.Database(project).Collection("role")
		roleTmp := make(map[string]interface{})
		err := mongodb.FindByID(ctx, col, &roleTmp, id)
		if err != nil {
			return "", err
		}
		roleBytes, err := json.Marshal(&roleTmp)
		if err != nil {
			return "", err
		}
		_, err = redisClient.HMSet(ctx, fmt.Sprintf("%s/role", project), map[string]interface{}{id: roleBytes}).Result()
		if err != nil {
			return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
		}
		role = string(roleBytes)
	}
	return role, nil
}

// TriggerUpdate 更新redis资产数据,并发送消息通知
func TriggerUpdate(ctx context.Context, mqCli mq.MQ, redisClient *redis.Client, project, id string, role []byte) error {
	_, err := redisClient.HMSet(ctx, fmt.Sprintf("%s/role", project), map[string]interface{}{id: role}).Result()
	if err != nil {
		return fmt.Errorf("更新redis数据错误, %s", err.Error())
	}
	topics := []string{CacheRolePrefix, string(cache.Update), project, id}
	ctx = context.WithValue(ctx, "exchange", CacheRolePrefix)
	err = mqCli.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}

// TriggerDelete 删除redis资产数据,并发送消息通知
func TriggerDelete(ctx context.Context, mqCli mq.MQ, redisClient *redis.Client, project, id string) error {
	_, err := redisClient.HDel(context.Background(), fmt.Sprintf("%s/role", project), id).Result()
	if err != nil {
		return fmt.Errorf("删除redis数据错误, %s", err.Error())
	}
	topics := []string{CacheRolePrefix, string(cache.Delete), project, id}
	ctx = context.WithValue(ctx, "exchange", CacheRolePrefix)
	err = mqCli.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}
