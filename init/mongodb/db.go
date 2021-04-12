package mongodb

import (
	"context"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/air-iot/service/errors"
	"github.com/air-iot/service/util/json"
)

type (
	// PipelineFunc 声明了一个Pipeline修改函数类型
	PipelineFunc func() mongo.Pipeline
)

// QueryOption 查询条件
type QueryOption struct {
	Limit       *int                   `json:"limit,omitempty"`       // 查询数据长度
	Skip        *int                   `json:"skip,omitempty"`        // 跳过数据长度
	Sort        map[string]int         `json:"sort,omitempty"`        // 排序
	Filter      map[string]interface{} `json:"filter,omitempty"`      // 过滤条件
	WithCount   *bool                  `json:"withCount,omitempty"`   // 是否返回总数
	WithoutBody *bool                  `json:"withoutBody,omitempty"` // 是否只返回总数
	Project     map[string]interface{} `json:"project,omitempty"`     // 返回字段

	GroupFields     map[string]interface{} `json:"groupFields,omitempty"`     // 聚合分组查询字段
	GroupBy         map[string]interface{} `json:"groupBy,omitempty"`         // 聚合分组查询
	HasForeignKey   *bool                  `json:"hasForeignKey,omitempty"`   // 是否有外键关联字段筛选,有就需要管理去查询数据
	QueryLookupList []interface{}          `json:"queryLookupList,omitempty"` //
	SortFirst       *bool                  `json:"sortFirst,omitempty"`       //
}

// QueryCount 查询数量
type QueryCount struct {
	Count *int `json:"count" bson:"count"`
}

// ConvertKeyUnderlineID 转换key _id为id
func ConvertKeyUnderlineID(data interface{}) interface{} {
	switch val := data.(type) {
	case map[string]interface{}:
		for k, v := range val {
			if k == "_id" {
				delete(val, "_id")
				val["id"] = ConvertKeyUnderlineID(v)
			} else {
				val[k] = ConvertKeyUnderlineID(v)
			}
		}
		return val
	case *map[string]interface{}:
		for k, v := range *val {
			if k == "_id" {
				delete(*val, "_id")
				(*val)["id"] = ConvertKeyUnderlineID(v)
			} else {
				(*val)[k] = ConvertKeyUnderlineID(v)
			}
		}
		return val
	case bson.M:
		for k, v := range val {
			if k == "_id" {
				delete(val, "_id")
				val["id"] = ConvertKeyUnderlineID(v)
			} else {
				val[k] = ConvertKeyUnderlineID(v)
			}
		}
		return val
	case *bson.M:
		for k, v := range *val {
			if k == "_id" {
				delete(*val, "_id")
				(*val)["id"] = ConvertKeyUnderlineID(v)
			} else {
				(*val)[k] = ConvertKeyUnderlineID(v)
			}
		}
		return val
	case []interface{}:
		for i, v := range val {
			val[i] = ConvertKeyUnderlineID(v)
		}
		return val
	case *[]interface{}:
		for i, v := range *val {
			(*val)[i] = ConvertKeyUnderlineID(v)
		}
		return val
	case []map[string]interface{}:
		var vtmp = make([]interface{}, len(val))
		for i, v := range val {
			vtmp[i] = ConvertKeyUnderlineID(v)
		}
		return vtmp
	case *[]map[string]interface{}:
		var vtmp = make([]interface{}, len(*val))
		for i, v := range *val {
			vtmp[i] = ConvertKeyUnderlineID(v)
		}
		return vtmp
	case primitive.A:
		for i, v := range val {
			val[i] = ConvertKeyUnderlineID(v)
		}
		return val
	case *primitive.A:
		for i, v := range *val {
			(*val)[i] = ConvertKeyUnderlineID(v)
		}
		return val
	default:
		return data
	}
}

// ConvertKeyID 转换key id为_id
func ConvertKeyID(data interface{}) interface{} {
	switch val := data.(type) {
	case map[string]interface{}:
		for k, v := range val {
			if k == "id" {
				delete(val, "id")
				val["_id"] = ConvertKeyUnderlineID(v)
			} else {
				val[k] = ConvertKeyUnderlineID(v)
			}
		}
		return val
	case *map[string]interface{}:
		for k, v := range *val {
			if k == "id" {
				delete(*val, "id")
				(*val)["_id"] = ConvertKeyUnderlineID(v)
			} else {
				(*val)[k] = ConvertKeyUnderlineID(v)
			}
		}
		return val
	case bson.M:
		for k, v := range val {
			if k == "id" {
				delete(val, "id")
				val["_id"] = ConvertKeyUnderlineID(v)
			} else {
				val[k] = ConvertKeyUnderlineID(v)
			}
		}
		return val
	case *bson.M:
		for k, v := range *val {
			if k == "id" {
				delete(*val, "id")
				(*val)["_id"] = ConvertKeyUnderlineID(v)
			} else {
				(*val)[k] = ConvertKeyUnderlineID(v)
			}
		}
		return val
	case []interface{}:
		for i, v := range val {
			val[i] = ConvertKeyUnderlineID(v)
		}
		return val
	case *[]interface{}:
		for i, v := range *val {
			(*val)[i] = ConvertKeyUnderlineID(v)
		}
		return val
	case []map[string]interface{}:
		var vtmp = make([]interface{}, len(val))
		for i, v := range val {
			vtmp[i] = ConvertKeyUnderlineID(v)
		}
		return vtmp
	case *[]map[string]interface{}:
		var vtmp = make([]interface{}, len(*val))
		for i, v := range *val {
			vtmp[i] = ConvertKeyUnderlineID(v)
		}
		return vtmp
	case primitive.A:
		for i, v := range val {
			val[i] = ConvertKeyUnderlineID(v)
		}
		return val
	case *primitive.A:
		for i, v := range *val {
			(*val)[i] = ConvertKeyUnderlineID(v)
		}
		return val
	default:
		return data
	}
}

// QueryOptionToPipeline 转换查询到pipeline
func QueryOptionToPipeline(query QueryOption) (pipeLine mongo.Pipeline, countPipeLine mongo.Pipeline, err error) {
	pipeLine = mongo.Pipeline{}
	projectMap := make(map[string]interface{})

	var groupMap = make(map[string]interface{})
	if query.GroupFields != nil {
		groupMap = query.GroupFields
		for k := range query.GroupFields {
			projectMap[k] = 1
		}
	}
	if query.GroupBy != nil {
		groupMap["_id"] = query.GroupBy
	} else if query.GroupFields != nil && len(query.GroupFields) > 0 {
		groupMap["_id"] = nil
	}
	delete(groupMap, "id")

	// TODO filter nil
	if query.Filter != nil {
		if lookups, ok := query.Filter["$lookups"].([]interface{}); ok {
			lookupsOtherList := make([]interface{}, 0)
			if len(lookups) != 0 {
				if projectM, ok := lookups[0].(map[string]interface{})["$project"].(map[string]interface{}); ok {
					for k := range projectMap {
						projectM[k] = 1
					}
					lookupsOtherList = append(lookupsOtherList, lookups[0])
					lookupsOtherList = append(lookupsOtherList, map[string]interface{}{"$group": groupMap})
					lookupsOtherList = append(lookupsOtherList, lookups[1:]...)
					lookups = lookupsOtherList
					query.Filter["$lookups"] = lookups
				} else {
					lookupsOtherList = append(lookupsOtherList, map[string]interface{}{"$group": groupMap})
					lookupsOtherList = append(lookupsOtherList, lookups[1:]...)
					lookups = lookupsOtherList
					query.Filter["$lookups"] = lookups
				}
			} else {
				lookups = []interface{}{groupMap}
			}
		} else {

			lookupsTmp := make([]interface{}, 0)
			if len(projectMap) > 0 {
				lookupsTmp = append(lookupsTmp, bson.M{"$project": projectMap})
			}
			if len(groupMap) > 0 {
				lookupsTmp = append(lookupsTmp, groupMap)
			}

			if len(lookupsTmp) > 0 {
				query.Filter["$lookups"] = lookupsTmp
			}
		}
	}

	hasGroup := false
	hasForeignKey := query.HasForeignKey
	if hasForeignKey != nil && *hasForeignKey {
		queryLookupList := make([]interface{}, 0)
		if query.QueryLookupList != nil {
			queryLookupList = query.QueryLookupList
		}
		sortFirst := query.SortFirst
		if sortFirst != nil {
			if *sortFirst {
				if query.Filter != nil {
					for k, v := range query.Filter {
						if k == "$lookup" {
							pipeLine = append(pipeLine, bson.D{bson.E{Key: k, Value: v}})
							// 特殊处理lookup数组
						} else if k != "$lookups" {
							//判断空对象
							switch emptyObject := v.(type) {
							case map[string]interface{}:
								if len(emptyObject) == 0 {
									delete(query.Filter, k)
									continue
								}
							case bson.M:
								if len(emptyObject) == 0 {
									delete(query.Filter, k)
									continue
								}
							}
							if k == "id" {
								k = "_id"
							}
							if strings.HasSuffix(k, "Id") && k != "requestId" {
								k = k[:len(k)-2]
								k = k + "._id"
							}
							pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{k: v}}})
						}
					}
				}
				for _, lookup := range queryLookupList {
					pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
				}
			} else {
				for _, lookup := range queryLookupList {
					pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
				}
				if query.Filter != nil {
					for k, v := range query.Filter {
						if k == "$lookup" {
							pipeLine = append(pipeLine, bson.D{bson.E{Key: k, Value: v}})
							// 特殊处理lookup数组
						} else if k != "$lookups" {
							//判断空对象
							switch emptyObject := v.(type) {
							case map[string]interface{}:
								if len(emptyObject) == 0 {
									delete(query.Filter, k)
									continue
								}
							case bson.M:
								if len(emptyObject) == 0 {
									delete(query.Filter, k)
									continue
								}
							}
							if k == "id" {
								k = "_id"
							}
							if strings.HasSuffix(k, "Id") && k != "requestId" {
								k = k[:len(k)-2]
								k = k + "._id"
							}
							pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{k: v}}})
						}
					}
				}
			}
		}
	} else {
		if query.Filter != nil {
			for k, v := range query.Filter {
				if k == "$lookup" {
					pipeLine = append(pipeLine, bson.D{bson.E{Key: k, Value: v}})
					// 特殊处理lookup数组
				} else if k != "$lookups" {
					//判断空对象
					switch emptyObject := v.(type) {
					case map[string]interface{}:
						if len(emptyObject) == 0 {
							delete(query.Filter, k)
							continue
						}
					case bson.M:
						if len(emptyObject) == 0 {
							delete(query.Filter, k)
							continue
						}
					}

					if k == "id" {
						k = "_id"
					}
					if strings.HasSuffix(k, "Id") && k != "requestId" {
						k = k[:len(k)-2]
						k = k + "._id"
					}
					pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{k: v}}})
				}
			}
		}
	}
	if query.WithCount != nil && *(query.WithCount) {
		countPipeLine = mongo.Pipeline{}
		if err := json.CopyByJson(&countPipeLine, &pipeLine); err != nil {
			return nil, nil, err
		}
	}
	if query.Sort != nil && len(query.Sort) > 0 {
		pipeLine = append(pipeLine, bson.D{bson.E{Key: "$sort", Value: query.Sort}})
	} else {
		pipeLine = append(pipeLine, bson.D{bson.E{Key: "$sort", Value: bson.M{"_id": 1}}})
	}
	if query.Skip != nil && *(query.Skip) >= 0 {
		pipeLine = append(pipeLine, bson.D{bson.E{Key: "$skip", Value: query.Skip}})
	}
	if query.Limit != nil && *(query.Limit) >= 0 {
		pipeLine = append(pipeLine, bson.D{bson.E{Key: "$limit", Value: query.Limit}})
	}

	if query.WithoutBody != nil && *(query.WithoutBody) {
		return nil, pipeLine, nil
	}

	if query.Filter != nil {
		for k, v := range query.Filter {
			if k != "$lookups" {
				continue
			}
			lookups, ok := v.([]interface{})
			if !ok {
				return nil, nil, errors.New(`$lookups的值数据格式不正确`)
			}
			// 图形数据查询
			// 递归转换判断value值是否为ObjectID
			for _, lookup := range lookups {
				switch lookupMap := lookup.(type) {
				case map[string]interface{}:
					if _, ok := lookupMap["$group"]; ok {
						hasGroup = true
						break
						//pipeLine = append(pipeLine, bson.D{bson.E{Key: "$group", Value: lookup.(primitive.M)["$group"]}})
					} else if project, ok := lookupMap["$project"]; ok {
						switch projectM := project.(type) {
						case map[string]interface{}:
							projectMap = projectM
						case bson.M:
							projectMap = projectM
						default:
							return nil, nil, fmt.Errorf("%s的关联查询时内部project格式错误，不是Map", k)
						}
					} else {
						pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
					}
				case bson.M:
					if _, ok := lookupMap["$group"]; ok {
						hasGroup = true
						break
						//pipeLine = append(pipeLine, bson.D{bson.E{Key: "$group", Value: lookup.(primitive.M)["$group"]}})
					} else if project, ok := lookupMap["$project"]; ok {
						switch projectM := project.(type) {
						case map[string]interface{}:
							projectMap = projectM
						case bson.M:
							projectMap = projectM
						default:
							return nil, nil, fmt.Errorf("%s的关联查询时内部project格式错误，不是Map", k)
						}
					} else {
						pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
					}
				}

			}
		}

		if hasGroup {
			for k, v := range query.Filter {
				if k != "$lookups" {
					continue
				}
				lookups, ok := v.([]interface{})
				if !ok {
					return nil, nil, errors.New(`$lookups的值数据格式不正确`)
				}
				afterGroupFlag := false
				for _, lookup := range lookups {
					if afterGroupFlag {
						pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
					}

					switch lookupM := lookup.(type) {
					case map[string]interface{}:
						if group, ok := lookupM["$group"]; ok {
							pipeLine = append(pipeLine, bson.D{bson.E{Key: "$group", Value: group}})
							afterGroupFlag = true
						}
					case bson.M:
						if group, ok := lookupM["$group"]; ok {
							pipeLine = append(pipeLine, bson.D{bson.E{Key: "$group", Value: group}})
							afterGroupFlag = true
						}
					}
				}
			}
		}
	}
	if query.Project != nil {
		for k, v := range query.Project {
			projectMap[k] = v
		}
	}
	if len(projectMap) > 0 {
		pipeLine = append(pipeLine, bson.D{bson.E{Key: "$project", Value: projectMap}})
	}
	return pipeLine, countPipeLine, nil
}

// FindFilter 根据条件查询数据(原版)
// result:查询结果 query:查询条件
func FindFilter(ctx context.Context, col *mongo.Collection, result interface{}, query QueryOption) (count *int, err error) {
	pipeLine, countPipeLine, err := QueryOptionToPipeline(query)
	if err != nil {
		return nil, err
	}
	if countPipeLine != nil {
		count, err = FindCount(ctx, col, countPipeLine)
		if err != nil {
			return nil, fmt.Errorf("query count err: %s", err.Error())
		}
	}
	if pipeLine != nil {
		err = FindPipeline(ctx, col, result, pipeLine)
		if err != nil {
			return nil, fmt.Errorf("query data: %s", err.Error())
		}
	}
	return
}

// FindFilterLimit 根据条件查询数据(先根据条件分页)
// result:查询结果 query:查询条件
func FindFilterLimit(ctx context.Context, col *mongo.Collection, result interface{}, query QueryOption) (*int, error) {
	return FindFilter(ctx, col, result, query)
}

// FindPipeline 根据mongo pipeline
func FindPipeline(ctx context.Context, col *mongo.Collection, result interface{}, pipeLine mongo.Pipeline) error {
	//增加允许使用硬盘缓存
	AllowDiskUse := true
	cur, err := col.Aggregate(ctx, pipeLine,
		&options.AggregateOptions{AllowDiskUse: &AllowDiskUse},
	)
	if err != nil {
		return err
	}
	if err := cur.All(ctx, result); err != nil {
		return err
	}
	result = ConvertKeyUnderlineID(result)
	return nil
}

// FindByID 根据id查询数据
func FindByID(ctx context.Context, col *mongo.Collection, result interface{}, id string) error {
	singleResult := col.FindOne(ctx, bson.M{"_id": id})
	if singleResult.Err() != nil {
		return singleResult.Err()
	}
	if err := singleResult.Decode(result); err != nil {
		return err
	}
	result = ConvertKeyUnderlineID(result)
	return nil
}

// FindCount 根据Pipeline查询数据量
// pipeLine:查询管道
func FindCount(ctx context.Context, col *mongo.Collection, pipeLine mongo.Pipeline) (*int, error) {
	pipeLine = append(pipeLine, bson.D{bson.E{Key: "$count", Value: "count"}})
	cur, err := col.Aggregate(ctx, pipeLine)
	if err != nil {
		return nil, err
	}
	for cur.Next(ctx) {
		var r QueryCount
		err := cur.Decode(&r)
		if err != nil {
			return nil, err
		}
		if r.Count != nil {
			return r.Count, nil
		}
	}
	return nil, errors.ErrNotFound
}

// SaveOne 保存数
// model:数据
func SaveOne(ctx context.Context, col *mongo.Collection, item interface{}) (*mongo.InsertOneResult, error) {
	item = ConvertKeyID(item)
	return col.InsertOne(ctx, item)
}

// UpdateMany 全部数据更新
// conditions:更新条件 model:更新的数据
func UpdateMany(ctx context.Context, col *mongo.Collection, conditions, item interface{}) (*mongo.UpdateResult, error) {
	return col.UpdateMany(ctx, conditions, item)
}

// UpdateByID 根据ID及数据更新
// id:主键_id model:更新的数据
func UpdateByID(ctx context.Context, col *mongo.Collection, id string, item interface{}) (*mongo.UpdateResult, error) {
	return col.UpdateOne(ctx, bson.M{"_id": id}, bson.D{bson.E{Key: "$set", Value: item}})
}

// ReplaceByID 根据ID及数据替换
// id:主键_id model:替换的数据
func ReplaceByID(ctx context.Context, col *mongo.Collection, id string, item interface{}) (*mongo.UpdateResult, error) {
	return col.ReplaceOne(ctx, bson.M{"_id": id}, bson.D{bson.E{Key: "$set", Value: item}})
}

// DeleteByID 根据ID删除数据
// id:主键_id
func DeleteByID(ctx context.Context, col *mongo.Collection, id string) (*mongo.DeleteResult, error) {
	return col.DeleteOne(ctx, bson.M{"_id": id})
}

// DeleteOne 根据条件删除单条数据
// condition:删除条件
func DeleteOne(ctx context.Context, col *mongo.Collection, filter interface{}) (*mongo.DeleteResult, error) {
	return col.DeleteOne(ctx, filter)
}

// DeleteMany 根据条件进行多条数据删除
// condition:删除条件
func DeleteMany(ctx context.Context, col *mongo.Collection, filter interface{}) (*mongo.DeleteResult, error) {
	return col.DeleteMany(ctx, filter)
}

// SaveMany 批量保存数据
// models:数据
func SaveMany(ctx context.Context, col *mongo.Collection, item []interface{}) (*mongo.InsertManyResult, error) {
	for i, v := range item {
		item[i] = ConvertKeyID(v)
	}
	return col.InsertMany(ctx, item)
}

// UpdateAll 全部数据更新
func UpdateAll(ctx context.Context, col *mongo.Collection, item interface{}) (*mongo.UpdateResult, error) {
	return col.UpdateMany(ctx, bson.M{}, bson.D{bson.E{Key: "$set", Value: item}})
}

// UpdateManyByIDList 根据id数组进行多条数据更新
func UpdateManyByIDList(ctx *context.Context, col *mongo.Collection, id []string, item interface{}) (*mongo.UpdateResult, error) {
	return col.UpdateMany(*ctx, bson.M{"_id": bson.M{"$in": id}}, bson.D{bson.E{Key: "$set", Value: item}})
}

// UpdateManyWithOption 数组中的数据更新
// conditions:更新条件 model:更新的数据
func UpdateManyWithOption(ctx context.Context, col *mongo.Collection, filter, item interface{}, options *options.UpdateOptions) (*mongo.UpdateResult, error) {
	return col.UpdateMany(ctx, filter, item, options)
}
