package user

import (
	"context"
	"fmt"
	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/mongodb"
	"github.com/air-iot/service/util/formatx"
	"go.mongodb.org/mongo-driver/bson"
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

const CacheUserPrefix = "cacheUser"

var MemoryUserData = struct {
	sync.Mutex
	Cache       map[string]map[string]string
	CacheByDept map[string]map[string]string
	CacheByRole map[string]map[string]string
}{
	Cache:       map[string]map[string]string{},
	CacheByDept: map[string]map[string]string{},
	CacheByRole: map[string]map[string]string{},
}

// CacheHandler 初始化user缓存
func CacheHandler(ctx context.Context, redisClient *redis.Client, mqCli mq.MQ) (func(), error) {
	topic := []string{CacheUserPrefix, "#"}

	ctx = context.WithValue(ctx, "exchange", CacheUserPrefix)
	ctx = context.WithValue(ctx, "queue", fmt.Sprintf("user%d", rand.Int()))

	// 0: 前缀 1: 处理方式 2: 项目id 3: 模型id
	if err := mqCli.Consume(ctx, topic, 4, func(topic1 string, topics []string, payload []byte) {
		if len(topics) != 4 || topics[0] != CacheUserPrefix {
			logger.Errorln("缓存模型数据,消息主题(topic)格式错误")
			return
		}
		project, id := topics[2], topics[3]
		switch cache.CacheMethod(topics[1]) {
		case cache.Update:
			result, err := redisClient.HGet(ctx, fmt.Sprintf("%s/user", project), id).Result()
			if err != nil {
				logger.Errorf("缓存模型数据,查询redis缓存[ %s %s ]错误, %s", project, id, err.Error())
				return
			}
			MemoryUserData.Lock()
			userMap, ok := MemoryUserData.Cache[project]
			if ok {
				userMap[id] = result
			} else {
				userMap = make(map[string]string)
				userMap[id] = result
			}
			MemoryUserData.Cache[project] = userMap

			//部门用户map
			userInfo := entity.User{}
			err = json.Unmarshal([]byte(result), &userInfo)
			if err != nil {
				logger.Errorf("解序列化事件失败[ %s %s ]错误, %s", project, id, err.Error())
				return
			}
			deptUserTypeMap, ok := MemoryUserData.CacheByDept[project]
			if ok {
				for _, deptID := range userInfo.Department {
					if userListForType, ok := deptUserTypeMap[deptID]; ok {
						userInfoList := make([]entity.User, 0)
						err = json.Unmarshal([]byte(userListForType), &userInfoList)
						if err != nil {
							logger.Errorf("解序列化缓存的部门下用户列表失败[ %s %s ]错误, %s", project, id, err.Error())
							return
						}
						b, err := json.Marshal(formatx.AddNonRepUserByLoop(userInfoList, userInfo))
						if err != nil {
							logger.Errorf("序列化合并的部门下用户列表失败[ %s %s ]错误, %s", project, id, err.Error())
							return
						}
						deptUserTypeMap[deptID] = string(b)
					}
				}
				//eventMap[id] = result
			}
			MemoryUserData.CacheByDept[project] = deptUserTypeMap

			//角色用户map
			roleUserTypeMap, ok := MemoryUserData.CacheByRole[project]
			if ok {
				for _, roleID := range userInfo.Department {
					if userListForType, ok := roleUserTypeMap[roleID]; ok {
						userInfoList := make([]entity.User, 0)
						err = json.Unmarshal([]byte(userListForType), &userInfoList)
						if err != nil {
							logger.Errorf("解序列化缓存的角色下用户列表失败[ %s %s ]错误, %s", project, id, err.Error())
							return
						}
						b, err := json.Marshal(formatx.AddNonRepUserByLoop(userInfoList, userInfo))
						if err != nil {
							logger.Errorf("序列化合并的角色下用户列表失败[ %s %s ]错误, %s", project, id, err.Error())
							return
						}
						roleUserTypeMap[roleID] = string(b)
					}
				}
				//eventMap[id] = result
			}
			MemoryUserData.CacheByRole[project] = roleUserTypeMap

			MemoryUserData.Unlock()
		case cache.Delete:
			MemoryUserData.Lock()
			userMap, ok := MemoryUserData.Cache[project]
			if ok {
				delete(userMap, id)
				MemoryUserData.Cache[project] = userMap
			}
			deptUserTypeMap, ok := MemoryUserData.CacheByDept[project]
			if ok {
				for index, listString := range deptUserTypeMap {
					if listString != "" {
						userInfoList := make([]entity.User, 0)
						err := json.Unmarshal([]byte(listString), &userInfoList)
						if err != nil {
							logger.Errorf("解序列化缓存的部门用户列表失败[ %s %s ]错误, %s", project, id, err.Error())
							return
						}
						b, err := json.Marshal(formatx.DelEleUserByLoop(userInfoList, id))
						if err != nil {
							logger.Errorf("序列化合并的部门用户列表失败[ %s %s ]错误, %s", project, id, err.Error())
							return
						}
						deptUserTypeMap[index] = string(b)
					}
				}
				//eventMap[id] = result
			}
			MemoryUserData.CacheByDept[project] = deptUserTypeMap

			roleUserTypeMap, ok := MemoryUserData.CacheByRole[project]
			if ok {
				for index, listString := range roleUserTypeMap {
					if listString != "" {
						userInfoList := make([]entity.User, 0)
						err := json.Unmarshal([]byte(listString), &userInfoList)
						if err != nil {
							logger.Errorf("解序列化缓存的角色用户列表失败[ %s %s ]错误, %s", project, id, err.Error())
							return
						}
						b, err := json.Marshal(formatx.DelEleUserByLoop(userInfoList, id))
						if err != nil {
							logger.Errorf("序列化合并的角色用户列表失败[ %s %s ]错误, %s", project, id, err.Error())
							return
						}
						roleUserTypeMap[index] = string(b)
					}
				}
				//eventMap[id] = result
			}
			MemoryUserData.CacheByRole[project] = roleUserTypeMap

			MemoryUserData.Unlock()
		default:
			return
		}

	}); err != nil {
		return nil, errors.WithStack(err)
	}

	cleanFunc := func() {
		if err := mqCli.UnSubscription(ctx, topic); err != nil {
			logger.Errorln("取消订阅user缓存错误,", err.Error())
		}
	}

	return cleanFunc, nil
}

// Get 根据项目ID和资产ID查询数据
func Get(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, id string, result interface{}) (err error) {
	MemoryUserData.Lock()
	defer MemoryUserData.Unlock()
	var user string
	userMap, ok := MemoryUserData.Cache[project]
	if ok {
		user, ok = userMap[id]
		if !ok {
			user, err = getByDB(ctx, redisClient, mongoClient, project, id)
			if err != nil {
				return errors.WithStack(err)
			}
			userMap[id] = user
			MemoryUserData.Cache[project] = userMap
		}
	} else {
		user, err = getByDB(ctx, redisClient, mongoClient, project, id)
		if err != nil {
			return errors.WithStack(err)
		}
		userMap = make(map[string]string)
		userMap[id] = user
		MemoryUserData.Cache[project] = userMap
	}

	if err := json.Unmarshal([]byte(user), result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

// Get 根据项目ID和资产ID查询数据
func GetByDept(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project string, ids []string, result interface{}) (err error) {
	MemoryUserData.Lock()
	defer MemoryUserData.Unlock()
	var user string
	userList := make([]map[string]interface{}, 0)
	for _, id := range ids {
		userMap, ok := MemoryUserData.CacheByDept[project]
		if ok {
			user, ok = userMap[id]
			if !ok {
				user, err = getByDBAndDept(ctx, redisClient, mongoClient, project, id)
				if err != nil {
					return errors.WithStack(err)
				}
				userMap[id] = user
				MemoryUserData.CacheByDept[project] = userMap
			}
		} else {
			user, err = getByDBAndDept(ctx, redisClient, mongoClient, project, id)
			if err != nil {
				return errors.WithStack(err)
			}
			userMap = make(map[string]string)
			userMap[id] = user
			MemoryUserData.CacheByDept[project] = userMap
		}
		userInfo := map[string]interface{}{}
		if err := json.Unmarshal([]byte(user), &userInfo); err != nil {
			return errors.WithStack(err)
		}
		userList = formatx.AddNonRepMapByLoop(userList, userInfo)
	}

	resultByte, err := json.Marshal(userList)
	if err != nil {
		return errors.WithStack(err)
	}
	if err := json.Unmarshal(resultByte, result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func getByDBAndDept(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, id string) (string, error) {
	//user, err := redisClient.HGet(ctx, fmt.Sprintf("%s/user", project), id).Result()
	//if err != nil && err != redis.Nil {
	//	return "", err
	//} else if err == redis.Nil {
	col := mongoClient.Database(project).Collection("user")
	userTmp := make([]entity.UserMongo, 0)
	paramMatch := bson.D{bson.E{Key: "$match", Value: bson.M{"department": id}}}
	pipeline := mongo.Pipeline{}
	pipeline = append(pipeline, paramMatch)
	err := mongodb.FindPipeline(ctx, col, &userTmp, pipeline)
	if err != nil {
		return "", err
	}
	userBytes, err := json.Marshal(&userTmp)
	if err != nil {
		return "", err
	}
	for _, user := range userTmp {
		userEleBytes, err := json.Marshal(&user)
		if err != nil {
			return "", err
		}
		_, err = redisClient.HMSet(ctx, fmt.Sprintf("%s/user", project), map[string]interface{}{user.ID: userEleBytes}).Result()
		if err != nil {
			return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
		}
	}
	user := string(userBytes)
	//}
	return user, nil
}

// Get 根据项目ID和资产ID查询数据
func GetByRole(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project string, ids []string, result interface{}) (err error) {
	MemoryUserData.Lock()
	defer MemoryUserData.Unlock()
	var user string
	userList := make([]map[string]interface{}, 0)
	for _, id := range ids {
		userMap, ok := MemoryUserData.CacheByRole[project]
		if ok {
			user, ok = userMap[id]
			if !ok {
				user, err = getByDBAndRole(ctx, redisClient, mongoClient, project, id)
				if err != nil {
					return errors.WithStack(err)
				}
				userMap[id] = user
				MemoryUserData.CacheByRole[project] = userMap
			}
		} else {
			user, err = getByDBAndRole(ctx, redisClient, mongoClient, project, id)
			if err != nil {
				return errors.WithStack(err)
			}
			userMap = make(map[string]string)
			userMap[id] = user
			MemoryUserData.CacheByRole[project] = userMap
		}
		userInfo := map[string]interface{}{}
		if err := json.Unmarshal([]byte(user), &userInfo); err != nil {
			return errors.WithStack(err)
		}
		userList = formatx.AddNonRepMapByLoop(userList, userInfo)
	}

	resultByte, err := json.Marshal(userList)
	if err != nil {
		return errors.WithStack(err)
	}
	if err := json.Unmarshal(resultByte, result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func getByDBAndRole(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, id string) (string, error) {
	//user, err := redisClient.HGet(ctx, fmt.Sprintf("%s/user", project), id).Result()
	//if err != nil && err != redis.Nil {
	//	return "", err
	//} else if err == redis.Nil {
	col := mongoClient.Database(project).Collection("user")
	userTmp := make([]entity.UserMongo, 0)
	paramMatch := bson.D{bson.E{Key: "$match", Value: bson.M{"roles": id}}}
	pipeline := mongo.Pipeline{}
	pipeline = append(pipeline, paramMatch)
	err := mongodb.FindPipeline(ctx, col, &userTmp, pipeline)
	if err != nil {
		return "", err
	}
	userBytes, err := json.Marshal(&userTmp)
	if err != nil {
		return "", err
	}
	for _, user := range userTmp {
		userEleBytes, err := json.Marshal(&user)
		if err != nil {
			return "", err
		}
		_, err = redisClient.HMSet(ctx, fmt.Sprintf("%s/user", project), map[string]interface{}{user.ID: userEleBytes}).Result()
		if err != nil {
			return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
		}
	}
	user := string(userBytes)
	//}
	return user, nil
}

func getByDB(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, id string) (string, error) {
	user, err := redisClient.HGet(ctx, fmt.Sprintf("%s/user", project), id).Result()
	if err != nil && err != redis.Nil {
		return "", err
	} else if err == redis.Nil {
		col := mongoClient.Database(project).Collection("user")
		userTmp := make(map[string]interface{})
		err := mongodb.FindByID(ctx, col, &userTmp, id)
		if err != nil {
			return "", err
		}
		userBytes, err := json.Marshal(&userTmp)
		if err != nil {
			return "", err
		}
		_, err = redisClient.HMSet(ctx, fmt.Sprintf("%s/user", project), map[string]interface{}{id: userBytes}).Result()
		if err != nil {
			return "", fmt.Errorf("更新redis数据错误, %s", err.Error())
		}
		user = string(userBytes)
	}
	return user, nil
}

// GetWithoutLock 根据项目ID和资产ID查询数据
func GetWithoutLock(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project, id string, result interface{}) (err error) {
	var user string
	userMap, ok := MemoryUserData.Cache[project]
	if ok {
		user, ok = userMap[id]
		if !ok {
			user, err = getByDB(ctx, redisClient, mongoClient, project, id)
			if err != nil {
				return errors.WithStack(err)
			}
			userMap[id] = user
			MemoryUserData.Cache[project] = userMap
		}
	} else {
		user, err = getByDB(ctx, redisClient, mongoClient, project, id)
		if err != nil {
			return errors.WithStack(err)
		}
		userMap = make(map[string]string)
		userMap[id] = user
		MemoryUserData.Cache[project] = userMap
	}

	if err := json.Unmarshal([]byte(user), result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}


// Get 根据项目ID和资产ID查询数据
func GetByList(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client, project string, ids []string, result interface{}) (err error) {
	MemoryUserData.Lock()
	defer MemoryUserData.Unlock()
	userList := make([]map[string]interface{},0)
	for _, id := range ids {
		userInfo := map[string]interface{}{}
		err := GetWithoutLock(ctx,redisClient,mongoClient,project,id,&userInfo)
		if err != nil {
			return errors.WithStack(err)
		}
		userList = append(userList,userInfo)
	}

	resultByte,err := json.Marshal(userList)
	if err != nil {
		return errors.WithStack(err)
	}
	if err := json.Unmarshal(resultByte, result); err != nil {
		return errors.WithStack(err)
	}
	return nil
}


// TriggerUpdate 更新redis资产数据,并发送消息通知
func TriggerUpdate(ctx context.Context, mqCli mq.MQ, redisClient *redis.Client, project, id string, user []byte) error {
	_, err := redisClient.HMSet(ctx, fmt.Sprintf("%s/user", project), map[string]interface{}{id: user}).Result()
	if err != nil {
		return fmt.Errorf("更新redis数据错误, %s", err.Error())
	}
	topics := []string{CacheUserPrefix, string(cache.Update), project, id}
	ctx = context.WithValue(ctx, "exchange", CacheUserPrefix)
	err = mqCli.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}

// TriggerDelete 删除redis资产数据,并发送消息通知
func TriggerDelete(ctx context.Context, mqCli mq.MQ, redisClient *redis.Client, project, id string) error {
	_, err := redisClient.HDel(context.Background(), fmt.Sprintf("%s/user", project), id).Result()
	if err != nil {
		return fmt.Errorf("删除redis数据错误, %s", err.Error())
	}
	topics := []string{CacheUserPrefix, string(cache.Delete), project, id}
	ctx = context.WithValue(ctx, "exchange", CacheUserPrefix)
	err = mqCli.Publish(ctx, topics, []byte{})
	if err != nil {
		return fmt.Errorf("发送消息通知错误, %s", err.Error())
	}
	return nil
}
