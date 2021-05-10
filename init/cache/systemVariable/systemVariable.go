package systemVariable

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

const CacheSystemVariablePrefix = "cacheSystemVariable"

var MemorySystemVariableData = struct {
	sync.Mutex
	Cache map[string]map[string]string
}{
	Cache: map[string]map[string]string{},
}

// CacheHandler 初始化systemVariable缓存
func CacheHandler(ctx context.Context, redisClient *redis.Client, mqCli mq.MQ) (func(), error) {
	topic := []string{CacheSystemVariablePrefix, "#"}

	ctx = context.WithValue(ctx, "exchange", CacheSystemVariablePrefix)
	ctx = context.WithValue(ctx, "queue", fmt.Sprintf("systemVariable%d", rand.Int()))

	// 0: 前缀 1: 处理方式 2: 项目id 3: 系统变量id
	if err := mqCli.Consume(ctx, topic, 4, func(topic1 string, topics []string, payload []byte) {
		if len(topics) != 4 || topics[0] != CacheSystemVariablePrefix {
			logger.Errorln("缓存系统变量数据,消息主题(topic)格式错误")
			return
		}
		project, id := topics[2], topics[3]
		switch cache.CacheMethod(topics[1]) {
		case cache.Update:
			result, err := redisClient.HGet(ctx, fmt.Sprintf("%s/systemVariable", project), id).Result()
			if err != nil {
				logger.Errorf("缓存系统变量数据,查询redis缓存[ %s %s ]错误, %s", project, id, err.Error())
				return
			}
			MemorySystemVariableData.Lock()
			systemVariableMap, ok := MemorySystemVariableData.Cache[project]
			if ok {
				systemVariableMap[id] = result
			} else {
				systemVariableMap = make(map[string]string)
				systemVariableMap[id] = result
			}
			MemorySystemVariableData.Cache[project] = systemVariableMap
			MemorySystemVariableData.Unlock()
		case cache.Delete:
			MemorySystemVariableData.Lock()
			systemVariableMap, ok := MemorySystemVariableData.Cache[project]
			if ok {
				delete(systemVariableMap, id)
				MemorySystemVariableData.Cache[project] = systemVariableMap
			}
			MemorySystemVariableData.Unlock()
		default:
			return
		}

	}); err != nil {
		return nil, errors.WithStack(err)
	}

	cleanFunc := func() {
		if err := mqCli.UnSubscription(ctx, topic); err != nil {
			logger.Errorln("取消订阅systemVariable缓存错误,", err.Error())
		}
	}

	return cleanFunc, nil
}

// Get 根据项目ID和资产ID查询数据
func Get(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, id string, result interface{}) (err error) {
	MemorySystemVariableData.Lock()
	defer MemorySystemVariableData.Unlock()
	var systemVariable string
	systemVariableMap, ok := MemorySystemVariableData.Cache[project]
	if ok {
		systemVariable, ok = systemVariableMap[id]
		if !ok {
			systemVariable, err = getByDB(ctx, redisClient, mongoClient, project, id)
			if err != nil {
				return errors.WithStack(err)
			}
			systemVariableMap[id] = systemVariable
			MemorySystemVariableData.Cache[project] = systemVariableMap
		}
	} else {
		systemVariable, err = getByDB(ctx, redisClient, mongoClient, project, id)
		if err != nil {
			return errors.WithStack(err)
		}
		systemVariableMap = make(map[string]string)
		systemVariableMap[id] = systemVariable
		MemorySystemVariableData.Cache[project] = systemVariableMap
	}

	if err := json.Unmarshal([]byte(systemVariable), result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func getByDB(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, id string) (string, error) {
	systemVariable, err := redisClient.HGet(ctx, fmt.Sprintf("%s/systemVariable", project), id).Result()
	if err != nil && err != redis.Nil {
		return "", err
	} else if err == redis.Nil {
		col := mongoClient.Database(project).Collection("systemVariable")
		systemVariableTmp := make(map[string]interface{})
		err := mongodb.FindByID(ctx, col, &systemVariableTmp, id)
		if err != nil {
			return "", err
		}
		systemVariableBytes, err := json.Marshal(&systemVariableTmp)
		if err != nil {
			return "", err
		}
		_, err = redisClient.HMSet(ctx, fmt.Sprintf("%s/systemVariable", project), map[string]interface{}{id: systemVariableBytes}).Result()
		if err != nil {
			return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
		}
		systemVariable = string(systemVariableBytes)
	}
	return systemVariable, nil
}

// TriggerUpdate 更新redis资产数据,并发送消息通知
func TriggerUpdate(ctx context.Context, mqCli mq.MQ, redisClient *redis.Client, project, id string, systemVariable []byte) error {
	_, err := redisClient.HMSet(ctx, fmt.Sprintf("%s/systemVariable", project), map[string]interface{}{id: systemVariable}).Result()
	if err != nil {
		return fmt.Errorf("更新redis数据错误, %s", err.Error())
	}
	topics := []string{CacheSystemVariablePrefix, string(cache.Update), project, id}
	ctx = context.WithValue(ctx, "exchange", CacheSystemVariablePrefix)
	err = mqCli.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}

// TriggerDelete 删除redis资产数据,并发送消息通知
func TriggerDelete(ctx context.Context, mqCli mq.MQ, redisClient *redis.Client, project, id string) error {
	_, err := redisClient.HDel(context.Background(), fmt.Sprintf("%s/systemVariable", project), id).Result()
	if err != nil {
		return fmt.Errorf("删除redis数据错误, %s", err.Error())
	}
	topics := []string{CacheSystemVariablePrefix, string(cache.Delete), project, id}
	ctx = context.WithValue(ctx, "exchange", CacheSystemVariablePrefix)
	err = mqCli.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}
