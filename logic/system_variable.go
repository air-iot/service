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

var SystemVariableLogic = new(systemVariableLogic)

type systemVariableLogic struct {
	systemVariableCache             *sync.Map
	systemVariableMapCache          *sync.Map
	systemVariableNameValueMapCache *sync.Map
}

func (p *systemVariableLogic) FindLocalCache(systemVariableID string) (result *model.SystemVariable, err error) {
	a, b := p.systemVariableCache.Load(systemVariableID)
	if b {
		a1, ok := a.(model.SystemVariable)
		if ok {
			return &a1, nil
		} else {
			return nil, errors.New("结构不正确")
		}
	} else {
		return nil, errors.New("未查询到相关数据")
	}
}

func (p *systemVariableLogic) FindLocalMapCache(systemVariableID string) (result map[string]interface{}, err error) {
	a, b := p.systemVariableMapCache.Load(systemVariableID)
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

func (p *systemVariableLogic) FindLocalNameValueMapCache(systemVariableUid string) (result interface{}, err error) {
	a, b := p.systemVariableNameValueMapCache.Load(systemVariableUid)
	if b {
		return a, nil
	} else {
		return nil, errors.New("未查询到相关数据")
	}
}

func (p *systemVariableLogic) FindLocalMapCacheList(systemVariableIDs []string) (result []map[string]interface{}, err error) {
	result = make([]map[string]interface{}, 0)
	for _, systemVariableID := range systemVariableIDs {
		a, b := p.systemVariableMapCache.Load(systemVariableID)
		if b {
			a1, ok := a.(map[string]interface{})
			if ok {
				result = append(result, a1)
			} else {
				return nil, errors.New("结构不正确")
			}
		} else {
			return nil, errors.New("未查询到相关数据")
		}
	}
	return result, nil
}

func (p *systemVariableLogic) FindBsonMByPipeline(pipeLine mongo.Pipeline) (result []bson.M, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	AllowDiskUse := true
	cur, err := imo.Database.Collection(model.SYSTEMVARIABLE).Aggregate(ctx, pipeLine,
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

func (p *systemVariableLogic) FindByPipeline(pipeLine mongo.Pipeline) (result []model.SystemVariableMongo, err error) {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	AllowDiskUse := true
	cur, err := imo.Database.Collection(model.SYSTEMVARIABLE).Aggregate(ctx, pipeLine,
		&options.AggregateOptions{AllowDiskUse: &AllowDiskUse},
	)
	if err != nil {
		return nil, err
	}
	result = make([]model.SystemVariableMongo, 0)
	for cur.Next(ctx) {
		var r = model.SystemVariableMongo{}
		err := cur.Decode(&r)
		if err != nil {
			return nil, err
		}

		result = append(result, r)
	}
	return result, nil
}
