package node

import (
	"context"
	"fmt"
	"sync"

	"github.com/go-redis/redis/v8"

	"github.com/air-iot/service/errors"
	"github.com/air-iot/service/init/cache"
	"github.com/air-iot/service/init/mq"
	"github.com/air-iot/service/logger"
	"github.com/air-iot/service/util/json"
)

const CacheNodePrefix = "cachenode"

type MemNode struct {
	sync.Mutex
	config      cache.CacheParams
	mqClient    mq.MQ
	redisClient redis.Client
	Cache       map[string]map[string]string
}

// NewMemNode 创建Node缓存
func NewMemNode(mqClient mq.MQ, redisClient redis.Client, config cache.CacheParams) (*MemNode, func(), error) {
	node := new(MemNode)
	node.redisClient = redisClient
	node.mqClient = mqClient
	node.config = config
	node.config.Exchange = CacheNodePrefix
	node.Cache = make(map[string]map[string]string)
	cleanFunc, err := node.cacheHandler(context.Background())
	return node, cleanFunc, err
}

// cacheHandler 初始化node缓存
func (p *MemNode) cacheHandler(ctx context.Context) (func(), error) {
	topic := []string{CacheNodePrefix, "#"}

	ctx = context.WithValue(ctx, "exchange", p.config.Exchange)
	ctx = context.WithValue(ctx, "queue", p.config.Queue)

	if err := p.mqClient.Consume(ctx, topic, 4, func(topic1 string, topics []string, payload []byte) {
		if len(topics) != 4 || topics[0] != CacheNodePrefix {
			logger.Errorln("缓存node数据,消息主题(topic)格式错误")
			return
		}
		project, id := topics[2], topics[3]
		switch cache.CacheMethod(topics[1]) {
		case cache.Update:
			result, err := p.redisClient.HGet(ctx, fmt.Sprintf("%s/node", project), id).Result()
			if err != nil {
				logger.Errorf("缓存node数据,查询redis缓存[ %s %s ]错误, %s", project, id, err.Error())
				return
			}
			p.Lock()
			nodeMap, ok := p.Cache[project]
			if ok {
				nodeMap[id] = result
			} else {
				nodeMap = make(map[string]string)
				nodeMap[id] = result
			}
			p.Cache[project] = nodeMap
			p.Unlock()
		case cache.Delete:
			p.Lock()
			nodeMap, ok := p.Cache[project]
			if ok {
				delete(nodeMap, id)
				p.Cache[project] = nodeMap
			}
			p.Unlock()
		default:
			return
		}

	}); err != nil {
		return nil, errors.WithStack(err)
	}

	cleanFunc := func() {
		if err := p.mqClient.UnSubscription(ctx, topic); err != nil {
			logger.Errorln("取消订阅node缓存错误,", err.Error())
		}
	}

	return cleanFunc, nil
}

// Get 根据项目ID和资产ID查询数据
func (p *MemNode) Get(ctx context.Context, project, id string, result interface{}) (err error) {
	p.Lock()
	defer p.Unlock()
	var node string
	nodeMap, ok := p.Cache[project]
	if ok {
		node, ok = nodeMap[id]
		if !ok {
			node, err = p.redisClient.HGet(ctx, fmt.Sprintf("%s/node", project), id).Result()
			if err != nil {
				return errors.WithStack(err)
			}
			nodeMap[id] = node
			p.Cache[project] = nodeMap
		}
	} else {
		node, err = p.redisClient.HGet(ctx, fmt.Sprintf("%s/node", project), id).Result()
		if err != nil {
			return errors.WithStack(err)
		}
		nodeMap = make(map[string]string)
		nodeMap[id] = node
		p.Cache[project] = nodeMap
	}

	if err := json.Unmarshal([]byte(node), result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// TriggerUpdate 更新redis资产数据,并发送消息通知
func (p *MemNode) TriggerUpdate(ctx context.Context, project, id string, node []byte) error {
	_, err := p.redisClient.HMSet(ctx, fmt.Sprintf("%s/node", project), map[string]interface{}{id: node}).Result()
	if err != nil {
		return fmt.Errorf("更新redis数据错误, %s", err.Error())
	}
	topics := []string{CacheNodePrefix, string(cache.Update), project, id}
	ctx = context.WithValue(ctx, "exchange", p.config.Exchange)
	ctx = context.WithValue(ctx, "queue", p.config.Queue)
	err = p.mqClient.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}

// TriggerDelete 删除redis资产数据,并发送消息通知
func (p *MemNode) TriggerDelete(ctx context.Context, project, id string) error {
	_, err := p.redisClient.HDel(context.Background(), fmt.Sprintf("%s/node", project), id).Result()
	if err != nil {
		return fmt.Errorf("删除redis数据错误, %s", err.Error())
	}
	topics := []string{CacheNodePrefix, string(cache.Delete), project, id}
	ctx = context.WithValue(ctx, "exchange", p.config.Exchange)
	ctx = context.WithValue(ctx, "queue", p.config.Queue)
	err = p.mqClient.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}
