package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	idb "github.com/air-iot/service/db/mongo"
	"github.com/air-iot/service/logger"
	imqtt "github.com/air-iot/service/mq/mqtt"
	"github.com/air-iot/service/restful-api"
	"github.com/air-iot/service/tools"
)

var eventDeviceModifyLog = map[string]interface{}{"name": "资产修改事件触发"}

func TriggerDeviceModify(data map[string]interface{}) error {
	//logger.Debugf(eventDeviceModifyLog, "开始执行资产修改事件触发器")
	//logger.Debugf(eventDeviceModifyLog, "传入参数为:%+v", data)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(30)*time.Second)
	defer cancel()

	nodeID, ok := data["node"].(string)
	if !ok {
		return fmt.Errorf("数据消息中nodeId字段不存在或类型错误")
	}

	modelID, ok := data["model"].(string)
	if !ok {
		return fmt.Errorf("数据消息中model字段不存在或类型错误")
	}

	departmentList, ok := data["department"].([]interface{})
	if !ok {
		return fmt.Errorf("数据消息中department字段不存在或类型错误")
	}

	departmentObjectIDList, err := tools.StringListToObjectIdList(tools.InterfaceListToStringList(departmentList))
	if err != nil {
		return fmt.Errorf("数据消息中department中的ID转ObjectID失败")
	}

	modifyType, ok := data["type"].(string)
	if !ok {
		return fmt.Errorf("数据消息中command对象中name字段不存在或类型错误")
	}

	modifyTypeMapping := map[string]string{
		"资产增加": "增加资产",
		"资产修改": "修改资产属性",
		"资产删除": "删除资产",
	}

	modifyTypeAfterMapping := modifyTypeMapping[modifyType]

	nodeObjectID, err := primitive.ObjectIDFromHex(nodeID)
	if err != nil {
		return fmt.Errorf("数据消息中nodeId转ObjectID失败")
	}

	modelObjectID, err := primitive.ObjectIDFromHex(modelID)
	if err != nil {
		return fmt.Errorf("数据消息中modelId转ObjectID失败")
	}

	//logger.Debugf(eventDeviceModifyLog, "开始获取当前资产修改内容类型的资产修改逻辑事件")
	//获取当前资产修改内容类型的资产修改逻辑事件=============================================
	paramMatch := bson.D{
		bson.E{
			Key: "$match",
			Value: bson.M{
				"type":          DeviceModify,
				"settings.type": modifyTypeAfterMapping,
				//"$or":
				//bson.A{
				//	bson.D{{
				//		"settings.node.id", nodeID,
				//	}},
				//	bson.D{
				//		{
				//			"settings.department.id", bson.M{"$in": departmentList},
				//		},
				//		{
				//			"settings.model.id", modelID,
				//		},
				//	},
				//	bson.D{
				//		{
				//			"settings.department.id", bson.M{"$in": departmentList},
				//		},
				//	},
				//	bson.D{
				//		{
				//			"settings.model.id", modelID,
				//		},
				//	},
				//},
			},
		},
	}
	paramLookup := bson.D{
		bson.E{
			Key: "$lookup",
			Value: bson.M{
				"from":         "eventhandler",
				"localField":   "_id",
				"foreignField": "event",
				"as":           "handlers",
			},
		},
	}


	pipeline := mongo.Pipeline{}
	pipeline = append(pipeline, paramMatch, paramLookup)
	eventInfoList := make([]bson.M, 0)
	err = restfulapi.FindPipeline(ctx, idb.Database.Collection("event"), &eventInfoList, pipeline, nil)
	if err != nil {
		return fmt.Errorf("获取当前资产修改内容类型(%s)的资产修改逻辑事件失败:%s", modelID, err.Error())
	}

	//logger.Debugf(eventDeviceModifyLog, "开始遍历事件列表")
	for _, eventInfo := range eventInfoList {
		logger.Debugf(eventDeviceModifyLog, "事件信息为:%+v", eventInfo)
		handlers, ok := eventInfo["handlers"].(primitive.A)
		if !ok {
			logger.Warnln(eventDeviceModifyLog, "handlers字段不存在或类型错误")
			continue
		}
		if len(handlers) == 0 {
			logger.Warnln(eventDeviceModifyLog, "handlers字段数组长度为0")
			continue
		}

		paramMatch = bson.D{
			bson.E{
				Key: "$match",
				Value: bson.M{
					"_id": nodeObjectID,
				},
			},
		}
		paramLookup = bson.D{
			bson.E{
				Key: "$lookup",
				Value: bson.M{
					"from":         "dept",
					"localField":   "department",
					"foreignField": "_id",
					"as":           "department",
				},
			},
		}
		paramLookupModel := bson.D{
			bson.E{
				Key: "$lookup",
				Value: bson.M{
					"from":         "model",
					"localField":   "model",
					"foreignField": "_id",
					"as":           "model",
				},
			},
		}
		pipeline = mongo.Pipeline{}
		pipeline = append(pipeline, paramMatch, paramLookup, paramLookupModel)
		nodeInfoList := make([]bson.M, 0)
		err = restfulapi.FindPipeline(ctx, idb.Database.Collection("node"), &nodeInfoList, pipeline, nil)
		if err != nil {
			return fmt.Errorf("获取当前资产(%s)的详情失败:%s", nodeID, err.Error())
		}

		if len(nodeInfoList) == 0 {
			return fmt.Errorf("当前查询的资产(%s)不存在", nodeID)
		}

		nodeInfo := nodeInfoList[0]

		departmentList, ok := nodeInfo["department"].(primitive.A)
		if !ok {
			logger.Warnln(eventExecCmdLog, "资产(%s)的department字段不存在或类型错误", nodeID)
			continue
		}
		departmentMList := make([]bson.M, 0)
		for _, department := range departmentList {
			if departmentMap, ok := department.(bson.M); ok {
				departmentMList = append(departmentMList, departmentMap)
			}
		}

		data["departmentName"] = tools.FormatKeyInfoList(departmentMList, "name")

		modelList, ok := nodeInfo["model"].(primitive.A)
		if !ok {
			logger.Warnln(eventExecCmdLog, "资产(%s)的model字段不存在或类型错误", nodeID)
			continue
		}
		modelMList := make([]bson.M, 0)
		for _, model := range modelList {
			if modelMap, ok := model.(bson.M); ok {
				modelMList = append(modelMList, modelMap)
			}
		}

		data["modelName"] = tools.FormatKeyInfoList(modelMList, "name")
		data["nodeName"] = tools.FormatKeyInfo(nodeInfo, "name")
		data["nodeUid"] = tools.FormatKeyInfo(nodeInfo, "uid")
		data["time"] = tools.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05")

		//fmt.Println("data:", data)
		//break

		logger.Debugf(eventDeviceModifyLog, "开始分析事件")
		if eventID, ok := eventInfo["id"].(primitive.ObjectID); ok {
			if settings, ok := eventInfo["settings"].(primitive.M); ok {
				//departmentConditionList := make([]string, 0)
				//modelConditionList := make([]string, 0)
				departmentListInSettings := make([]primitive.ObjectID, 0)
				modelListInSettings := make([]primitive.ObjectID, 0)
				content := ""
				if contentInSettings, ok := settings["content"].(string); ok {
					content = contentInSettings
				}
				//判断该事件是否指定了特定资产
				if nodeList, ok := settings["node"].(primitive.A); ok {
					for _, nodeEle := range nodeList {
						if nodeMap, ok := nodeEle.(primitive.M); ok {
							if nodeIDInSettings, ok := nodeMap["id"].(string); ok {
								if nodeID == nodeIDInSettings {
									data["content"] = content
									sendMap := data
									b, err := json.Marshal(sendMap)
									if err != nil {
										continue
									}
									err = imqtt.Send(fmt.Sprintf("event/%s", eventID.Hex()), b)
									if err != nil {
										logger.Warnf(eventDeviceModifyLog, "发送事件(%s)错误:%s", eventID.Hex(), err.Error())
									} else {
										logger.Debugf(eventDeviceModifyLog, "发送事件成功:%s,数据为:%+v", eventID.Hex(), sendMap)
									}
									continue
								} else {
									continue
								}
							}
						}
					}
					//if nodeMap, ok := settings["node"].(primitive.M); ok {
					//	if nodeIDInSettings, ok := nodeMap["id"].(string); ok {
					//		if nodeID == nodeIDInSettings {
					//			data["content"] = content
					//			sendMap := map[string]interface{}{
					//				"data": data,
					//			}
					//			err := imqtt.Send(emqttConn, fmt.Sprintf("event/%s", eventID.Hex()), sendMap)
					//			if err != nil {
					//				logger.Warnf(eventDeviceModifyLog, "发送事件(%s)错误:%s", eventID.Hex(), err.Error())
					//			} else {
					//				logger.Debugf(eventDeviceModifyLog, "发送事件成功:%s,数据为:%+v", eventID.Hex(), sendMap)
					//			}
					//			continue
					//		} else {
					//			continue
					//		}
					//	}
					//}
				}
				//没有指定特定资产时，结合部门与模型进行判断
				err = tools.FormatObjectIDPrimitiveList(&settings, "department", "id")
				if err != nil {
					logger.Warnln(eventDeviceModifyLog, "事件配置的部门对象中id字段不存在或类型错误")
					continue
				}
				err = tools.FormatObjectIDPrimitiveList(&settings, "model", "id")
				if err != nil {
					logger.Warnln(eventDeviceModifyLog, "事件配置的部门对象中id字段不存在或类型错误")
					continue
				}
				departmentListInSettings, ok = settings["department"].([]primitive.ObjectID)
				if !ok {
					departmentListInSettings = make([]primitive.ObjectID, 0)
				}
				modelListInSettings, ok = settings["model"].([]primitive.ObjectID)
				if !ok {
					modelListInSettings = make([]primitive.ObjectID, 0)
				}
				isValid := false
				if len(departmentListInSettings) != 0 && len(modelListInSettings) != 0 {
					for _, modelIDInSettings := range modelListInSettings {
						if modelObjectID == modelIDInSettings {
							for _, departmentIDInSettings := range departmentListInSettings {
								for _, departmentID := range departmentObjectIDList {
									if departmentIDInSettings == departmentID {
										isValid = true
									}
								}
							}
						}
					}
				} else if len(departmentListInSettings) != 0 && len(modelListInSettings) == 0 {
					for _, departmentIDInSettings := range departmentListInSettings {
						for _, departmentID := range departmentObjectIDList {
							if departmentIDInSettings == departmentID {
								isValid = true
							}
						}
					}
				} else if len(departmentListInSettings) == 0 && len(modelListInSettings) != 0 {
					for _, modelIDInSettings := range modelListInSettings {
						if modelObjectID == modelIDInSettings {
							isValid = true
						}
					}
				} else {
					continue
				}
				if isValid {
					data["content"] = content
					sendMap := data

					b, err := json.Marshal(sendMap)
					if err != nil {
						continue
					}
					err = imqtt.Send(fmt.Sprintf("event/%s", eventID.Hex()), b)
					if err != nil {
						logger.Warnf(eventDeviceModifyLog, "发送事件(%s)错误:%s", eventID.Hex(), err.Error())
					} else {
						logger.Debugf(eventDeviceModifyLog, "发送事件成功:%s,数据为:%+v", eventID.Hex(), sendMap)
					}
				}
			}
		}
	}

	//logger.Debugf(eventDeviceModifyLog, "资产修改事件触发器执行结束")
	return nil
}
