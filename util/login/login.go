package login

import (
	"context"
	"fmt"
	"github.com/air-iot/service/init/cache"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/util/json"
	"github.com/go-redis/redis/v8"
)

// Get 根据项目ID和资产ID查询数据
func Get(ctx context.Context, redisClient redisdb.Client, project, id string, result interface{}) (err error) {
	nodeBytes, err := GetByDB(ctx, redisClient, project,  id)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(nodeBytes, result); err != nil {
		return err
	}
	return nil
}

func GetByDB(ctx context.Context, cli redisdb.Client, project, id string) ([]byte, error) {
	model, err := cli.HGet(ctx, fmt.Sprintf("%s/%s", project, "loginIPCache"), id).Result()
	if err != nil && err != redis.Nil {
		return nil, err
	} else if err == redis.Nil {
		b,err := json.Marshal([]string{})
		if err != nil {
			return nil, err
		}
		return b, nil
	}
	return []byte(model), nil
}

// TriggerUpdate 更新redis资产数据,并发送消息通知
func TriggerUpdate(ctx context.Context, redisClient redisdb.Client, project,id string, data interface{}) error {
	return cache.Update(ctx, redisClient, project, "loginIPCache", id, data)
}