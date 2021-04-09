package restfulapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/air-iot/service/model"
	"github.com/go-redis/redis/v8"
	_ "github.com/influxdata/influxdb1-client" // this is important because of the bug in go mod
	"github.com/influxdata/influxdb1-client/v2"
	"github.com/zhgqiang/jsonpath"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	iredis "github.com/air-iot/service/db/redis"
)

type (
	// APIView 模型API视图接口
	APIView struct {
		mutex *sync.Mutex
	}

	// PipelineFunc 声明了一个Pipeline修改函数类型
	PipelineFunc func() mongo.Pipeline

	// CalcPipelineFunc 声明了一个修改返回值函数
	CalcPipelineFunc func(*bson.M)
)

// GetCollection 获取mongo collection
func (p *APIView) GetCollection(database, collection string, cli *mongo.Client) *mongo.Collection {
	return cli.Database(database).Collection(collection)
}

// SaveOne 保存数
// model:数据
func (p *APIView) SaveOne(ctx context.Context, col *mongo.Collection, model bson.M) (*mongo.InsertOneResult, error) {
	r, err := ConvertOID("", model)
	if err != nil {
		return nil, fmt.Errorf(`数据格式ObjectID转换错误.%s`, err.Error())
	}
	return col.InsertOne(ctx, r)
}

// SaveOne 保存数
// model:数据
func SaveOne(ctx context.Context, col *mongo.Collection, model bson.M) (*mongo.InsertOneResult, error) {
	r, err := ConvertOID("", model)
	if err != nil {
		return nil, fmt.Errorf(`数据格式ObjectID转换错误.%s`, err.Error())
	}
	return col.InsertOne(ctx, r)
}

// SaveOne 保存数
// model:数据
func (p *APIView) SaveOneInterface(ctx context.Context, col *mongo.Collection, model interface{}) (*mongo.InsertOneResult, error) {
	r, err := ConvertOID("", model)
	if err != nil {
		return nil, fmt.Errorf(`数据格式ObjectID转换错误.%s`, err.Error())
	}
	return col.InsertOne(ctx, r)
}

// SaveMany 批量保存数据
// models:数据
func (p *APIView) SaveMany(ctx context.Context, col *mongo.Collection, models []bson.M) (*mongo.InsertManyResult, error) {
	rList := make([]interface{}, 0)
	for _, model := range models {
		r, err := ConvertOID("", model)
		if err != nil {
			return nil, fmt.Errorf(`数据格式ObjectID转换错误.%s`, err.Error())
		}
		rList = append(rList, r)
	}
	return col.InsertMany(ctx, rList)
}

// SaveMany 批量保存数据
// models:数据
func (p *APIView) SaveManyWithConvertOID(ctx context.Context, col *mongo.Collection, models []bson.M) (*mongo.InsertManyResult, error) {
	rList := make([]interface{}, 0)
	for _, model := range models {
		rList = append(rList, model)
	}
	return col.InsertMany(ctx, rList)
}

// DeleteByID 根据ID删除数据
// id:主键_id
func (p *APIView) DeleteByID(ctx context.Context, col *mongo.Collection, id string) (*mongo.DeleteResult, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}
	return col.DeleteOne(ctx, bson.M{"_id": oid})
}

// DeleteOne 根据条件删除单条数据
// condition:删除条件
func (p *APIView) DeleteOne(ctx context.Context, col *mongo.Collection, condition *bson.M) (*mongo.DeleteResult, error) {
	return col.DeleteOne(ctx, *condition)
}

// DeleteMany 根据条件进行多条数据删除
// condition:删除条件
func (p *APIView) DeleteMany(ctx context.Context, col *mongo.Collection, condition *bson.M) (*mongo.DeleteResult, error) {
	return col.DeleteMany(ctx, *condition)
}

// UpdateByID 根据ID及数据更新
// id:主键_id model:更新的数据
func (p *APIView) UpdateByID(ctx context.Context, col *mongo.Collection, id string, model bson.M) (*mongo.UpdateResult, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}
	r, err := ConvertOID("", model)
	if err != nil {
		return nil, fmt.Errorf(`数据格式ObjectID转换错误.%s`, err.Error())
	}

	return col.UpdateOne(ctx, bson.M{"_id": oid}, bson.D{bson.E{Key: "$set", Value: r}})
}

// UpdateByID 根据ID及数据更新
// id:主键_id model:更新的数据
func UpdateByID(ctx context.Context, col *mongo.Collection, id string, model bson.M) (*mongo.UpdateResult, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}
	r, err := ConvertOID("", model)
	if err != nil {
		return nil, fmt.Errorf(`数据格式ObjectID转换错误.%s`, err.Error())
	}

	return col.UpdateOne(ctx, bson.M{"_id": oid}, bson.D{bson.E{Key: "$set", Value: r}})
}

// UpdateAll 全部数据更新
func (p *APIView) UpdateAll(ctx context.Context, col *mongo.Collection, model *bson.M) (*mongo.UpdateResult, error) {
	r, err := ConvertOID("", *model)
	if err != nil {
		return nil, fmt.Errorf(`数据格式ObjectID转换错误.%s`, err.Error())
	}
	return col.UpdateMany(ctx, bson.M{}, bson.D{bson.E{Key: "$set", Value: r}})
}

// UpdateManyByIDList 根据id数组进行多条数据更新
func (p *APIView) UpdateManyByIDList(ctx *context.Context, col *mongo.Collection, id []string, model *bson.M) (*mongo.UpdateResult, error) {
	oidList := make([]primitive.ObjectID, 0)
	for _, v := range id {
		oid, err := primitive.ObjectIDFromHex(v)
		if err != nil {
			return nil, err
		}
		oidList = append(oidList, oid)
	}
	r, err := ConvertOID("", *model)
	if err != nil {
		return nil, fmt.Errorf(`数据格式ObjectID转换错误.%s`, err.Error())
	}
	return col.UpdateMany(*ctx, bson.M{"_id": bson.M{"$in": oidList}}, bson.D{bson.E{Key: "$set", Value: r}})
}

// UpdateMany 全部数据更新
// conditions:更新条件 model:更新的数据
func (p *APIView) UpdateMany(ctx context.Context, col *mongo.Collection, conditions, model bson.M) (*mongo.UpdateResult, error) {
	r, err := ConvertOID("", model)
	if err != nil {
		return nil, fmt.Errorf(`数据格式ObjectID转换错误.%s`, err.Error())
	}
	return col.UpdateMany(ctx, conditions, r)
}

// UpdateMany 全部数据更新
// conditions:更新条件 model:更新的数据
func UpdateMany(ctx context.Context, col *mongo.Collection, conditions, model bson.M) (*mongo.UpdateResult, error) {
	r, err := ConvertOID("", model)
	if err != nil {
		return nil, fmt.Errorf(`数据格式ObjectID转换错误.%s`, err.Error())
	}
	return col.UpdateMany(ctx, conditions, r)
}

// UpdateMany 全部数据更新
// conditions:更新条件 model:更新的数据
func (p *APIView) UpdateManyWithOption(ctx context.Context, col *mongo.Collection, conditions, model bson.M, options *options.UpdateOptions) (*mongo.UpdateResult, error) {
	r, err := ConvertOID("", model)
	if err != nil {
		return nil, fmt.Errorf(`数据格式ObjectID转换错误.%s`, err.Error())
	}
	return col.UpdateMany(ctx, conditions, r, options)
}

// ReplaceByID 根据ID及数据替换原有数据
// id:主键_id model:更新的数据
func (p *APIView) ReplaceByID(ctx context.Context, col *mongo.Collection, id string, model bson.M) (*mongo.UpdateResult, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}
	r, err := ConvertOID("", model)
	if err != nil {
		return nil, fmt.Errorf(`数据格式ObjectID转换错误.%s`, err.Error())
	}
	return col.ReplaceOne(ctx, bson.M{"_id": oid}, r)
}

// FindByID 根据ID查询数据
// id:主键_id result:查询结果
func (p *APIView) FindByID(ctx context.Context, col *mongo.Collection, result *bson.M, id string, calc CalcPipelineFunc) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	singleResult := col.FindOne(ctx, bson.M{"_id": oid})

	if singleResult.Err() != nil {
		return singleResult.Err()
	}
	if err := singleResult.Decode(result); err != nil {
		return err
	}

	if calc != nil {
		calc(result)
	}

	ConvertKeyID(result)
	return nil
}

func FindByID(ctx context.Context, col *mongo.Collection, result *bson.M, id string, calc CalcPipelineFunc) error {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	singleResult := col.FindOne(ctx, bson.M{"_id": oid})

	if singleResult.Err() != nil {
		return singleResult.Err()
	}
	if err := singleResult.Decode(result); err != nil {
		return err
	}

	if calc != nil {
		calc(result)
	}

	ConvertKeyID(result)
	return nil
}

// FindQueryConvertPipeline 转换查询到pipeline
func (p *APIView) FindQueryConvertPipeline(query bson.M) (mongo.Pipeline, mongo.Pipeline, error) {
	pipeLine := mongo.Pipeline{}
	var newPipeLine mongo.Pipeline
	if filter, ok := query["filter"]; ok {
		if filterMap, ok := filter.(bson.M); ok {
			for k, v := range filterMap {
				val, err := ConvertOID(k, v)
				if err != nil {
					return nil, nil, err
				}
				pipeLine = append(pipeLine, bson.D{bson.E{Key: k, Value: val}})
			}
		} else {
			return nil, nil, errors.New(`filter格式不正确`)
		}
	}

	if withCount, ok := query["withCount"]; ok {
		if b, ok := withCount.(bool); ok {
			if b {
				newPipeLine = DeepCopy(pipeLine).(mongo.Pipeline)
			}
		} else {
			return nil, nil, errors.New(`withCount格式不正确`)
		}
	}

	if sort, ok := query["sort"]; ok {
		if s, ok := sort.(bson.M); ok {
			if len(s) > 0 {
				pipeLine = append(pipeLine, bson.D{bson.E{Key: "$sort", Value: s}})
			}
		}
	}

	if skip, ok := query["skip"]; ok {
		pipeLine = append(pipeLine, bson.D{bson.E{Key: "$skip", Value: skip}})
	}

	if limit, ok := query["limit"]; ok {
		pipeLine = append(pipeLine, bson.D{bson.E{Key: "$limit", Value: limit}})
	}

	if project, ok := query["project"]; ok {
		pipeLine = append(pipeLine, bson.D{bson.E{Key: "$project", Value: project}})
	}
	return pipeLine, newPipeLine, nil
}

// FindFilter 根据条件查询数据(原版)
// result:查询结果 query:查询条件
func (p *APIView) FindFilter(ctx context.Context, col *mongo.Collection, result *[]bson.M, query bson.M, calc CalcPipelineFunc) (int, error) {

	var count int
	projectMap := bson.M{}

	pipeLine := mongo.Pipeline{}

	hasGroup := false

	//$group 放入 $lookups的$project后面
	if filter, ok := query["filter"]; ok {
		if filterMap, ok := filter.(bson.M); ok {
			groupMap := primitive.M{}
			hasGroupFields := false
			if groupFieldsMap, ok := query["groupFields"].(primitive.M); ok {
				groupMap = groupFieldsMap
				hasGroupFields = true
				for k := range groupFieldsMap {
					projectMap[k] = 1
				}
				delete(query, "groupFields")
			}
			if groupByMap, ok := query["groupBy"].(primitive.M); ok {
				groupMap["_id"] = groupByMap
				query["group"] = groupMap
				delete(query, "groupBy")
			} else if hasGroupFields {
				groupMap["_id"] = nil
				query["group"] = groupMap
				delete(query, "groupBy")
			}
			if groupMap, ok := query["group"].(primitive.M); ok {
				//groupMap["_id"] = groupMap["id"]
				delete(groupMap, "id")
				if lookups, ok := filterMap["$lookups"].(primitive.A); ok {
					lookupsOtherList := primitive.A{}
					if len(lookups) != 0 {
						if projectM, ok := lookups[0].(primitive.M)["$project"].(primitive.M); ok {
							for k := range projectMap {
								projectM[k] = 1
							}
							lookupsOtherList = append(lookupsOtherList, lookups[0])
							lookupsOtherList = append(lookupsOtherList, bson.M{"$group": groupMap})
							lookupsOtherList = append(lookupsOtherList, lookups[1:]...)
							lookups = lookupsOtherList
							filterMap["$lookups"] = lookups
						} else {
							lookupsOtherList = append(lookupsOtherList, bson.M{"$group": groupMap})
							lookupsOtherList = append(lookupsOtherList, lookups[1:]...)
							lookups = lookupsOtherList
							filterMap["$lookups"] = lookups
						}
					} else {
						lookups = primitive.A{groupMap}
					}
				} else {
					filterMap["$lookups"] = primitive.A{bson.M{"$project": projectMap}, groupMap}
				}
				delete(query, "group")
			}
			// }
		} else {
			return 0, errors.New(`filter格式不正确`)
		}
	}
	//if filter, ok := query["filter"]; ok {
	//	if filterMap, ok := filter.(bson.M); ok {
	//		if groupMap,ok := filterMap["$group"].(primitive.M);ok{
	//			if lookups,ok := filterMap["$lookups"].(primitive.A);ok{
	//				lookupsOtherList := primitive.A{}
	//				if len(lookups) != 0{
	//					if _, ok := lookups[0].(primitive.M)["$project"]; ok {
	//						lookupsOtherList = append(lookupsOtherList,lookups[0])
	//						lookupsOtherList = append(lookupsOtherList,bson.M{"$group":groupMap})
	//						lookupsOtherList = append(lookupsOtherList,lookups[1:]...)
	//						lookups = lookupsOtherList
	//						filterMap["$lookups"] = lookups
	//					}else{
	//						lookupsOtherList = append(lookupsOtherList,bson.M{"$group":groupMap})
	//						lookupsOtherList = append(lookupsOtherList,lookups[1:]...)
	//						lookups = lookupsOtherList
	//						filterMap["$lookups"] = lookups
	//					}
	//				}else{
	//					lookups = primitive.A{groupMap}
	//				}
	//			}else{
	//				filterMap["$lookups"] = primitive.A{groupMap}
	//			}
	//			delete(filterMap,"$group")
	//		}
	//		// }
	//	} else {
	//		return 0, errors.New(`filter格式不正确`)
	//	}
	//}

	if filter, ok := query["filter"]; ok {
		if filterMap, ok := filter.(bson.M); ok {
			for k, v := range filterMap {
				// 图形数据查询
				if k == "$lookups" {
					// 递归转换判断value值是否为ObjectID
					if lookups, ok := v.(primitive.A); ok {
						for _, lookup := range lookups {
							if _, ok := lookup.(primitive.M)["$group"]; ok {
								hasGroup = true
								break
								//pipeLine = append(pipeLine, bson.D{bson.E{Key: "$group", Value: lookup.(primitive.M)["$group"]}})
							} else if _, ok := lookup.(primitive.M)["$project"]; ok {
								if _, projectOk := lookup.(primitive.M)["$project"].(bson.M); !projectOk {
									return 0, fmt.Errorf("%s的关联查询时内部project格式错误，不是bson.M", k)
								}
								projectMap = lookup.(primitive.M)["$project"].(bson.M)
							} else {
								pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
							}
						}
					} else {
						return 0, errors.New(`$lookups的值数据格式不正确`)
					}
				}
			}
			// }
		} else {
			return 0, errors.New(`filter格式不正确`)
		}
	}

	if filter, ok := query["filter"]; ok {
		if filterMap, ok := filter.(bson.M); ok {
			for k, v := range filterMap {
				if k == "$lookup" {
					pipeLine = append(pipeLine, bson.D{bson.E{Key: k, Value: v}})
					// 特殊处理lookup数组
				} else if k != "$lookups" {
					//判断空对象
					if emptyObject, ok := v.(primitive.M); ok {
						flag := false
						for range emptyObject {
							flag = true
						}
						if !flag {
							delete(filterMap, k)
							continue
						}
					}
					if k == "id" {
						k = "_id"
					}
					// 递归转换判断value值是否为ObjectID
					r, err := ConvertOID(k, v)
					if err != nil {
						return 0, fmt.Errorf(`%s的值数据检验错误.%s`, k, err.Error())
					}
					//if strings.HasSuffix(k, "id") && k != "_id" && !strings.HasSuffix(k, "_id") && !strings.HasSuffix(k, "uid") {
					//	k = k[:len(k)-2]
					//	k = k + "_id"
					//} else
					if hasGroup && k == "modelId" {
						k = "model"
					} else {
						if strings.HasSuffix(k, "Id") && k != "requestId" {
							k = k[:len(k)-2]
							k = k + "._id"
						}
					}
					pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{k: r}}})
				}
			}
			// }
		} else {
			return 0, errors.New(`filter格式不正确`)
		}
	}

	if hasGroup {
		afterGroupFlag := false

		if filter, ok := query["filter"]; ok {
			if filterMap, ok := filter.(bson.M); ok {
				for k, v := range filterMap {
					// 图形数据查询
					if k == "$lookups" {
						// 递归转换判断value值是否为ObjectID
						if lookups, ok := v.(primitive.A); ok {
							for _, lookup := range lookups {
								if afterGroupFlag {
									pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
								}
								if _, ok := lookup.(primitive.M)["$group"]; ok {
									pipeLine = append(pipeLine, bson.D{bson.E{Key: "$group", Value: lookup.(primitive.M)["$group"]}})
									afterGroupFlag = true
								}
							}
						} else {
							return 0, errors.New(`$lookups的值数据格式不正确`)
						}
					}
				}
				// }
			} else {
				return 0, errors.New(`filter格式不正确`)
			}
		}
	}

	//if filter, ok := (*query)["filter"]; ok {
	//	if filterMap, ok := filter.(bson.M); ok {
	//		for k, v := range filterMap {
	//			// 图形数据查询
	//			if k == "ancestorId" {
	//				if idStr, ok := v.(string); ok {
	//					id, err := primitive.ObjectIDFromHex(idStr)
	//					if err != nil {
	//						return count, fmt.Errorf(`%s的ObjectID值格式不正确.%s`, k, err.Error())
	//					}
	//					pipeLine = append(pipeLine, bson.D{bson.E{Key: "$graphLookup", Value: bson.M{
	//						"from":             collection,
	//						"startWith":        "$parentId",
	//						"connectFromField": "parentId",
	//						"connectToField":   "_id",
	//						"as":               "parents",
	//					}}})
	//					pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{
	//						"parents": bson.D{
	//							bson.E{
	//								Key:   "$elemMatch",
	//								Value: bson.M{"_id": id},
	//							},
	//						},
	//					}}})
	//					pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: bson.M{
	//						"from":         "model",
	//						"localField":   "modelId",
	//						"foreignField": "_id",
	//						"as":           "model",
	//					}}})
	//				} else {
	//					return count, fmt.Errorf(`%s的ObjectID值格式不正确.非字符串`, k)
	//				}
	//				// 特殊处理lookup
	//			} else if k == "$lookup" {
	//				pipeLine = append(pipeLine, bson.D{bson.E{Key: k, Value: v}})
	//				// 特殊处理lookup数组
	//			} else if k == "$lookups" {
	//				// 递归转换判断value值是否为ObjectID
	//				if lookups, ok := v.(primitive.A); ok {
	//					for _, lookup := range lookups {
	//						if _, ok := lookup.(primitive.M)["$group"]; ok {
	//							pipeLine = append(pipeLine, bson.D{bson.E{Key: "$group", Value: lookup.(primitive.M)["$group"]}})
	//						} else if _, ok := lookup.(primitive.M)["$project"]; ok {
	//							if _, projectOk := lookup.(primitive.M)["$project"].(bson.M); !projectOk {
	//								return count, fmt.Errorf("%s的关联查询时内部project格式错误，不是bson.M", k)
	//							}
	//							projectMap = lookup.(primitive.M)["$project"].(bson.M)
	//						} else {
	//							pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
	//						}
	//					}
	//				} else {
	//					return count, errors.New(`$lookups的值数据格式不正确`)
	//				}
	//			} else {
	//				// 递归转换判断value值是否为ObjectID
	//				r, err := p.deepConvert(k, v)
	//				if err != nil {
	//					return count, fmt.Errorf(`%s的值数据检验错误.%s`, k, err.Error())
	//				}
	//
	//				pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{k: r}}})
	//			}
	//		}
	//		// }
	//	} else {
	//		return count, errors.New(`filter格式不正确`)
	//	}
	//}

	if withCount, ok := query["withCount"]; ok {
		if b, ok := withCount.(bool); ok {
			if b {
				newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
				c, err := p.FindCount(ctx, col, newPipeLine)
				if err != nil {
					return 0, err
				}
				count = c
			}
		} else {
			return count, errors.New(`withCount格式不正确`)
		}
	}

	if sort, ok := query["sort"]; ok {
		if s, ok := sort.(bson.M); ok {
			if len(s) > 0 {
				pipeLine = append(pipeLine, bson.D{bson.E{Key: "$sort", Value: s}})
			}
		}
	}else{
		pipeLine = append(pipeLine, bson.D{bson.E{Key: "$sort", Value: bson.M{"_id": 1}}})
	}

	if skip, ok := query["skip"]; ok {
		pipeLine = append(pipeLine, bson.D{bson.E{Key: "$skip", Value: skip}})
	}

	if limit, ok := query["limit"]; ok {
		pipeLine = append(pipeLine, bson.D{bson.E{Key: "$limit", Value: limit}})
	}

	if withoutBody, ok := query["withoutBody"]; ok {
		if b, ok := withoutBody.(bool); ok {
			if b {
				newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
				c, err := p.FindCount(ctx, col, newPipeLine)
				if err != nil {
					return 0, err
				}
				count = c
				return count, nil
			}
		} else {
			return count, errors.New(`withoutBody格式不正确`)
		}
	}

	if project, ok := query["project"]; ok {
		if projectList, ok := query["project"].(map[string]interface{}); ok {
			for k, v := range projectMap {
				projectList[k] = v
			}
		} else if projectList, ok := query["project"].(bson.M); ok {
			for k, v := range projectMap {
				projectList[k] = v
			}
		}
		pipeLine = append(pipeLine, bson.D{bson.E{Key: "$project", Value: project}})
	} else {
		flag := false
		for range projectMap {
			flag = true
		}
		if flag {
			pipeLine = append(pipeLine, bson.D{bson.E{Key: "$project", Value: projectMap}})
		}
	}

	//if project, ok := (*query)["project"]; ok {
	//	pipeLine = append(pipeLine, bson.D{bson.E{Key: "$project", Value: project}})
	//}
	//newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
	if err := p.FindPipeline(ctx, col, result, pipeLine, calc); err != nil {
		return 0, err
	}
	return count, nil
}

// FindFilter 根据条件查询数据(原版)
// result:查询结果 query:查询条件
func (p *APIView) FindFilterDeptQuery(ctx context.Context, col *mongo.Collection, result *[]bson.M, query bson.M, calc CalcPipelineFunc) (int, error) {

	var count int
	projectMap := bson.M{}

	pipeLine := mongo.Pipeline{}

	hasGroup := false

	//$group 放入 $lookups的$project后面
	if filter, ok := query["filter"]; ok {
		if filterMap, ok := filter.(bson.M); ok {
			groupMap := primitive.M{}
			hasGroupFields := false
			if groupFieldsMap, ok := query["groupFields"].(primitive.M); ok {
				groupMap = groupFieldsMap
				hasGroupFields = true
				for k := range groupFieldsMap {
					projectMap[k] = 1
				}
				delete(query, "groupFields")
			}
			if groupByMap, ok := query["groupBy"].(primitive.M); ok {
				groupMap["_id"] = groupByMap
				query["group"] = groupMap
				delete(query, "groupBy")
			} else if hasGroupFields {
				groupMap["_id"] = nil
				query["group"] = groupMap
				delete(query, "groupBy")
			}
			if groupMap, ok := query["group"].(primitive.M); ok {
				//groupMap["_id"] = groupMap["id"]
				delete(groupMap, "id")
				if lookups, ok := filterMap["$lookups"].(primitive.A); ok {
					lookupsOtherList := primitive.A{}
					if len(lookups) != 0 {
						if projectM, ok := lookups[0].(primitive.M)["$project"].(primitive.M); ok {
							for k := range projectMap {
								projectM[k] = 1
							}
							lookupsOtherList = append(lookupsOtherList, lookups[0])
							lookupsOtherList = append(lookupsOtherList, bson.M{"$group": groupMap})
							lookupsOtherList = append(lookupsOtherList, lookups[1:]...)
							lookups = lookupsOtherList
							filterMap["$lookups"] = lookups
						} else {
							lookupsOtherList = append(lookupsOtherList, bson.M{"$group": groupMap})
							lookupsOtherList = append(lookupsOtherList, lookups[1:]...)
							lookups = lookupsOtherList
							filterMap["$lookups"] = lookups
						}
					} else {
						lookups = primitive.A{groupMap}
					}
				} else {
					filterMap["$lookups"] = primitive.A{bson.M{"$project": projectMap}, groupMap}
				}
				delete(query, "group")
			}
			// }
		} else {
			return 0, errors.New(`filter格式不正确`)
		}
	}

	if filter, ok := query["filter"]; ok {
		if filterMap, ok := filter.(bson.M); ok {
			for k, v := range filterMap {
				// 图形数据查询
				if k == "$lookups" {
					// 递归转换判断value值是否为ObjectID
					if lookups, ok := v.(primitive.A); ok {
						for _, lookup := range lookups {
							if _, ok := lookup.(primitive.M)["$group"]; ok {
								hasGroup = true
								break
								//pipeLine = append(pipeLine, bson.D{bson.E{Key: "$group", Value: lookup.(primitive.M)["$group"]}})
							} else if _, ok := lookup.(primitive.M)["$project"]; ok {
								if _, projectOk := lookup.(primitive.M)["$project"].(bson.M); !projectOk {
									return 0, fmt.Errorf("%s的关联查询时内部project格式错误，不是bson.M", k)
								}
								projectMap = lookup.(primitive.M)["$project"].(bson.M)
							} else {
								pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
							}
						}
					} else {
						return 0, errors.New(`$lookups的值数据格式不正确`)
					}
				}
			}
			// }
		} else {
			return 0, errors.New(`filter格式不正确`)
		}
	}

	if filter, ok := query["filter"]; ok {
		if filterMap, ok := filter.(bson.M); ok {
			for k, v := range filterMap {
				if k == "$lookup" {
					pipeLine = append(pipeLine, bson.D{bson.E{Key: k, Value: v}})
					// 特殊处理lookup数组
				} else if k != "$lookups" {
					//判断空对象
					if emptyObject, ok := v.(primitive.M); ok {
						flag := false
						for range emptyObject {
							flag = true
						}
						if !flag {
							delete(filterMap, k)
							continue
						}
					}
					if k == "id" {
						k = "_id"
					}
					// 递归转换判断value值是否为ObjectID
					r, err := ConvertOID(k, v)
					if err != nil {
						return 0, fmt.Errorf(`%s的值数据检验错误.%s`, k, err.Error())
					}
					//if strings.HasSuffix(k, "id") && k != "_id" && !strings.HasSuffix(k, "_id") && !strings.HasSuffix(k, "uid") {
					//	k = k[:len(k)-2]
					//	k = k + "_id"
					//} else
					if hasGroup && k == "modelId" {
						k = "model"
					} else {
						if strings.HasSuffix(k, "Id") && k != "requestId" {
							k = k[:len(k)-2]
							k = k + "._id"
						}
					}
					pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{k: r}}})
				}
			}
			// }
		} else {
			return 0, errors.New(`filter格式不正确`)
		}
	}

	if hasGroup {
		afterGroupFlag := false

		if filter, ok := query["filter"]; ok {
			if filterMap, ok := filter.(bson.M); ok {
				for k, v := range filterMap {
					// 图形数据查询
					if k == "$lookups" {
						// 递归转换判断value值是否为ObjectID
						if lookups, ok := v.(primitive.A); ok {
							for _, lookup := range lookups {
								if afterGroupFlag {
									pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
								}
								if _, ok := lookup.(primitive.M)["$group"]; ok {
									pipeLine = append(pipeLine, bson.D{bson.E{Key: "$group", Value: lookup.(primitive.M)["$group"]}})
									afterGroupFlag = true
								}
							}
						} else {
							return 0, errors.New(`$lookups的值数据格式不正确`)
						}
					}
				}
				// }
			} else {
				return 0, errors.New(`filter格式不正确`)
			}
		}
	}

	//if filter, ok := (*query)["filter"]; ok {
	//	if filterMap, ok := filter.(bson.M); ok {
	//		for k, v := range filterMap {
	//			// 图形数据查询
	//			if k == "ancestorId" {
	//				if idStr, ok := v.(string); ok {
	//					id, err := primitive.ObjectIDFromHex(idStr)
	//					if err != nil {
	//						return count, fmt.Errorf(`%s的ObjectID值格式不正确.%s`, k, err.Error())
	//					}
	//					pipeLine = append(pipeLine, bson.D{bson.E{Key: "$graphLookup", Value: bson.M{
	//						"from":             collection,
	//						"startWith":        "$parentId",
	//						"connectFromField": "parentId",
	//						"connectToField":   "_id",
	//						"as":               "parents",
	//					}}})
	//					pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{
	//						"parents": bson.D{
	//							bson.E{
	//								Key:   "$elemMatch",
	//								Value: bson.M{"_id": id},
	//							},
	//						},
	//					}}})
	//					pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: bson.M{
	//						"from":         "model",
	//						"localField":   "modelId",
	//						"foreignField": "_id",
	//						"as":           "model",
	//					}}})
	//				} else {
	//					return count, fmt.Errorf(`%s的ObjectID值格式不正确.非字符串`, k)
	//				}
	//				// 特殊处理lookup
	//			} else if k == "$lookup" {
	//				pipeLine = append(pipeLine, bson.D{bson.E{Key: k, Value: v}})
	//				// 特殊处理lookup数组
	//			} else if k == "$lookups" {
	//				// 递归转换判断value值是否为ObjectID
	//				if lookups, ok := v.(primitive.A); ok {
	//					for _, lookup := range lookups {
	//						if _, ok := lookup.(primitive.M)["$group"]; ok {
	//							pipeLine = append(pipeLine, bson.D{bson.E{Key: "$group", Value: lookup.(primitive.M)["$group"]}})
	//						} else if _, ok := lookup.(primitive.M)["$project"]; ok {
	//							if _, projectOk := lookup.(primitive.M)["$project"].(bson.M); !projectOk {
	//								return count, fmt.Errorf("%s的关联查询时内部project格式错误，不是bson.M", k)
	//							}
	//							projectMap = lookup.(primitive.M)["$project"].(bson.M)
	//						} else {
	//							pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
	//						}
	//					}
	//				} else {
	//					return count, errors.New(`$lookups的值数据格式不正确`)
	//				}
	//			} else {
	//				// 递归转换判断value值是否为ObjectID
	//				r, err := p.deepConvert(k, v)
	//				if err != nil {
	//					return count, fmt.Errorf(`%s的值数据检验错误.%s`, k, err.Error())
	//				}
	//
	//				pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{k: r}}})
	//			}
	//		}
	//		// }
	//	} else {
	//		return count, errors.New(`filter格式不正确`)
	//	}
	//}
	hasRemoveFirst := false
	if withCount, ok := query["withCount"]; ok {
		if b, ok := withCount.(bool); ok {
			if b {
				newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
				if col.Name() == model.DEPT && !hasGroup && len(newPipeLine) > 1 {
					newPipeLine = newPipeLine[1:]
					hasRemoveFirst = true
				}
				c, err := p.FindCount(ctx, col, newPipeLine)
				if err != nil {
					return 0, err
				}
				count = c
			}
		} else {
			return count, errors.New(`withCount格式不正确`)
		}
	}

	if sort, ok := query["sort"]; ok {
		if s, ok := sort.(bson.M); ok {
			if len(s) > 0 {
				pipeLine = append(pipeLine, bson.D{bson.E{Key: "$sort", Value: s}})
			}
		}
	}

	if skip, ok := query["skip"]; ok {
		pipeLine = append(pipeLine, bson.D{bson.E{Key: "$skip", Value: skip}})
	}

	if limit, ok := query["limit"]; ok {
		pipeLine = append(pipeLine, bson.D{bson.E{Key: "$limit", Value: limit}})
	}

	if withoutBody, ok := query["withoutBody"]; ok {
		if b, ok := withoutBody.(bool); ok {
			if b {
				newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
				c, err := p.FindCount(ctx, col, newPipeLine)
				if err != nil {
					return 0, err
				}
				count = c
				return count, nil
			}
		} else {
			return count, errors.New(`withoutBody格式不正确`)
		}
	}

	if project, ok := query["project"]; ok {
		if projectList, ok := query["project"].(map[string]interface{}); ok {
			for k, v := range projectMap {
				projectList[k] = v
			}
		} else if projectList, ok := query["project"].(bson.M); ok {
			for k, v := range projectMap {
				projectList[k] = v
			}
		}
		pipeLine = append(pipeLine, bson.D{bson.E{Key: "$project", Value: project}})
	} else {
		flag := false
		for range projectMap {
			flag = true
		}
		if flag {
			pipeLine = append(pipeLine, bson.D{bson.E{Key: "$project", Value: projectMap}})
		}
	}

	//if project, ok := (*query)["project"]; ok {
	//	pipeLine = append(pipeLine, bson.D{bson.E{Key: "$project", Value: project}})
	//}
	//newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
	if hasRemoveFirst {
		pipeLine = pipeLine[1:]
		//pipeLine = append(pipeLine,newPipeLine[0])
	}
	if err := p.FindPipeline(ctx, col, result, pipeLine, calc); err != nil {
		return 0, err
	}
	return count, nil
}

// FindFilterLimit 根据条件查询数据(先根据条件分页)
// result:查询结果 query:查询条件
func (p *APIView) FindFilterLimit(ctx context.Context, col *mongo.Collection, result *[]bson.M, query bson.M, calc CalcPipelineFunc) (int, error) {

	var count int
	projectMap := bson.M{}

	pipeLine := mongo.Pipeline{}

	hasGroup := false

	//$group 放入 $lookups的$project后面
	if filter, ok := query["filter"]; ok {
		if filterMap, ok := filter.(bson.M); ok {
			groupMap := primitive.M{}
			hasGroupFields := false
			if groupFieldsMap, ok := query["groupFields"].(primitive.M); ok {
				groupMap = groupFieldsMap
				hasGroupFields = true
				for k := range groupFieldsMap {
					projectMap[k] = 1
				}
				delete(query, "groupFields")
			}
			if groupByMap, ok := query["groupBy"].(primitive.M); ok {
				groupMap["_id"] = groupByMap
				query["group"] = groupMap
				delete(query, "groupBy")
			} else if hasGroupFields {
				groupMap["_id"] = nil
				query["group"] = groupMap
				delete(query, "groupBy")
			}
			if groupMap, ok := query["group"].(primitive.M); ok {
				//groupMap["_id"] = groupMap["id"]
				delete(groupMap, "id")
				if lookups, ok := filterMap["$lookups"].(primitive.A); ok {
					lookupsOtherList := primitive.A{}
					if len(lookups) != 0 {
						if projectM, ok := lookups[0].(primitive.M)["$project"].(primitive.M); ok {
							for k := range projectMap {
								projectM[k] = 1
							}
							lookupsOtherList = append(lookupsOtherList, lookups[0])
							lookupsOtherList = append(lookupsOtherList, bson.M{"$group": groupMap})
							lookupsOtherList = append(lookupsOtherList, lookups[1:]...)
							lookups = lookupsOtherList
							filterMap["$lookups"] = lookups
						} else {
							lookupsOtherList = append(lookupsOtherList, bson.M{"$group": groupMap})
							lookupsOtherList = append(lookupsOtherList, lookups[1:]...)
							lookups = lookupsOtherList
							filterMap["$lookups"] = lookups
						}
					} else {
						lookups = primitive.A{groupMap}
					}
				} else {
					filterMap["$lookups"] = primitive.A{bson.M{"$project": projectMap}, groupMap}
				}
				delete(query, "group")
			}
			// }
		} else {
			return 0, errors.New(`filter格式不正确`)
		}
	}

	hasForeignKey, ok := query["hasForeignKey"].(bool)
	if ok {
		if hasForeignKey {
			queryLookupList := primitive.A{}
			hasForeignKey, ok := query["queryLookupList"].(primitive.A)
			if ok {
				queryLookupList = hasForeignKey
			}
			if sortFirst, ok := query["sortFirst"].(bool); ok {
				if sortFirst {
					if filter, ok := query["filter"]; ok {
						if filterMap, ok := filter.(bson.M); ok {
							for k, v := range filterMap {
								if k == "$lookup" {
									pipeLine = append(pipeLine, bson.D{bson.E{Key: k, Value: v}})
									// 特殊处理lookup数组
								} else if k != "$lookups" {
									//判断空对象
									if emptyObject, ok := v.(primitive.M); ok {
										flag := false
										for range emptyObject {
											flag = true
										}
										if !flag {
											delete(filterMap, k)
											continue
										}
									}
									if k == "id" {
										k = "_id"
									}
									// 递归转换判断value值是否为ObjectID
									r, err := ConvertOID(k, v)
									if err != nil {
										return 0, fmt.Errorf(`%s的值数据检验错误.%s`, k, err.Error())
									}
									//if strings.HasSuffix(k, "id") && k != "_id" && !strings.HasSuffix(k, "_id") && !strings.HasSuffix(k, "uid") {
									//	k = k[:len(k)-2]
									//	k = k + "_id"
									//} else
									if hasGroup && k == "modelId" {
										k = "model"
									} else {
										if strings.HasSuffix(k, "Id") && k != "requestId" {
											k = k[:len(k)-2]
											k = k + "._id"
										}
									}
									pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{k: r}}})
								}
							}
							// }
						} else {
							return 0, errors.New(`filter格式不正确`)
						}
					}
					for _, lookup := range queryLookupList {
						pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
					}
				} else {
					for _, lookup := range queryLookupList {
						pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
					}
					if filter, ok := query["filter"]; ok {
						if filterMap, ok := filter.(bson.M); ok {
							for k, v := range filterMap {
								if k == "$lookup" {
									pipeLine = append(pipeLine, bson.D{bson.E{Key: k, Value: v}})
									// 特殊处理lookup数组
								} else if k != "$lookups" {
									//判断空对象
									if emptyObject, ok := v.(primitive.M); ok {
										flag := false
										for range emptyObject {
											flag = true
										}
										if !flag {
											delete(filterMap, k)
											continue
										}
									}
									if k == "id" {
										k = "_id"
									}
									// 递归转换判断value值是否为ObjectID
									r, err := ConvertOID(k, v)
									if err != nil {
										return 0, fmt.Errorf(`%s的值数据检验错误.%s`, k, err.Error())
									}
									//if strings.HasSuffix(k, "id") && k != "_id" && !strings.HasSuffix(k, "_id") && !strings.HasSuffix(k, "uid") {
									//	k = k[:len(k)-2]
									//	k = k + "_id"
									//} else
									if hasGroup && k == "modelId" {
										k = "model"
									} else {
										if strings.HasSuffix(k, "Id") && k != "requestId" {
											k = k[:len(k)-2]
											k = k + "._id"
										}
									}
									pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{k: r}}})
								}
							}
							// }
						} else {
							return 0, errors.New(`filter格式不正确`)
						}
					}
				}
			}else{
				for _, lookup := range queryLookupList {
					pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
				}
				if filter, ok := query["filter"]; ok {
					if filterMap, ok := filter.(bson.M); ok {
						for k, v := range filterMap {
							if k == "$lookup" {
								pipeLine = append(pipeLine, bson.D{bson.E{Key: k, Value: v}})
								// 特殊处理lookup数组
							} else if k != "$lookups" {
								//判断空对象
								if emptyObject, ok := v.(primitive.M); ok {
									flag := false
									for range emptyObject {
										flag = true
									}
									if !flag {
										delete(filterMap, k)
										continue
									}
								}
								if k == "id" {
									k = "_id"
								}
								// 递归转换判断value值是否为ObjectID
								r, err := ConvertOID(k, v)
								if err != nil {
									return 0, fmt.Errorf(`%s的值数据检验错误.%s`, k, err.Error())
								}
								//if strings.HasSuffix(k, "id") && k != "_id" && !strings.HasSuffix(k, "_id") && !strings.HasSuffix(k, "uid") {
								//	k = k[:len(k)-2]
								//	k = k + "_id"
								//} else
								if hasGroup && k == "modelId" {
									k = "model"
								} else {
									if strings.HasSuffix(k, "Id") && k != "requestId" {
										k = k[:len(k)-2]
										k = k + "._id"
									}
								}
								pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{k: r}}})
							}
						}
						// }
					} else {
						return 0, errors.New(`filter格式不正确`)
					}
				}
			}
			//for _, lookup := range queryLookupList {
			//	pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
			//}
			//if filter, ok := query["filter"]; ok {
			//	if filterMap, ok := filter.(bson.M); ok {
			//		for k, v := range filterMap {
			//			if k == "$lookup" {
			//				pipeLine = append(pipeLine, bson.D{bson.E{Key: k, Value: v}})
			//				// 特殊处理lookup数组
			//			} else if k != "$lookups" {
			//				//判断空对象
			//				if emptyObject, ok := v.(primitive.M); ok {
			//					flag := false
			//					for range emptyObject {
			//						flag = true
			//					}
			//					if !flag {
			//						delete(filterMap, k)
			//						continue
			//					}
			//				}
			//				if k == "id" {
			//					k = "_id"
			//				}
			//				// 递归转换判断value值是否为ObjectID
			//				r, err := ConvertOID(k, v)
			//				if err != nil {
			//					return 0, fmt.Errorf(`%s的值数据检验错误.%s`, k, err.Error())
			//				}
			//				//if strings.HasSuffix(k, "id") && k != "_id" && !strings.HasSuffix(k, "_id") && !strings.HasSuffix(k, "uid") {
			//				//	k = k[:len(k)-2]
			//				//	k = k + "_id"
			//				//} else
			//				if hasGroup && k == "modelId" {
			//					k = "model"
			//				} else {
			//					if strings.HasSuffix(k, "Id") && k != "requestId" {
			//						k = k[:len(k)-2]
			//						k = k + "._id"
			//					}
			//				}
			//				pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{k: r}}})
			//			}
			//		}
			//		// }
			//	} else {
			//		return 0, errors.New(`filter格式不正确`)
			//	}
			//}
			if withoutBody, ok := query["withoutBody"]; ok {
				if b, ok := withoutBody.(bool); ok {
					if b {
						newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
						c, err := p.FindCount(ctx, col, newPipeLine)
						if err != nil {
							return 0, err
						}
						count = c
						return count, nil
					}
				} else {
					return count, errors.New(`withoutBody格式不正确`)
				}
			}
			if filter, ok := query["filter"]; ok {
				if filterMap, ok := filter.(bson.M); ok {
					for k, v := range filterMap {
						// 图形数据查询
						if k == "$lookups" {
							// 递归转换判断value值是否为ObjectID
							if lookups, ok := v.(primitive.A); ok {
								for _, lookup := range lookups {
									if _, ok := lookup.(primitive.M)["$group"]; ok {
										hasGroup = true
										break
										//pipeLine = append(pipeLine, bson.D{bson.E{Key: "$group", Value: lookup.(primitive.M)["$group"]}})
									} else if _, ok := lookup.(primitive.M)["$project"]; ok {
										if _, projectOk := lookup.(primitive.M)["$project"].(bson.M); !projectOk {
											return 0, fmt.Errorf("%s的关联查询时内部project格式错误，不是bson.M", k)
										}
										projectMap = lookup.(primitive.M)["$project"].(bson.M)
									} else {
										pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
									}
								}
							} else {
								return 0, errors.New(`$lookups的值数据格式不正确`)
							}
						}
					}
					// }
				} else {
					return 0, errors.New(`filter格式不正确`)
				}
			}
			if hasGroup {
				afterGroupFlag := false

				if filter, ok := query["filter"]; ok {
					if filterMap, ok := filter.(bson.M); ok {
						for k, v := range filterMap {
							// 图形数据查询
							if k == "$lookups" {
								// 递归转换判断value值是否为ObjectID
								if lookups, ok := v.(primitive.A); ok {
									for _, lookup := range lookups {
										if afterGroupFlag {
											pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
										}
										if _, ok := lookup.(primitive.M)["$group"]; ok {
											pipeLine = append(pipeLine, bson.D{bson.E{Key: "$group", Value: lookup.(primitive.M)["$group"]}})
											afterGroupFlag = true
										}
									}
								} else {
									return 0, errors.New(`$lookups的值数据格式不正确`)
								}
							}
						}
						// }
					} else {
						return 0, errors.New(`filter格式不正确`)
					}
				}
			}

			if withCount, ok := query["withCount"]; ok {
				if b, ok := withCount.(bool); ok {
					if b {
						newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
						c, err := p.FindCount(ctx, col, newPipeLine)
						if err != nil {
							return 0, err
						}
						count = c
					}
				} else {
					return count, errors.New(`withCount格式不正确`)
				}
			}
			if sort, ok := query["sort"]; ok {
				if s, ok := sort.(bson.M); ok {
					if len(s) > 0 {
						pipeLine = append(pipeLine, bson.D{bson.E{Key: "$sort", Value: s}})
					}
				}
			}else{
				pipeLine = append(pipeLine, bson.D{bson.E{Key: "$sort", Value: bson.M{"_id": 1}}})
			}

			if skip, ok := query["skip"]; ok {
				pipeLine = append(pipeLine, bson.D{bson.E{Key: "$skip", Value: skip}})
			}

			if limit, ok := query["limit"]; ok {
				pipeLine = append(pipeLine, bson.D{bson.E{Key: "$limit", Value: limit}})
			}
			if project, ok := query["project"]; ok {
				if projectList, ok := query["project"].(map[string]interface{}); ok {
					for k, v := range projectMap {
						projectList[k] = v
					}
				} else if projectList, ok := query["project"].(bson.M); ok {
					for k, v := range projectMap {
						projectList[k] = v
					}
				}
				pipeLine = append(pipeLine, bson.D{bson.E{Key: "$project", Value: project}})
			} else {
				flag := false
				for range projectMap {
					flag = true
				}
				if flag {
					pipeLine = append(pipeLine, bson.D{bson.E{Key: "$project", Value: projectMap}})
				}
			}
		} else {
			if filter, ok := query["filter"]; ok {
				if filterMap, ok := filter.(bson.M); ok {
					for k, v := range filterMap {
						if k == "$lookup" {
							pipeLine = append(pipeLine, bson.D{bson.E{Key: k, Value: v}})
							// 特殊处理lookup数组
						} else if k != "$lookups" {
							//判断空对象
							if emptyObject, ok := v.(primitive.M); ok {
								flag := false
								for range emptyObject {
									flag = true
								}
								if !flag {
									delete(filterMap, k)
									continue
								}
							}
							if k == "id" {
								k = "_id"
							}
							// 递归转换判断value值是否为ObjectID
							r, err := ConvertOID(k, v)
							if err != nil {
								return 0, fmt.Errorf(`%s的值数据检验错误.%s`, k, err.Error())
							}
							//if strings.HasSuffix(k, "id") && k != "_id" && !strings.HasSuffix(k, "_id") && !strings.HasSuffix(k, "uid") {
							//	k = k[:len(k)-2]
							//	k = k + "_id"
							//} else
							if hasGroup && k == "modelId" {
								k = "model"
							} else {
								if strings.HasSuffix(k, "Id") && k != "requestId" {
									k = k[:len(k)-2]
									k = k + "._id"
								}
							}
							pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{k: r}}})
						}
					}
					// }
				} else {
					return 0, errors.New(`filter格式不正确`)
				}
			}
			if withoutBody, ok := query["withoutBody"]; ok {
				if b, ok := withoutBody.(bool); ok {
					if b {
						newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
						c, err := p.FindCount(ctx, col, newPipeLine)
						if err != nil {
							return 0, err
						}
						count = c
						return count, nil
					}
				} else {
					return count, errors.New(`withoutBody格式不正确`)
				}
			}

			if filter, ok := query["filter"]; ok {
				if filterMap, ok := filter.(bson.M); ok {
					for k, v := range filterMap {
						// 图形数据查询
						if k == "$lookups" {
							// 递归转换判断value值是否为ObjectID
							if lookups, ok := v.(primitive.A); ok {
								for _, lookup := range lookups {
									if _, ok := lookup.(primitive.M)["$group"]; ok {
										hasGroup = true
										break
										//pipeLine = append(pipeLine, bson.D{bson.E{Key: "$group", Value: lookup.(primitive.M)["$group"]}})
									} else if _, ok := lookup.(primitive.M)["$project"]; ok {
										if _, projectOk := lookup.(primitive.M)["$project"].(bson.M); !projectOk {
											return 0, fmt.Errorf("%s的关联查询时内部project格式错误，不是bson.M", k)
										}
										projectMap = lookup.(primitive.M)["$project"].(bson.M)
									} else {
										pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
									}
								}
							} else {
								return 0, errors.New(`$lookups的值数据格式不正确`)
							}
						}
					}
					// }
				} else {
					return 0, errors.New(`filter格式不正确`)
				}
			}
			if hasGroup {
				afterGroupFlag := false

				if filter, ok := query["filter"]; ok {
					if filterMap, ok := filter.(bson.M); ok {
						for k, v := range filterMap {
							// 图形数据查询
							if k == "$lookups" {
								// 递归转换判断value值是否为ObjectID
								if lookups, ok := v.(primitive.A); ok {
									for _, lookup := range lookups {
										if afterGroupFlag {
											pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
										}
										if _, ok := lookup.(primitive.M)["$group"]; ok {
											pipeLine = append(pipeLine, bson.D{bson.E{Key: "$group", Value: lookup.(primitive.M)["$group"]}})
											afterGroupFlag = true
										}
									}
								} else {
									return 0, errors.New(`$lookups的值数据格式不正确`)
								}
							}
						}
						// }
					} else {
						return 0, errors.New(`filter格式不正确`)
					}
				}
			}

			if withCount, ok := query["withCount"]; ok {
				if b, ok := withCount.(bool); ok {
					if b {
						newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
						c, err := p.FindCount(ctx, col, newPipeLine)
						if err != nil {
							return 0, err
						}
						count = c
					}
				} else {
					return count, errors.New(`withCount格式不正确`)
				}
			}

			if sort, ok := query["sort"]; ok {
				if s, ok := sort.(bson.M); ok {
					if len(s) > 0 {
						pipeLine = append(pipeLine, bson.D{bson.E{Key: "$sort", Value: s}})
					}
				}
			}else{
				pipeLine = append(pipeLine, bson.D{bson.E{Key: "$sort", Value: bson.M{"_id": 1}}})
			}

			if skip, ok := query["skip"]; ok {
				pipeLine = append(pipeLine, bson.D{bson.E{Key: "$skip", Value: skip}})
			}

			if limit, ok := query["limit"]; ok {
				pipeLine = append(pipeLine, bson.D{bson.E{Key: "$limit", Value: limit}})
			}
			if project, ok := query["project"]; ok {
				if projectList, ok := query["project"].(map[string]interface{}); ok {
					for k, v := range projectMap {
						projectList[k] = v
					}
				} else if projectList, ok := query["project"].(bson.M); ok {
					for k, v := range projectMap {
						projectList[k] = v
					}
				}
				pipeLine = append(pipeLine, bson.D{bson.E{Key: "$project", Value: project}})
			} else {
				flag := false
				for range projectMap {
					flag = true
				}
				if flag {
					pipeLine = append(pipeLine, bson.D{bson.E{Key: "$project", Value: projectMap}})
				}
			}
		}
	} else {
		if filter, ok := query["filter"]; ok {
			if filterMap, ok := filter.(bson.M); ok {
				for k, v := range filterMap {
					if k == "$lookup" {
						pipeLine = append(pipeLine, bson.D{bson.E{Key: k, Value: v}})
						// 特殊处理lookup数组
					} else if k != "$lookups" {
						//判断空对象
						if emptyObject, ok := v.(primitive.M); ok {
							flag := false
							for range emptyObject {
								flag = true
							}
							if !flag {
								delete(filterMap, k)
								continue
							}
						}
						if k == "id" {
							k = "_id"
						}
						// 递归转换判断value值是否为ObjectID
						r, err := ConvertOID(k, v)
						if err != nil {
							return 0, fmt.Errorf(`%s的值数据检验错误.%s`, k, err.Error())
						}
						//if strings.HasSuffix(k, "id") && k != "_id" && !strings.HasSuffix(k, "_id") && !strings.HasSuffix(k, "uid") {
						//	k = k[:len(k)-2]
						//	k = k + "_id"
						//} else
						if hasGroup && k == "modelId" {
							k = "model"
						} else {
							if strings.HasSuffix(k, "Id") && k != "requestId" {
								k = k[:len(k)-2]
								k = k + "._id"
							}
						}
						pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{k: r}}})
					}
				}
				// }
			} else {
				return 0, errors.New(`filter格式不正确`)
			}
		}


		if withoutBody, ok := query["withoutBody"]; ok {
			if b, ok := withoutBody.(bool); ok {
				if b {
					newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
					c, err := p.FindCount(ctx, col, newPipeLine)
					if err != nil {
						return 0, err
					}
					count = c
					return count, nil
				}
			} else {
				return count, errors.New(`withoutBody格式不正确`)
			}
		}

		if filter, ok := query["filter"]; ok {
			if filterMap, ok := filter.(bson.M); ok {
				for k, v := range filterMap {
					// 图形数据查询
					if k == "$lookups" {
						// 递归转换判断value值是否为ObjectID
						if lookups, ok := v.(primitive.A); ok {
							for _, lookup := range lookups {
								if _, ok := lookup.(primitive.M)["$group"]; ok {
									hasGroup = true
									break
									//pipeLine = append(pipeLine, bson.D{bson.E{Key: "$group", Value: lookup.(primitive.M)["$group"]}})
								}
							}
						} else {
							return 0, errors.New(`$lookups的值数据格式不正确`)
						}
					}
				}
				// }
			} else {
				return 0, errors.New(`filter格式不正确`)
			}
		}
		if hasGroup {
			afterGroupFlag := false

			if filter, ok := query["filter"]; ok {
				if filterMap, ok := filter.(bson.M); ok {
					for k, v := range filterMap {
						// 图形数据查询
						if k == "$lookups" {
							// 递归转换判断value值是否为ObjectID
							if lookups, ok := v.(primitive.A); ok {
								for _, lookup := range lookups {
									if afterGroupFlag {
										pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
									}
									if _, ok := lookup.(primitive.M)["$group"]; ok {
										pipeLine = append(pipeLine, bson.D{bson.E{Key: "$group", Value: lookup.(primitive.M)["$group"]}})
										afterGroupFlag = true
									}
								}
							} else {
								return 0, errors.New(`$lookups的值数据格式不正确`)
							}
						}
					}
					// }
				} else {
					return 0, errors.New(`filter格式不正确`)
				}
			}
		}
		if withCount, ok := query["withCount"]; ok {
			if b, ok := withCount.(bool); ok {
				if b {
					newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
					c, err := p.FindCount(ctx, col, newPipeLine)
					if err != nil {
						return 0, err
					}
					count = c
				}
			} else {
				return count, errors.New(`withCount格式不正确`)
			}
		}
		if sort, ok := query["sort"]; ok {
			if s, ok := sort.(bson.M); ok {
				if len(s) > 0 {
					pipeLine = append(pipeLine, bson.D{bson.E{Key: "$sort", Value: s}})
				}
			}
		}else{
			pipeLine = append(pipeLine, bson.D{bson.E{Key: "$sort", Value: bson.M{"_id": 1}}})
		}

		if skip, ok := query["skip"]; ok {
			pipeLine = append(pipeLine, bson.D{bson.E{Key: "$skip", Value: skip}})
		}

		if limit, ok := query["limit"]; ok {
			pipeLine = append(pipeLine, bson.D{bson.E{Key: "$limit", Value: limit}})
		}

		if filter, ok := query["filter"]; ok {
			if filterMap, ok := filter.(bson.M); ok {
				for k, v := range filterMap {
					// 图形数据查询
					if k == "$lookups" {
						// 递归转换判断value值是否为ObjectID
						if lookups, ok := v.(primitive.A); ok {
							for _, lookup := range lookups {
								if _, ok := lookup.(primitive.M)["$project"]; ok {
									if _, projectOk := lookup.(primitive.M)["$project"].(bson.M); !projectOk {
										return 0, fmt.Errorf("%s的关联查询时内部project格式错误，不是bson.M", k)
									}
									projectMap = lookup.(primitive.M)["$project"].(bson.M)
								} else {
									pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
								}
							}
						} else {
							return 0, errors.New(`$lookups的值数据格式不正确`)
						}
					}
				}
				// }
			} else {
				return 0, errors.New(`filter格式不正确`)
			}
		}

		if project, ok := query["project"]; ok {
			if projectList, ok := query["project"].(map[string]interface{}); ok {
				for k, v := range projectMap {
					projectList[k] = v
				}
			} else if projectList, ok := query["project"].(bson.M); ok {
				for k, v := range projectMap {
					projectList[k] = v
				}
			}
			pipeLine = append(pipeLine, bson.D{bson.E{Key: "$project", Value: project}})
		} else {
			flag := false
			for range projectMap {
				flag = true
			}
			if flag {
				pipeLine = append(pipeLine, bson.D{bson.E{Key: "$project", Value: projectMap}})
			}
		}
	}

	//if filter, ok := query["filter"]; ok {
	//	if filterMap, ok := filter.(bson.M); ok {
	//		for k, v := range filterMap {
	//			// 图形数据查询
	//			if k == "$lookups" {
	//				// 递归转换判断value值是否为ObjectID
	//				if lookups, ok := v.(primitive.A); ok {
	//					for _, lookup := range lookups {
	//						if _, ok := lookup.(primitive.M)["$group"]; ok {
	//							hasGroup = true
	//							break
	//							//pipeLine = append(pipeLine, bson.D{bson.E{Key: "$group", Value: lookup.(primitive.M)["$group"]}})
	//						} else
	//						if _, ok := lookup.(primitive.M)["$project"]; ok {
	//							if _, projectOk := lookup.(primitive.M)["$project"].(bson.M); !projectOk {
	//								return 0, fmt.Errorf("%s的关联查询时内部project格式错误，不是bson.M", k)
	//							}
	//							projectMap = lookup.(primitive.M)["$project"].(bson.M)
	//						} else {
	//							pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
	//						}
	//					}
	//				} else {
	//					return 0, errors.New(`$lookups的值数据格式不正确`)
	//				}
	//			}
	//		}
	//		// }
	//	} else {
	//		return 0, errors.New(`filter格式不正确`)
	//	}
	//}
	//
	//if filter, ok := query["filter"]; ok {
	//	if filterMap, ok := filter.(bson.M); ok {
	//		for k, v := range filterMap {
	//			if k == "$lookup" {
	//				pipeLine = append(pipeLine, bson.D{bson.E{Key: k, Value: v}})
	//				// 特殊处理lookup数组
	//			} else if k != "$lookups" {
	//				//判断空对象
	//				if emptyObject, ok := v.(primitive.M); ok {
	//					flag := false
	//					for range emptyObject {
	//						flag = true
	//					}
	//					if !flag {
	//						delete(filterMap, k)
	//						continue
	//					}
	//				}
	//				if k == "id" {
	//					k = "_id"
	//				}
	//				// 递归转换判断value值是否为ObjectID
	//				r, err := ConvertOID(k, v)
	//				if err != nil {
	//					return 0, fmt.Errorf(`%s的值数据检验错误.%s`, k, err.Error())
	//				}
	//				//if strings.HasSuffix(k, "id") && k != "_id" && !strings.HasSuffix(k, "_id") && !strings.HasSuffix(k, "uid") {
	//				//	k = k[:len(k)-2]
	//				//	k = k + "_id"
	//				//} else
	//				if hasGroup && k == "modelId" {
	//					k = "model"
	//				} else {
	//					if strings.HasSuffix(k, "Id") && k != "requestId" {
	//						k = k[:len(k)-2]
	//						k = k + "._id"
	//					}
	//				}
	//				pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{k: r}}})
	//			}
	//		}
	//		// }
	//	} else {
	//		return 0, errors.New(`filter格式不正确`)
	//	}
	//}

	//if project, ok := (*query)["project"]; ok {
	//	pipeLine = append(pipeLine, bson.D{bson.E{Key: "$project", Value: project}})
	//}
	if err := p.FindPipeline(ctx, col, result, pipeLine, calc); err != nil {
		return 0, err
	}
	return count, nil
}

// FindPipeline 根据Pipeline查询数据
// result:查询结果 pipeLine:查询管道
func (p *APIView) FindPipeline(ctx context.Context, col *mongo.Collection, result *[]bson.M, pipeLine mongo.Pipeline, calc CalcPipelineFunc) error {
	//增加允许使用硬盘缓存
	AllowDiskUse := true
	cur, err := col.Aggregate(ctx, pipeLine,
		&options.AggregateOptions{AllowDiskUse: &AllowDiskUse},
	)
	if err != nil {
		return err
	}

	for cur.Next(ctx) {
		var r bson.M
		err := cur.Decode(&r)
		if err != nil {
			return err
		}
		if calc != nil {
			calc(&r)
		}
		ConvertKeyID(&r)
		*result = append(*result, r)
	}

	return nil
}

func FindPipeline(ctx context.Context, col *mongo.Collection, result *[]bson.M, pipeLine mongo.Pipeline, calc CalcPipelineFunc) error {
	//增加允许使用硬盘缓存
	AllowDiskUse := true
	cur, err := col.Aggregate(ctx, pipeLine,
		&options.AggregateOptions{AllowDiskUse: &AllowDiskUse},
	)
	if err != nil {
		return err
	}

	for cur.Next(ctx) {
		var r bson.M
		err := cur.Decode(&r)
		if err != nil {
			return err
		}
		if calc != nil {
			calc(&r)
		}
		ConvertKeyID(&r)
		*result = append(*result, r)
	}

	return nil
}

// FindPipelineFunc 根据Pipeline函数查询数据
// result:查询结果 pipeLine:查询管道函数
func (p *APIView) FindPipelineFunc(ctx context.Context, col *mongo.Collection, result *[]bson.M, pipeLine PipelineFunc, calc CalcPipelineFunc) error {
	cur, err := col.Aggregate(ctx, pipeLine)
	if err != nil {
		return err
	}

	for cur.Next(ctx) {
		var r bson.M
		err := cur.Decode(&r)
		if err != nil {
			return err
		}
		calc(&r)
		ConvertKeyID(&r)
		*result = append(*result, r)
	}

	return nil
}

// FindCount 根据Pipeline查询数据量
// pipeLine:查询管道
func (p *APIView) FindCount(ctx context.Context, col *mongo.Collection, pipeLine mongo.Pipeline) (int, error) {
	pipeLine = append(pipeLine, bson.D{bson.E{Key: "$count", Value: "count"}})
	AllowDiskUse := true
	cur, err := col.Aggregate(ctx, pipeLine,
		&options.AggregateOptions{AllowDiskUse: &AllowDiskUse},
	)
	if err != nil {
		return 0, err
	}
	var count int
	for cur.Next(ctx) {
		var r bson.M
		err := cur.Decode(&r)
		if err != nil {
			return 0, err
		}
		if c, ok := r["count"]; ok {
			if c1, ok := c.(int32); ok {
				count = int(c1)
			}
			break
		}
	}

	return count, nil
}

// FindCount 根据Pipeline查询数据量
// pipeLine:查询管道
func FindCount(ctx context.Context, col *mongo.Collection, pipeLine mongo.Pipeline) (int, error) {
	pipeLine = append(pipeLine, bson.D{bson.E{Key: "$count", Value: "count"}})
	cur, err := col.Aggregate(ctx, pipeLine)
	if err != nil {
		return 0, err
	}
	var count int
	for cur.Next(ctx) {
		var r bson.M
		err := cur.Decode(&r)
		if err != nil {
			return 0, err
		}
		if c, ok := r["count"]; ok {
			if c1, ok := c.(int32); ok {
				count = int(c1)
			}
			break
		}
	}

	return count, nil
}

//============Redis==================

// SaveRedis 存储Redis值
func (p *APIView) SaveRedis(key, value string, t int) error {
	if iredis.ClusterBool {
		return iredis.ClusterClient.Set(context.Background(), key, value, time.Duration(t)).Err()
	} else {
		return iredis.Client.Set(context.Background(), key, value, time.Duration(t)).Err()
	}
}

// SaveRedisByPath 使用jsonpath的方式存储Redis值
func (p *APIView) SaveRedisByPath(key string, data map[string]interface{}, t int) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	valStr, err := p.FindByRedisKey(key)
	if err != nil {
		return err
	}
	valMap := map[string]interface{}{}
	err = json.Unmarshal([]byte(valStr), &valMap)
	if err != nil {
		return fmt.Errorf("redis整体数据解序列化失败：%s", err)
	}

	for key, value := range data {
		err = jsonpath.BaseSet(valMap, key, ".", value)
		if err != nil {
			return err
		}
	}

	err = p.SaveRedisMap(key, valMap, t)
	if err != nil {
		return fmt.Errorf("redis整体数据保存失败：%s", err)
	}
	return nil
}

// SaveRedisMap 存储Redis值(存储map)
func (p *APIView) SaveRedisMap(key string, data map[string]interface{}, t int) error {
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return p.SaveRedis(key, string(b), t)
}

// DeleteByRedisKey 删除Redis的Key
func (p *APIView) DeleteByRedisKey(keys ...string) (err error) {
	if iredis.ClusterBool {
		return iredis.ClusterClient.Del(context.Background(), keys...).Err()
	} else {
		return iredis.Client.Del(context.Background(), keys...).Err()
	}
}

// FindByRedisKey 使用Key获取Redis值
func (p *APIView) FindByRedisKey(key string) (string, error) {
	var cmd *redis.StringCmd
	if iredis.ClusterBool {
		cmd = iredis.ClusterClient.Get(context.Background(), key)
	} else {
		cmd = iredis.Client.Get(context.Background(), key)
	}
	if cmd.Err() != nil {
		return "", cmd.Err()
	}
	return cmd.Val(), nil
}

// FindLockByRedisKey 使用Key获取Redis值（带锁）
func (p *APIView) FindLockByRedisKey(key string) (string, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	var cmd *redis.StringCmd
	if iredis.ClusterBool {
		cmd = iredis.ClusterClient.Get(context.Background(), key)
	} else {
		cmd = iredis.Client.Get(context.Background(), key)
	}
	if cmd.Err() != nil {
		return "", cmd.Err()
	}
	return cmd.Val(), nil
}

//============InfluxDB==================

// FindInfluxBySQL convenience function to query the database
func (p *APIView) FindInfluxBySQL(cli client.Client, database, cmd string) (res []client.Result, err error) {
	q := client.Query{
		Command:  cmd,
		Database: database,
	}

	if response, err := cli.Query(q); err == nil {
		if response.Error() != nil {
			return res, response.Error()
		}
		res = response.Results
	} else {
		return res, err
	}
	return res, nil
}

// InfluxResultConvertMap InfluxDB的查询结果[]api.Result转换成map[string]interface{}
func (p *APIView) InfluxResultConvertMap(results []client.Result) (map[string]interface{}, error) {
	newResultMap := map[string]interface{}{}
	newResults := make([]map[string]interface{}, 0)
	for _, v := range results {
		if v.Err == "" {
			r := map[string]interface{}{}

			for _, row := range v.Series {
				timeIndex := 0
				hasTime := false
				for i, col := range row.Columns {
					if col == "time" {
						timeIndex = i
						hasTime = true
						break
					}
				}
				if hasTime {
					for _, list := range row.Values {
						for j, ele := range list {
							if j == timeIndex {
								if _, ok := ele.(string); !ok {
									if timeInt, ok := ele.(int64); ok {
										queryTime := time.Unix(0, int64(timeInt)*int64(time.Millisecond))
										list[j] = queryTime.Format("2006-01-02T15:04:05.000+08:00")
									}
								}
							}
						}
					}
				}
			}
			r["series"] = v.Series
			newResults = append(newResults, r)
		} else {
			continue
		}
	}

	newResultMap["results"] = newResults

	return newResultMap, nil

}

func ChangeTimeIntToString() {

}

// influxResultConvert InfluxDB的查询结果[]api.Result转换成[][]map[string]interface{}
func (p *APIView) InfluxResultConvert(results []client.Result) ([][]map[string]interface{}, error) {
	newResults := make([][]map[string]interface{}, 0)
	for _, v := range results {
		if v.Err == "" {
			rList := make([]map[string]interface{}, 0)
			for _, v1 := range v.Series {
				for _, v2 := range v1.Values {
					r := make(map[string]interface{})
					if v1.Tags != nil && len(v1.Tags) >= 0 {
						r["id"] = v1.Tags["id"]
						//r[TIME] = v2.Tags[TIME]
					}
					for i := 0; i < len(v1.Columns); i++ {
						err := p.setInfluxResult(r, v1.Columns[i], v2[i])
						if err != nil {
							fmt.Println(err)
							continue
						}
					}
					rList = append(rList, r)
				}
			}
			newResults = append(newResults, rList)
		} else {
			continue
		}
	}

	return newResults, nil

}

// influxResultConvert InfluxDB的查询结果[]api.Result转换成[][]map[string]interface{}
func (p *APIView) influxResultConvert(results []client.Result) ([][]map[string]interface{}, error) {
	newResults := make([][]map[string]interface{}, 0)
	for _, v := range results {
		if v.Err == "" {
			rList := make([]map[string]interface{}, 0)
			for _, v1 := range v.Series {
				for _, v2 := range v1.Values {
					r := make(map[string]interface{})
					if v1.Tags != nil && len(v1.Tags) >= 0 {
						r["id"] = v1.Tags["id"]
						//r[TIME] = v2.Tags[TIME]
					}
					for i := 0; i < len(v1.Columns); i++ {
						err := p.setInfluxResult(r, v1.Columns[i], v2[i])
						if err != nil {
							fmt.Println(err)
							continue
						}
					}
					rList = append(rList, r)
				}
			}
			newResults = append(newResults, rList)
		} else {
			continue
		}
	}

	return newResults, nil

}

// setInfluxResult 结果实体赋值
func (p *APIView) setInfluxResult(result map[string]interface{}, flag interface{}, val interface{}) error {
	switch flag {
	case "time":
		v, ok := val.(string)
		if !ok {
			return errors.New("时间转换错误")
		}
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return err
		}
		result["time"] = t.Unix()
	default:
		v, ok := flag.(string)
		if !ok {
			return errors.New("flag转换错误")
		}
		result[v] = val
	}
	return nil
}

// GenericAPIView 通用服务创建
type GenericAPIView struct {
	APIView
	Cli                  *mongo.Client
	Col                  *mongo.Collection
	Database, Collection string
	Timeout              int
}
