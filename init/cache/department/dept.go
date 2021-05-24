package department

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

const CacheDeptPrefix = "cacheDept"

var MemoryDeptData = struct {
	sync.Mutex
	Cache       map[string]map[string]string
}{
	Cache:       map[string]map[string]string{},
}

// CacheHandler 初始化department缓存
func CacheHandler(ctx context.Context, redisClient *redis.Client, mqCli mq.MQ) (func(), error) {
	topic := []string{CacheDeptPrefix, "#"}

	ctx = context.WithValue(ctx, "exchange", CacheDeptPrefix)
	ctx = context.WithValue(ctx, "queue", fmt.Sprintf("department%d", rand.Int()))

	// 0: 前缀 1: 处理方式 2: 项目id 3: 模型id
	if err := mqCli.Consume(ctx, topic, 4, func(topic1 string, topics []string, payload []byte) {
		if len(topics) != 4 || topics[0] != CacheDeptPrefix {
			logger.Errorln("缓存模型数据,消息主题(topic)格式错误")
			return
		}
		project, id := topics[2], topics[3]
		switch cache.CacheMethod(topics[1]) {
		case cache.Update:
			result, err := redisClient.HGet(ctx, fmt.Sprintf("%s/department", project), id).Result()
			if err != nil {
				logger.Errorf("缓存模型数据,查询redis缓存[ %s %s ]错误, %s", project, id, err.Error())
				return
			}
			MemoryDeptData.Lock()
			departmentMap, ok := MemoryDeptData.Cache[project]
			if ok {
				departmentMap[id] = result
			} else {
				departmentMap = make(map[string]string)
				departmentMap[id] = result
			}
			MemoryDeptData.Cache[project] = departmentMap
			MemoryDeptData.Unlock()
		case cache.Delete:
			MemoryDeptData.Lock()
			departmentMap, ok := MemoryDeptData.Cache[project]
			if ok {
				delete(departmentMap, id)
				MemoryDeptData.Cache[project] = departmentMap
			}
			MemoryDeptData.Unlock()
		default:
			return
		}

	}); err != nil {
		return nil, errors.WithStack(err)
	}

	cleanFunc := func() {
		if err := mqCli.UnSubscription(ctx, topic); err != nil {
			logger.Errorln("取消订阅department缓存错误,", err.Error())
		}
	}

	return cleanFunc, nil
}

// Get 根据项目ID和资产ID查询数据
func Get(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, id string, result interface{}) (err error) {
	MemoryDeptData.Lock()
	defer MemoryDeptData.Unlock()
	var department string
	departmentMap, ok := MemoryDeptData.Cache[project]
	if ok {
		department, ok = departmentMap[id]
		if !ok {
			department, err = getByDB(ctx, redisClient, mongoClient, project, id)
			if err != nil {
				return errors.WithStack(err)
			}
			departmentMap[id] = department
			MemoryDeptData.Cache[project] = departmentMap
		}
	} else {
		department, err = getByDB(ctx, redisClient, mongoClient, project, id)
		if err != nil {
			return errors.WithStack(err)
		}
		departmentMap = make(map[string]string)
		departmentMap[id] = department
		MemoryDeptData.Cache[project] = departmentMap
	}

	if err := json.Unmarshal([]byte(department), result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// GetWithoutLock 根据项目ID和资产ID查询数据
func GetWithoutLock(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, id string, result interface{}) (err error) {
	var department string
	departmentMap, ok := MemoryDeptData.Cache[project]
	if ok {
		department, ok = departmentMap[id]
		if !ok {
			department, err = getByDB(ctx, redisClient, mongoClient, project, id)
			if err != nil {
				return errors.WithStack(err)
			}
			departmentMap[id] = department
			MemoryDeptData.Cache[project] = departmentMap
		}
	} else {
		department, err = getByDB(ctx, redisClient, mongoClient, project, id)
		if err != nil {
			return errors.WithStack(err)
		}
		departmentMap = make(map[string]string)
		departmentMap[id] = department
		MemoryDeptData.Cache[project] = departmentMap
	}

	if err := json.Unmarshal([]byte(department), result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Get 根据项目ID和资产ID查询数据
func GetByList(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project string, ids []string, result interface{}) (err error) {
	MemoryDeptData.Lock()
	defer MemoryDeptData.Unlock()
	departmentList := make([]map[string]interface{},0)
	for _, id := range ids {
		deptInfo := map[string]interface{}{}
		err := GetWithoutLock(ctx,redisClient,mongoClient,project,id,&deptInfo)
		if err != nil {
			return errors.WithStack(err)
		}
		departmentList = append(departmentList,deptInfo)
	}

	resultByte,err := json.Marshal(departmentList)
	if err != nil {
		return errors.WithStack(err)
	}
	if err := json.Unmarshal(resultByte, result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}


func getByDB(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, id string) (string, error) {
	department, err := redisClient.HGet(ctx, fmt.Sprintf("%s/department", project), id).Result()
	if err != nil && err != redis.Nil {
		return "", err
	} else if err == redis.Nil {
		col := mongoClient.Database(project).Collection("department")
		departmentTmp := make(map[string]interface{})
		err := mongodb.FindByID(ctx, col, &departmentTmp, id)
		if err != nil {
			return "", err
		}
		departmentBytes, err := json.Marshal(&departmentTmp)
		if err != nil {
			return "", err
		}
		_, err = redisClient.HMSet(ctx, fmt.Sprintf("%s/department", project), map[string]interface{}{id: departmentBytes}).Result()
		if err != nil {
			return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
		}
		department = string(departmentBytes)
	}
	return department, nil
}

// TriggerUpdate 更新redis资产数据,并发送消息通知
func TriggerUpdate(ctx context.Context, mqCli mq.MQ, redisClient *redis.Client, project, id string, department []byte) error {
	_, err := redisClient.HMSet(ctx, fmt.Sprintf("%s/department", project), map[string]interface{}{id: department}).Result()
	if err != nil {
		return fmt.Errorf("更新redis数据错误, %s", err.Error())
	}
	topics := []string{CacheDeptPrefix, string(cache.Update), project, id}
	ctx = context.WithValue(ctx, "exchange", CacheDeptPrefix)
	err = mqCli.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}

// TriggerDelete 删除redis资产数据,并发送消息通知
func TriggerDelete(ctx context.Context, mqCli mq.MQ, redisClient *redis.Client, project, id string) error {
	_, err := redisClient.HDel(context.Background(), fmt.Sprintf("%s/department", project), id).Result()
	if err != nil {
		return fmt.Errorf("删除redis数据错误, %s", err.Error())
	}
	topics := []string{CacheDeptPrefix, string(cache.Delete), project, id}
	ctx = context.WithValue(ctx, "exchange", CacheDeptPrefix)
	err = mqCli.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}
