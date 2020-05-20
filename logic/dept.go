package logic

import (
	"errors"
	"sync"
)

var DeptLogic = new(deptLogic)

type deptLogic struct {
	deptMapCache *sync.Map
}

func (p *deptLogic) FindLocalCacheList(deptIDs []string) (result []map[string]interface{}, err error) {
	result = make([]map[string]interface{},0)
	for _, deptID := range deptIDs {
		a, b := p.deptMapCache.Load(deptID)
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

//
//func (p *deptLogic) FindByPipeline(pipeLine mongo.Pipeline) (result []model.EventMongo, err error) {
//
//	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
//	defer cancel()
//	AllowDiskUse := true
//	cur, err := imo.Database.Collection(model.EVENT).Aggregate(ctx, pipeLine,
//		&options.AggregateOptions{AllowDiskUse: &AllowDiskUse},
//	)
//	if err != nil {
//		return nil, err
//	}
//	result = make([]model.EventMongo, 0)
//	for cur.Next(ctx) {
//		var r = model.EventMongo{}
//		err := cur.Decode(&r)
//		if err != nil {
//			return nil, err
//		}
//
//		result = append(result, r)
//	}
//	return result, nil
//}
