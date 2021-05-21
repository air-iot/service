package node

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

const CacheNodePrefix = "cacheNode"

var MemoryNodeData = struct {
	sync.Mutex
	Cache map[string]map[string]string
	CacheByModel map[string]map[string]string
}{
	Cache: map[string]map[string]string{},
	CacheByModel: map[string]map[string]string{},
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

			nodeInfo := entity.Node{}
			err = json.Unmarshal([]byte(result), &nodeInfo)
			if err != nil {
				logger.Errorf("解序列化资产失败[ %s %s ]错误, %s", project, id, err.Error())
				return
			}
			nodeMap, ok = MemoryNodeData.CacheByModel[project]
			if ok {
				if nodeListForType, ok := nodeMap[nodeInfo.Model]; ok {
					nodeInfoList := make([]entity.Node, 0)
					err = json.Unmarshal([]byte(nodeListForType), &nodeInfoList)
					if err != nil {
						logger.Errorf("解序列化缓存的资产列表失败[ %s %s ]错误, %s", project, id, err.Error())
						return
					}
					b, err := json.Marshal(formatx.AddNonRepNodeByLoop(nodeInfoList, nodeInfo))
					if err != nil {
						logger.Errorf("序列化合并的资产列表失败[ %s %s ]错误, %s", project, id, err.Error())
						return
					}
					nodeMap[nodeInfo.Model] = string(b)
				}
				//eventMap[id] = result
			}
			MemoryNodeData.CacheByModel[project] = nodeMap

			MemoryNodeData.Unlock()
		case cache.Delete:
			MemoryNodeData.Lock()
			nodeMap, ok := MemoryNodeData.Cache[project]
			if ok {
				delete(nodeMap, id)
				MemoryNodeData.Cache[project] = nodeMap
			}

			nodeMap, ok = MemoryNodeData.CacheByModel[project]
			if ok {
				for index, listString := range nodeMap {
					if listString != "" {
						nodeInfoList := make([]entity.Node, 0)
						err := json.Unmarshal([]byte(listString), &nodeInfoList)
						if err != nil {
							logger.Errorf("解序列化缓存的事件handle列表失败[ %s %s ]错误, %s", project, id, err.Error())
							return
						}
						b, err := json.Marshal(formatx.DelEleNodeByLoop(nodeInfoList, id))
						if err != nil {
							logger.Errorf("序列化合并的事件handle列表失败[ %s %s ]错误, %s", project, id, err.Error())
							return
						}
						nodeMap[index] = string(b)
					}
				}
				//eventMap[id] = result
			}
			MemoryNodeData.CacheByModel[project] = nodeMap


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
func Get(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, id string, result interface{}) (err error) {
	MemoryNodeData.Lock()
	defer MemoryNodeData.Unlock()
	var node string
	nodeMap, ok := MemoryNodeData.Cache[project]
	if ok {
		node, ok = nodeMap[id]
		if !ok {
			node, err = getByDB(ctx, redisClient, mongoClient, project, id)
			if err != nil {
				return errors.WithStack(err)
			}
			nodeMap[id] = node
			MemoryNodeData.Cache[project] = nodeMap
		}
	} else {
		node, err = getByDB(ctx, redisClient, mongoClient, project, id)
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

// Get 根据项目ID和资产ID查询数据
func GetByList(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project string, ids []string, result interface{}) (err error) {
	MemoryNodeData.Lock()
	defer MemoryNodeData.Unlock()
	nodeList := make([]map[string]interface{},0)
	for _, id := range ids {
		deptInfo := map[string]interface{}{}
		err := Get(ctx,redisClient,mongoClient,project,id,&deptInfo)
		if err != nil {
			return errors.WithStack(err)
		}
		nodeList = append(nodeList,deptInfo)
	}

	resultByte,err := json.Marshal(nodeList)
	if err != nil {
		return errors.WithStack(err)
	}
	if err := json.Unmarshal(resultByte, result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func getByDB(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, id string) (string, error) {
	node, err := redisClient.HGet(ctx, fmt.Sprintf("%s/node", project), id).Result()
	if err != nil && err != redis.Nil {
		return "", err
	} else if err == redis.Nil {
		col := mongoClient.Database(project).Collection("node")
		nodeTmp := make(map[string]interface{})
		err := mongodb.FindByID(ctx, col, &nodeTmp, id)
		if err != nil {
			return "", err
		}
		nodeBytes, err := json.Marshal(&nodeTmp)
		if err != nil {
			return "", err
		}
		_, err = redisClient.HMSet(ctx, fmt.Sprintf("%s/node", project), map[string]interface{}{id: nodeBytes}).Result()
		if err != nil {
			return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
		}
		node = string(nodeBytes)
	}
	return node, nil
}

// Get 根据项目ID和资产ID查询数据
func GetByModel(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, modelID string, result interface{}) (err error) {
	MemoryNodeData.Lock()
	defer MemoryNodeData.Unlock()
	var node string
	nodeMap, ok := MemoryNodeData.CacheByModel[project]
	if ok {
		node, ok = nodeMap[modelID]
		if !ok {
			node, err = getByDBAndModel(ctx, redisClient, mongoClient, project, modelID)
			if err != nil {
				return errors.WithStack(err)
			}
			nodeMap[modelID] = node
			MemoryNodeData.CacheByModel[project] = nodeMap
		}
	} else {
		node, err = getByDBAndModel(ctx, redisClient, mongoClient, project, modelID)
		if err != nil {
			return errors.WithStack(err)
		}
		nodeMap = make(map[string]string)
		nodeMap[modelID] = node
		MemoryNodeData.CacheByModel[project] = nodeMap
	}

	if err := json.Unmarshal([]byte(node), result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func getByDBAndModel(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, id string) (string, error) {
	//event, err := redisClient.HGet(ctx, fmt.Sprintf("%s/event", project), id).Result()
	//if err != nil && err != redis.Nil {
	//	return "", err
	//} else if err == redis.Nil {
	col := mongoClient.Database(project).Collection("node")
	nodeTmp := make([]entity.NodeMongo, 0)
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
		nodeEleBytes, err := json.Marshal(&nodeEle)
		if err != nil {
			return "", err
		}
		_, err = redisClient.HMSet(ctx, fmt.Sprintf("%s/node", project), map[string]interface{}{nodeEle.ID: nodeEleBytes}).Result()
		if err != nil {
			return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
		}
	}
	node := string(nodeBytes)
	//}
	return node, nil
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