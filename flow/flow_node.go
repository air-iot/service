package flow

import (
	"context"
	"fmt"
	"github.com/air-iot/service/logger"
	"github.com/air-iot/service/util/flowx"
	"github.com/camunda-cloud/zeebe/clients/go/pkg/zbc"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/gin/ginx"
	"github.com/air-iot/service/init/cache/department"
	"github.com/air-iot/service/init/cache/model"
	"github.com/air-iot/service/init/cache/node"
	"github.com/air-iot/service/init/mq"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/util/formatx"
	"github.com/air-iot/service/util/timex"
)

var flowDeviceModifyLog = map[string]interface{}{"name": "模型资产流程触发"}

func TriggerDeviceModifyFlow(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, mq mq.MQ, apiClient api.Client, zbClient zbc.Client, projectName string, data map[string]interface{}) error {
	////logger.Debugf(eventDeviceModifyLog, "开始执行资产修改流程触发器")
	////logger.Debugf(eventDeviceModifyLog, "传入参数为:%+v", data)
	//ctx, cancel := context.WithTimeout(context.Background(), time.Duration(30)*time.Second)
	//defer cancel()
	headerMap := map[string]string{ginx.XRequestProject: projectName}
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
	flowInfoList := make([]bson.M, 0)
	//err = restfulapi.FindPipeline(ctx, idb.Database.Collection("event"), &eventInfoList, pipeline, nil)

	query := map[string]interface{}{
		"filter": map[string]interface{}{
			"type":               DeviceModify,
			"settings.flowType":  modifyTypeAfterMapping,
			"settings.flowRange": "node",
		},
	}
	err := apiClient.FindFlowQuery(headerMap, query, &flowInfoList)

	if err != nil {
		return fmt.Errorf("获取当前资产修改内容类型(%s)的资产修改逻辑流程失败:%s", modelID, err.Error())
	}

	////logger.Debugf(eventDeviceModifyLog, "开始遍历流程列表")
flowloop:
	for _, flowInfo := range flowInfoList {

		//logger.Debugf(eventDeviceModifyLog, "开始分析流程")
		if flowID, ok := flowInfo["id"].(string); ok {
			if settings, ok := flowInfo["settings"].(primitive.M); ok {

				//判断是否已经失效
				if invalid, ok := settings["invalid"].(bool); ok {
					if invalid {
						//logger.Warnln(eventLog, "流程(%s)已经失效", eventID)
						continue
					}
				}

				//判断禁用
				if disable, ok := settings["disable"].(bool); ok {
					if disable {
						//logger.Warnln(eventDeviceModifyLog, "流程(%s)已经被禁用", eventID)
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
										//logger.Debugf(eventDeviceModifyLog, "流程(%s)的定时任务开始时间未到，不执行", eventID)
										continue
									}
								}

								if endTime, ok := settings["endTime"].(primitive.DateTime); ok {
									endTimeInt := int64(endTime / 1e3)
									if timex.GetLocalTimeNow(time.Now()).Unix() >= endTimeInt {
										//logger.Debugf(eventDeviceModifyLog, "流程(%s)的定时任务结束时间已到，不执行", eventID)
										//修改流程为失效
										updateMap := bson.M{"settings.invalid": true}
										//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
										var r = make(map[string]interface{})
										err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
										if err != nil {
											//logger.Errorf(eventDeviceModifyLog, "失效流程(%s)失败:%s", eventID, err.Error())
											continue
										}
										continue
									}
								}
							}
						}
					}
				}

				//判断流程是否已经触发
				hasExecute := false

				//if modifyTypeAfterMapping == "修改资产属性" {
				//	isValidModifyProp := false
				//	if customProp, ok := settings["customProp"].(string); ok {
				//		if _, ok := operateDataMap[customProp]; ok {
				//			isValidModifyProp = true
				//		}
				//	}
				//	if !isValidModifyProp {
				//		//logger.Warnln(eventDeviceModifyLog, "流程(%s)修改属性不是触发流程需要的:%+v", eventID, operateDataMap)
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
						//判断该流程是否指定了特定资产
						if nodeList, ok := settings["node"].(primitive.A); ok {
							for _, nodeEle := range nodeList {
								if nodeMap, ok := nodeEle.(primitive.M); ok {
									if nodeIDInSettings, ok := nodeMap["id"].(string); ok {
										if nodeID == nodeIDInSettings {
											nodeInfo := bson.M{}
											if initNode, ok := data["initNode"].(map[string]interface{}); ok {
												nodeInfo = initNode
											} else {
												err = node.Get(ctx, redisClient, mongoClient, projectName, nodeID, &nodeInfo)
												if err != nil {
													//logger.Errorf(eventDeviceModifyLog, "流程(%s)的资产缓存(%+v)查询失败", eventID, nodeID)
													nodeInfo = bson.M{}
													continue
												}
											}

											departmentStringIDList := departmentObjectIDList
											departmentMList := make([]map[string]interface{}, 0)
											err := department.GetByList(ctx, redisClient, mongoClient, projectName, departmentStringIDList, &departmentMList)
											if err != nil {
												//logger.Errorf(eventDeviceModifyLog, "流程(%s)的部门缓存(%+v)查询失败", eventID, departmentStringIDList)
												departmentMList = make([]map[string]interface{}, 0)
											}

											data["departmentName"] = formatx.FormatKeyInfoMapList(departmentMList, "name")

											modelEle := bson.M{}
											err = model.Get(ctx, redisClient, mongoClient, projectName, modelID, &modelEle)
											if err != nil {
												//logger.Errorf(eventDeviceModifyLog, "流程(%s)的模型缓存(%+v)查询失败", eventID, departmentStringIDList)
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
													if dashboardInfoID, ok := data["dashboard"].(string); ok {
														data["$#dashboard"] = bson.M{dashboardInfoID: bson.M{"id": dashboardInfoID, "_tableName": "dashboard"}}
													} else {
														continue
													}
													//logger.Errorf(eventDeviceModifyLog, "数据消息中dashboard字段不存在或类型错误")
												} else {
													data["dashboardName"] = formatx.FormatKeyInfo(dashboardInfo, "name")
													if dashboardInfoID, ok := dashboardInfo["id"].(string); ok {
														data["$#dashboard"] = bson.M{dashboardInfoID: bson.M{"id": dashboardInfoID, "_tableName": "dashboard"}}
													}
												}
											}

											deptMap := bson.M{}
											for _, id := range departmentStringIDList {
												deptMap[id] = bson.M{"id": id, "_tableName": "dept"}
											}
											data["$#department"] = deptMap
											data["$#model"] = bson.M{modelID: bson.M{"id": modelID, "_tableName": "model"}}
											data["$#node"] = bson.M{nodeID: bson.M{"id": nodeID, "_tableName": "node"}}
											if loginTimeRaw, ok := data["time"].(string); ok {
												loginTime, err := timex.ConvertStringToTime("2006-01-02 15:04:05", loginTimeRaw, time.Local)
												if err != nil {
													continue
												}
												data["time"] = loginTime.UnixNano() / 1e6
											}

											if flowXml, ok := flowInfo["flowXml"].(string); ok {
												err = flowx.StartFlow(zbClient, flowXml, projectName, data)
												if err != nil {
													logger.Errorf("流程推进到下一阶段失败:%s", err.Error())
												}
											} else {
												logger.Errorf("流程(%s)的xml不存在或类型错误", flowID)
											}

											continue flowloop
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
							//logger.Warnln(eventDeviceModifyLog, "流程配置的部门对象中id字段不存在或类型错误")
							continue
						}
						err = formatx.FormatObjectIDPrimitiveList(&settings, "model", "id")
						if err != nil {
							//logger.Warnln(eventDeviceModifyLog, "流程配置的部门对象中id字段不存在或类型错误")
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
						//logger.Errorf(eventDeviceModifyLog, "流程(%s)的流程范围字段值(%s)未匹配到", eventID, rangType)
						continue
					}
				}
				if isValid {
					nodeInfo := bson.M{}
					if initNode, ok := data["initNode"].(map[string]interface{}); ok {
						nodeInfo = initNode
					} else {
						err = node.Get(ctx, redisClient, mongoClient, projectName, nodeID, &nodeInfo)
						if err != nil {
							//logger.Errorf(eventDeviceModifyLog, "流程(%s)的资产缓存(%+v)查询失败", eventID, nodeID)
							nodeInfo = bson.M{}
							continue
						}
					}

					departmentStringIDList := departmentObjectIDList
					departmentMList := make([]map[string]interface{}, 0)
					err := department.GetByList(ctx, redisClient, mongoClient, projectName, departmentStringIDList, &departmentMList)
					if err != nil {
						//logger.Errorf(eventDeviceModifyLog, "流程(%s)的部门缓存(%+v)查询失败", eventID, departmentStringIDList)
						departmentMList = make([]map[string]interface{}, 0)
					}

					data["departmentName"] = formatx.FormatKeyInfoMapList(departmentMList, "name")

					modelEle := bson.M{}
					err = model.Get(ctx, redisClient, mongoClient, projectName, modelID, &modelEle)
					if err != nil {
						//logger.Errorf(eventDeviceModifyLog, "流程(%s)的模型缓存(%+v)查询失败", eventID, departmentStringIDList)
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
							if dashboardInfoID, ok := data["dashboard"].(string); ok {
								data["$#dashboard"] = bson.M{dashboardInfoID: bson.M{"id": dashboardInfoID, "_tableName": "dashboard"}}
							} else {
								continue
							}
							//logger.Errorf(eventDeviceModifyLog, "数据消息中dashboard字段不存在或类型错误")
						} else {
							data["dashboardName"] = formatx.FormatKeyInfo(dashboardInfo, "name")
							if dashboardInfoID, ok := dashboardInfo["id"].(string); ok {
								data["$#dashboard"] = bson.M{dashboardInfoID: bson.M{"id": dashboardInfoID, "_tableName": "dashboard"}}
							}
						}
					}

					deptMap := bson.M{}
					for _, id := range departmentStringIDList {
						deptMap[id] = bson.M{"id": id, "_tableName": "dept"}
					}
					data["$#department"] = deptMap
					data["$#model"] = bson.M{modelID: bson.M{"id": modelID, "_tableName": "model"}}
					data["$#node"] = bson.M{nodeID: bson.M{"id": nodeID, "_tableName": "node"}}
					if loginTimeRaw, ok := data["time"].(string); ok {
						loginTime, err := timex.ConvertStringToTime("2006-01-02 15:04:05", loginTimeRaw, time.Local)
						if err != nil {
							continue
						}
						data["time"] = loginTime.UnixNano() / 1e6
					}

					if flowXml, ok := flowInfo["flowXml"].(string); ok {
						err = flowx.StartFlow(zbClient, flowXml, projectName, data)
						if err != nil {
							logger.Errorf("流程推进到下一阶段失败:%s", err.Error())
							continue
						}
					} else {
						logger.Errorf("流程(%s)的xml不存在或类型错误", flowID)
						continue
					}
					hasExecute = true
				}

				//对只能执行一次的流程进行失效
				if validTime == "timeLimit" {
					if rangeDefine == "once" && hasExecute {
						//logger.Warnln(eventDeviceModifyLog, "流程(%s)为只执行一次的流程", eventID)
						//修改流程为失效
						updateMap := bson.M{"settings.invalid": true}
						//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
						var r = make(map[string]interface{})
						err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
						if err != nil {
							//logger.Errorf(eventDeviceModifyLog, "失效流程(%s)失败:%s", eventID, err.Error())
							continue
						}
					}
				}
			}
		}
	}

	////logger.Debugf(eventDeviceModifyLog, "资产修改流程触发器执行结束")
	return nil
}

func TriggerModelModifyFlow(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, mq mq.MQ, apiClient api.Client, zbClient zbc.Client, projectName string, data map[string]interface{}) error {
	////logger.Debugf(eventDeviceModifyLog, "开始执行资产修改流程触发器")
	////logger.Debugf(eventDeviceModifyLog, "传入参数为:%+v", data)
	//ctx, cancel := context.WithTimeout(context.Background(), time.Duration(30)*time.Second)
	//defer cancel()

	//nodeID, ok := data["node"].(string)
	//if !ok {
	//	return fmt.Errorf("数据消息中nodeId字段不存在或类型错误")
	//}
	headerMap := map[string]string{ginx.XRequestProject: projectName}
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

	////logger.Debugf(eventDeviceModifyLog, "开始获取当前资产修改内容类型的资产修改逻辑流程")
	//获取当前资产修改内容类型的资产修改逻辑流程=============================================
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
	flowInfoList := make([]bson.M, 0)
	//err = restfulapi.FindPipeline(ctx, idb.Database.Collection("event"), &eventInfoList, pipeline, nil)

	query := map[string]interface{}{
		"filter": map[string]interface{}{
			"type":               DeviceModify,
			"settings.flowType":  modifyTypeAfterMapping,
			"settings.flowRange": "model",
		},
	}

	err := apiClient.FindFlowQuery(headerMap, query, &flowInfoList)
	if err != nil {
		//logger.Errorf(eventDeviceModifyLog, "获取当前资产修改内容类型(%s)的资产修改逻辑流程失败:%s", modelID, err.Error())
		return fmt.Errorf("获取当前资产修改内容类型(%s)的资产修改逻辑流程失败:%s", modelID, err.Error())
	}

	////logger.Debugf(eventDeviceModifyLog, "开始遍历流程列表")
	for _, flowInfo := range flowInfoList {

		//fmt.Println("data:", data)
		//break

		//logger.Debugf(eventDeviceModifyLog, "开始分析流程")
		if flowID, ok := flowInfo["id"].(string); ok {
			if settings, ok := flowInfo["settings"].(primitive.M); ok {

				//判断是否已经失效
				if invalid, ok := settings["invalid"].(bool); ok {
					if invalid {
						//logger.Warnln(eventLog, "流程(%s)已经失效", eventID)
						continue
					}
				}

				//判断禁用
				if disable, ok := settings["disable"].(bool); ok {
					if disable {
						//logger.Warnln(eventDeviceModifyLog, "流程(%s)已经被禁用", eventID)
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
										//logger.Debugf(eventDeviceModifyLog, "流程(%s)的定时任务开始时间未到，不执行", eventID)
										continue
									}
								}

								if endTime, ok := settings["endTime"].(primitive.DateTime); ok {
									endTimeInt := int64(endTime / 1e3)
									if timex.GetLocalTimeNow(time.Now()).Unix() >= endTimeInt {
										//logger.Debugf(eventDeviceModifyLog, "流程(%s)的定时任务结束时间已到，不执行", eventID)
										//修改流程为失效
										updateMap := bson.M{"settings.invalid": true}
										//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
										var r = make(map[string]interface{})
										err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
										if err != nil {
											//logger.Errorf(eventDeviceModifyLog, "失效流程(%s)失败:%s", eventID, err.Error())
											continue
										}
										continue
									}
								}
							}
						}
					}
				}

				//判断流程是否已经触发
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
							//logger.Warnln(eventDeviceModifyLog, "流程配置的模型对象中id字段不存在或类型错误")
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
						//logger.Errorf(eventDeviceModifyLog, "流程(%s)的流程范围字段值(%s)未匹配到", eventID, rangType)
						continue
					}
				}
				if isValid {
					departmentStringIDList := departmentObjectIDList
					departmentMList := make([]map[string]interface{}, 0)
					err := department.GetByList(ctx, redisClient, mongoClient, projectName, departmentStringIDList, &departmentMList)
					if err != nil {
						//logger.Errorf(eventDeviceModifyLog, "流程(%s)的部门缓存(%+v)查询失败", eventID, departmentStringIDList)
						departmentMList = make([]map[string]interface{}, 0)
					}

					data["departmentName"] = formatx.FormatKeyInfoMapList(departmentMList, "name")

					modelEle := bson.M{}
					err = model.Get(ctx, redisClient, mongoClient, projectName, modelID, &modelEle)
					if err != nil {
						//logger.Errorf(eventDeviceModifyLog, "流程(%s)的模型缓存(%+v)查询失败", eventID, departmentStringIDList)
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
							if dashboardInfoID, ok := data["dashboard"].(string); ok {
								data["$#dashboard"] = bson.M{dashboardInfoID: bson.M{"id": dashboardInfoID, "_tableName": "dashboard"}}
							} else {
								continue
							}
							//logger.Errorf(eventDeviceModifyLog, "数据消息中dashboard字段不存在或类型错误")
						} else {
							data["dashboardName"] = formatx.FormatKeyInfo(dashboardInfo, "name")
							if dashboardInfoID, ok := dashboardInfo["id"].(string); ok {
								data["$#dashboard"] = bson.M{dashboardInfoID: bson.M{"id": dashboardInfoID, "_tableName": "dashboard"}}
							}
						}
					}

					deptMap := bson.M{}
					for _, id := range departmentStringIDList {
						deptMap[id] = bson.M{"id": id, "_tableName": "dept"}
					}
					data["$#department"] = deptMap
					data["$#model"] = bson.M{modelID: bson.M{"id": modelID, "_tableName": "model"}}
					if loginTimeRaw, ok := data["time"].(string); ok {
						loginTime, err := timex.ConvertStringToTime("2006-01-02 15:04:05", loginTimeRaw, time.Local)
						if err != nil {
							continue
						}
						data["time"] = loginTime.UnixNano() / 1e6
					}

					if flowXml, ok := flowInfo["flowXml"].(string); ok {
						err = flowx.StartFlow(zbClient, flowXml, projectName, data)
						if err != nil {
							logger.Errorf("流程推进到下一阶段失败:%s", err.Error())
							continue
						}
					} else {
						logger.Errorf("流程(%s)的xml不存在或类型错误", flowID)
						continue
					}
					hasExecute = true
				}

				//对只能执行一次的流程进行失效
				if validTime == "timeLimit" {
					if rangeDefine == "once" && hasExecute {
						//logger.Warnln(eventDeviceModifyLog, "流程(%s)为只执行一次的流程", eventID)
						//修改流程为失效
						updateMap := bson.M{"settings.invalid": true}
						//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
						var r = make(map[string]interface{})
						err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
						if err != nil {
							//logger.Errorf(eventDeviceModifyLog, "失效流程(%s)失败:%s", eventID, err.Error())
							continue
						}
					}
				}
			}
		}
	}

	////logger.Debugf(eventDeviceModifyLog, "资产修改流程触发器执行结束")
	return nil
}
