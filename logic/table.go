package logic

import (
	"sync"
)

var TableLogic = new(tableLogic)

type tableLogic struct {
	tableMapCache *sync.Map
}

func (p *tableLogic) FindLocalCacheAll() (result []map[string]interface{}, err error) {
	result = make([]map[string]interface{},0)
	p.tableMapCache.Range(func(k, v interface{}) bool {
		//这个函数的入参、出参的类型都已经固定，不能修改
		//可以在函数体内编写自己的代码，调用map中的k,v
		if ele, ok := v.(map[string]interface{}); ok {
			result = append(result,ele)
		}
		return true
	})
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
