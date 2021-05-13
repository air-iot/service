package handler

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

const CacheHandlerPrefix = "cacheHandler"

var MemoryHandlerData = struct {
	sync.Mutex
	Cache map[string]map[string]string
	CacheByEvent map[string]map[string]string
}{
	Cache: map[string]map[string]string{},
	CacheByEvent: map[string]map[string]string{},
}

// CacheHandler 初始化handler缓存
func CacheHandler(ctx context.Context, redisClient *redis.Client, mqCli mq.MQ) (func(), error) {
	topic := []string{CacheHandlerPrefix, "#"}

	ctx = context.WithValue(ctx, "exchange", CacheHandlerPrefix)
	ctx = context.WithValue(ctx, "queue", fmt.Sprintf("handler%d", rand.Int()))

	// 0: 前缀 1: 处理方式 2: 项目id 3: 资产id
	if err := mqCli.Consume(ctx, topic, 4, func(topic1 string, topics []string, payload []byte) {
		if len(topics) != 4 || topics[0] != CacheHandlerPrefix {
			logger.Errorln("缓存资产数据,消息主题(topic)格式错误")
			return
		}
		project, id := topics[2], topics[3]
		switch cache.CacheMethod(topics[1]) {
		case cache.Update:
			result, err := redisClient.HGet(ctx, fmt.Sprintf("%s/handler", project), id).Result()
			if err != nil {
				logger.Errorf("缓存资产数据,查询redis缓存[ %s %s ]错误, %s", project, id, err.Error())
				return
			}
			MemoryHandlerData.Lock()
			handlerMap, ok := MemoryHandlerData.Cache[project]
			if ok {
				handlerMap[id] = result
			} else {
				handlerMap = make(map[string]string)
				handlerMap[id] = result
			}
			MemoryHandlerData.Cache[project] = handlerMap

			eventHanlderInfo := entity.EventHandler{}
			err = json.Unmarshal([]byte(result), &eventHanlderInfo)
			if err != nil {
				logger.Errorf("解序列化事件handle失败[ %s %s ]错误, %s", project, id, err.Error())
				return
			}
			eventHanlderMap, ok := MemoryHandlerData.CacheByEvent[project]
			if ok {
				if eventHanlderListForType, ok := eventHanlderMap[eventHanlderInfo.Event]; ok {
					eventHandlerInfoList := make([]entity.EventHandler, 0)
					err = json.Unmarshal([]byte(eventHanlderListForType), &eventHandlerInfoList)
					if err != nil {
						logger.Errorf("解序列化缓存的事件handle列表失败[ %s %s ]错误, %s", project, id, err.Error())
						return
					}
					b, err := json.Marshal(formatx.AddNonRepEventHandlerByLoop(eventHandlerInfoList, eventHanlderInfo))
					if err != nil {
						logger.Errorf("序列化合并的事件handle列表失败[ %s %s ]错误, %s", project, id, err.Error())
						return
					}
					eventHanlderMap[eventHanlderInfo.Event] = string(b)
				}
				//eventMap[id] = result
			}
			MemoryHandlerData.CacheByEvent[project] = eventHanlderMap

			MemoryHandlerData.Unlock()
		case cache.Delete:
			MemoryHandlerData.Lock()
			handlerMap, ok := MemoryHandlerData.Cache[project]
			if ok {
				delete(handlerMap, id)
				MemoryHandlerData.Cache[project] = handlerMap
			}

			eventHandlerMap, ok := MemoryHandlerData.CacheByEvent[project]
			if ok {
				for index, listString := range eventHandlerMap {
					if listString != "" {
						eventHandlerInfoList := make([]entity.EventHandler, 0)
						err := json.Unmarshal([]byte(listString), &eventHandlerInfoList)
						if err != nil {
							logger.Errorf("解序列化缓存的事件handle列表失败[ %s %s ]错误, %s", project, id, err.Error())
							return
						}
						b, err := json.Marshal(formatx.DelEleEventHandlerByLoop(eventHandlerInfoList, id))
						if err != nil {
							logger.Errorf("序列化合并的事件handle列表失败[ %s %s ]错误, %s", project, id, err.Error())
							return
						}
						eventHandlerMap[index] = string(b)
					}
				}
				//eventMap[id] = result
			}
			MemoryHandlerData.CacheByEvent[project] = eventHandlerMap

			MemoryHandlerData.Unlock()
		default:
			return
		}

	}); err != nil {
		return nil, errors.WithStack(err)
	}

	cleanFunc := func() {
		if err := mqCli.UnSubscription(ctx, topic); err != nil {
			logger.Errorln("取消订阅handler缓存错误,", err.Error())
		}
	}

	return cleanFunc, nil
}

// Get 根据项目ID和资产ID查询数据
func Get(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, id string, result interface{}) (err error) {
	MemoryHandlerData.Lock()
	defer MemoryHandlerData.Unlock()
	var handler string
	handlerMap, ok := MemoryHandlerData.Cache[project]
	if ok {
		handler, ok = handlerMap[id]
		if !ok {
			handler, err = getByDB(ctx, redisClient, mongoClient, project, id)
			if err != nil {
				return errors.WithStack(err)
			}
			handlerMap[id] = handler
			MemoryHandlerData.Cache[project] = handlerMap
		}
	} else {
		handler, err = getByDB(ctx, redisClient, mongoClient, project, id)
		if err != nil {
			return errors.WithStack(err)
		}
		handlerMap = make(map[string]string)
		handlerMap[id] = handler
		MemoryHandlerData.Cache[project] = handlerMap
	}

	if err := json.Unmarshal([]byte(handler), result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Get 根据项目ID和资产ID查询数据
func GetByList(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project string, ids []string, result interface{}) (err error) {
	MemoryHandlerData.Lock()
	defer MemoryHandlerData.Unlock()
	handlerList := make([]map[string]interface{},0)
	for _, id := range ids {
		deptInfo := map[string]interface{}{}
		err := Get(ctx,redisClient,mongoClient,project,id,&deptInfo)
		if err != nil {
			return errors.WithStack(err)
		}
		handlerList = append(handlerList,deptInfo)
	}

	resultByte,err := json.Marshal(handlerList)
	if err != nil {
		return errors.WithStack(err)
	}
	if err := json.Unmarshal(resultByte, result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func getByDB(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, id string) (string, error) {
	handler, err := redisClient.HGet(ctx, fmt.Sprintf("%s/handler", project), id).Result()
	if err != nil && err != redis.Nil {
		return "", err
	} else if err == redis.Nil {
		col := mongoClient.Database(project).Collection("eventhandler")
		handlerTmp := make(map[string]interface{})
		err := mongodb.FindByID(ctx, col, &handlerTmp, id)
		if err != nil {
			return "", err
		}
		handlerBytes, err := json.Marshal(&handlerTmp)
		if err != nil {
			return "", err
		}
		_, err = redisClient.HMSet(ctx, fmt.Sprintf("%s/handler", project), map[string]interface{}{id: handlerBytes}).Result()
		if err != nil {
			return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
		}
		handler = string(handlerBytes)
	}
	return handler, nil
}

// Get 根据项目ID和资产ID查询数据
func GetByEvent(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, eventID string, result interface{}) (err error) {
	MemoryHandlerData.Lock()
	defer MemoryHandlerData.Unlock()
	var event string
	eventMap, ok := MemoryHandlerData.CacheByEvent[project]
	if ok {
		event, ok = eventMap[eventID]
		if !ok {
			event, err = getByDBAndEvent(ctx, redisClient, mongoClient, project, eventID)
			if err != nil {
				return errors.WithStack(err)
			}
			eventMap[eventID] = event
			MemoryHandlerData.CacheByEvent[project] = eventMap
		}
	} else {
		event, err = getByDBAndEvent(ctx, redisClient, mongoClient, project, eventID)
		if err != nil {
			return errors.WithStack(err)
		}
		eventMap = make(map[string]string)
		eventMap[eventID] = event
		MemoryHandlerData.CacheByEvent[project] = eventMap
	}

	if err := json.Unmarshal([]byte(event), result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func getByDBAndEvent(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, eventID string) (string, error) {
	//event, err := redisClient.HGet(ctx, fmt.Sprintf("%s/event", project), id).Result()
	//if err != nil && err != redis.Nil {
	//	return "", err
	//} else if err == redis.Nil {
	col := mongoClient.Database(project).Collection("eventhandler")
	eventHandlerTmp := make([]entity.EventHandlerMongo, 0)
	_, err := mongodb.FindFilter(ctx, col, &eventHandlerTmp, mongodb.QueryOption{Filter: map[string]interface{}{
		"event": eventID,
	}})
	if err != nil {
		return "", err
	}
	eventHandlerBytes, err := json.Marshal(&eventHandlerTmp)
	if err != nil {
		return "", err
	}
	for _, handler := range eventHandlerTmp {
		eventHandlerEleBytes, err := json.Marshal(&handler)
		if err != nil {
			return "", err
		}
		_, err = redisClient.HMSet(ctx, fmt.Sprintf("%s/handler", project), map[string]interface{}{handler.ID: eventHandlerEleBytes}).Result()
		if err != nil {
			return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
		}
	}
	eventHandler := string(eventHandlerBytes)
	//}
	return eventHandler, nil
}

// TriggerUpdate 更新redis资产数据,并发送消息通知
func TriggerUpdate(ctx context.Context, mqCli mq.MQ, redisClient *redis.Client, project, id string, handler []byte) error {
	_, err := redisClient.HMSet(ctx, fmt.Sprintf("%s/handler", project), map[string]interface{}{id: handler}).Result()
	if err != nil {
		return fmt.Errorf("更新redis数据错误, %s", err.Error())
	}
	topics := []string{CacheHandlerPrefix, string(cache.Update), project, id}
	ctx = context.WithValue(ctx, "exchange", CacheHandlerPrefix)
	err = mqCli.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}

// TriggerDelete 删除redis资产数据,并发送消息通知
func TriggerDelete(ctx context.Context, mqCli mq.MQ, redisClient *redis.Client, project, id string) error {
	_, err := redisClient.HDel(context.Background(), fmt.Sprintf("%s/handler", project), id).Result()
	if err != nil {
		return fmt.Errorf("删除redis数据错误, %s", err.Error())
	}
	topics := []string{CacheHandlerPrefix, string(cache.Delete), project, id}
	ctx = context.WithValue(ctx, "exchange", CacheHandlerPrefix)
	err = mqCli.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}
