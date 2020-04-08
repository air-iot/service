package logic

import (
	"context"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"common/restful-api"
)

// recursionFindIDList 递归ID列表
func RecursionFindIDList(ctx context.Context, objectIDList []primitive.ObjectID, collection *mongo.Collection, recursionKey string) (*[]primitive.ObjectID, error) {
	//查询该用户所属的部门
	objectIDCombineList := make([]primitive.ObjectID, 0)
	objectIDCombineList = append(objectIDCombineList, objectIDList...)
	for _, v := range objectIDList {
		err := recursionFindOps(ctx, &objectIDCombineList, v, collection, recursionKey)
		if err != nil {
			return nil, err
		}
	}

	return &objectIDCombineList, nil
}

// recursionFindOps 递归查询
func recursionFindOps(ctx context.Context, checkedList *[]primitive.ObjectID, data primitive.ObjectID, collection *mongo.Collection, recursionKey string) error {
	//CallCount++
	//递归查询
	objectList := new([]bson.M)
	paramMatch := bson.D{
		bson.E{
			Key: "$match",
			Value: bson.M{
				"recursionKey": data,
			},
		},
	}
	pipeline := mongo.Pipeline{}
	pipeline = append(pipeline, paramMatch)
	err := restfulapi.FindPipeline(ctx, collection, objectList, pipeline, func(d *bson.M) {})
	if err != nil {
		return err
	}

	if len(*objectList) == 0 {
		//fmt.Println("out")
		//OutCount++
		return nil
	}

	for _, v := range *objectList {
		id := v["id"].(primitive.ObjectID)
		//fmt.Println("objectIdList V:", id)
		flag := false
		for _, val := range *checkedList {
			if val == id {
				flag = true
				break
			}
		}
		if flag {
			continue
		}

		//IterateCount++
		*checkedList = append(*checkedList, id)
		if flagVal, ok := v["hasChild"].(bool); ok {
			if flagVal == true {
				err := recursionFindOps(ctx, checkedList, id, collection, recursionKey)
				if err != nil {
					return err
				}
			}
		} else {
			err := recursionFindOps(ctx, checkedList, id, collection, recursionKey)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
