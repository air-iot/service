package setting

import (
	"context"
	"fmt"
	"github.com/air-iot/service/init/cache/entity"
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

const CacheSettingPrefix = "cacheSetting"

var MemorySettingData = struct {
	sync.Mutex
	Cache       map[string]string
	CacheByType map[string]map[string]string
}{
	Cache:       map[string]string{},
	CacheByType: map[string]map[string]string{},
}

// CacheHandler 初始化setting缓存
func CacheHandler(ctx context.Context, redisClient *redis.Client, mqCli mq.MQ) (func(), error) {
	topic := []string{CacheSettingPrefix, "#"}

	ctx = context.WithValue(ctx, "exchange", CacheSettingPrefix)
	ctx = context.WithValue(ctx, "queue", fmt.Sprintf("setting%d", rand.Int()))

	// 0: 前缀 1: 处理方式 2: 项目id 3: 模型id
	if err := mqCli.Consume(ctx, topic, 4, func(topic1 string, topics []string, payload []byte) {
		if len(topics) != 4 || topics[0] != CacheSettingPrefix {
			logger.Errorln("缓存模型数据,消息主题(topic)格式错误")
			return
		}
		project, id := topics[2], topics[3]
		switch cache.CacheMethod(topics[1]) {
		case cache.Update:
			result, err := redisClient.Get(ctx, fmt.Sprintf("%s/setting", project)).Result()
			if err != nil {
				logger.Errorf("缓存模型数据,查询redis缓存[ %s %s ]错误, %s", project, id, err.Error())
				return
			}
			MemorySettingData.Lock()
			MemorySettingData.Cache[project] = result

			settingInfo := entity.Setting{}
			err = json.Unmarshal([]byte(result), &settingInfo)
			if err != nil {
				return
			}
			settingMap := map[string]string{}
			for _, warnKind := range settingInfo.Warning.WarningKind {
				if warnKind.ID != "" {
					settingMap[warnKind.ID] = warnKind.Name
				}
			}
			MemorySettingData.CacheByType[project] = settingMap
			MemorySettingData.Unlock()
		case cache.Delete:
			MemorySettingData.Lock()
			delete(MemorySettingData.Cache, project)
			delete(MemorySettingData.CacheByType, project)
			MemorySettingData.Unlock()
		default:
			return
		}

	}); err != nil {
		return nil, errors.WithStack(err)
	}

	cleanFunc := func() {
		if err := mqCli.UnSubscription(ctx, topic); err != nil {
			logger.Errorln("取消订阅setting缓存错误,", err.Error())
		}
	}

	return cleanFunc, nil
}

// Get 根据项目ID和资产ID查询数据
func Get(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project string, result interface{}) (err error) {
	MemorySettingData.Lock()
	defer MemorySettingData.Unlock()
	var setting string
	settingMap, ok := MemorySettingData.Cache[project]
	if ok {
		setting = settingMap
	} else {
		setting, err = getByDB(ctx, redisClient, mongoClient, project)
		if err != nil {
			return errors.WithStack(err)
		}
		MemorySettingData.Cache[project] = setting
	}

	if err := json.Unmarshal([]byte(setting), result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Get 根据项目ID和资产ID查询数据
func GetByWarnKindID(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, warnKindID string) (string,error) {
	MemorySettingData.Lock()
	defer MemorySettingData.Unlock()
	var resultName string
	settingMap, ok := MemorySettingData.CacheByType[project]
	if ok {
		resultName, ok = settingMap[warnKindID]
		if !ok {
			setting, err := getByDBAndWarnKindID(ctx, redisClient, mongoClient, project, warnKindID)
			if err != nil {
				return "",errors.WithStack(err)
			}
			settingInfo := entity.Setting{}
			err = json.Unmarshal([]byte(setting), &settingInfo)
			if err != nil {
				return "",errors.WithStack(err)
			}
			for _, warnKind := range settingInfo.Warning.WarningKind {
				if warnKind.ID != "" {
					settingMap[warnKind.ID] = warnKind.Name
				}
			}
			MemorySettingData.CacheByType[project] = settingMap
			resultName = settingMap[warnKindID]
		}
	} else {
		setting, err := getByDBAndWarnKindID(ctx, redisClient, mongoClient, project, warnKindID)
		if err != nil {
			return "",errors.WithStack(err)
		}
		settingInfo := entity.Setting{}
		err = json.Unmarshal([]byte(setting), &settingInfo)
		if err != nil {
			return "",errors.WithStack(err)
		}
		settingMap = make(map[string]string)
		for _, warnKind := range settingInfo.Warning.WarningKind {
			if warnKind.ID != "" {
				settingMap[warnKind.ID] = warnKind.Name
			}
		}
		MemorySettingData.CacheByType[project] = settingMap
		resultName = settingMap[warnKindID]
	}

	return resultName,nil
}

func getByDBAndWarnKindID(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, warnKindID string) (string, error) {
	//setting, err := redisClient.HGet(ctx, fmt.Sprintf("%s/setting", project), id).Result()
	//if err != nil && err != redis.Nil {
	//	return "", err
	//} else if err == redis.Nil {
	col := mongoClient.Database(project).Collection("setting")
	settingTmpList := make([]map[string]interface{}, 0)
	_, err := mongodb.FindFilter(ctx, col, &settingTmpList, mongodb.QueryOption{})
	if err != nil {
		return "", err
	}
	settingTmp := make(map[string]interface{})
	if len(settingTmpList) != 0 {
		settingTmp = settingTmpList[0]
	} else {
		return "", fmt.Errorf("未查询到系统配置")
	}
	settingBytes, err := json.Marshal(&settingTmp)
	if err != nil {
		return "", err
	}
	_, err = redisClient.Set(ctx, fmt.Sprintf("%s/setting", project), string(settingBytes), 0).Result()
	if err != nil {
		return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
	}
	setting := string(settingBytes)
	//}
	return setting, nil
}

func getByDB(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project string) (string, error) {
	setting, err := redisClient.Get(ctx, fmt.Sprintf("%s/setting", project)).Result()
	if err != nil && err != redis.Nil {
		return "", err
	} else if err == redis.Nil {
		col := mongoClient.Database(project).Collection("setting")
		settingTmp := make(map[string]interface{})
		settingTmpList := make([]map[string]interface{}, 0)
		_, err := mongodb.FindFilter(ctx, col, &settingTmp, mongodb.QueryOption{})
		if err != nil {
			return "", err
		}
		if len(settingTmpList) != 0 {
			settingTmp = settingTmpList[0]
		} else {
			return "", fmt.Errorf("未查询到系统配置")
		}
		settingBytes, err := json.Marshal(&settingTmp)
		if err != nil {
			return "", err
		}
		_, err = redisClient.Set(ctx, fmt.Sprintf("%s/setting", project), string(settingBytes), 0).Result()
		if err != nil {
			return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
		}
		setting = string(settingBytes)
	}
	return setting, nil
}

// TriggerUpdate 更新redis资产数据,并发送消息通知
func TriggerUpdate(ctx context.Context, mqCli mq.MQ, redisClient *redis.Client, project, id string, setting []byte) error {
	_, err := redisClient.Set(ctx, fmt.Sprintf("%s/setting", project), string(setting), 0).Result()
	if err != nil {
		return fmt.Errorf("更新redis数据错误, %s", err.Error())
	}
	topics := []string{CacheSettingPrefix, string(cache.Update), project, id}
	ctx = context.WithValue(ctx, "exchange", CacheSettingPrefix)
	err = mqCli.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}

// TriggerDelete 删除redis资产数据,并发送消息通知
func TriggerDelete(ctx context.Context, mqCli mq.MQ, redisClient *redis.Client, project, id string) error {
	_, err := redisClient.Del(context.Background(), fmt.Sprintf("%s/setting", project)).Result()
	if err != nil {
		return fmt.Errorf("删除redis数据错误, %s", err.Error())
	}
	topics := []string{CacheSettingPrefix, string(cache.Delete), project, id}
	ctx = context.WithValue(ctx, "exchange", CacheSettingPrefix)
	err = mqCli.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}
