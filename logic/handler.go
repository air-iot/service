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

var EventHandlerLogic = new(eventHandlerLogic)

type eventHandlerLogic struct {
	eventHandlerCache sync.Map
}

func (p *eventHandlerLogic) FindLocalCache(eventID string) (result *[]model.EventHandler, err error) {
	a, b := p.eventHandlerCache.Load(eventID)
	if b {
		a1, ok := a.([]model.EventHandler)
		if ok {
			return &a1, nil
		} else {
			return nil, errors.New("结构不正确")
		}
	} else {
		return nil, errors.New("未查询到相关数据")
	}
}

func (p *eventHandlerLogic) FindByPipeline(pipeLine mongo.Pipeline) (result []model.EventHandlerMongo, err error) {

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	AllowDiskUse := true
	cur, err := imo.Database.Collection(model.EVENTHANDLER).Aggregate(ctx, pipeLine,
		&options.AggregateOptions{AllowDiskUse: &AllowDiskUse},
	)
	if err != nil {
		return nil, err
	}
	result = make([]model.EventHandlerMongo, 0)
	for cur.Next(ctx) {
		var r = model.EventHandlerMongo{}
		err := cur.Decode(&r)
		if err != nil {
			return nil, err
		}

		result = append(result, r)
	}
	return result, nil
}
