package model

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

const CacheModelPrefix = "cachemodel"

type MemModel struct {
	sync.Mutex
	config      cache.CacheParams
	mqClient    mq.MQ
	redisClient redis.Client
	Cache       map[string]map[string]string
}

// NewMemModel 创建模型缓存
func NewMemModel(mqClient mq.MQ, redisClient redis.Client, config cache.CacheParams) (*MemModel, func(), error) {
	model := new(MemModel)
	model.redisClient = redisClient
	model.mqClient = mqClient
	model.config = config
	model.config.Exchange = CacheModelPrefix
	model.Cache = make(map[string]map[string]string)
	cleanFunc, err := model.cacheHandler(context.Background())
	return model, cleanFunc, err
}

// CacheHandler 初始化model缓存
func (p *MemModel) cacheHandler(ctx context.Context) (func(), error) {
	topic := []string{CacheModelPrefix, "#"}

	ctx = context.WithValue(ctx, "exchange", p.config.Exchange)
	ctx = context.WithValue(ctx, "queue", p.config.Queue)

	if err := p.mqClient.Consume(ctx, topic, 4, func(topic1 string, topics []string, payload []byte) {
		if len(topics) != 4 || topics[0] != CacheModelPrefix {
			logger.Errorln("缓存model数据,消息主题(topic)格式错误")
			return
		}
		project, id := topics[2], topics[3]
		switch cache.CacheMethod(topics[1]) {
		case cache.Update:
			result, err := p.redisClient.HGet(ctx, fmt.Sprintf("%s/model", project), id).Result()
			if err != nil {
				logger.Errorf("缓存model数据,查询redis缓存[ %s %s ]错误, %s", project, id, err.Error())
				return
			}
			p.Lock()
			modelMap, ok := p.Cache[project]
			if ok {
				modelMap[id] = result
			} else {
				modelMap = make(map[string]string)
				modelMap[id] = result
			}
			p.Cache[project] = modelMap
			p.Unlock()
		case cache.Delete:
			p.Lock()
			modelMap, ok := p.Cache[project]
			if ok {
				delete(modelMap, id)
				p.Cache[project] = modelMap
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
			logger.Errorln("取消订阅model缓存错误,", err.Error())
		}
	}

	return cleanFunc, nil
}

// Get 根据项目ID和资产ID查询数据
func (p *MemModel) Get(ctx context.Context, project, id string, result interface{}) (err error) {
	p.Lock()
	defer p.Unlock()
	var model string
	modelMap, ok := p.Cache[project]
	if ok {
		model, ok = modelMap[id]
		if !ok {
			model, err = p.redisClient.HGet(ctx, fmt.Sprintf("%s/model", project), id).Result()
			if err != nil {
				return errors.WithStack(err)
			}
			modelMap[id] = model
			p.Cache[project] = modelMap
		}
	} else {
		model, err = p.redisClient.HGet(ctx, fmt.Sprintf("%s/model", project), id).Result()
		if err != nil {
			return errors.WithStack(err)
		}
		modelMap = make(map[string]string)
		modelMap[id] = model
		p.Cache[project] = modelMap
	}

	if err := json.Unmarshal([]byte(model), result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// TriggerUpdate 更新redis资产数据,并发送消息通知
func (p *MemModel) TriggerUpdate(ctx context.Context, project, id string, model []byte) error {
	_, err := p.redisClient.HMSet(ctx, fmt.Sprintf("%s/model", project), map[string]interface{}{id: model}).Result()
	if err != nil {
		return fmt.Errorf("更新redis数据错误, %s", err.Error())
	}
	topics := []string{CacheModelPrefix, string(cache.Update), project, id}
	ctx = context.WithValue(ctx, "exchange", p.config.Exchange)
	ctx = context.WithValue(ctx, "queue", p.config.Queue)
	err = p.mqClient.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}

// TriggerDelete 删除redis资产数据,并发送消息通知
func (p *MemModel) TriggerDelete(ctx context.Context, project, id string) error {
	_, err := p.redisClient.HDel(context.Background(), fmt.Sprintf("%s/model", project), id).Result()
	if err != nil {
		return fmt.Errorf("删除redis数据错误, %s", err.Error())
	}
	topics := []string{CacheModelPrefix, string(cache.Delete), project, id}
	ctx = context.WithValue(ctx, "exchange", p.config.Exchange)
	ctx = context.WithValue(ctx, "queue", p.config.Queue)
	err = p.mqClient.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}
