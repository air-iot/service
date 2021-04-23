package model

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

const CacheModelPrefix = "cacheModel"

var MemoryModelData = struct {
	sync.Mutex
	Cache map[string]map[string]string
}{
	Cache: map[string]map[string]string{},
}

// CacheHandler 初始化model缓存
func CacheHandler(ctx context.Context, redisClient *redis.Client, mqCli mq.MQ) (func(), error) {
	topic := []string{CacheModelPrefix, "#"}

	ctx = context.WithValue(ctx, "exchange", CacheModelPrefix)
	ctx = context.WithValue(ctx, "queue", fmt.Sprintf("model%d", rand.Int()))

	// 0: 前缀 1: 处理方式 2: 项目id 3: 模型id
	if err := mqCli.Consume(ctx, topic, 4, func(topic1 string, topics []string, payload []byte) {
		if len(topics) != 4 || topics[0] != CacheModelPrefix {
			logger.Errorln("缓存模型数据,消息主题(topic)格式错误")
			return
		}
		project, id := topics[2], topics[3]
		switch cache.CacheMethod(topics[1]) {
		case cache.Update:
			result, err := redisClient.HGet(ctx, fmt.Sprintf("%s/model", project), id).Result()
			if err != nil {
				logger.Errorf("缓存模型数据,查询redis缓存[ %s %s ]错误, %s", project, id, err.Error())
				return
			}
			MemoryModelData.Lock()
			modelMap, ok := MemoryModelData.Cache[project]
			if ok {
				modelMap[id] = result
			} else {
				modelMap = make(map[string]string)
				modelMap[id] = result
			}
			MemoryModelData.Cache[project] = modelMap
			MemoryModelData.Unlock()
		case cache.Delete:
			MemoryModelData.Lock()
			modelMap, ok := MemoryModelData.Cache[project]
			if ok {
				delete(modelMap, id)
				MemoryModelData.Cache[project] = modelMap
			}
			MemoryModelData.Unlock()
		default:
			return
		}

	}); err != nil {
		return nil, errors.WithStack(err)
	}

	cleanFunc := func() {
		if err := mqCli.UnSubscription(ctx, topic); err != nil {
			logger.Errorln("取消订阅model缓存错误,", err.Error())
		}
	}

	return cleanFunc, nil
}

// Get 根据项目ID和资产ID查询数据
func Get(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, id string, result interface{}) (err error) {
	MemoryModelData.Lock()
	defer MemoryModelData.Unlock()
	var model string
	modelMap, ok := MemoryModelData.Cache[project]
	if ok {
		model, ok = modelMap[id]
		if !ok {
			model, err = getByDB(ctx, redisClient, mongoClient, project, id)
			if err != nil {
				return errors.WithStack(err)
			}
			modelMap[id] = model
			MemoryModelData.Cache[project] = modelMap
		}
	} else {
		model, err = getByDB(ctx, redisClient, mongoClient, project, id)
		if err != nil {
			return errors.WithStack(err)
		}
		modelMap = make(map[string]string)
		modelMap[id] = model
		MemoryModelData.Cache[project] = modelMap
	}

	if err := json.Unmarshal([]byte(model), result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func getByDB(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, id string) (string, error) {
	model, err := redisClient.HGet(ctx, fmt.Sprintf("%s/model", project), id).Result()
	if err != nil && err != redis.Nil {
		return "", err
	} else if err == redis.Nil {
		col := mongoClient.Database(project).Collection("model")
		modelTmp := make(map[string]interface{})
		err := mongodb.FindByID(ctx, col, &modelTmp, id)
		if err != nil {
			return "", err
		}
		modelBytes, err := json.Marshal(&modelTmp)
		if err != nil {
			return "", err
		}
		_, err = redisClient.HMSet(ctx, fmt.Sprintf("%s/model", project), map[string]interface{}{id: modelBytes}).Result()
		if err != nil {
			return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
		}
		model = string(modelBytes)
	}
	return model, nil
}

// TriggerUpdate 更新redis资产数据,并发送消息通知
func TriggerUpdate(ctx context.Context, mqCli mq.MQ, redisClient *redis.Client, project, id string, model []byte) error {
	_, err := redisClient.HMSet(ctx, fmt.Sprintf("%s/model", project), map[string]interface{}{id: model}).Result()
	if err != nil {
		return fmt.Errorf("更新redis数据错误, %s", err.Error())
	}
	topics := []string{CacheModelPrefix, string(cache.Update), project, id}
	ctx = context.WithValue(ctx, "exchange", CacheModelPrefix)
	err = mqCli.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}

// TriggerDelete 删除redis资产数据,并发送消息通知
func TriggerDelete(ctx context.Context, mqCli mq.MQ, redisClient *redis.Client, project, id string) error {
	_, err := redisClient.HDel(context.Background(), fmt.Sprintf("%s/model", project), id).Result()
	if err != nil {
		return fmt.Errorf("删除redis数据错误, %s", err.Error())
	}
	topics := []string{CacheModelPrefix, string(cache.Delete), project, id}
	ctx = context.WithValue(ctx, "exchange", CacheModelPrefix)
	err = mqCli.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}
