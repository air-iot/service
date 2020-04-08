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
	restfulapi "github.com/air-iot/service/restful-api"
)

var ModelLogic = new(modelLogic)

type modelLogic struct {
	modelNapCache *sync.Map
	modelCache    *sync.Map
}

func (p *modelLogic) FindLocalCache(modelID string) (result *model.Model, err error) {
	a, b := p.modelCache.Load(modelID)
	if b {
		a1, ok := a.(model.Model)
		if ok {
			return &a1, nil
		} else {
			return nil, errors.New("结构不正确")
		}
	} else {
		return nil, errors.New("未查询到相关数据")
	}
}

func (p *modelLogic) FindLocalMapCache(modelID string) (result map[string]interface{}, err error) {
	a, b := p.modelNapCache.Load(modelID)
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

//////Deprecated

func (p *modelLogic) FindCacheTag(modelID, tagID string) (result *model.Tag, err error) {
	a, b := p.modelCache.Load(modelID)
	if !b {
		return nil, errors.New("未查询到相关数据")
	}
	r, ok := a.(model.Model)
	if !ok {
		return nil, errors.New("结构不正确")
	}
	for _, t := range r.Device.Tags {
		if t.ID == tagID {
			return &t, nil
		}
	}
	for _, t := range r.Computed.Tags {
		if t.ID == tagID {
			return &t, nil
		}
	}
	return nil, errors.New("未查询到数据点")
}

func (*modelLogic) FindByPipeline(pipeLine mongo.Pipeline) (result []model.ModelMongo, err error) {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	AllowDiskUse := true
	cur, err := imo.Database.Collection(model.MODEL).Aggregate(ctx, pipeLine,
		&options.AggregateOptions{AllowDiskUse: &AllowDiskUse},
	)
	if err != nil {
		return nil, err
	}
	result = make([]model.ModelMongo, 0)
	for cur.Next(ctx) {
		var r = model.ModelMongo{}
		err := cur.Decode(&r)
		if err != nil {
			return nil, err
		}

		result = append(result, r)
	}
	return result, nil
}

func (*modelLogic) FindBsonMByPipeline(pipeLine mongo.Pipeline) (result []bson.M, err error) {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	AllowDiskUse := true
	cur, err := imo.Database.Collection(model.MODEL).Aggregate(ctx, pipeLine,
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
