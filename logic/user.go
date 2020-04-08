package logic

import (
	imo "common/db/mongo"
	"common/model"
	"common/restful-api"
	"context"
	"errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"sync"
	"time"
)

var UserLogic = new(userLogic)

type userLogic struct {
	userCache    sync.Map
	userMapCache sync.Map
}

func (p *userLogic) FindLocalMapCache(userID string) (result map[string]interface{}, err error) {
	a, b := p.userMapCache.Load(userID)
	if b {
		a1, ok := a.(map[string]interface{})
		if ok {
			return a1, nil
		} else {
			return nil, errors.New("结构不正确")
		}
	} else {
		return nil, errors.New("未查询到相关数据")
	}
}

func (p *userLogic) FindLocalMapCacheList(userIDs []string) (result map[string]*map[string]interface{}, err error) {
	result = make(map[string]*map[string]interface{})
	for _, userID := range userIDs {
		a, b := p.userMapCache.Load(userID)
		if b {
			a1, ok := a.(map[string]interface{})
			if ok {
				if a1ID, ok := a1["id"].(string); ok {
					result[a1ID] = &a1
				}
			} else {
				return nil, errors.New("结构不正确")
			}
		} else {
			return nil, errors.New("未查询到相关数据")
		}
	}
	return result, nil
}

func (p *userLogic) FindBsonMByPipeline(pipeLine mongo.Pipeline) (result []bson.M, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	AllowDiskUse := true
	cur, err := imo.Database.Collection(model.USER).Aggregate(ctx, pipeLine,
		&options.AggregateOptions{AllowDiskUse: &AllowDiskUse},
	)
	if err != nil {
		return nil, err
	}
	result = make([]bson.M, 0)
	for cur.Next(ctx) {
		var r = bson.M{}
		err := cur.Decode(&r)
		if err != nil {
			return nil, err
		}
		restfulapi.ConvertKeyID(&r)
		result = append(result, r)
	}
	return result, nil
}
