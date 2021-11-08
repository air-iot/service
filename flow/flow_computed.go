package flow

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/air-iot/service/gin/ginx"
	"github.com/air-iot/service/init/cache/department"
	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/cache/flow"
	"github.com/air-iot/service/init/cache/model"
	"github.com/air-iot/service/init/cache/tag"
	"github.com/air-iot/service/logger"
	"github.com/air-iot/service/util/flowx"
	"github.com/air-iot/service/util/formatx"
	"github.com/air-iot/service/util/numberx"
	"github.com/camunda-cloud/zeebe/clients/go/pkg/zbc"
	"github.com/go-redis/redis/v8"
	"github.com/tidwall/gjson"
	"go.mongodb.org/mongo-driver/mongo"
	"strconv"
	"time"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/init/cache/node"
	"github.com/air-iot/service/init/mq"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/util/timex"
	"go.mongodb.org/mongo-driver/bson"
)

var flowComputeLogicLog = map[string]interface{}{"name": "数据流程触发"}

func TriggerComputedNodeFlow(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, mq mq.MQ, apiClient api.Client, zbClient zbc.Client, projectName string, data entity.DataMessage) error {
	//logger.Debugf(eventComputeLogicLog, "开始执行计算流程触发器")
	//logger.Debugf(eventComputeLogicLog, "传入参数为:%+v", data)
	headerMap := map[string]string{ginx.XRequestProject: projectName}
	nodeID := data.NodeID
	if nodeID == "" {
		return fmt.Errorf("数据消息中nodeId字段不存在或类型错误")
	}

	modelID := data.ModelID
	if modelID == "" {
		return fmt.Errorf("数据消息中modelId字段不存在或类型错误")
	}

	inputMap := data.InputMap

	fieldsMap := data.Fields
	if len(fieldsMap) == 0 {
		return fmt.Errorf("数据消息中fields字段不存在或类型错误")
	}

	dataByte, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("序列化当前模型(%s)的数据失败:%s", modelID, err.Error())
	}
	//logger.Debugf(eventComputeLogicLog, "开始获取当前模型的数据流程")
	//获取当前模型的数据流程=============================================
	flowInfoList := new([]entity.Flow)
	err = flow.GetByType(ctx, redisClient, mongoClient, projectName, string(ComputeNodeLogic), flowInfoList)
	if err != nil {
		return fmt.Errorf("获取当前模型(%s)的数据流程失败:%s", modelID, err.Error())
	}

	nodeInfo := entity.Node{}
	err = node.Get(ctx, redisClient, mongoClient, projectName, nodeID, &nodeInfo)
	if err != nil {
		return fmt.Errorf("获取当前模型(%s)的资产(%s)失败:%s", modelID, nodeID, err.Error())
	}

	data.Uid = nodeID
	nodeUIDInData := data.Uid
	if nodeUIDInData == "" {
		return fmt.Errorf("数据消息中uid字段不存在或类型错误")
	}

	//logger.Debugf(flowComputeLogicLog, "开始遍历流程列表")
	for _, flowInfo := range *flowInfoList {
		flowID := flowInfo.ID
		settings := flowInfo.Settings

		//判断是否已经失效
		if flowInfo.Invalid {
			logger.Warnln("流程(%s)已经失效", flowID)
			continue
		}

		//判断禁用
		if flowInfo.Disable {
			logger.Warnln("流程(%s)已经被禁用", flowID)
			continue
		}

		//if flowInfo.ValidTime == "timeLimit" {
		//	if flowInfo.Range != "once" {
		//判断有效期
		startTime := flowInfo.StartTime
		formatLayout := timex.FormatTimeFormat(startTime)
		if startTime != "" {
			formatStartTime, err := timex.ConvertStringToTime(formatLayout, startTime, time.Local)
			if err != nil {
				logger.Errorf("开始时间范围字段值格式错误:%s", err.Error())
				continue
			}
			if timex.GetLocalTimeNow(time.Now()).Unix() < formatStartTime.Unix() {
				logger.Debugf("流程(%s)的定时任务开始时间未到，不执行", flowID)
				continue
			}
		}

		endTime := flowInfo.EndTime
		formatLayout = timex.FormatTimeFormat(endTime)
		if endTime != "" {
			formatEndTime, err := timex.ConvertStringToTime(formatLayout, endTime, time.Local)
			if err != nil {
				logger.Errorf("时间范围字段值格式错误:%s", err.Error())
				continue
			}
			if timex.GetLocalTimeNow(time.Now()).Unix() >= formatEndTime.Unix() {
				logger.Debugf("流程(%s)的定时任务结束时间已到，不执行", flowID)
				//修改流程为失效
				updateMap := bson.M{"invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("flow"), eventID, updateMap)
				var r = make(map[string]interface{})
				err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
				if err != nil {
					logger.Errorf("失效流程(%s)失败:%s", flowID, err.Error())
					continue
				}
				continue
			}
		}
		//	}
		//}

		//判断流程是否已经触发
		hasExecute := false

		hasSendMap := map[string]bool{}
		if tags, ok := settings["tags"].([]interface{}); ok {
			nodeUIDFieldsMap := map[string][]string{}
			nodeUIDModelMap := map[string]string{}
			nodeUIDNodeMap := map[string]string{}
			nodeCustomFieldsMap := map[string]map[string]interface{}{}
			for _, tag := range tags {
				if tagMap, ok := tag.(map[string]interface{}); ok {
					if nodeInfoInMap, ok := tagMap["node"].(map[string]interface{}); ok {
						if nodeUID, ok := nodeInfoInMap["id"].(string); ok {
							//if fields, ok := tagMap["fields"].([]interface{}); ok {
							//	fieldsList := formatx.InterfaceListToStringList(fields)
							//	nodeUIDFieldsMap[nodeUID] = fieldsList
							//}
							if dataType, ok := tagMap["dataType"].(string); ok {
								switch dataType {
								case "custom":
									if tagIDInMap, ok := tagMap["id"].(string); ok {
										customVal := gjson.Get(string(dataByte), "custom."+tagIDInMap)
										formatx.MergeDataInterfaceMap(nodeUID, tagIDInMap, customVal.Value(), &nodeCustomFieldsMap)
									}
								default:
									if tagIDInMap, ok := tagMap["id"].(string); ok {
										formatx.MergeDataMap(nodeUID, tagIDInMap, &nodeUIDFieldsMap)
									}
								}
							} else {
								if tagIDInMap, ok := tagMap["id"].(string); ok {
									formatx.MergeDataMap(nodeUID, tagIDInMap, &nodeUIDFieldsMap)
								}
							}
							if nodeIDInInfo, ok := nodeInfoInMap["id"].(string); ok {
								nodeUIDNodeMap[nodeUID] = nodeIDInInfo
								nodeInfo := entity.Node{}
								err = node.Get(ctx, redisClient, mongoClient, projectName, nodeIDInInfo, &nodeInfo)
								if err != nil {
									continue
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
					dataMap := map[string]interface{}{}
				ruleloop:
					for uidInMap, tagIDList := range nodeUIDFieldsMap {
						computeFieldsMap := map[string]interface{}{}

						//判断是否存在纯数字的数据点ID
						for _, tagIDInList := range tagIDList {
							if numberx.IsNumber(tagIDInList) {
								continue ruleloop
							}
						}
						cmdList := make([]*redis.StringCmd, 0)
						var pipe redis.Pipeliner
						pipe = redisClient.Pipeline()
						for _, tagIDInList := range tagIDList {
							//不在fieldsMap中的tagId就去查redis
							if nodeUIDNodeMap[uidInMap] != nodeID {
								hashKey := uidInMap + "|" + tagIDInList
								cmd := pipe.HGet(ctx, fmt.Sprintf("%s/data", projectName), hashKey)
								cmdList = append(cmdList, cmd)
							} else {
								if fieldsVal, ok := fieldsMap[tagIDInList]; !ok {
									//如果公式中的该参数为输入值类型则不用查Redis，直接套用
									if inputVal, ok := inputMap[tagIDInList]; ok {
										computeFieldsMap[tagIDInList] = inputVal
										continue
									} else {
										hashKey := uidInMap + "|" + tagIDInList
										cmd := pipe.HGet(ctx, fmt.Sprintf("%s/data", projectName), hashKey)
										cmdList = append(cmdList, cmd)
									}
								} else {
									computeFieldsMap[tagIDInList] = fieldsVal
								}
							}
						}
						_, err = pipe.Exec(context.Background())
						if err != nil {
							continue
						}
						resultIndex := 0
						for _, tagIDInList := range tagIDList {
							//if tagID != tagIDInList {
							if resultIndex >= len(cmdList) {
								break
							}
							if cmdList[resultIndex].Err() != nil {
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
								redisData := map[string]interface{}{}
								err = json.Unmarshal([]byte(cmdList[resultIndex].Val()), &redisData)
								if err != nil {
									logger.Errorf("Redis批量查询中查询条件为%+v的查询结果解序列化失败:%s", cmdList[resultIndex].Args(), err.Error())
									continue ruleloop
								}
								resVal, err := numberx.GetFloat64NumberFromMongoDB(redisData, "value")
								if err != nil {
									logger.Errorf("Redis批量查询中查询条件为%+v的查询结果不是数字:%s", cmdList[resultIndex].Args(), err.Error())
									continue ruleloop
								}
								computeFieldsMap[tagIDInList] = resVal
								resultIndex++
							}
						}

						//获取当前模型对应资产ID的资产
						nodeInfoMap := map[string]interface{}{}
						err = node.Get(ctx, redisClient, mongoClient, projectName, uidInMap, &nodeInfoMap)
						if err != nil {
							continue
						}

						modelIDInMap, ok := nodeInfoMap["model"].(string)
						if !ok {
							continue
						}

						modelInfoMap := map[string]interface{}{}
						err = model.Get(ctx, redisClient, mongoClient, projectName, modelIDInMap, &modelInfoMap)
						if err != nil {
							continue
						}

						//生成发送消息
						departmentStringIDList := make([]string, 0)
						//var departmentObjectList primitive.A
						if departmentIDList, ok := nodeInfoMap["department"].([]interface{}); ok {
							departmentStringIDList = formatx.InterfaceListToStringList(departmentIDList)
						} else {
							//logger.Warnf(flowComputeLogicLog, "资产(%s)的部门字段不存在或类型错误", nodeID)
						}

						deptInfoList := make([]map[string]interface{}, 0)
						if len(departmentStringIDList) != 0 {
							err := department.GetByList(ctx, redisClient, mongoClient, projectName, departmentStringIDList, &deptInfoList)
							if err != nil {
								deptInfoList = make([]map[string]interface{}, 0)
							}
						}

						fields := make([]map[string]interface{}, 0)
						dataMapInLoop := map[string]interface{}{}
						deptMap := bson.M{}
						for _, id := range departmentStringIDList {
							deptMap[id] = bson.M{"id": id, "_tableName": "dept"}
						}
						sendTime := ""
						if data.Time == 0 {
							sendTime = timex.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05")
						} else {
							time.Unix(data.Time, 0).Format("2006-01-02 15:04:05")
						}
						dataMapInLoop = map[string]interface{}{
							"time":         sendTime,
							"#$model":      bson.M{"id": modelIDInMap, "_tableName": "model"},
							"#$department": deptMap,
							"#$node":       bson.M{"id": uidInMap, "_tableName": "node", "uid": uidInMap},
							//"modelId":        nodeUIDModelMap[uidInMap],
							//"nodeId":         nodeUIDNodeMap[uidInMap],
							//"departmentName": formatx.FormatKeyInfoListMap(deptInfoList, "name"),
							//"modelName":      formatx.FormatKeyInfo(modelInfoMap, "name"),
							//"nodeName":       formatx.FormatKeyInfo(nodeInfoMap, "name"),
						}
						for k, v := range computeFieldsMap {
							tagCache, err := tag.FindLocalCache(ctx, redisClient, mongoClient, projectName, modelID, nodeID, k)
							if err != nil {
								continue
							}
							formatVal := v
							if tagCache.Fixed >= 0 {
								formatVal, err = strconv.ParseFloat(fmt.Sprintf("%."+strconv.Itoa(int(tagCache.Fixed))+"f", v), 64)
								if err != nil {
									logger.Errorf("流程[%s]结果小数转化失败:%+v", flowID, v)
									continue
								}
							} else {
								formatVal, err = strconv.ParseFloat(fmt.Sprintf("%.3f", v), 64)
								if err != nil {
									logger.Errorf("流程[%s]结果3位小数转化失败:%+v", flowID, v)
									continue
								}
							}
							fields = append(fields, map[string]interface{}{
								"id":    k,
								"name":  tagCache.Name,
								"value": formatVal,
							})
							dataMapInLoop[k] = formatVal
							//dataMapInLoop["tagInfo"] = formatx.FormatDataInfoList(fields)
						}
						if customMap, ok := nodeCustomFieldsMap[data.Uid]; ok {
							customDataMap := map[string]interface{}{}
							for k, v := range customMap {
								customDataMap[k] = v
							}
							dataMapInLoop["custom"] = customDataMap
						}
						dataMap[nodeUIDNodeMap[uidInMap]] = dataMapInLoop
						if uidInMap == nodeID {
							for k, v := range dataMapInLoop {
								dataMap[k] = v
							}
						}

						hasSendMap[nodeUIDNodeMap[uidInMap]] = true
						for key, val := range nodeUIDNodeMap {
							if val != nodeUIDNodeMap[uidInMap] {
								if _, ok := hasSendMap[val]; !ok {
									dataMap[val] = map[string]interface{}{
										"time":         sendTime,
										"#$model":      bson.M{"id": nodeUIDModelMap[key], "_tableName": "model"},
										"#$department": deptMap,
										"#$node":       bson.M{"id": val, "_tableName": "node", "uid": val},
									}
								}
							}
						}
					}

					err = flowx.StartFlow(mongoClient,zbClient, flowInfo.FlowXml, projectName,string(NodeDataTrigger), dataMap,settings)
					if err != nil {
						logger.Errorf("流程(%s)推进到下一阶段失败:%s", flowID, err.Error())
						continue
					}
					hasExecute = true
				}
			}
		}

		//对只能执行一次的流程进行失效
		if flowInfo.ValidTime == "timeLimit" {
			if flowInfo.Range == "once" && hasExecute {
				logger.Warnln(flowComputeLogicLog, "流程(%s)为只执行一次的流程", flowID)
				//修改流程为失效
				updateMap := bson.M{"invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("flow"), flowID, updateMap)
				var r = make(map[string]interface{})
				err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
				if err != nil {
					//logger.Errorf(flowComputeLogicLog, "失效流程(%s)失败:%s", flowID, err.Error())
					continue
				}
			}
		}
	}

	//logger.Debugf(eventComputeLogicLog, "计算流程触发器执行结束")
	return nil
}

func TriggerComputedModelFlow(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, mq mq.MQ, apiClient api.Client, zbClient zbc.Client, projectName string, data entity.DataMessage) error {
	//logger.Debugf(eventComputeLogicLog, "开始执行计算流程触发器")
	//logger.Debugf(eventComputeLogicLog, "传入参数为:%+v", data)
	headerMap := map[string]string{ginx.XRequestProject: projectName}
	nodeID := data.NodeID
	if nodeID == "" {
		return fmt.Errorf("数据消息中nodeId字段不存在或类型错误")
	}

	modelID := data.ModelID
	if modelID == "" {
		return fmt.Errorf("数据消息中modelId字段不存在或类型错误")
	}

	inputMap := data.InputMap

	fieldsMap := data.Fields
	if len(fieldsMap) == 0 {
		return fmt.Errorf("数据消息中fields字段不存在或类型错误")
	}

	//dataByte, err := json.Marshal(data)
	//if err != nil {
	//	return fmt.Errorf("序列化当前模型(%s)的数据失败:%s", modelID, err.Error())
	//}
	//logger.Debugf(eventComputeLogicLog, "开始获取当前模型的数据流程")
	//获取当前模型的数据流程=============================================
	flowInfoList := new([]entity.Flow)
	err := flow.GetByType(ctx, redisClient, mongoClient, projectName, string(ComputeModelLogic), flowInfoList)
	if err != nil {
		return fmt.Errorf("获取当前模型(%s)的数据流程失败:%s", modelID, err.Error())
	}

	nodeInfo := entity.Node{}
	err = node.Get(ctx, redisClient, mongoClient, projectName, nodeID, &nodeInfo)
	if err != nil {
		return fmt.Errorf("获取当前模型(%s)的资产(%s)失败:%s", modelID, nodeID, err.Error())
	}

	data.Uid = nodeID
	nodeUIDInData := data.Uid
	if nodeUIDInData == "" {
		return fmt.Errorf("数据消息中uid字段不存在或类型错误")
	}

	//logger.Debugf(flowComputeLogicLog, "开始遍历流程列表")
	for _, flowInfo := range *flowInfoList {
		flowID := flowInfo.ID
		settings := flowInfo.Settings

		//判断是否已经失效
		if flowInfo.Invalid {
			logger.Warnln("流程(%s)已经失效", flowID)
			continue
		}

		//判断禁用
		if flowInfo.Disable {
			logger.Warnln("流程(%s)已经被禁用", flowID)
			continue
		}

		//if flowInfo.ValidTime == "timeLimit" {
		//	if flowInfo.Range != "once" {
		//判断有效期
		startTime := flowInfo.StartTime
		formatLayout := timex.FormatTimeFormat(startTime)
		if startTime != "" {
			formatStartTime, err := timex.ConvertStringToTime(formatLayout, startTime, time.Local)
			if err != nil {
				logger.Errorf("开始时间范围字段值格式错误:%s", err.Error())
				continue
			}
			if timex.GetLocalTimeNow(time.Now()).Unix() < formatStartTime.Unix() {
				logger.Debugf("流程(%s)的定时任务开始时间未到，不执行", flowID)
				continue
			}
		}

		endTime := flowInfo.EndTime
		formatLayout = timex.FormatTimeFormat(endTime)
		if endTime != "" {
			formatEndTime, err := timex.ConvertStringToTime(formatLayout, endTime, time.Local)
			if err != nil {
				logger.Errorf("时间范围字段值格式错误:%s", err.Error())
				continue
			}
			if timex.GetLocalTimeNow(time.Now()).Unix() >= formatEndTime.Unix() {
				logger.Debugf("流程(%s)的定时任务结束时间已到，不执行", flowID)
				//修改流程为失效
				updateMap := bson.M{"invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("flow"), eventID, updateMap)
				var r = make(map[string]interface{})
				err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
				if err != nil {
					logger.Errorf("失效流程(%s)失败:%s", flowID, err.Error())
					continue
				}
				continue
			}
		}
		//	}
		//}
		//判断流程是否已经触发
		hasExecute := false

		if tags, ok := settings["tags"].([]interface{}); ok {
			nodeUIDFieldsMap := map[string][]string{}
			nodeUIDModelMap := map[string]string{}
			nodeUIDNodeMap := map[string]string{}
			nodeUIDNodeMap[nodeUIDInData] = nodeID
			//if modelInfoInMap, ok := settings["model"].(map[string]interface{}); ok {
			//	if modelIDInInfo, ok := modelInfoInMap["id"].(string); ok {
			//		nodeUIDModelMap[nodeUIDInData] = modelIDInInfo
			//	}
			//}
			for _, tag := range tags {
				//if tagMap, ok := settings["tags"].(map[string]interface{}); ok {
				if tagMap, ok := tag.(map[string]interface{}); ok {
					if tagIDInMap, ok := tagMap["id"].(string); ok {
						if modelInfoInMap, ok := tagMap["model"].(map[string]interface{}); ok {
							if modelIDInInfo, ok := modelInfoInMap["id"].(string); ok {
								nodeUIDModelMap[nodeUIDInData] = modelIDInInfo
							}
						}
						formatx.MergeDataMap(nodeUIDInData, tagIDInMap, &nodeUIDFieldsMap)
					}
					//if modelInfoInMap, ok := tagMap["model"].(map[string]interface{}); ok {
					//	if modelIDInInfo, ok := modelInfoInMap["id"].(string); ok {
					//		nodeUIDModelMap[nodeUIDInData] = modelIDInInfo
					//	}
					//}
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
						dataMap := map[string]interface{}{}
					ruleloopModel:
						for uidInMap, tagIDList := range nodeUIDFieldsMap {
							computeFieldsMap := map[string]interface{}{}

							//判断是否存在纯数字的数据点ID
							for _, tagIDInList := range tagIDList {
								if numberx.IsNumber(tagIDInList) {
									continue ruleloopModel
								}
							}
							cmdList := make([]*redis.StringCmd, 0)
							var pipe redis.Pipeliner
							pipe = redisClient.Pipeline()
							for _, tagIDInList := range tagIDList {
								//不在fieldsMap中的tagId就去查redis
								if nodeUIDNodeMap[uidInMap] != nodeID {
									hashKey := uidInMap + "|" + tagIDInList
									cmd := pipe.HGet(ctx, fmt.Sprintf("%s/data", projectName), hashKey)
									cmdList = append(cmdList, cmd)
								} else {
									if fieldsVal, ok := fieldsMap[tagIDInList]; !ok {
										//如果公式中的该参数为输入值类型则不用查Redis，直接套用
										if inputVal, ok := inputMap[tagIDInList]; ok {
											computeFieldsMap[tagIDInList] = inputVal
											continue
										} else {
											hashKey := uidInMap + "|" + tagIDInList
											cmd := pipe.HGet(ctx, fmt.Sprintf("%s/data", projectName), hashKey)
											cmdList = append(cmdList, cmd)
										}
									} else {
										computeFieldsMap[tagIDInList] = fieldsVal
									}
								}
							}
							_, err = pipe.Exec(context.Background())
							if err != nil {
								continue
							}
							resultIndex := 0
							for _, tagIDInList := range tagIDList {
								//if tagID != tagIDInList {
								if resultIndex >= len(cmdList) {
									break
								}
								if cmdList[resultIndex].Err() != nil {
									continue ruleloopModel
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
									redisData := map[string]interface{}{}
									err = json.Unmarshal([]byte(cmdList[resultIndex].Val()), &redisData)
									if err != nil {
										logger.Errorf("Redis批量查询中查询条件为%+v的查询结果解序列化失败:%s", cmdList[resultIndex].Args(), err.Error())
										continue ruleloopModel
									}
									resVal, err := numberx.GetFloat64NumberFromMongoDB(redisData, "value")
									if err != nil {
										logger.Errorf("Redis批量查询中查询条件为%+v的查询结果不是数字:%s", cmdList[resultIndex].Args(), err.Error())
										continue ruleloopModel
									}
									computeFieldsMap[tagIDInList] = resVal
									resultIndex++
								}
							}

							//获取当前模型对应资产ID的资产
							nodeInfoMap := map[string]interface{}{}
							err = node.Get(ctx, redisClient, mongoClient, projectName, uidInMap, &nodeInfoMap)
							if err != nil {
								continue
							}

							modelInfoMap := map[string]interface{}{}
							err = model.Get(ctx, redisClient, mongoClient, projectName, modelID, &modelInfoMap)
							if err != nil {
								continue
							}

							//生成发送消息
							departmentStringIDList := make([]string, 0)
							//var departmentObjectList primitive.A
							if departmentIDList, ok := nodeInfoMap["department"].([]interface{}); ok {
								departmentStringIDList = formatx.InterfaceListToStringList(departmentIDList)
							} else {
								//logger.Warnf(flowComputeLogicLog, "资产(%s)的部门字段不存在或类型错误", nodeID)
							}

							deptInfoList := make([]map[string]interface{}, 0)
							if len(departmentStringIDList) != 0 {
								err := department.GetByList(ctx, redisClient, mongoClient, projectName, departmentStringIDList, &deptInfoList)
								if err != nil {
									deptInfoList = make([]map[string]interface{}, 0)
								}
							}

							fields := make([]map[string]interface{}, 0)
							deptMap := bson.M{}
							for _, id := range departmentStringIDList {
								deptMap[id] = bson.M{"id": id, "_tableName": "dept"}
							}
							sendTime := ""
							if data.Time == 0 {
								sendTime = timex.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05")
							} else {
								time.Unix(data.Time, 0).Format("2006-01-02 15:04:05")
							}
							dataMap = map[string]interface{}{
								"time":         sendTime,
								"#$model":      bson.M{"id": modelID, "_tableName": "model"},
								"#$department": deptMap,
								"#$node":       bson.M{"id": nodeID, "_tableName": "node", "uid": nodeID},
								//"modelId":        nodeUIDModelMap[uidInMap],
								//"nodeId":         nodeUIDNodeMap[uidInMap],
								//"departmentName": formatx.FormatKeyInfoListMap(deptInfoList, "name"),
								//"modelName":      formatx.FormatKeyInfo(modelInfoMap, "name"),
								//"nodeName":       formatx.FormatKeyInfo(nodeInfoMap, "name"),
							}
							for k, v := range computeFieldsMap {
								tagCache, err := tag.FindLocalCache(ctx, redisClient, mongoClient, projectName, modelID, nodeID, k)
								if err != nil {
									continue
								}
								formatVal := v
								if tagCache.Fixed >= 0 {
									formatVal, err = strconv.ParseFloat(fmt.Sprintf("%."+strconv.Itoa(int(tagCache.Fixed))+"f", v), 64)
									if err != nil {
										logger.Errorf("流程[%s]结果小数转化失败:%+v", flowID, v)
										continue
									}
								} else {
									formatVal, err = strconv.ParseFloat(fmt.Sprintf("%.3f", v), 64)
									if err != nil {
										logger.Errorf("流程[%s]结果3位小数转化失败:%+v", flowID, v)
										continue
									}
								}
								fields = append(fields, map[string]interface{}{
									"id":    k,
									"name":  tagCache.Name,
									"value": formatVal,
								})
								dataMap[k] = formatVal
								//dataMap["tagInfo"] = formatx.FormatDataInfoList(fields)
							}
						}

						err = flowx.StartFlow(mongoClient,zbClient, flowInfo.FlowXml, projectName,string(ModelDataTrigger), dataMap,settings)
						if err != nil {
							logger.Errorf("流程(%s)推进到下一阶段失败:%s", flowID, err.Error())
							continue
						}
						hasExecute = true
					}
				}
			}
		}

		//对只能执行一次的流程进行失效
		if flowInfo.ValidTime == "timeLimit" {
			if flowInfo.Range == "once" && hasExecute {
				logger.Warnln(flowComputeLogicLog, "流程(%s)为只执行一次的流程", flowID)
				//修改流程为失效
				updateMap := bson.M{"invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("flow"), flowID, updateMap)
				var r = make(map[string]interface{})
				err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
				if err != nil {
					//logger.Errorf(flowComputeLogicLog, "失效流程(%s)失败:%s", flowID, err.Error())
					continue
				}
			}
		}
	}

	//logger.Debugf(eventComputeLogicLog, "计算流程触发器执行结束")
	return nil
}
