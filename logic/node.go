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

var NodeLogic = new(nodeLogic)

type nodeLogic struct {
	sync.Mutex
	nodeCache            *sync.Map
	nodeMapCache         *sync.Map
	nodeCacheWithModelID *sync.Map
	nodeUidMapCache      *sync.Map
}

func (p *nodeLogic) FindLocalCacheUid(uid string) (result *model.Node, err error) {
	a, b := p.nodeUidMapCache.Load(uid)
	if b {
		a1, ok := a.(model.Node)
		if ok {
			return &a1, nil
		} else {
			return nil, errors.New("结构不正确")
		}
	} else {
		return nil, errors.New("未查询到相关数据")
	}
}

func (p *nodeLogic) FindLocalCache(nodeID string) (result *model.Node, err error) {
	a, b := p.nodeCache.Load(nodeID)
	if b {
		a1, ok := a.(model.Node)
		if ok {
			return &a1, nil
		} else {
			return nil, errors.New("结构不正确")
		}
	} else {
		return nil, errors.New("未查询到相关数据")
	}
}

func (p *nodeLogic) FindLocalCacheByModelID(modelID string) (result *[]model.Node, err error) {
	a, b := p.nodeCacheWithModelID.Load(modelID)
	if b {
		a1, ok := a.([]model.Node)
		if ok {
			return &a1, nil
		} else {
			return nil, errors.New("结构不正确")
		}
	} else {
		return nil, errors.New("未查询到相关数据")
	}
}

func (p *nodeLogic) FindLocalMapCache(nodeID string) (result map[string]interface{}, err error) {
	a, b := p.nodeMapCache.Load(nodeID)
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

func (p *nodeLogic) FindLocalCacheList(nodeIDs []string) (result map[string]*model.Node, err error) {
	result = make(map[string]*model.Node)
	for _, nodeID := range nodeIDs {
		a, b := p.nodeCache.Load(nodeID)
		if b {
			a1, ok := a.(model.Node)
			if ok {
				result[a1.ID] = &a1
			} else {
				return nil, errors.New("结构不正确")
			}
		}
		//} else {
		//	return nil, errors.New("未查询到相关数据")
		//}
	}
	return result, nil
}

///////Deprecated

func (p *nodeLogic) FindCacheTag(nodeID, tagID string) (result *model.Tag, err error) {
	a, b := p.nodeCache.Load(nodeID)
	if !b {
		return nil, errors.New("未查询到相关数据")
	}
	r, ok := a.(model.Node)
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

func (*nodeLogic) FindByPipeline(pipeLine mongo.Pipeline) (result []model.NodeMongo, err error) {

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	AllowDiskUse := true
	cur, err := imo.Database.Collection(model.NODE).Aggregate(ctx, pipeLine,
		&options.AggregateOptions{AllowDiskUse: &AllowDiskUse},
	)
	if err != nil {
		return nil, err
	}
	result = make([]model.NodeMongo, 0)
	for cur.Next(ctx) {
		var r = model.NodeMongo{}
		err := cur.Decode(&r)
		if err != nil {
			return nil, err
		}

		result = append(result, r)
	}
	return result, nil
}

func (*nodeLogic) FindBsonMByPipeline(pipeLine mongo.Pipeline) (result []bson.M, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Second)
	defer cancel()
	AllowDiskUse := true
	cur, err := imo.Database.Collection(model.NODE).Aggregate(ctx, pipeLine,
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
