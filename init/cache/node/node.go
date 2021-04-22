package node

import (
	"context"
	"fmt"
	"math/rand"
	"sync"

	"github.com/go-redis/redis/v8"

	"github.com/air-iot/service/errors"
	"github.com/air-iot/service/init/cache"
	"github.com/air-iot/service/init/mq"
	"github.com/air-iot/service/logger"
	"github.com/air-iot/service/util/json"
)

const CacheNodePrefix = "cacheNode"

var MemoryNodeData = struct {
	sync.Mutex
	Cache map[string]map[string]string
}{
	Cache: map[string]map[string]string{},
}

// CacheHandler 初始化node缓存
func CacheHandler(ctx context.Context, redisClient *redis.Client, mqCli mq.MQ) (func(), error) {
	topic := []string{CacheNodePrefix, "#"}

	ctx = context.WithValue(ctx, "exchange", CacheNodePrefix)
	ctx = context.WithValue(ctx, "queue", fmt.Sprintf("node%d", rand.Int()))

	// 0: 前缀 1: 处理方式 2: 项目id 3: 资产id
	if err := mqCli.Consume(ctx, topic, 4, func(topic1 string, topics []string, payload []byte) {
		if len(topics) != 4 || topics[0] != CacheNodePrefix {
			logger.Errorln("缓存资产数据,消息主题(topic)格式错误")
			return
		}
		project, id := topics[2], topics[3]
		switch cache.CacheMethod(topics[1]) {
		case cache.Update:
			result, err := redisClient.HGet(ctx, fmt.Sprintf("%s/node", project), id).Result()
			if err != nil {
				logger.Errorf("缓存资产数据,查询redis缓存[ %s %s ]错误, %s", project, id, err.Error())
				return
			}
			MemoryNodeData.Lock()
			nodeMap, ok := MemoryNodeData.Cache[project]
			if ok {
				nodeMap[id] = result
			} else {
				nodeMap = make(map[string]string)
				nodeMap[id] = result
			}
			MemoryNodeData.Cache[project] = nodeMap
			MemoryNodeData.Unlock()
		case cache.Delete:
			MemoryNodeData.Lock()
			nodeMap, ok := MemoryNodeData.Cache[project]
			if ok {
				delete(nodeMap, id)
				MemoryNodeData.Cache[project] = nodeMap
			}
			MemoryNodeData.Unlock()
		default:
			return
		}

	}); err != nil {
		return nil, errors.WithStack(err)
	}

	cleanFunc := func() {
		if err := mqCli.UnSubscription(ctx, topic); err != nil {
			logger.Errorln("取消订阅node缓存错误,", err.Error())
		}
	}

	return cleanFunc, nil
}

// Get 根据项目ID和资产ID查询数据
func Get(ctx context.Context, redisClient *redis.Client, project, id string, result interface{}) (err error) {
	MemoryNodeData.Lock()
	defer MemoryNodeData.Unlock()
	var node string
	nodeMap, ok := MemoryNodeData.Cache[project]
	if ok {
		node, ok = nodeMap[id]
		if !ok {
			node, err = redisClient.HGet(ctx, fmt.Sprintf("%s/node", project), id).Result()
			if err != nil {
				return errors.WithStack(err)
			}
			nodeMap[id] = node
			MemoryNodeData.Cache[project] = nodeMap
		}
	} else {
		node, err = redisClient.HGet(ctx, fmt.Sprintf("%s/node", project), id).Result()
		if err != nil {
			return errors.WithStack(err)
		}
		nodeMap = make(map[string]string)
		nodeMap[id] = node
		MemoryNodeData.Cache[project] = nodeMap
	}

	if err := json.Unmarshal([]byte(node), result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// TriggerUpdate 更新redis资产数据,并发送消息通知
func TriggerUpdate(ctx context.Context, mqCli mq.MQ, redisClient *redis.Client, project, id string, node []byte) error {
	_, err := redisClient.HMSet(ctx, fmt.Sprintf("%s/node", project), map[string]interface{}{id: node}).Result()
	if err != nil {
		return fmt.Errorf("更新redis数据错误, %s", err.Error())
	}
	topics := []string{CacheNodePrefix, string(cache.Update), project, id}
	ctx = context.WithValue(ctx, "exchange", CacheNodePrefix)
	err = mqCli.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}

// TriggerDelete 删除redis资产数据,并发送消息通知
func TriggerDelete(ctx context.Context, mqCli mq.MQ, redisClient *redis.Client, project, id string) error {
	_, err := redisClient.HDel(context.Background(), fmt.Sprintf("%s/node", project), id).Result()
	if err != nil {
		return fmt.Errorf("删除redis数据错误, %s", err.Error())
	}
	topics := []string{CacheNodePrefix, string(cache.Delete), project, id}
	ctx = context.WithValue(ctx, "exchange", CacheNodePrefix)
	err = mqCli.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}
