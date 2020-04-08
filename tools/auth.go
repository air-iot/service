package tools

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	restfulapi "github.com/air-iot/service/restful-api"
	"github.com/air-iot/service/model"
	idb "github.com/air-iot/service/db/mongo"
)



// GetPointAndUserCount 算数据点用户数
func GetPointAndUserCount(ctx context.Context) (map[string]interface{},error) {

	pointCount, err := calcPointNum(ctx)
	if err != nil {
		return nil,fmt.Errorf("计算数据点总数失败:%s", err.Error())
	}

	pipeLine := mongo.Pipeline{}
	//获取当前系统用户数量
	currentUserCount, err := restfulapi.FindCount(ctx, idb.Database.Collection(model.USER), pipeLine)
	if err != nil {
		return nil,fmt.Errorf("查询用户总数失败:%s", err.Error())
	}

	returnResult := map[string]interface{}{
		"pointCounts": pointCount,
		"userCounts":  currentUserCount,
	}
	return returnResult,nil
}

// statsCount 统计各个模型的属性点数数据.
func statsTagCount(ctx context.Context) (*[]bson.M, error) {
	pipeLine := mongo.Pipeline{
		bson.D{bson.E{Key: "$project", Value: bson.M{"sizeOfTags": bson.M{"$size": "$device.tags"}, "sizeOfProps": bson.M{"$size": "$computed.tags"}}}},
	}
	result := make([]bson.M, 0)
	err := restfulapi.FindPipeline(ctx, idb.Database.Collection(model.MODEL), &result, pipeLine,nil)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// statsNodeCustomTagCount 统计各个资产自定义的属性点数数据.
func statsNodeCustomTagCount(ctx context.Context) (*[]bson.M, error) {
	pipeLine := mongo.Pipeline{
		bson.D{bson.E{Key: "$project", Value: bson.M{"sizeOfTags": bson.M{"$size": "$device.tags"}, "sizeOfProps": bson.M{"$size": "$computed.tags"}}}},
	}
	result := make([]bson.M, 0)
	err := restfulapi.FindPipeline(ctx, idb.Database.Collection(model.NODE), &result, pipeLine,nil)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// statsNodeCount 统计各个模型下的资产总和数据.
func statsNodeCount(ctx context.Context) (*[]bson.M, error) {
	pipeLine := mongo.Pipeline{
		bson.D{bson.E{Key: "$group", Value: bson.M{"_id": "$model", "count": bson.M{"$sum": 1}}}},
	}
	result := make([]bson.M, 0)
	err := restfulapi.FindPipeline(ctx, idb.Database.Collection(model.NODE), &result, pipeLine,nil)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// calcPointNum 计算属性点总数量
func calcPointNum(ctx context.Context) (int32, error) {
	tagCountList, err := statsTagCount(ctx)
	if err != nil {
		return 0, err
	}
	nodeCountList, err := statsNodeCount(ctx)
	if err != nil {
		return 0, err
	}

	pointCount := int32(0)
	tagCountMap := map[string]int32{}
	nodeCountMap := map[string]int32{}

	for _, tagMap := range *tagCountList {
		tagCountMapKey := ""
		tagCountMapValue := int32(0)
		for tagKey, tagValue := range tagMap {
			switch tagKey {
			case "id":
				if _, ok := tagValue.(primitive.ObjectID); !ok {
					tagCountMapKey = ""
				} else {
					tagCountMapKey = tagValue.(primitive.ObjectID).Hex()
				}
			case "sizeOfTags", "sizeOfProps":
				if _, ok := tagValue.(int32); ok {
					tagCountMapValue = tagCountMapValue + tagValue.(int32)
				}
			}
		}
		tagCountMap[tagCountMapKey] = tagCountMapValue
	}

	for _, nodeMap := range *nodeCountList {
		nodeCountMapKey := ""
		nodeCountMapValue := int32(0)
		for tagKey, tagValue := range nodeMap {
			switch tagKey {
			case "id":
				if _, ok := tagValue.(primitive.ObjectID); !ok {
					nodeCountMapKey = ""
				} else {
					nodeCountMapKey = tagValue.(primitive.ObjectID).Hex()
				}
			case "count":
				if _, ok := tagValue.(int32); !ok {
					nodeCountMapValue = 0
				} else {
					nodeCountMapValue = tagValue.(int32)
				}
			}
		}
		nodeCountMap[nodeCountMapKey] = nodeCountMapValue
	}

	for tagKey, tagValue := range tagCountMap {
		for nodeKey, nodeValue := range nodeCountMap {
			if tagKey == nodeKey {
				pointCount = pointCount + tagValue*nodeValue
				break
			}
		}
	}

	nodeCustomTagCountList, err := statsNodeCustomTagCount(ctx)
	if err != nil {
		return 0, err
	}

	nodeCustomTagCount := int32(0)

	for _, nodeCustomTagMap := range *nodeCustomTagCountList {
		for nodeCustomTagKey, nodeCustomTagValue := range nodeCustomTagMap {
			switch nodeCustomTagKey {
			case "sizeOfTags", "sizeOfProps":
				if ele, ok := nodeCustomTagValue.(int32); ok {
					nodeCustomTagCount = nodeCustomTagCount + ele
				}
			}
		}
	}

	pointCount = pointCount + nodeCustomTagCount
	return pointCount, nil
}
