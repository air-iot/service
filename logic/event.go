package logic

import (
	"context"
	"errors"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	imo "github.com/air-iot/service/db/mongo"
	"github.com/air-iot/service/model"
)

var EventLogic = new(eventLogic)

type eventLogic struct {
	eventCache            *sync.Map
	eventCacheWithEventID *sync.Map
}

func (p *eventLogic) FindLocalCache(modelIDPlusType string) (result *[]model.Event, err error) {
	a, b := p.eventCache.Load(modelIDPlusType)
	if b {
		a1, ok := a.([]model.Event)
		if ok {
			return &a1, nil
		} else {
			return nil, errors.New("结构不正确")
		}
	} else {
		return nil, errors.New("未查询到相关数据")
	}
}

func (p *eventLogic) FindLocalCacheByEventID(eventID string) (result *model.Event, err error) {
	a, b := p.eventCacheWithEventID.Load(eventID)
	if b {
		a1, ok := a.(model.Event)
		if ok {
			return &a1, nil
		} else {
			return nil, errors.New("结构不正确")
		}
	} else {
		return nil, errors.New("未查询到相关数据")
	}
}

func (p *eventLogic) FindLocalCacheByType(eventType string) (result *[]model.Event, err error) {
	a, b := p.eventCache.Load(eventType)
	if b {
		a1, ok := a.([]model.Event)
		if ok {
			return &a1, nil
		} else {
			return nil, errors.New("结构不正确")
		}
	} else {
		return nil, errors.New("未查询到相关数据")
	}
}


func (p *eventLogic) FindByPipeline(pipeLine mongo.Pipeline) (result []model.EventMongo, err error) {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	AllowDiskUse := true
	cur, err := imo.Database.Collection(model.EVENT).Aggregate(ctx, pipeLine,
		&options.AggregateOptions{AllowDiskUse: &AllowDiskUse},
	)
	if err != nil {
		return nil, err
	}
	result = make([]model.EventMongo, 0)
	for cur.Next(ctx) {
		var r = model.EventMongo{}
		err := cur.Decode(&r)
		if err != nil {
			return nil, err
		}

		result = append(result, r)
	}
	return result, nil
}
