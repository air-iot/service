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

// ConvertKeyID 转换查询的ObjectID
func ConvertKeyID(data *bson.M) error {
	for k, v := range *data {
		switch val := v.(type) {
		case primitive.M:
			if k == "_id" {
				for keyIn, valIn := range val {
					(*data)[keyIn] = valIn
				}
				delete(*data, k)
				continue
			}
			value, err := ConvertPrimitiveMapToID(data, k, val)
			if err != nil {
				return err
			}
			(*data)[k] = value
		case primitive.A:
			value, err := ConvertPrimitiveAToID(data, k, val)
			if err != nil {
				return err
			}
			(*data)[k] = value
		case string:
			if k == "_id" {
				delete(*data, k)
				(*data)["id"] = val
			}
		}
	}

	return nil
}

// ConvertPrimitiveAToID 转换interface类型数组
func ConvertPrimitiveAToID(data *bson.M, key string, val primitive.A) (interface{}, error) {
	result := make(primitive.A, 0)
	for _, outValue := range val {
		switch val := outValue.(type) {
		case primitive.M:
			value, err := ConvertPrimitiveMapToID(data, key, val)
			if err != nil {
				return nil, nil
			}
			result = append(result, value)
			//(*data)[k] = value
		case primitive.A:
			value, err := ConvertPrimitiveAToID(data, key, val)
			if err != nil {
				return nil, nil
			}
			result = append(result, value)
			//(*data)[k] = value
		case string:
			result = append(result, val)
		default:
			result = append(result, val)
		}

	}
	return result, nil
}

// ConvertPrimitiveMapToID 转换为primitive.M
func ConvertPrimitiveMapToID(data *bson.M, key string, value primitive.M) (interface{}, error) {
	for k, v := range value {
		switch val := v.(type) {
		case primitive.M:
			r, err := ConvertPrimitiveMapToID(data, k, val)
			if err != nil {
				return nil, err
			}
			value[k] = r
		case primitive.A:
			r, err := ConvertPrimitiveAToID(data, k, val)
			if err != nil {
				return nil, err
			}
			value[k] = r
		case string:
			if k == "_id" {
				delete(value, k)
				value["id"] = val
			}
		default:
			value[k] = val
		}
	}
	return value, nil
}

// FindQueryConvertPipeline 转换查询到pipeline
func FindQueryConvertPipeline(query bson.M) (mongo.Pipeline, mongo.Pipeline, error) {
	pipeLine := mongo.Pipeline{}
	var newPipeLine mongo.Pipeline
	if filter, ok := query["filter"]; ok {
		if filterMap, ok := filter.(bson.M); ok {
			for k, v := range filterMap {
				pipeLine = append(pipeLine, bson.D{bson.E{Key: k, Value: v}})
			}
		} else {
			return nil, nil, errors.New(`filter格式不正确`)
		}
	}

	if withCount, ok := query["withCount"]; ok {
		if b, ok := withCount.(bool); ok {
			if b {
				newPipeLine := mongo.Pipeline{}
				if err := json.CopyByJson(&newPipeLine, pipeLine); err != nil {
					return nil, nil, err
				}
				//newPipeLine = DeepCopy(pipeLine).(mongo.Pipeline)
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
func FindFilter(ctx context.Context, col *mongo.Collection, result *[]bson.M, query bson.M) (int, error) {

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
					pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{k: v}}})
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
				newPipeLine := mongo.Pipeline{}
				if err := json.CopyByJson(&newPipeLine, pipeLine); err != nil {
					return 0, err
				}
				//newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
				c, err := FindCount(ctx, col, newPipeLine)
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
	} else {
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
				newPipeLine := mongo.Pipeline{}
				if err := json.CopyByJson(&newPipeLine, pipeLine); err != nil {
					return 0, err
				}
				//newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
				c, err := FindCount(ctx, col, newPipeLine)
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
	if err := FindPipeline(ctx, col, result, pipeLine); err != nil {
		return 0, err
	}
	return count, nil
}

// FindFilter 根据条件查询数据(原版)
// result:查询结果 query:查询条件
//func FindFilterDeptQuery(ctx context.Context, col *mongo.Collection, result *[]bson.M, query bson.M) (int, error) {
//
//	var count int
//	projectMap := bson.M{}
//
//	pipeLine := mongo.Pipeline{}
//
//	hasGroup := false
//
//	//$group 放入 $lookups的$project后面
//	if filter, ok := query["filter"]; ok {
//		if filterMap, ok := filter.(bson.M); ok {
//			groupMap := primitive.M{}
//			hasGroupFields := false
//			if groupFieldsMap, ok := query["groupFields"].(primitive.M); ok {
//				groupMap = groupFieldsMap
//				hasGroupFields = true
//				for k := range groupFieldsMap {
//					projectMap[k] = 1
//				}
//				delete(query, "groupFields")
//			}
//			if groupByMap, ok := query["groupBy"].(primitive.M); ok {
//				groupMap["_id"] = groupByMap
//				query["group"] = groupMap
//				delete(query, "groupBy")
//			} else if hasGroupFields {
//				groupMap["_id"] = nil
//				query["group"] = groupMap
//				delete(query, "groupBy")
//			}
//			if groupMap, ok := query["group"].(primitive.M); ok {
//				//groupMap["_id"] = groupMap["id"]
//				delete(groupMap, "id")
//				if lookups, ok := filterMap["$lookups"].(primitive.A); ok {
//					lookupsOtherList := primitive.A{}
//					if len(lookups) != 0 {
//						if projectM, ok := lookups[0].(primitive.M)["$project"].(primitive.M); ok {
//							for k := range projectMap {
//								projectM[k] = 1
//							}
//							lookupsOtherList = append(lookupsOtherList, lookups[0])
//							lookupsOtherList = append(lookupsOtherList, bson.M{"$group": groupMap})
//							lookupsOtherList = append(lookupsOtherList, lookups[1:]...)
//							lookups = lookupsOtherList
//							filterMap["$lookups"] = lookups
//						} else {
//							lookupsOtherList = append(lookupsOtherList, bson.M{"$group": groupMap})
//							lookupsOtherList = append(lookupsOtherList, lookups[1:]...)
//							lookups = lookupsOtherList
//							filterMap["$lookups"] = lookups
//						}
//					} else {
//						lookups = primitive.A{groupMap}
//					}
//				} else {
//					filterMap["$lookups"] = primitive.A{bson.M{"$project": projectMap}, groupMap}
//				}
//				delete(query, "group")
//			}
//			// }
//		} else {
//			return 0, errors.New(`filter格式不正确`)
//		}
//	}
//
//	if filter, ok := query["filter"]; ok {
//		if filterMap, ok := filter.(bson.M); ok {
//			for k, v := range filterMap {
//				// 图形数据查询
//				if k == "$lookups" {
//					// 递归转换判断value值是否为ObjectID
//					if lookups, ok := v.(primitive.A); ok {
//						for _, lookup := range lookups {
//							if _, ok := lookup.(primitive.M)["$group"]; ok {
//								hasGroup = true
//								break
//								//pipeLine = append(pipeLine, bson.D{bson.E{Key: "$group", Value: lookup.(primitive.M)["$group"]}})
//							} else if _, ok := lookup.(primitive.M)["$project"]; ok {
//								if _, projectOk := lookup.(primitive.M)["$project"].(bson.M); !projectOk {
//									return 0, fmt.Errorf("%s的关联查询时内部project格式错误，不是bson.M", k)
//								}
//								projectMap = lookup.(primitive.M)["$project"].(bson.M)
//							} else {
//								pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
//							}
//						}
//					} else {
//						return 0, errors.New(`$lookups的值数据格式不正确`)
//					}
//				}
//			}
//			// }
//		} else {
//			return 0, errors.New(`filter格式不正确`)
//		}
//	}
//
//	if filter, ok := query["filter"]; ok {
//		if filterMap, ok := filter.(bson.M); ok {
//			for k, v := range filterMap {
//				if k == "$lookup" {
//					pipeLine = append(pipeLine, bson.D{bson.E{Key: k, Value: v}})
//					// 特殊处理lookup数组
//				} else if k != "$lookups" {
//					//判断空对象
//					if emptyObject, ok := v.(primitive.M); ok {
//						flag := false
//						for range emptyObject {
//							flag = true
//						}
//						if !flag {
//							delete(filterMap, k)
//							continue
//						}
//					}
//					if k == "id" {
//						k = "_id"
//					}
//					//if strings.HasSuffix(k, "id") && k != "_id" && !strings.HasSuffix(k, "_id") && !strings.HasSuffix(k, "uid") {
//					//	k = k[:len(k)-2]
//					//	k = k + "_id"
//					//} else
//					if hasGroup && k == "modelId" {
//						k = "model"
//					} else {
//						if strings.HasSuffix(k, "Id") && k != "requestId" {
//							k = k[:len(k)-2]
//							k = k + "._id"
//						}
//					}
//					pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{k: v}}})
//				}
//			}
//			// }
//		} else {
//			return 0, errors.New(`filter格式不正确`)
//		}
//	}
//
//	if hasGroup {
//		afterGroupFlag := false
//
//		if filter, ok := query["filter"]; ok {
//			if filterMap, ok := filter.(bson.M); ok {
//				for k, v := range filterMap {
//					// 图形数据查询
//					if k == "$lookups" {
//						// 递归转换判断value值是否为ObjectID
//						if lookups, ok := v.(primitive.A); ok {
//							for _, lookup := range lookups {
//								if afterGroupFlag {
//									pipeLine = append(pipeLine, bson.D{bson.E{Key: "$lookup", Value: lookup}})
//								}
//								if _, ok := lookup.(primitive.M)["$group"]; ok {
//									pipeLine = append(pipeLine, bson.D{bson.E{Key: "$group", Value: lookup.(primitive.M)["$group"]}})
//									afterGroupFlag = true
//								}
//							}
//						} else {
//							return 0, errors.New(`$lookups的值数据格式不正确`)
//						}
//					}
//				}
//				// }
//			} else {
//				return 0, errors.New(`filter格式不正确`)
//			}
//		}
//	}
//
//	hasRemoveFirst := false
//	if withCount, ok := query["withCount"]; ok {
//		if b, ok := withCount.(bool); ok {
//			if b {
//				newPipeLine := mongo.Pipeline{}
//				if err := json.CopyByJson(&newPipeLine, pipeLine); err != nil {
//					return 0, err
//				}
//				//newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
//				if col.Name() == model.DEPT && !hasGroup && len(newPipeLine) > 1 {
//					newPipeLine = newPipeLine[1:]
//					hasRemoveFirst = true
//				}
//				c, err := FindCount(ctx, col, newPipeLine)
//				if err != nil {
//					return 0, err
//				}
//				count = c
//			}
//		} else {
//			return count, errors.New(`withCount格式不正确`)
//		}
//	}
//
//	if sort, ok := query["sort"]; ok {
//		if s, ok := sort.(bson.M); ok {
//			if len(s) > 0 {
//				pipeLine = append(pipeLine, bson.D{bson.E{Key: "$sort", Value: s}})
//			}
//		}
//	}
//
//	if skip, ok := query["skip"]; ok {
//		pipeLine = append(pipeLine, bson.D{bson.E{Key: "$skip", Value: skip}})
//	}
//
//	if limit, ok := query["limit"]; ok {
//		pipeLine = append(pipeLine, bson.D{bson.E{Key: "$limit", Value: limit}})
//	}
//
//	if withoutBody, ok := query["withoutBody"]; ok {
//		if b, ok := withoutBody.(bool); ok {
//			if b {
//				newPipeLine := mongo.Pipeline{}
//				if err := json.CopyByJson(&newPipeLine, pipeLine); err != nil {
//					return 0, err
//				}
//				//newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
//				c, err := FindCount(ctx, col, newPipeLine)
//				if err != nil {
//					return 0, err
//				}
//				count = c
//				return count, nil
//			}
//		} else {
//			return count, errors.New(`withoutBody格式不正确`)
//		}
//	}
//
//	if project, ok := query["project"]; ok {
//		if projectList, ok := query["project"].(map[string]interface{}); ok {
//			for k, v := range projectMap {
//				projectList[k] = v
//			}
//		} else if projectList, ok := query["project"].(bson.M); ok {
//			for k, v := range projectMap {
//				projectList[k] = v
//			}
//		}
//		pipeLine = append(pipeLine, bson.D{bson.E{Key: "$project", Value: project}})
//	} else {
//		flag := false
//		for range projectMap {
//			flag = true
//		}
//		if flag {
//			pipeLine = append(pipeLine, bson.D{bson.E{Key: "$project", Value: projectMap}})
//		}
//	}
//
//	//if project, ok := (*query)["project"]; ok {
//	//	pipeLine = append(pipeLine, bson.D{bson.E{Key: "$project", Value: project}})
//	//}
//	//newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
//	if hasRemoveFirst {
//		pipeLine = pipeLine[1:]
//		//pipeLine = append(pipeLine,newPipeLine[0])
//	}
//	if err := FindPipeline(ctx, col, result, pipeLine, calc); err != nil {
//		return 0, err
//	}
//	return count, nil
//}

// FindFilterLimit 根据条件查询数据(先根据条件分页)
// result:查询结果 query:查询条件
func FindFilterLimit(ctx context.Context, col *mongo.Collection, result *[]bson.M, query bson.M) (int, error) {

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
									if strings.HasSuffix(k, "Id") && k != "requestId" {
										k = k[:len(k)-2]
										k = k + "._id"
									}
									pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{k: v}}})
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
									//if strings.HasSuffix(k, "id") && k != "_id" && !strings.HasSuffix(k, "_id") && !strings.HasSuffix(k, "uid") {
									//	k = k[:len(k)-2]
									//	k = k + "_id"
									//} else
									if strings.HasSuffix(k, "Id") && k != "requestId" {
										k = k[:len(k)-2]
										k = k + "._id"
									}
									pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{k: v}}})
								}
							}
							// }
						} else {
							return 0, errors.New(`filter格式不正确`)
						}
					}
				}
			}
			if withCount, ok := query["withCount"]; ok {
				if b, ok := withCount.(bool); ok {
					if b {
						newPipeLine := mongo.Pipeline{}
						if err := json.CopyByJson(&newPipeLine, pipeLine); err != nil {
							return 0, err
						}
						//newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
						c, err := FindCount(ctx, col, newPipeLine)
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
			} else {
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
						newPipeLine := mongo.Pipeline{}
						if err := json.CopyByJson(&newPipeLine, pipeLine); err != nil {
							return 0, err
						}
						//newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
						c, err := FindCount(ctx, col, newPipeLine)
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
							pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{k: v}}})
						}
					}
					// }
				} else {
					return 0, errors.New(`filter格式不正确`)
				}
			}
			if withCount, ok := query["withCount"]; ok {
				if b, ok := withCount.(bool); ok {
					if b {
						newPipeLine := mongo.Pipeline{}
						if err := json.CopyByJson(&newPipeLine, pipeLine); err != nil {
							return 0, err
						}
						//newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
						c, err := FindCount(ctx, col, newPipeLine)
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
			} else {
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
						newPipeLine := mongo.Pipeline{}
						if err := json.CopyByJson(&newPipeLine, pipeLine); err != nil {
							return 0, err
						}
						//newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
						c, err := FindCount(ctx, col, newPipeLine)
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
						pipeLine = append(pipeLine, bson.D{bson.E{Key: "$match", Value: bson.M{k: v}}})
					}
				}
				// }
			} else {
				return 0, errors.New(`filter格式不正确`)
			}
		}

		if withCount, ok := query["withCount"]; ok {
			if b, ok := withCount.(bool); ok {
				if b {
					newPipeLine := mongo.Pipeline{}
					if err := json.CopyByJson(&newPipeLine, pipeLine); err != nil {
						return 0, err
					}
					//newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
					c, err := FindCount(ctx, col, newPipeLine)
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
		} else {
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
					newPipeLine := mongo.Pipeline{}
					if err := json.CopyByJson(&newPipeLine, pipeLine); err != nil {
						return 0, err
					}
					//newPipeLine := DeepCopy(pipeLine).(mongo.Pipeline)
					c, err := FindCount(ctx, col, newPipeLine)
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
	if err := FindPipeline(ctx, col, result, pipeLine); err != nil {
		return 0, err
	}
	return count, nil
}

// FindPipeline 根据mongo pipeline
func FindPipeline(ctx context.Context, col *mongo.Collection, result *[]bson.M, pipeLine mongo.Pipeline) error {
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
		err = ConvertKeyID(&r)
		if err != nil {
			return err
		}
		*result = append(*result, r)
	}

	return nil
}

// FindByID 根据id查询数据
func FindByID(ctx context.Context, col *mongo.Collection, result *bson.M, id string) error {
	singleResult := col.FindOne(ctx, bson.M{"_id": id})

	if singleResult.Err() != nil {
		return singleResult.Err()
	}
	if err := singleResult.Decode(result); err != nil {
		return err
	}

	return ConvertKeyID(result)
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

// SaveOne 保存数
// model:数据
func SaveOne(ctx context.Context, col *mongo.Collection, model bson.M) (*mongo.InsertOneResult, error) {
	if _, ok := model["_id"]; !ok {
		model["_id"] = primitive.NewObjectID().Hex()
	}
	return col.InsertOne(ctx, model)
}

// UpdateMany 全部数据更新
// conditions:更新条件 model:更新的数据
func UpdateMany(ctx context.Context, col *mongo.Collection, conditions, model bson.M) (*mongo.UpdateResult, error) {
	return col.UpdateMany(ctx, conditions, model)
}

// UpdateByID 根据ID及数据更新
// id:主键_id model:更新的数据
func UpdateByID(ctx context.Context, col *mongo.Collection, id string, model bson.M) (*mongo.UpdateResult, error) {
	return col.UpdateOne(ctx, bson.M{"_id": id}, bson.D{bson.E{Key: "$set", Value: model}})
}
