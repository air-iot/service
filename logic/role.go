package logic

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	imo "github.com/air-iot/service/db/mongo"
	"github.com/air-iot/service/model"
	"github.com/air-iot/service/restful-api"
)

var RoleLogic = new(roleLogic)

type roleLogic struct {
	roleCache    *sync.Map
	roleMapCache *sync.Map
}

func (p *roleLogic) FindLocalCache(roleID string) (result *model.Role, err error) {
	a, b := p.roleCache.Load(roleID)
	if b {
		a1, ok := a.(model.Role)
		if ok {
			return &a1, nil
		} else {
			return nil, errors.New("结构不正确")
		}
	} else {
		return nil, errors.New("未查询到相关数据")
	}
}


func (p *roleLogic) FindLocalMapCache(roleID string) (result map[string]interface{}, err error) {
	a, b := p.roleMapCache.Load(roleID)
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

func (p *roleLogic) FindLocalMapCacheList(roleIDs []string) (result []map[string]interface{}, err error) {
	result = make([]map[string]interface{},0)
	for _, roleID := range roleIDs {
		a, b := p.roleMapCache.Load(roleID)
		if b {
			a1, ok := a.(map[string]interface{})
			if ok {
				result = append(result,a1)
			} else {
				return nil, errors.New("结构不正确")
			}
		} else {
			return nil, errors.New("未查询到相关数据")
		}
	}
	return result, nil
}

func (p *roleLogic) FindBsonMByPipeline(pipeLine mongo.Pipeline) (result []bson.M, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	AllowDiskUse := true
	cur, err := imo.Database.Collection(model.ROLE).Aggregate(ctx, pipeLine,
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

func (p *roleLogic) FindByPipeline(pipeLine mongo.Pipeline) (result []model.RoleMongo, err error) {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	AllowDiskUse := true
	cur, err := imo.Database.Collection(model.ROLE).Aggregate(ctx, pipeLine,
		&options.AggregateOptions{AllowDiskUse: &AllowDiskUse},
	)
	if err != nil {
		return nil, err
	}
	result = make([]model.RoleMongo, 0)
	for cur.Next(ctx) {
		var r = model.RoleMongo{}
		err := cur.Decode(&r)
		if err != nil {
			return nil, err
		}

		result = append(result, r)
	}
	return result, nil
}
