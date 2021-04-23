package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/air-iot/service/api"
	"github.com/air-iot/service/gin/ginx"
	"github.com/air-iot/service/init/cache/department"
	"github.com/air-iot/service/init/cache/model"
	"github.com/air-iot/service/init/cache/node"
	"github.com/air-iot/service/init/mq"
	"github.com/air-iot/service/util/formatx"
	"github.com/air-iot/service/util/timex"
	"github.com/go-redis/redis/v8"
	"go.mongodb.org/mongo-driver/mongo"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var eventDeviceModifyLog = map[string]interface{}{"name": "模型资产事件触发"}

func TriggerDeviceModify(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client,mq mq.MQ, apiClient api.Client,projectName string,data map[string]interface{}) error {
	////logger.Debugf(eventDeviceModifyLog, "开始执行资产修改事件触发器")
	////logger.Debugf(eventDeviceModifyLog, "传入参数为:%+v", data)
	//ctx, cancel := context.WithTimeout(context.Background(), time.Duration(30)*time.Second)
	//defer cancel()
	headerMap := map[string]string{ginx.XRequestProject:projectName}
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

	//operateDataMap := map[string]interface{}{}
	//operateDataMapRaw, ok := data["data"].(map[string]interface{})
	//if ok {
	//	operateDataMapCustom, ok := operateDataMapRaw["custom"].(map[string]interface{})
	//	if ok {
	//		operateDataMap = operateDataMapCustom
	//	}
	//	//return fmt.Errorf("数据消息中department字段不存在或类型错误")
	//}

	departmentObjectIDList := formatx.InterfaceListToStringList(departmentList)

	modifyType, ok := data["type"].(string)
	if !ok {
		return fmt.Errorf("数据消息中command对象中name字段不存在或类型错误")
	}

	modifyTypeMapping := map[string]string{
		"资产增加":   "增加资产",
		"资产修改":   "修改资产属性",
		"资产删除":   "删除资产",
		"编辑资产画面": "编辑资产画面",
		"删除资产画面": "删除资产画面",
		"新增资产画面": "新增资产画面",
		//"编辑模型":   "编辑模型",
		//"删除模型":   "删除模型",
		//"编辑模型画面": "编辑模型画面",
		//"删除模型画面": "删除模型画面",
		//"新增模型画面": "新增模型画面",
	}

	modifyTypeAfterMapping := modifyTypeMapping[modifyType]

	modelObjectID := modelID
	eventInfoList := make([]bson.M, 0)
	//err = restfulapi.FindPipeline(ctx, idb.Database.Collection("event"), &eventInfoList, pipeline, nil)

	query := map[string]interface{}{
		"filter": map[string]interface{}{
			"type":                DeviceModify,
			"settings.eventType":  modifyTypeAfterMapping,
			"settings.eventRange": "node",
			"$lookups": []map[string]interface{}{
				{
					"from":         "eventhandler",
					"localField":   "_id",
					"foreignField": "event",
					"as":           "handlers",
				},
			},
		},
	}
	err := apiClient.FindEventQuery(headerMap,query, &eventInfoList)

	if err != nil {
		return fmt.Errorf("获取当前资产修改内容类型(%s)的资产修改逻辑事件失败:%s", modelID, err.Error())
	}

	////logger.Debugf(eventDeviceModifyLog, "开始遍历事件列表")
eventloop:
	for _, eventInfo := range eventInfoList {
		//logger.Debugf(eventDeviceModifyLog, "事件信息为:%+v", eventInfo)
		handlers, ok := eventInfo["handlers"].(primitive.A)
		if !ok {
			//logger.Warnln(eventDeviceModifyLog, "handlers字段不存在或类型错误")
			continue
		}
		if len(handlers) == 0 {
			//logger.Warnln(eventDeviceModifyLog, "handlers字段数组长度为0")
			continue
		}


		//logger.Debugf(eventDeviceModifyLog, "开始分析事件")
		if eventID, ok := eventInfo["id"].(string); ok {
			if settings, ok := eventInfo["settings"].(primitive.M); ok {

				//判断是否已经失效
				if invalid, ok := settings["invalid"].(bool); ok {
					if invalid {
						//logger.Warnln(eventLog, "事件(%s)已经失效", eventID)
						continue
					}
				}

				//判断禁用
				if disable, ok := settings["disable"].(bool); ok {
					if disable {
						//logger.Warnln(eventDeviceModifyLog, "事件(%s)已经被禁用", eventID)
						continue
					}
				}

				rangeDefine := ""
				validTime, ok := settings["validTime"].(string)
				if ok {
					if validTime == "timeLimit" {
						if rangeDefine, ok = settings["range"].(string); ok {
							if rangeDefine != "once" {
								//判断有效期
								if startTime, ok := settings["startTime"].(primitive.DateTime); ok {
									startTimeInt := int64(startTime / 1e3)
									if timex.GetLocalTimeNow(time.Now()).Unix() < startTimeInt {
										//logger.Debugf(eventDeviceModifyLog, "事件(%s)的定时任务开始时间未到，不执行", eventID)
										continue
									}
								}

								if endTime, ok := settings["endTime"].(primitive.DateTime); ok {
									endTimeInt := int64(endTime / 1e3)
									if timex.GetLocalTimeNow(time.Now()).Unix() >= endTimeInt {
										//logger.Debugf(eventDeviceModifyLog, "事件(%s)的定时任务结束时间已到，不执行", eventID)
										//修改事件为失效
										updateMap := bson.M{"settings.invalid": true}
										//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
										var r = make(map[string]interface{})
										err := apiClient.UpdateEventById(headerMap,eventID, updateMap, &r)
										if err != nil {
											//logger.Errorf(eventDeviceModifyLog, "失效事件(%s)失败:%s", eventID, err.Error())
											continue
										}
										continue
									}
								}
							}
						}
					}
				}

				//判断事件是否已经触发
				hasExecute := false

				//if modifyTypeAfterMapping == "修改资产属性" {
				//	isValidModifyProp := false
				//	if customProp, ok := settings["customProp"].(string); ok {
				//		if _, ok := operateDataMap[customProp]; ok {
				//			isValidModifyProp = true
				//		}
				//	}
				//	if !isValidModifyProp {
				//		//logger.Warnln(eventDeviceModifyLog, "事件(%s)修改属性不是触发事件需要的:%+v", eventID, operateDataMap)
				//		continue
				//	}
				//}

				content := ""
				if contentInSettings, ok := settings["content"].(string); ok {
					content = contentInSettings
				}
				//departmentConditionList := make([]string, 0)
				//modelConditionList := make([]string, 0)

				isValid := false
				if rangType, ok := settings["eventRange"].(string); ok {
					switch rangType {
					case "node":
						departmentListInSettings := make([]string, 0)
						modelListInSettings := make([]string, 0)
						//判断该事件是否指定了特定资产
						if nodeList, ok := settings["node"].(primitive.A); ok {
							for _, nodeEle := range nodeList {
								if nodeMap, ok := nodeEle.(primitive.M); ok {
									if nodeIDInSettings, ok := nodeMap["id"].(string); ok {
										if nodeID == nodeIDInSettings {
											nodeInfo := bson.M{}
											if initNode, ok := data["initNode"].(map[string]interface{}); ok {
												nodeInfo = initNode
											} else {
												err = node.Get(ctx,redisClient,mongoClient,projectName,nodeID,&nodeInfo)
												if err != nil {
													//logger.Errorf(eventDeviceModifyLog, "事件(%s)的资产缓存(%+v)查询失败", eventID, nodeID)
													nodeInfo = bson.M{}
													continue
												}
											}

											departmentStringIDList := departmentObjectIDList
											departmentMList := make([]map[string]interface{}, 0)
											err := department.GetByList(ctx,redisClient,mongoClient,projectName,departmentStringIDList,&departmentMList)
											if err != nil {
												//logger.Errorf(eventDeviceModifyLog, "事件(%s)的部门缓存(%+v)查询失败", eventID, departmentStringIDList)
												departmentMList = make([]map[string]interface{}, 0)
											}

											data["departmentName"] = formatx.FormatKeyInfoMapList(departmentMList, "name")

											modelEle := bson.M{}
											err = model.Get(ctx,redisClient,mongoClient,projectName,modelID,&modelEle)
											if err != nil {
												//logger.Errorf(eventDeviceModifyLog, "事件(%s)的模型缓存(%+v)查询失败", eventID, departmentStringIDList)
												continue
											}

											modelMList := make([]bson.M, 0)
											modelMList = append(modelMList, modelEle)

											data["modelName"] = formatx.FormatKeyInfoList(modelMList, "name")
											data["nodeName"] = formatx.FormatKeyInfo(nodeInfo, "name")
											data["nodeUid"] = formatx.FormatKeyInfo(nodeInfo, "uid")
											data["time"] = timex.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05")

											data["content"] = content

											switch modifyTypeAfterMapping {
											case "编辑资产画面", "删除资产画面":
												dashboardInfo, ok := data["dashboard"].(map[string]interface{})
												if !ok {
													//logger.Errorf(eventDeviceModifyLog, "数据消息中dashboard字段不存在或类型错误")
													continue
												}
												data["dashboardName"] = formatx.FormatKeyInfo(dashboardInfo, "name")
											}
											sendMap := data

											b, err := json.Marshal(sendMap)
											if err != nil {
												continue
											}
											err = mq.Publish(ctx,[]string{"event",projectName,eventID}, b)
											if err != nil {
												//logger.Warnf(eventDeviceModifyLog, "发送事件(%s)错误:%s", eventID, err.Error())
											} else {
												//logger.Debugf(eventDeviceModifyLog, "发送事件成功:%s,数据为:%+v", eventID, sendMap)
											}
											continue eventloop
										} else {
											continue
										}
									}
								}
							}
						}
						//没有指定特定资产时，结合部门与模型进行判断
						err = formatx.FormatObjectIDPrimitiveList(&settings, "department", "id")
						if err != nil {
							//logger.Warnln(eventDeviceModifyLog, "事件配置的部门对象中id字段不存在或类型错误")
							continue
						}
						err = formatx.FormatObjectIDPrimitiveList(&settings, "model", "id")
						if err != nil {
							//logger.Warnln(eventDeviceModifyLog, "事件配置的部门对象中id字段不存在或类型错误")
							continue
						}
						departmentListInSettings, ok = settings["department"].([]string)
						if !ok {
							departmentListInSettings = make([]string, 0)
						}
						modelListInSettings, ok = settings["model"].([]string)
						if !ok {
							modelListInSettings = make([]string, 0)
						}
						if len(departmentListInSettings) != 0 && len(modelListInSettings) != 0 {
						loop1:
							for _, modelIDInSettings := range modelListInSettings {
								if modelObjectID == modelIDInSettings {
									for _, departmentIDInSettings := range departmentListInSettings {
										for _, departmentID := range departmentObjectIDList {
											if departmentIDInSettings == departmentID {
												isValid = true
												break loop1
											}
										}
									}
								}
							}
						} else if len(departmentListInSettings) != 0 && len(modelListInSettings) == 0 {
						loop2:
							for _, departmentIDInSettings := range departmentListInSettings {
								for _, departmentID := range departmentObjectIDList {
									if departmentIDInSettings == departmentID {
										isValid = true
										break loop2
									}
								}
							}
						} else if len(departmentListInSettings) == 0 && len(modelListInSettings) != 0 {
							for _, modelIDInSettings := range modelListInSettings {
								if modelObjectID == modelIDInSettings {
									isValid = true
									break
								}
							}
						} else {
							continue
						}

					default:
						//logger.Errorf(eventDeviceModifyLog, "事件(%s)的事件范围字段值(%s)未匹配到", eventID, rangType)
						continue
					}
				}
				if isValid {
					nodeInfo := bson.M{}
					if initNode, ok := data["initNode"].(map[string]interface{}); ok {
						nodeInfo = initNode
					} else {
						err = node.Get(ctx,redisClient,mongoClient,projectName,nodeID,&nodeInfo)
						if err != nil {
							//logger.Errorf(eventDeviceModifyLog, "事件(%s)的资产缓存(%+v)查询失败", eventID, nodeID)
							nodeInfo = bson.M{}
							continue
						}
					}

					departmentStringIDList := departmentObjectIDList
					departmentMList := make([]map[string]interface{}, 0)
					err := department.GetByList(ctx,redisClient,mongoClient,projectName,departmentStringIDList,&departmentMList)
					if err != nil {
						//logger.Errorf(eventDeviceModifyLog, "事件(%s)的部门缓存(%+v)查询失败", eventID, departmentStringIDList)
						departmentMList = make([]map[string]interface{}, 0)
					}

					data["departmentName"] = formatx.FormatKeyInfoMapList(departmentMList, "name")

					modelEle := bson.M{}
					err = model.Get(ctx,redisClient,mongoClient,projectName,modelID,&modelEle)
					if err != nil {
						//logger.Errorf(eventDeviceModifyLog, "事件(%s)的模型缓存(%+v)查询失败", eventID, departmentStringIDList)
						continue
					}

					modelMList := make([]bson.M, 0)
					modelMList = append(modelMList, modelEle)

					data["modelName"] = formatx.FormatKeyInfoList(modelMList, "name")
					data["nodeName"] = formatx.FormatKeyInfo(nodeInfo, "name")
					data["nodeUid"] = formatx.FormatKeyInfo(nodeInfo, "uid")
					data["time"] = timex.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05")

					data["content"] = content

					switch modifyTypeAfterMapping {
					case "编辑资产画面", "删除资产画面":
						dashboardInfo, ok := data["dashboard"].(map[string]interface{})
						if !ok {
							//logger.Errorf(eventDeviceModifyLog, "数据消息中dashboard字段不存在或类型错误")
							continue
						}
						data["dashboardName"] = formatx.FormatKeyInfo(dashboardInfo, "name")
					}
					sendMap := data

					b, err := json.Marshal(sendMap)
					if err != nil {
						continue
					}
					err = mq.Publish(ctx,[]string{"event",projectName,eventID}, b)
					if err != nil {
						//logger.Warnf(eventDeviceModifyLog, "发送事件(%s)错误:%s", eventID, err.Error())
					} else {
						//logger.Debugf(eventDeviceModifyLog, "发送事件成功:%s,数据为:%+v", eventID, sendMap)
					}

					hasExecute = true
				}

				//对只能执行一次的事件进行失效
				if validTime == "timeLimit" {
					if rangeDefine == "once" && hasExecute {
						//logger.Warnln(eventDeviceModifyLog, "事件(%s)为只执行一次的事件", eventID)
						//修改事件为失效
						updateMap := bson.M{"settings.invalid": true}
						//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
						var r = make(map[string]interface{})
						err := apiClient.UpdateEventById(headerMap,eventID, updateMap, &r)
						if err != nil {
							//logger.Errorf(eventDeviceModifyLog, "失效事件(%s)失败:%s", eventID, err.Error())
							continue
						}
					}
				}
			}
		}
	}

	////logger.Debugf(eventDeviceModifyLog, "资产修改事件触发器执行结束")
	return nil
}

func TriggerModelModify(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client,mq mq.MQ, apiClient api.Client,projectName string,data map[string]interface{}) error {
	////logger.Debugf(eventDeviceModifyLog, "开始执行资产修改事件触发器")
	////logger.Debugf(eventDeviceModifyLog, "传入参数为:%+v", data)
	//ctx, cancel := context.WithTimeout(context.Background(), time.Duration(30)*time.Second)
	//defer cancel()

	//nodeID, ok := data["node"].(string)
	//if !ok {
	//	return fmt.Errorf("数据消息中nodeId字段不存在或类型错误")
	//}
	headerMap := map[string]string{ginx.XRequestProject:projectName}
	modelID, ok := data["model"].(string)
	if !ok {
		return fmt.Errorf("数据消息中model字段不存在或类型错误")
	}

	departmentList, ok := data["department"].([]interface{})
	if !ok {
		return fmt.Errorf("数据消息中department字段不存在或类型错误")
	}

	//operateDataMap := map[string]interface{}{}
	//operateDataMapRaw, ok := data["data"].(map[string]interface{})
	//if ok {
	//	operateDataMapCustom, ok := operateDataMapRaw["custom"].(map[string]interface{})
	//	if ok {
	//		operateDataMap = operateDataMapCustom
	//	}
	//	//return fmt.Errorf("数据消息中department字段不存在或类型错误")
	//}

	departmentObjectIDList  := formatx.InterfaceListToStringList(departmentList)

	modifyType, ok := data["type"].(string)
	if !ok {
		return fmt.Errorf("数据消息中command对象中name字段不存在或类型错误")
	}

	modifyTypeMapping := map[string]string{
		//"资产增加":   "增加资产",
		//"资产修改":   "修改资产属性",
		//"资产删除":   "删除资产",
		//"编辑资产画面": "编辑资产画面",
		//"删除资产画面": "删除资产画面",
		"模型修改":   "编辑模型",
		"模型删除":   "删除模型",
		"编辑模型画面": "编辑模型画面",
		"删除模型画面": "删除模型画面",
		"新增模型画面": "新增模型画面",
	}

	modifyTypeAfterMapping := modifyTypeMapping[modifyType]

	modelObjectID := modelID

	////logger.Debugf(eventDeviceModifyLog, "开始获取当前资产修改内容类型的资产修改逻辑事件")
	//获取当前资产修改内容类型的资产修改逻辑事件=============================================
	//paramMatch := bson.D{
	//	bson.E{
	//		Key: "$match",
	//		Value: bson.M{
	//			"type":                DeviceModify,
	//			"settings.eventType":  modifyTypeAfterMapping,
	//			"settings.eventRange": "model",
	//			//"$or":
	//			//bson.A{
	//			//	bson.D{{
	//			//		"settings.node.id", nodeID,
	//			//	}},
	//			//	bson.D{
	//			//		{
	//			//			"settings.department.id", bson.M{"$in": departmentList},
	//			//		},
	//			//		{
	//			//			"settings.model.id", modelID,
	//			//		},
	//			//	},
	//			//	bson.D{
	//			//		{
	//			//			"settings.department.id", bson.M{"$in": departmentList},
	//			//		},
	//			//	},
	//			//	bson.D{
	//			//		{
	//			//			"settings.model.id", modelID,
	//			//		},
	//			//	},
	//			//},
	//		},
	//	},
	//}
	//paramLookup := bson.D{
	//	bson.E{
	//		Key: "$lookup",
	//		Value: bson.M{
	//			"from":         "eventhandler",
	//			"localField":   "_id",
	//			"foreignField": "event",
	//			"as":           "handlers",
	//		},
	//	},
	//}

	//pipeline := mongo.Pipeline{}
	//pipeline = append(pipeline, paramMatch, paramLookup)
	eventInfoList := make([]bson.M, 0)
	//err = restfulapi.FindPipeline(ctx, idb.Database.Collection("event"), &eventInfoList, pipeline, nil)

	query := map[string]interface{}{
		"filter": map[string]interface{}{
			"type":                DeviceModify,
			"settings.eventType":  modifyTypeAfterMapping,
			"settings.eventRange": "model",
			"$lookups": []map[string]interface{}{
				{
					"from":         "eventhandler",
					"localField":   "_id",
					"foreignField": "event",
					"as":           "handlers",
				},
			},
		},
	}

	err := apiClient.FindEventQuery(headerMap,query, &eventInfoList)
	if err != nil {
		//logger.Errorf(eventDeviceModifyLog, "获取当前资产修改内容类型(%s)的资产修改逻辑事件失败:%s", modelID, err.Error())
		return fmt.Errorf("获取当前资产修改内容类型(%s)的资产修改逻辑事件失败:%s", modelID, err.Error())
	}

	////logger.Debugf(eventDeviceModifyLog, "开始遍历事件列表")
	for _, eventInfo := range eventInfoList {
		//logger.Debugf(eventDeviceModifyLog, "事件信息为:%+v", eventInfo)
		handlers, ok := eventInfo["handlers"].(primitive.A)
		if !ok {
			//logger.Warnln(eventDeviceModifyLog, "handlers字段不存在或类型错误")
			continue
		}
		if len(handlers) == 0 {
			//logger.Warnln(eventDeviceModifyLog, "handlers字段数组长度为0")
			continue
		}

		//paramMatch = bson.D{
		//	bson.E{
		//		Key: "$match",
		//		Value: bson.M{
		//			"_id": nodeObjectID,
		//		},
		//	},
		//}
		//paramLookup = bson.D{
		//	bson.E{
		//		Key: "$lookup",
		//		Value: bson.M{
		//			"from":         "dept",
		//			"localField":   "department",
		//			"foreignField": "_id",
		//			"as":           "department",
		//		},
		//	},
		//}
		//paramLookupModel := bson.D{
		//	bson.E{
		//		Key: "$lookup",
		//		Value: bson.M{
		//			"from":         "model",
		//			"localField":   "model",
		//			"foreignField": "_id",
		//			"as":           "model",
		//		},
		//	},
		//}
		//pipeline = mongo.Pipeline{}
		//pipeline = append(pipeline, paramMatch, paramLookup, paramLookupModel)
		//nodeInfoList := make([]bson.M, 0)
		//err = restfulapi.FindPipeline(ctx, idb.Database.Collection("node"), &nodeInfoList, pipeline, nil)
		//if err != nil {
		//	 fmt.Errorf("获取当前资产(%s)的详情失败:%s", nodeID, err.Error())
		//}
		//
		//if len(nodeInfoList) == 0 {
		//	 fmt.Errorf("当前查询的资产(%s)不存在", nodeID)
		//}
		//
		//nodeInfo := nodeInfoList[0]
		//
		//departmentList, ok := nodeInfo["department"].(primitive.A)
		//if !ok {
		//	//logger.Warnln(eventExecCmdLog, "资产(%s)的department字段不存在或类型错误", nodeID)
		//	continue
		//}
		//departmentMList := make([]bson.M, 0)
		//for _, department := range departmentList {
		//	if departmentMap, ok := department.(bson.M); ok {
		//		departmentMList = append(departmentMList, departmentMap)
		//	}
		//}
		//
		//data["departmentName"] = formatx.FormatKeyInfoList(departmentMList, "name")
		//
		//modelList, ok := nodeInfo["model"].(primitive.A)
		//if !ok {
		//	//logger.Warnln(eventExecCmdLog, "资产(%s)的model字段不存在或类型错误", nodeID)
		//	continue
		//}
		//modelMList := make([]bson.M, 0)
		//for _, model := range modelList {
		//	if modelMap, ok := model.(bson.M); ok {
		//		modelMList = append(modelMList, modelMap)
		//	}
		//}
		//
		//data["modelName"] = formatx.FormatKeyInfoList(modelMList, "name")
		//data["nodeName"] = formatx.FormatKeyInfo(nodeInfo, "name")
		//data["nodeUid"] = formatx.FormatKeyInfo(nodeInfo, "uid")
		//data["time"] = timex.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05")

		//fmt.Println("data:", data)
		//break

		//logger.Debugf(eventDeviceModifyLog, "开始分析事件")
		if eventID, ok := eventInfo["id"].(string); ok {
			if settings, ok := eventInfo["settings"].(primitive.M); ok {

				//判断是否已经失效
				if invalid, ok := settings["invalid"].(bool); ok {
					if invalid {
						//logger.Warnln(eventLog, "事件(%s)已经失效", eventID)
						continue
					}
				}

				//判断禁用
				if disable, ok := settings["disable"].(bool); ok {
					if disable {
						//logger.Warnln(eventDeviceModifyLog, "事件(%s)已经被禁用", eventID)
						continue
					}
				}

				rangeDefine := ""
				validTime, ok := settings["validTime"].(string)
				if ok {
					if validTime == "timeLimit" {
						if rangeDefine, ok = settings["range"].(string); ok {
							if rangeDefine != "once" {
								//判断有效期
								if startTime, ok := settings["startTime"].(primitive.DateTime); ok {
									startTimeInt := int64(startTime / 1e3)
									if timex.GetLocalTimeNow(time.Now()).Unix() < startTimeInt {
										//logger.Debugf(eventDeviceModifyLog, "事件(%s)的定时任务开始时间未到，不执行", eventID)
										continue
									}
								}

								if endTime, ok := settings["endTime"].(primitive.DateTime); ok {
									endTimeInt := int64(endTime / 1e3)
									if timex.GetLocalTimeNow(time.Now()).Unix() >= endTimeInt {
										//logger.Debugf(eventDeviceModifyLog, "事件(%s)的定时任务结束时间已到，不执行", eventID)
										//修改事件为失效
										updateMap := bson.M{"settings.invalid": true}
										//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
										var r = make(map[string]interface{})
										err := apiClient.UpdateEventById(headerMap,eventID, updateMap, &r)
										if err != nil {
											//logger.Errorf(eventDeviceModifyLog, "失效事件(%s)失败:%s", eventID, err.Error())
											continue
										}
										continue
									}
								}
							}
						}
					}
				}

				//判断事件是否已经触发
				hasExecute := false

				content := ""
				if contentInSettings, ok := settings["content"].(string); ok {
					content = contentInSettings
				}
				//departmentConditionList := make([]string, 0)
				//modelConditionList := make([]string, 0)

				isValid := false
				if rangType, ok := settings["eventRange"].(string); ok {
					switch rangType {
					case "model":
						modelListInSettings := make([]string, 0)
						//没有指定特定资产时，结合部门与模型进行判断
						err = formatx.FormatObjectIDPrimitiveList(&settings, "model", "id")
						if err != nil {
							//logger.Warnln(eventDeviceModifyLog, "事件配置的模型对象中id字段不存在或类型错误")
							continue
						}
						modelListInSettings, ok = settings["model"].([]string)
						if !ok {
							modelListInSettings = make([]string, 0)
						}
						for _, modelIDInSettings := range modelListInSettings {
							if modelObjectID == modelIDInSettings {
								isValid = true
								break
							}
						}
					default:
						//logger.Errorf(eventDeviceModifyLog, "事件(%s)的事件范围字段值(%s)未匹配到", eventID, rangType)
						continue
					}
				}
				if isValid {
					departmentStringIDList := departmentObjectIDList
					departmentMList := make([]map[string]interface{}, 0)
					err := department.GetByList(ctx,redisClient,mongoClient,projectName,departmentStringIDList,&departmentMList)
					if err != nil {
						//logger.Errorf(eventDeviceModifyLog, "事件(%s)的部门缓存(%+v)查询失败", eventID, departmentStringIDList)
						departmentMList = make([]map[string]interface{}, 0)
					}

					data["departmentName"] = formatx.FormatKeyInfoMapList(departmentMList, "name")

					modelEle := bson.M{}
					err = model.Get(ctx,redisClient,mongoClient,projectName,modelID,&modelEle)
					if err != nil {
						//logger.Errorf(eventDeviceModifyLog, "事件(%s)的模型缓存(%+v)查询失败", eventID, departmentStringIDList)
						continue
					}

					modelMList := make([]bson.M, 0)
					modelMList = append(modelMList, modelEle)

					data["modelName"] = formatx.FormatKeyInfoList(modelMList, "name")
					//data["nodeName"] = formatx.FormatKeyInfo(nodeInfo, "name")
					//data["nodeUid"] = formatx.FormatKeyInfo(nodeInfo, "uid")
					data["time"] = timex.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05")

					data["content"] = content
					switch modifyTypeAfterMapping {
					case "编辑模型画面", "新增模型画面", "删除模型画面":
						dashboardInfo, ok := data["dashboard"].(map[string]interface{})
						if !ok {
							fmt.Errorf("数据消息中dashboard字段不存在或类型错误")
						}
						data["dashboardName"] = formatx.FormatKeyInfo(dashboardInfo, "name")
					}
					sendMap := data

					b, err := json.Marshal(sendMap)
					if err != nil {
						continue
					}
					err = mq.Publish(ctx,[]string{"event",projectName,eventID}, b)
					if err != nil {
						//logger.Warnf(eventDeviceModifyLog, "发送事件(%s)错误:%s", eventID, err.Error())
					} else {
						//logger.Debugf(eventDeviceModifyLog, "发送事件成功:%s,数据为:%+v", eventID, sendMap)
					}

					hasExecute = true
				}

				//对只能执行一次的事件进行失效
				if validTime == "timeLimit" {
					if rangeDefine == "once" && hasExecute {
						//logger.Warnln(eventDeviceModifyLog, "事件(%s)为只执行一次的事件", eventID)
						//修改事件为失效
						updateMap := bson.M{"settings.invalid": true}
						//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
						var r = make(map[string]interface{})
						err := apiClient.UpdateEventById(headerMap,eventID, updateMap, &r)
						if err != nil {
							//logger.Errorf(eventDeviceModifyLog, "失效事件(%s)失败:%s", eventID, err.Error())
							continue
						}
					}
				}
			}
		}
	}

	////logger.Debugf(eventDeviceModifyLog, "资产修改事件触发器执行结束")
	return nil
}
