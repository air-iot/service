package event

import (
	"context"
	"fmt"
	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/mongodb"
	"github.com/air-iot/service/util/formatx"
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

const CacheEventPrefix = "cacheEvent"

var MemoryEventData = struct {
	sync.Mutex
	Cache       map[string]map[string]string
	CacheByType map[string]map[string]string
}{
	Cache:       map[string]map[string]string{},
	CacheByType: map[string]map[string]string{},
}

// CacheHandler 初始化event缓存
func CacheHandler(ctx context.Context, redisClient *redis.Client, mqCli mq.MQ) (func(), error) {
	topic := []string{CacheEventPrefix, "#"}

	ctx = context.WithValue(ctx, "exchange", CacheEventPrefix)
	ctx = context.WithValue(ctx, "queue", fmt.Sprintf("event%d", rand.Int()))

	// 0: 前缀 1: 处理方式 2: 项目id 3: 模型id
	if err := mqCli.Consume(ctx, topic, 4, func(topic1 string, topics []string, payload []byte) {
		if len(topics) != 4 || topics[0] != CacheEventPrefix {
			logger.Errorln("缓存模型数据,消息主题(topic)格式错误")
			return
		}
		project, id := topics[2], topics[3]
		switch cache.CacheMethod(topics[1]) {
		case cache.Update:
			result, err := redisClient.HGet(ctx, fmt.Sprintf("%s/event", project), id).Result()
			if err != nil {
				logger.Errorf("缓存模型数据,查询redis缓存[ %s %s ]错误, %s", project, id, err.Error())
				return
			}
			MemoryEventData.Lock()
			eventMap, ok := MemoryEventData.Cache[project]
			if ok {
				eventMap[id] = result
			} else {
				eventMap = make(map[string]string)
				eventMap[id] = result
			}
			MemoryEventData.Cache[project] = eventMap

			eventInfo := entity.Event{}
			err = json.Unmarshal([]byte(result), &eventInfo)
			if err != nil {
				logger.Errorf("解序列化事件失败[ %s %s ]错误, %s", project, id, err.Error())
				return
			}
			eventTypeMap, ok := MemoryEventData.CacheByType[project]
			if ok {
				if eventListForType, ok := eventTypeMap[eventInfo.Type]; ok {
					eventInfoList := make([]entity.Event, 0)
					err = json.Unmarshal([]byte(eventListForType), &eventInfoList)
					if err != nil {
						logger.Errorf("解序列化缓存的事件列表失败[ %s %s ]错误, %s", project, id, err.Error())
						return
					}
					b, err := json.Marshal(formatx.AddNonRepEventByLoop(eventInfoList, eventInfo))
					if err != nil {
						logger.Errorf("序列化合并的事件列表失败[ %s %s ]错误, %s", project, id, err.Error())
						return
					}
					eventTypeMap[eventInfo.Type] = string(b)
				}
				//eventMap[id] = result
			}
			MemoryEventData.CacheByType[project] = eventTypeMap

			MemoryEventData.Unlock()
		case cache.Delete:
			MemoryEventData.Lock()
			eventMap, ok := MemoryEventData.Cache[project]
			if ok {
				delete(eventMap, id)
				MemoryEventData.Cache[project] = eventMap
			}
			eventTypeMap, ok := MemoryEventData.CacheByType[project]
			if ok {
				for index, listString := range eventTypeMap {
					if listString != "" {
						eventInfoList := make([]entity.Event, 0)
						err := json.Unmarshal([]byte(listString), &eventInfoList)
						if err != nil {
							logger.Errorf("解序列化缓存的事件列表失败[ %s %s ]错误, %s", project, id, err.Error())
							return
						}
						b, err := json.Marshal(formatx.DelEleEventByLoop(eventInfoList, id))
						if err != nil {
							logger.Errorf("序列化合并的事件列表失败[ %s %s ]错误, %s", project, id, err.Error())
							return
						}
						eventTypeMap[index] = string(b)
					}
				}
				//eventMap[id] = result
			}
			MemoryEventData.CacheByType[project] = eventTypeMap
			MemoryEventData.Unlock()
		default:
			return
		}

	}); err != nil {
		return nil, errors.WithStack(err)
	}

	cleanFunc := func() {
		if err := mqCli.UnSubscription(ctx, topic); err != nil {
			logger.Errorln("取消订阅event缓存错误,", err.Error())
		}
	}

	return cleanFunc, nil
}

// Get 根据项目ID和资产ID查询数据
func Get(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, id string, result interface{}) (err error) {
	MemoryEventData.Lock()
	defer MemoryEventData.Unlock()
	var event string
	eventMap, ok := MemoryEventData.Cache[project]
	if ok {
		event, ok = eventMap[id]
		if !ok {
			event, err = getByDB(ctx, redisClient, mongoClient, project, id)
			if err != nil {
				return errors.WithStack(err)
			}
			eventMap[id] = event
			MemoryEventData.Cache[project] = eventMap
		}
	} else {
		event, err = getByDB(ctx, redisClient, mongoClient, project, id)
		if err != nil {
			return errors.WithStack(err)
		}
		eventMap = make(map[string]string)
		eventMap[id] = event
		MemoryEventData.Cache[project] = eventMap
	}

	if err := json.Unmarshal([]byte(event), result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Get 根据项目ID和资产ID查询数据
func GetByType(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, eventType string, result interface{}) (err error) {
	MemoryEventData.Lock()
	defer MemoryEventData.Unlock()
	var event string
	eventMap, ok := MemoryEventData.CacheByType[project]
	if ok {
		event, ok = eventMap[eventType]
		if !ok {
			event, err = getByDBAndType(ctx, redisClient, mongoClient, project, eventType)
			if err != nil {
				return errors.WithStack(err)
			}
			eventMap[eventType] = event
			MemoryEventData.CacheByType[project] = eventMap
		}
	} else {
		event, err = getByDBAndType(ctx, redisClient, mongoClient, project, eventType)
		if err != nil {
			return errors.WithStack(err)
		}
		eventMap = make(map[string]string)
		eventMap[eventType] = event
		MemoryEventData.CacheByType[project] = eventMap
	}

	if err := json.Unmarshal([]byte(event), result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func getByDBAndType(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, eventType string) (string, error) {
	//event, err := redisClient.HGet(ctx, fmt.Sprintf("%s/event", project), id).Result()
	//if err != nil && err != redis.Nil {
	//	return "", err
	//} else if err == redis.Nil {
	col := mongoClient.Database(project).Collection("event")
	eventTmp := make([]entity.EventMongo, 0)
	_, err := mongodb.FindFilter(ctx, col, &eventTmp, mongodb.QueryOption{Filter: map[string]interface{}{
		"type": eventType,
		"$lookups": []map[string]interface{}{
			{
				"from":         "eventhandler",
				"localField":   "_id",
				"foreignField": "event",
				"as":           "handlers",
			},
		},
	}})
	if err != nil {
		return "", err
	}
	eventBytes, err := json.Marshal(&eventTmp)
	if err != nil {
		return "", err
	}
	for _, event := range eventTmp {
		eventEleBytes, err := json.Marshal(&event)
		if err != nil {
			return "", err
		}
		_, err = redisClient.HMSet(ctx, fmt.Sprintf("%s/event", project), map[string]interface{}{event.ID: eventEleBytes}).Result()
		if err != nil {
			return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
		}
	}
	event := string(eventBytes)
	//}
	return event, nil
}

func getByDB(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, id string) (string, error) {
	event, err := redisClient.HGet(ctx, fmt.Sprintf("%s/event", project), id).Result()
	if err != nil && err != redis.Nil {
		return "", err
	} else if err == redis.Nil {
		col := mongoClient.Database(project).Collection("event")
		eventTmp := make(map[string]interface{})
		err := mongodb.FindByID(ctx, col, &eventTmp, id)
		if err != nil {
			return "", err
		}
		eventBytes, err := json.Marshal(&eventTmp)
		if err != nil {
			return "", err
		}
		_, err = redisClient.HMSet(ctx, fmt.Sprintf("%s/event", project), map[string]interface{}{id: eventBytes}).Result()
		if err != nil {
			return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
		}
		event = string(eventBytes)
	}
	return event, nil
}

// TriggerUpdate 更新redis资产数据,并发送消息通知
func TriggerUpdate(ctx context.Context, mqCli mq.MQ, redisClient *redis.Client, project, id string, event []byte) error {
	_, err := redisClient.HMSet(ctx, fmt.Sprintf("%s/event", project), map[string]interface{}{id: event}).Result()
	if err != nil {
		return fmt.Errorf("更新redis数据错误, %s", err.Error())
	}
	topics := []string{CacheEventPrefix, string(cache.Update), project, id}
	ctx = context.WithValue(ctx, "exchange", CacheEventPrefix)
	err = mqCli.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}

// TriggerDelete 删除redis资产数据,并发送消息通知
func TriggerDelete(ctx context.Context, mqCli mq.MQ, redisClient *redis.Client, project, id string) error {
	_, err := redisClient.HDel(context.Background(), fmt.Sprintf("%s/event", project), id).Result()
	if err != nil {
		return fmt.Errorf("删除redis数据错误, %s", err.Error())
	}
	topics := []string{CacheEventPrefix, string(cache.Delete), project, id}
	ctx = context.WithValue(ctx, "exchange", CacheEventPrefix)
	err = mqCli.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}
