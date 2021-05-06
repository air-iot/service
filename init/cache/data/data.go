package data

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
	"time"

	"github.com/air-iot/service/util/json"
)

func FindCacheByIDTagIDAndInterval(ctx context.Context, redisClient *redis.Client, project, id, tagID,interval string) (*map[string]interface{}, error) {
	dataString, err := redisClient.HGet(ctx, fmt.Sprintf("%s/data", project), fmt.Sprintf("%s|%s|%s", id, tagID,interval)).Result()
	if err != nil {
		return nil, err
	}
	resultMap := map[string]interface{}{}
	err = json.Unmarshal([]byte(dataString), &resultMap)
	if err != nil {
		return nil, err
	}
	_, ok1 := resultMap["value"]
	_, ok2 := resultMap["time"]
	if !ok1 || !ok2 {
		return nil, fmt.Errorf("未查询到相关数据值")
	}
	return &resultMap, nil
}


func SaveDifferenceCacheByIDTagID(ctx context.Context, redisClient *redis.Client, project, id string, t time.Time, data map[string]interface{}, interval string) error {
	//d := make(map[string]map[string]interface{})
	var p redis.Pipeliner
	p = redisClient.TxPipeline()
	for k, v := range data {
		b,err := json.Marshal(map[string]interface{}{
			"time":  t.Unix(),
			"value": v,
		})
		if err != nil {
			return fmt.Errorf("序列化时段差缓存数据失败, %s", err.Error())
		}
		_, err = p.HMSet(ctx, fmt.Sprintf("%s/data", project), map[string]interface{}{fmt.Sprintf("%s|%s|%s", id, k, interval): b}).Result()
		if err != nil {
			return fmt.Errorf("更新redis数据错误, %s", err.Error())
		}
	}
	if _, err := p.Exec(context.Background()); err != nil {
		return err
	}
	return nil
}
