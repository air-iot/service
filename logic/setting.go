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

var SettingLogic = new(settingLogic)

type settingLogic struct {
	settingCache *sync.Map
	warnTypeMap  *sync.Map
}

func (p *settingLogic) FindLocalCache() (result *model.Setting, err error) {
	a, b := p.settingCache.Load("setting")
	if b {
		a1, ok := a.(model.Setting)
		if ok {
			return &a1, nil
		} else {
			return nil, errors.New("结构不正确")
		}
	} else {
		return nil, errors.New("未查询到相关数据")
	}
}

func (p *settingLogic) FindLocalWarnTypeMapCache(id string) (result string, err error) {
	a, b := p.warnTypeMap.Load(id)
	if b {
		a1, ok := a.(string)
		if ok {
			return a1, nil
		} else {
			return "", errors.New("结构不正确")
		}
	} else {
		return "", errors.New("未查询到相关数据")
	}
}


//////Deprecated

func (*settingLogic) FindByPipeline(pipeLine mongo.Pipeline) (result []model.SettingMongo, err error) {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	AllowDiskUse := true
	cur, err := imo.Database.Collection(model.SETTING).Aggregate(ctx, pipeLine,
		&options.AggregateOptions{AllowDiskUse: &AllowDiskUse},
	)
	if err != nil {
		return nil, err
	}
	result = make([]model.SettingMongo, 0)
	for cur.Next(ctx) {
		var r = model.SettingMongo{}
		err := cur.Decode(&r)
		if err != nil {
			return nil, err
		}

		result = append(result, r)
	}
	return result, nil
}

func (*settingLogic) FindBsonMByPipeline(pipeLine mongo.Pipeline) (result []bson.M, err error) {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	AllowDiskUse := true
	cur, err := imo.Database.Collection(model.SETTING).Aggregate(ctx, pipeLine,
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
