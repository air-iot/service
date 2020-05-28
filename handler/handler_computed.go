package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/air-iot/service/restful-api"
	"time"

	idb "github.com/air-iot/service/db/mongo"
	iredis "github.com/air-iot/service/db/redis"
	"github.com/air-iot/service/logger"
	clogic "github.com/air-iot/service/logic"
	cmodel "github.com/air-iot/service/model"
	imqtt "github.com/air-iot/service/mq/mqtt"
	"github.com/air-iot/service/tools"
	"github.com/go-redis/redis"
	"go.mongodb.org/mongo-driver/bson"
)

var eventComputeLogicLog = map[string]interface{}{"name": "数据事件触发"}

func TriggerComputed(data cmodel.DataMessage) error {
	//logger.Debugf(eventComputeLogicLog, "开始执行计算事件触发器")
	//logger.Debugf(eventComputeLogicLog, "传入参数为:%+v", data)

	nodeID := data.NodeID
	if nodeID == "" {
		logger.Errorf(eventComputeLogicLog, fmt.Sprintf("数据消息中nodeId字段不存在或类型错误"))
		return fmt.Errorf("数据消息中nodeId字段不存在或类型错误")
	}

	modelID := data.ModelID
	if nodeID == "" {
		logger.Errorf(eventComputeLogicLog, fmt.Sprintf("数据消息中modelId字段不存在或类型错误"))
		return fmt.Errorf("数据消息中modelId字段不存在或类型错误")
	}

	inputMap := data.InputMap

	fieldsMap := data.Fields
	if len(fieldsMap) == 0 {
		logger.Errorf(eventComputeLogicLog, fmt.Sprintf("数据消息中fields字段不存在或类型错误"))
		return fmt.Errorf("数据消息中fields字段不存在或类型错误")
	}
	//logger.Debugf(eventComputeLogicLog, "开始获取当前模型的数据事件")
	//获取当前模型的数据事件=============================================
	eventInfoList, err := clogic.EventLogic.FindLocalCacheByType(string(ComputeLogic))
	if err != nil {
		logger.Debugf(eventComputeLogicLog, fmt.Sprintf("获取当前模型(%s)的数据事件失败:%s", modelID, err.Error()))
		return fmt.Errorf("获取当前模型(%s)的数据事件失败:%s", modelID, err.Error())
	}

	nodeInfo, err := clogic.NodeLogic.FindLocalCache(nodeID)
	if err != nil {
		logger.Errorf(eventComputeLogicLog, fmt.Sprintf("获取当前模型(%s)的资产(%s)失败:%s", modelID, nodeID, err.Error()))
		return fmt.Errorf("获取当前模型(%s)的资产(%s)失败:%s", modelID, nodeID, err.Error())
	}

	data.Uid = nodeInfo.Uid
	nodeUIDInData := data.Uid
	if nodeUIDInData == "" {
		logger.Errorf(eventComputeLogicLog, fmt.Sprintf("数据消息中uid字段不存在或类型错误"))
		return fmt.Errorf("数据消息中uid字段不存在或类型错误")
	}
	//
	//modelInfo, err := clogic.ModelLogic.FindLocalMapCache(modelID)
	//if err != nil {
	//	logger.Errorf(eventComputeLogicLog, fmt.Sprintf("获取当前模型(%s)详情失败:%s", modelID, err.Error()))
	//	return fmt.Errorf("获取当前模型(%s)详情失败:%s", modelID, err.Error())
	//}

	//logger.Debugf(eventComputeLogicLog, "开始遍历事件列表")
	for _, eventInfo := range *eventInfoList {
		logger.Debugf(eventComputeLogicLog, "事件信息为:%+v", eventInfo)
		if eventInfo.Handlers == nil || len(eventInfo.Handlers) == 0 {
			logger.Warnln(eventComputeLogicLog, "handlers字段数组长度为0")
			continue
		}
		logger.Debugf(eventComputeLogicLog, "开始分析事件")
		eventID := eventInfo.ID
		settings := eventInfo.Settings

		//判断是否已经失效
		invalid := eventInfo.Invalid
		if invalid {
			logger.Warnln(eventComputeLogicLog, "事件(%s)已经失效", eventID)
			continue
		}

		//判断禁用
		if disable, ok := settings["disable"].(bool); ok {
			if disable {
				logger.Warnln(eventComputeLogicLog, "事件(%s)已经被禁用", eventID)
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
						if startTime, ok := settings["startTime"].(time.Time); ok {
							if tools.GetLocalTimeNow(time.Now()).Unix() < startTime.Unix() {
								logger.Debugf(eventComputeLogicLog, "事件(%s)的定时任务开始时间未到，不执行", eventID)
								continue
							}
						}

						if endTime, ok := settings["endTime"].(time.Time); ok {
							if tools.GetLocalTimeNow(time.Now()).Unix() >= endTime.Unix() {
								logger.Debugf(eventComputeLogicLog, "事件(%s)的定时任务结束时间已到，不执行", eventID)
								//修改事件为失效
								updateMap := bson.M{"invalid": true}
								_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
								if err != nil {
									logger.Errorf(eventComputeLogicLog, "失效事件(%s)失败:%s", eventID, err.Error())
									return fmt.Errorf("失效事件(%s)失败:%s", eventID, err.Error())
								}
							}
						}
					}
				}
			}
		}

		//判断事件是否已经触发
		hasExecute := false

		if computeType, ok := settings["type"].(string); ok {
			switch computeType {
			case "model":
				if tags, ok := settings["tags"].([]interface{}); ok {
					nodeUIDFieldsMap := map[string][]string{}
					nodeUIDModelMap := map[string]string{}
					nodeUIDNodeMap := map[string]string{}
					nodeUIDNodeMap[nodeUIDInData] = nodeID
					for _, tag := range tags {
						//if tagMap, ok := settings["tags"].(map[string]interface{}); ok {
						if tagMap, ok := tag.(map[string]interface{}); ok {
							//if fields, ok := tagMap["fields"].([]interface{}); ok {
							//	fieldsList := tools.InterfaceListToStringList(fields)
							//	nodeUIDFieldsMap[nodeUIDInData] = fieldsList
							//}
							if tagIDInMap, ok := tagMap["id"].(string); ok {
								tools.MergeDataMap(nodeUIDInData,tagIDInMap,&nodeUIDFieldsMap)
							}
							if modelInfoInMap, ok := tagMap["model"].(map[string]interface{}); ok {
								if modelIDInInfo, ok := modelInfoInMap["id"].(string); ok {
									nodeUIDModelMap[nodeUIDInData] = modelIDInInfo
								}
							}
						}
					}
					if modelID == nodeUIDModelMap[nodeUIDInData] {
						if fields, ok := nodeUIDFieldsMap[data.Uid]; ok {
							hasField := false
						fieldLoopModel:
							for _, keyReq := range fields {
								for k := range fieldsMap {
									if keyReq == k {
										hasField = true
										break fieldLoopModel
									}
								}
							}
							if hasField {
								computeFields := make([]map[string]interface{}, 0)
							ruleloopModel:
								for uidInMap, tagIDList := range nodeUIDFieldsMap {
									computeFieldsMap := map[string]interface{}{}

									//判断是否存在纯数字的数据点ID
									for _, tagIDInList := range tagIDList {
										if tools.IsNumber(tagIDInList) {
											logger.Errorf(eventComputeLogicLog, "资产(%s)的数据点中存在纯数字的数据点ID:%s", uidInMap, tagIDInList)
											continue ruleloopModel
										}
									}
									cmdList := make([]*redis.StringCmd, 0)
									pipe := iredis.Client.Pipeline()
									for _, tagIDInList := range tagIDList {
										//不在fieldsMap中的tagId就去查redis
										if fieldsVal, ok := fieldsMap[tagIDInList]; !ok {
											//如果公式中的该参数为输入值类型则不用查Redis，直接套用
											if inputVal, ok := inputMap[tagIDInList]; ok {
												computeFieldsMap[tagIDInList] = inputVal
												continue
											} else {
												hashKey := uidInMap + "_" + tagIDInList
												cmd := pipe.HGet(hashKey, "value")
												cmdList = append(cmdList, cmd)
											}
										} else {
											computeFieldsMap[tagIDInList] = fieldsVal
										}
									}
									_, err = pipe.Exec()
									if err != nil {
										logger.Errorf(eventComputeLogicLog, "Redis批量查询tag最新数据(指令为:%+v)失败:%s", cmdList, err.Error())
										return fmt.Errorf("Redis批量查询tag最新数据(指令为:%+v)失败:%s", cmdList, err.Error())
									}
									resultIndex := 0
									//if len(tagIDList) != len(cmdList)+1 {
									//	fmt.Println("analyzeWarningRule:", "len(tagIDList) != len(cmdList)+1")
									//	return
									//}
									for _, tagIDInList := range tagIDList {
										//if tagID != tagIDInList {
										if resultIndex >= len(cmdList) {
											break
										}
										if cmdList[resultIndex].Err() != nil {
											logger.Errorf(eventComputeLogicLog, "Redis批量查询中查询条件为%+v的查询结果出现错误", cmdList[resultIndex].Args())
											continue ruleloopModel
										} else {
											if _, ok := fieldsMap[tagIDInList]; ok {
												continue
											}
											//如果公式中的该参数为输入值类型则不用查Redis，直接套用
											if _, ok := inputMap[tagIDInList]; ok {
												//logicMap[tagIDInList] = inputVal
												continue
											}
											//resVal, err := tools.InterfaceTypeToRedisMethod(cmdList[resultIndex])
											//if err != nil {
											//	return
											//}
											testAbnormalVal := cmdList[resultIndex].Val()
											if !tools.IsNumber(testAbnormalVal) {
												logger.Errorf(eventComputeLogicLog, "Redis批量查询中查询条件为%+v的查询结果不是合法数字(%s)", cmdList[resultIndex].Args(), testAbnormalVal)
												continue ruleloopModel
											}
											resVal, err := cmdList[resultIndex].Float64()
											if err != nil {
												logger.Errorf(eventComputeLogicLog, "Redis批量查询中查询条件为%+v的查询结果数值类型不为float64", cmdList[resultIndex].Args())
												continue ruleloopModel
											}
											computeFieldsMap[tagIDInList] = resVal
											resultIndex++
										}
									}

									dataMap := map[string]interface{}{
										"time": tools.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05"),
										//"status": "未处理",
										"modelId": nodeUIDModelMap[uidInMap],
										"nodeId":  nodeUIDNodeMap[uidInMap],
										"uid":     uidInMap,
										"fields":  computeFieldsMap,
									}
									for k, v := range computeFieldsMap {
										dataMap[k] = v
									}
									computeFields = append(computeFields, dataMap)
								}

								//生成计算对象并发送
								sendMap := bson.M{
									"data":     computeFields,
									"sendType": "dataCompute",
								}
								b, err := json.Marshal(sendMap)
								if err != nil {
									continue
								}
								err = imqtt.Send(fmt.Sprintf("event/%s", eventID), b)
								if err != nil {
									logger.Warnf(eventComputeLogicLog, "发送事件(%s)错误:%s", eventID, err.Error())
								} else {
									logger.Debugf(eventComputeLogicLog, "发送事件成功:%s,数据为:%+v", eventID, sendMap)
								}
								hasExecute = true
							}
						}
					}
				}
			case "node":
				if tags, ok := settings["tags"].([]interface{}); ok {
					nodeUIDFieldsMap := map[string][]string{}
					nodeUIDModelMap := map[string]string{}
					nodeUIDNodeMap := map[string]string{}
					for _, tag := range tags {
						if tagMap, ok := tag.(map[string]interface{}); ok {
							if nodeInfoInMap, ok := tagMap["node"].(map[string]interface{}); ok {
								if nodeUID, ok := nodeInfoInMap["uid"].(string); ok {
									//if fields, ok := tagMap["fields"].([]interface{}); ok {
									//	fieldsList := tools.InterfaceListToStringList(fields)
									//	nodeUIDFieldsMap[nodeUID] = fieldsList
									//}
									if tagIDInMap, ok := tagMap["id"].(string); ok {
										tools.MergeDataMap(nodeUID,tagIDInMap,&nodeUIDFieldsMap)
									}
									if nodeIDInInfo, ok := nodeInfoInMap["id"].(string); ok {
										nodeUIDNodeMap[nodeUID] = nodeIDInInfo
										nodeInfo, err := clogic.NodeLogic.FindLocalCache(nodeIDInInfo)
										if err != nil {
											logger.Errorf(eventComputeLogicLog, fmt.Sprintf("获取当前资产(%s)详情失败:%s", nodeID, err.Error()))
											return fmt.Errorf("获取当前资产(%s)详情失败:%s", nodeID, err.Error())
										}
										nodeUIDModelMap[nodeUID] = nodeInfo.Model
									}
									//
									//if modelInfoInMap, ok := tagMap["model"].(map[string]interface{}); ok {
									//	if modelIDInInfo, ok := modelInfoInMap["id"].(string); ok {
									//		nodeUIDModelMap[nodeUID] = modelIDInInfo
									//	}
									//}
								}
							}
						}
					}
					if fields, ok := nodeUIDFieldsMap[data.Uid]; ok {
						hasField := false
					fieldLoop:
						for _, keyReq := range fields {
							for k := range fieldsMap {
								if keyReq == k {
									hasField = true
									break fieldLoop
								}
							}
						}
						if hasField {
							computeFields := make([]map[string]interface{}, 0)
						ruleloop:
							for uidInMap, tagIDList := range nodeUIDFieldsMap {
								computeFieldsMap := map[string]interface{}{}

								//判断是否存在纯数字的数据点ID
								for _, tagIDInList := range tagIDList {
									if tools.IsNumber(tagIDInList) {
										logger.Errorf(eventComputeLogicLog, "资产(%s)的数据点中存在纯数字的数据点ID:%s", uidInMap, tagIDInList)
										continue ruleloop
									}
								}
								cmdList := make([]*redis.StringCmd, 0)
								pipe := iredis.Client.Pipeline()
								for _, tagIDInList := range tagIDList {
									//不在fieldsMap中的tagId就去查redis
									if fieldsVal, ok := fieldsMap[tagIDInList]; !ok {
										//如果公式中的该参数为输入值类型则不用查Redis，直接套用
										if inputVal, ok := inputMap[tagIDInList]; ok {
											if nodeUIDNodeMap[uidInMap] == nodeID {
												computeFieldsMap[tagIDInList] = inputVal
												continue
											}
										} else {
											hashKey := uidInMap + "_" + tagIDInList
											cmd := pipe.HGet(hashKey, "value")
											cmdList = append(cmdList, cmd)
										}
									} else {
										if nodeUIDNodeMap[uidInMap] == nodeID {
											computeFieldsMap[tagIDInList] = fieldsVal
										}
									}
									//if tagID != tagIDInList {
									//	hashKey := nodeID + "_" + tagIDInList
									//	cmd := pipe.HGet(hashKey, "value")
									//	cmdList = append(cmdList, cmd)
									//}
								}
								_, err = pipe.Exec()
								if err != nil {
									logger.Errorf(eventComputeLogicLog, "Redis批量查询tag最新数据(指令为:%+v)失败:%s", cmdList, err.Error())
									return fmt.Errorf("Redis批量查询tag最新数据(指令为:%+v)失败:%s", cmdList, err.Error())
								}
								resultIndex := 0
								//if len(tagIDList) != len(cmdList)+1 {
								//	fmt.Println("analyzeWarningRule:", "len(tagIDList) != len(cmdList)+1")
								//	return
								//}
								for _, tagIDInList := range tagIDList {
									//if tagID != tagIDInList {
									if resultIndex >= len(cmdList) {
										break
									}
									if cmdList[resultIndex].Err() != nil {
										logger.Errorf(eventComputeLogicLog, "Redis批量查询中查询条件为%+v的查询结果出现错误", cmdList[resultIndex].Args())
										continue ruleloop
									} else {
										if _, ok := fieldsMap[tagIDInList]; ok {
											if nodeUIDNodeMap[uidInMap] == nodeID {
												continue
											}
										}
										//如果公式中的该参数为输入值类型则不用查Redis，直接套用
										if _, ok := inputMap[tagIDInList]; ok {
											//logicMap[tagIDInList] = inputVal
											if nodeUIDNodeMap[uidInMap] == nodeID {
												continue
											}
										}
										//resVal, err := tools.InterfaceTypeToRedisMethod(cmdList[resultIndex])
										//if err != nil {
										//	return
										//}
										testAbnormalVal := cmdList[resultIndex].Val()
										if !tools.IsNumber(testAbnormalVal) {
											logger.Errorf(eventComputeLogicLog, "Redis批量查询中查询条件为%+v的查询结果不是合法数字(%s)", cmdList[resultIndex].Args(), testAbnormalVal)
											continue ruleloop
										}
										resVal, err := cmdList[resultIndex].Float64()
										if err != nil {
											logger.Errorf(eventComputeLogicLog, "Redis批量查询中查询条件为%+v的查询结果数值类型不为float64", cmdList[resultIndex].Args())
											continue ruleloop
										}
										computeFieldsMap[tagIDInList] = resVal
										resultIndex++
									}
								}

								dataMap := map[string]interface{}{
									"time": tools.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05"),
									//"status": "未处理",
									"modelId": nodeUIDModelMap[uidInMap],
									"nodeId":  nodeUIDNodeMap[uidInMap],
									"uid":     uidInMap,
									"fields":  computeFieldsMap,
								}
								for k, v := range computeFieldsMap {
									dataMap[k] = v
								}
								computeFields = append(computeFields, dataMap)
							}

							//生成计算对象并发送
							sendMap := bson.M{
								"data":     computeFields,
								"sendType": "dataCompute",
							}
							//b, err := json.Marshal(sendMap)
							//if err != nil {
							//	logger.Errorf(eventComputeLogicLog, "要发送到事件处理器的数据消息序列化失败:%s", err.Error())
							//	return fmt.Errorf("要发送到事件处理器的数据消息序列化失败:%s", err.Error())
							//}
							//logger.Debugf(eventComputeLogicLog, "发送的数据消息为:%s", string(b))
							//imqtt.SendMsg(emqttConn, "event/"+eventID.Hex(), string(b))
							b, err := json.Marshal(sendMap)
							if err != nil {
								continue
							}
							err = imqtt.Send(fmt.Sprintf("event/%s", eventID), b)
							if err != nil {
								logger.Warnf(eventComputeLogicLog, "发送事件(%s)错误:%s", eventID, err.Error())
							} else {
								logger.Debugf(eventComputeLogicLog, "发送事件成功:%s,数据为:%+v", eventID, sendMap)
							}
							hasExecute = true
						}
					}
				}
			}
		}

		//对只能执行一次的事件进行失效
		if validTime == "timeLimit" {
			if rangeDefine == "once" && hasExecute {
				logger.Warnln(eventComputeLogicLog, "事件(%s)为只执行一次的事件", eventID)
				//修改事件为失效
				updateMap := bson.M{"invalid": true}
				_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
				if err != nil {
					logger.Errorf(eventComputeLogicLog, "失效事件(%s)失败:%s", eventID, err.Error())
					return fmt.Errorf("失效事件(%s)失败:%s", eventID, err.Error())
				}
			}
		}
	}

	//logger.Debugf(eventComputeLogicLog, "计算事件触发器执行结束")
	return nil
}
