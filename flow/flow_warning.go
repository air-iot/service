package flow

import (
	"context"
	"fmt"
	"github.com/air-iot/service/init/cache/flow"
	"github.com/air-iot/service/logger"
	"github.com/air-iot/service/util/flowx"
	"github.com/camunda-cloud/zeebe/clients/go/pkg/zbc"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/gin/ginx"
	"github.com/air-iot/service/init/cache/department"
	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/cache/model"
	"github.com/air-iot/service/init/cache/node"
	"github.com/air-iot/service/init/cache/setting"
	"github.com/air-iot/service/init/mq"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/util/formatx"
	"github.com/air-iot/service/util/timex"
)

var eventAlarmLog = map[string]interface{}{"name": "报警流程触发"}


func TriggerWarningRulesFlow(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, mq mq.MQ, apiClient api.Client, zbClient zbc.Client, projectName string, data entity.WarningMessage, actionType string) error {
	////logger.Debugf(eventAlarmLog, "开始执行计算流程触发器")
	////logger.Debugf(eventAlarmLog, "传入参数为:%+v", data)
	headerMap := map[string]string{ginx.XRequestProject: projectName}
	nodeID := data.NodeID
	if nodeID == "" {
		//logger.Errorf(eventAlarmLog, fmt.Sprintf("数据消息中nodeId字段不存在或类型错误"))
		return fmt.Errorf("数据消息中nodeId字段不存在或类型错误")
	}

	modelID := data.ModelID
	if modelID == "" {
		//logger.Errorf(eventAlarmLog, fmt.Sprintf("数据消息中modelId字段不存在或类型错误"))
		return fmt.Errorf("数据消息中modelId字段不存在或类型错误")
	}

	nowTimeString := timex.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05")

	//fieldsMap := data.Fields
	//if len(fieldsMap) == 0 {
	//	//logger.Errorf(eventAlarmLog, fmt.Sprintf("数据消息中fields字段不存在或类型错误"))
	//	return fmt.Errorf("数据消息中fields字段不存在或类型错误")
	//}
	////logger.Debugf(eventAlarmLog, "开始获取当前模型的计算逻辑流程")
	//获取当前模型的报警规则逻辑流程=============================================
	flowInfoList := new([]entity.Flow)
	err := flow.GetByType(ctx, redisClient, mongoClient, projectName, string(Alarm), flowInfoList)
	if err != nil {
		//logger.Debugf(eventAlarmLog, fmt.Sprintf("获取当前模型(%s)的报警流程失败:%s", modelID, err.Error()))
		return fmt.Errorf("获取当前模型(%s)的报警流程失败:%s", modelID, err.Error())
	}
	////logger.Debugf(eventAlarmLog, "开始获取当前模型对应资产ID的资产")
	//获取当前模型对应资产ID的资产
	nodeInfo := map[string]interface{}{}
	err = node.Get(ctx, redisClient, mongoClient, projectName, nodeID, &nodeInfo)
	if err != nil {
		//logger.Errorf(eventAlarmLog, fmt.Sprintf("获取当前模型(%s)的资产(%s)失败:%s", modelID, nodeID, err.Error()))
		return fmt.Errorf("获取当前模型(%s)的资产(%s)失败:%s", modelID, nodeID, err.Error())
	}

	modelInfo := map[string]interface{}{}
	err = model.Get(ctx, redisClient, mongoClient, projectName, modelID, &modelInfo)
	if err != nil {
		//logger.Errorf(eventAlarmLog, fmt.Sprintf("获取当前模型(%s)详情失败:%s", modelID, err.Error()))
		return fmt.Errorf("获取当前模型(%s)详情失败:%s", modelID, err.Error())
	}

	////logger.Debugf(eventAlarmLog, "开始遍历流程列表")
flowloop:
	for _, flowInfo := range *flowInfoList {
		//logger.Debugf(eventAlarmLog, "开始分析流程")
		flowID := flowInfo.ID
		settings := flowInfo.Settings

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
				//logger.Warnln(eventAlarmLog, "流程(%s)已经被禁用", eventID)
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
						if startTime, ok := settings["startTime"].(string); ok {
							formatLayout := timex.FormatTimeFormat(startTime)
							formatStartTime, err := timex.ConvertStringToTime(formatLayout, startTime, time.Local)
							if err != nil {
								logger.Errorf( "开始时间范围字段值格式错误:%s", err.Error())
								continue
							}
							if timex.GetLocalTimeNow(time.Now()).Unix() < formatStartTime.Unix() {
								//logger.Debugf(eventAlarmLog, "流程(%s)的定时任务开始时间未到，不执行", eventID)
								continue
							}
						}

						if endTime, ok := settings["endTime"].(string); ok {
							formatLayout := timex.FormatTimeFormat(endTime)
							formatEndTime, err := timex.ConvertStringToTime(formatLayout, endTime, time.Local)
							if err != nil {
								logger.Errorf( "时间范围字段值格式错误:%s", err.Error())
								continue
							}
							if timex.GetLocalTimeNow(time.Now()).Unix() >= formatEndTime.Unix() {
								//logger.Debugf(eventAlarmLog, "流程(%s)的定时任务结束时间已到，不执行", eventID)
								//修改流程为失效
								updateMap := bson.M{"settings.invalid": true}
								//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
								var r = make(map[string]interface{})
								err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
								if err != nil {
									//logger.Errorf(eventAlarmLog, "失效流程(%s)失败:%s", eventID, err.Error())
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
		hasValidAction := false

		if rangeTypeRawList, ok := settings["eventRange"].([]interface{}); ok {
			rangeTypeList := formatx.InterfaceListToStringList(rangeTypeRawList)
			if actionList, ok := settings["action"].([]interface{}); ok {
				actionStringList := formatx.InterfaceListToStringList(actionList)
				for _, action := range actionStringList {
					if action == actionType {
						hasValidAction = true
						break
					}
				}
			}
			if !hasValidAction {
				////logger.Debugf(eventAlarmLog, "报警流程(%s)类型未对应当前信息", eventID)
				continue
			}

			hasValidWarn := false
			needCount := len(rangeTypeList)
			actualCount := 0

			for _, rangeType := range rangeTypeList {
				switch rangeType {
				case "warnType":
					hasValidWarn = false
					if warnTypeList, ok := settings["warnType"].([]interface{}); ok {
						warnTypeStringList := formatx.InterfaceListToStringList(warnTypeList)
						for _, warnType := range warnTypeStringList {
							if warnType == data.Type {
								hasValidWarn = true
								break
							}
						}
					}
					if hasValidWarn {
						actualCount++
					}
				case "warnLevel":
					hasValidWarn = false
					if warnLevelList, ok := settings["level"].([]interface{}); ok {
						warnLevelStringList := formatx.InterfaceListToStringList(warnLevelList)
						for _, warnLevel := range warnLevelStringList {
							if warnLevel == data.Level {
								hasValidWarn = true
								break
							}
						}
					}
					if hasValidWarn {
						actualCount++
					}
				case "department":
					hasValidWarn = false
					deptIDInDataList := data.Department
					if departmentList, ok := settings["department"].([]interface{}); ok {
					deptLoop:
						for _, dept := range departmentList {
							if deptMap, ok := dept.(map[string]interface{}); ok {
								if deptID, ok := deptMap["id"].(string); ok {
									for _, deptIDInData := range deptIDInDataList {
										if deptIDInData == deptID {
											hasValidWarn = true
											break deptLoop
										}
									}
								}
							}
						}
					}
					if hasValidWarn {
						actualCount++
					}
				case "model":
					hasValidWarn = false
					//判断该流程是否指定了特定报警规则
					if modelList, ok := settings["model"].([]interface{}); ok {
						for _, model := range modelList {
							if modelMap, ok := model.(map[string]interface{}); ok {
								if modelIDInSettings, ok := modelMap["id"].(string); ok {
									if modelID == modelIDInSettings {
										if ruleList, ok := settings["rule"].([]interface{}); ok {
											if len(ruleList) != 0 {
												for _, rule := range ruleList {
													if ruleMap, ok := rule.(map[string]interface{}); ok {
														if ruleIDInSettings, ok := ruleMap["id"].(string); ok {
															if data.RuleID == ruleIDInSettings {
																hasValidWarn = true
																break
															}
														}
													}
												}
											} else {
												hasValidWarn = true
												break
											}
										} else {
											hasValidWarn = true
											break
										}
									}
								}
							}
						}
					} else if ruleList, ok := settings["rule"].([]interface{}); ok {
						for _, rule := range ruleList {
							if ruleMap, ok := rule.(map[string]interface{}); ok {
								if ruleIDInSettings, ok := ruleMap["id"].(string); ok {
									if data.RuleID == ruleIDInSettings {
										hasValidWarn = true
										break
									}
								}
							}
						}
					}
					if hasValidWarn {
						actualCount++
					}
				case "node":
					hasValidWarn = false
					//判断该流程是否指定了特定报警规则
					if nodeList, ok := settings["node"].([]interface{}); ok {
						for _, node := range nodeList {
							if nodeMap, ok := node.(map[string]interface{}); ok {
								if nodeIDInSettings, ok := nodeMap["id"].(string); ok {
									if nodeID == nodeIDInSettings {
										if ruleList, ok := settings["rule"].([]interface{}); ok {
											if len(ruleList) != 0 {
												for _, rule := range ruleList {
													if ruleMap, ok := rule.(map[string]interface{}); ok {
														if ruleIDInSettings, ok := ruleMap["id"].(string); ok {
															if data.RuleID == ruleIDInSettings {
																hasValidWarn = true
																break
															}
														}
													}
												}
											} else {
												hasValidWarn = true
												break
											}
										} else {
											hasValidWarn = true
											break
										}
									}
								}
							}
						}
					} else if ruleList, ok := settings["rule"].([]interface{}); ok {
						for _, rule := range ruleList {
							if ruleMap, ok := rule.(map[string]interface{}); ok {
								if ruleIDInSettings, ok := ruleMap["id"].(string); ok {
									if data.RuleID == ruleIDInSettings {
										hasValidWarn = true
										break
									}
								}
							}
						}
					}
					if !hasValidWarn {
						////logger.Debugf(eventAlarmLog, "报警流程(%s)类型未对应当前信息", eventID)
						continue flowloop
					}
					if hasValidWarn {
						actualCount++
					}
				}
			}

			if !hasValidWarn || (actualCount != needCount) {
				////logger.Debugf(flowAlarmLog, "报警流程(%s)类型未对应当前信息", flowID)
				continue flowloop
			}

			//生成发送消息
			departmentStringIDList := make([]string, 0)
			//var departmentObjectList primitive.A
			if departmentIDList, ok := nodeInfo["department"].([]interface{}); ok {
				departmentStringIDList = formatx.InterfaceListToStringList(departmentIDList)
			} else {
				//logger.Warnf(flowAlarmLog, "资产(%s)的部门字段不存在或类型错误", nodeID)
			}

			deptInfoList := make([]map[string]interface{}, 0)
			if len(departmentStringIDList) != 0 {
				err := department.GetByList(ctx, redisClient, mongoClient, projectName, departmentStringIDList, &deptInfoList)
				if err != nil {
					return fmt.Errorf("获取当前资产(%s)所属部门失败:%s", nodeID, err.Error())
				}
			}
			dataMappingType, err := setting.GetByWarnKindID(ctx, redisClient, mongoClient, projectName, data.Type)
			if err != nil {
				//logger.Errorf(flowAlarmLog, fmt.Sprintf("获取当前资产(%s)的报警类型中文失败:%s", nodeID, err.Error()))
				continue
			}
			//生成报警对象并发送
			sendMap := bson.M{
				"time":           nowTimeString,
				"type":           dataMappingType,
				"status":         data.Status,
				"processed":      data.Processed,
				"desc":           data.Desc,
				"level":          data.Level,
				"departmentName": formatx.FormatKeyInfoListMap(deptInfoList, "name"),
				"modelName":      formatx.FormatKeyInfo(modelInfo, "name"),
				"nodeName":       formatx.FormatKeyInfo(nodeInfo, "name"),
				"nodeUid":        formatx.FormatKeyInfo(nodeInfo, "uid"),
				"tagInfo":        formatx.FormatDataInfoList(data.Fields),
				//"fields":         fieldsMap,
				"userName":  data.HandleUserName,
				"action":    actionType,
				"isWarning": true,
			}
			fieldsInSendMap := map[string]interface{}{}
			for _, fieldsMap := range data.Fields {
				for k, v := range fieldsMap {
					//sendMap[k] = v
					fieldsInSendMap[k] = v
				}
			}
			sendMap["fields"] = fieldsInSendMap


			err = flowx.StartFlow(zbClient,flowInfo.FlowXml,projectName,sendMap)
			if err != nil {
				logger.Errorf("流程(%s)推进到下一阶段失败:%s", flowID,err.Error())
				continue
			}
			hasExecute = true
		}

		//对只能执行一次的流程进行失效
		if validTime == "timeLimit" {
			if rangeDefine == "once" && hasExecute {
				//logger.Warnln(eventAlarmLog, "流程(%s)为只执行一次的流程", flowID)
				//修改流程为失效
				updateMap := bson.M{"settings.invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("flow"), flowID, updateMap)
				var r = make(map[string]interface{})
				err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
				if err != nil {
					//logger.Errorf(flowAlarmLog, "失效流程(%s)失败:%s", flowID, err.Error())
					return fmt.Errorf("失效流程(%s)失败:%s", flowID, err.Error())
				}
			}
		}
	}

	////logger.Debugf(flowAlarmLog, "报警规则触发器执行结束")
	return nil
}

func TriggerWarningDisableModelRulesFlow(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, mq mq.MQ, apiClient api.Client, zbClient zbc.Client, projectName string, data entity.WarningMessage, actionType string) error {
	////logger.Debugf(eventAlarmLog, "开始执行计算流程触发器")
	////logger.Debugf(eventAlarmLog, "传入参数为:%+v", data)
	headerMap := map[string]string{ginx.XRequestProject: projectName}
	modelID := data.ModelID
	if modelID == "" {
		//logger.Errorf(eventAlarmLog, fmt.Sprintf("数据消息中modelId字段不存在或类型错误"))
		return fmt.Errorf("数据消息中modelId字段不存在或类型错误")
	}

	nowTimeString := timex.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05")

	//fieldsMap := data.Fields
	//if len(fieldsMap) == 0 {
	//	//logger.Errorf(eventAlarmLog, fmt.Sprintf("数据消息中fields字段不存在或类型错误"))
	//	return fmt.Errorf("数据消息中fields字段不存在或类型错误")
	//}
	////logger.Debugf(eventAlarmLog, "开始获取当前模型的计算逻辑流程")
	//获取当前模型的报警规则逻辑流程=============================================
	flowInfoList := new([]entity.Flow)
	err := flow.GetByType(ctx, redisClient, mongoClient, projectName, string(Alarm), flowInfoList)
	if err != nil {
		//logger.Debugf(eventAlarmLog, fmt.Sprintf("获取当前模型(%s)的报警流程失败:%s", modelID, err.Error()))
		return fmt.Errorf("获取当前模型(%s)的报警流程失败:%s", modelID, err.Error())
	}
	////logger.Debugf(eventAlarmLog, "开始获取当前模型对应资产ID的资产")
	//获取当前模型对应资产ID的资产

	modelInfo := map[string]interface{}{}
	err = model.Get(ctx, redisClient, mongoClient, projectName, modelID, &modelInfo)
	if err != nil {
		//logger.Errorf(eventAlarmLog, fmt.Sprintf("获取当前模型(%s)详情失败:%s", modelID, err.Error()))
		return fmt.Errorf("获取当前模型(%s)详情失败:%s", modelID, err.Error())
	}

	////logger.Debugf(eventAlarmLog, "开始遍历流程列表")
flowloop:
	for _, flowInfo := range *flowInfoList {
		//logger.Debugf(eventAlarmLog, "流程信息为:%+v", eventInfo)
		//logger.Debugf(flowAlarmLog, "开始分析流程")
		flowID := flowInfo.ID
		settings := flowInfo.Settings

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
				//logger.Warnln(eventAlarmLog, "流程(%s)已经被禁用", eventID)
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
						if startTime, ok := settings["startTime"].(string); ok {
							formatLayout := timex.FormatTimeFormat(startTime)
							formatStartTime, err := timex.ConvertStringToTime(formatLayout, startTime, time.Local)
							if err != nil {
								logger.Errorf( "开始时间范围字段值格式错误:%s", err.Error())
								continue
							}
							if timex.GetLocalTimeNow(time.Now()).Unix() < formatStartTime.Unix() {
								//logger.Debugf(eventAlarmLog, "流程(%s)的定时任务开始时间未到，不执行", eventID)
								continue
							}
						}

						if endTime, ok := settings["endTime"].(string); ok {
							formatLayout := timex.FormatTimeFormat(endTime)
							formatEndTime, err := timex.ConvertStringToTime(formatLayout, endTime, time.Local)
							if err != nil {
								logger.Errorf( "时间范围字段值格式错误:%s", err.Error())
								continue
							}
							if timex.GetLocalTimeNow(time.Now()).Unix() >= formatEndTime.Unix() {
								//logger.Debugf(eventAlarmLog, "流程(%s)的定时任务结束时间已到，不执行", eventID)
								//修改流程为失效
								updateMap := bson.M{"settings.invalid": true}
								//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
								var r = make(map[string]interface{})
								err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
								if err != nil {
									//logger.Errorf(eventAlarmLog, "失效流程(%s)失败:%s", eventID, err.Error())
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
		hasValidAction := false

		if rangeTypeRawList, ok := settings["eventRange"].([]interface{}); ok {
			rangeTypeList := formatx.InterfaceListToStringList(rangeTypeRawList)
			if actionList, ok := settings["action"].([]interface{}); ok {
				actionStringList := formatx.InterfaceListToStringList(actionList)
				for _, action := range actionStringList {
					if action == actionType {
						hasValidAction = true
						break
					}
				}
			}
			if !hasValidAction {
				////logger.Debugf(eventAlarmLog, "报警流程(%s)类型未对应当前信息", eventID)
				continue
			}

			hasValidWarn := false
			needCount := len(rangeTypeList)
			actualCount := 0

			for _, rangeType := range rangeTypeList {
				switch rangeType {
				case "warnType":
					hasValidWarn = false
					if warnTypeList, ok := settings["warnType"].([]interface{}); ok {
						warnTypeStringList := formatx.InterfaceListToStringList(warnTypeList)
						for _, warnType := range warnTypeStringList {
							if warnType == data.Type {
								hasValidWarn = true
								break
							}
						}
					}

					if hasValidWarn {
						actualCount++
					}
				case "warnLevel":
					hasValidWarn = false
					if warnLevelList, ok := settings["level"].([]interface{}); ok {
						warnLevelStringList := formatx.InterfaceListToStringList(warnLevelList)
						for _, warnLevel := range warnLevelStringList {
							if warnLevel == data.Level {
								hasValidWarn = true
								break
							}
						}
					}

					if hasValidWarn {
						actualCount++
					}
				case "department":
					continue flowloop
					//hasValidWarn := false
					//deptIDInDataList, err := tools.ObjectIdListToStringList(data.Department)
					//if err != nil {
					//	return fmt.Errorf("当前资产(%s)所属部门ID数组转ObjectID数组失败:%s", nodeID, err.Error())
					//}
					//if departmentList, ok := settings["department"].([]interface{}); ok {
					//deptLoop:
					//	for _, dept := range departmentList {
					//		if deptMap, ok := dept.(map[string]interface{}); ok {
					//			if deptID, ok := deptMap["id"].(string); ok {
					//				for _, deptIDInData := range deptIDInDataList {
					//					if deptIDInData == deptID {
					//						hasValidWarn = true
					//						break deptLoop
					//					}
					//				}
					//			}
					//		}
					//	}
					//}
					//if !hasValidWarn {
					//	////logger.Debugf(eventAlarmLog, "报警流程(%s)类型未对应当前信息", eventID)
					//	continue eventloop
					//}

				case "model":
					hasValidWarn = false
					//判断该流程是否指定了特定报警规则
					if modelList, ok := settings["model"].([]interface{}); ok {
						for _, model := range modelList {
							if modelMap, ok := model.(map[string]interface{}); ok {
								if modelIDInSettings, ok := modelMap["id"].(string); ok {
									if modelID == modelIDInSettings {
										if ruleList, ok := settings["rule"].([]interface{}); ok {
											if len(ruleList) != 0 {
												for _, rule := range ruleList {
													if ruleMap, ok := rule.(map[string]interface{}); ok {
														if ruleIDInSettings, ok := ruleMap["id"].(string); ok {
															if data.RuleID == ruleIDInSettings {
																hasValidWarn = true
																break
															}
														}
													}
												}
											} else {
												hasValidWarn = true
												break
											}
										} else {
											hasValidWarn = true
											break
										}
									}
								}
							}
						}
					} else if ruleList, ok := settings["rule"].([]interface{}); ok {
						for _, rule := range ruleList {
							if ruleMap, ok := rule.(map[string]interface{}); ok {
								if ruleIDInSettings, ok := ruleMap["id"].(string); ok {
									if data.RuleID == ruleIDInSettings {
										hasValidWarn = true
										break
									}
								}
							}
						}
					}

					if hasValidWarn {
						actualCount++
					}
				case "node":
					continue flowloop
					//hasValidWarn := false
					////判断该流程是否指定了特定报警规则
					//if nodeList, ok := settings["node"].([]interface{}); ok {
					//	for _, node := range nodeList {
					//		if nodeMap, ok := node.(map[string]interface{}); ok {
					//			if nodeIDInSettings, ok := nodeMap["id"].(string); ok {
					//				if nodeID == nodeIDInSettings {
					//					if ruleList, ok := settings["rule"].([]interface{}); ok {
					//						if len(ruleList) != 0{
					//							for _, rule := range ruleList {
					//								if ruleMap, ok := rule.(map[string]interface{}); ok {
					//									if ruleIDInSettings, ok := ruleMap["id"].(string); ok {
					//										if data.RuleID == ruleIDInSettings {
					//											hasValidWarn = true
					//											break
					//										}
					//									}
					//								}
					//							}
					//						}else {
					//							hasValidWarn = true
					//							break
					//						}
					//					} else {
					//						hasValidWarn = true
					//						break
					//					}
					//				}
					//			}
					//		}
					//	}
					//} else if ruleList, ok := settings["rule"].([]interface{}); ok {
					//	for _, rule := range ruleList {
					//		if ruleMap, ok := rule.(map[string]interface{}); ok {
					//			if ruleIDInSettings, ok := ruleMap["id"].(string); ok {
					//				if data.RuleID == ruleIDInSettings {
					//					hasValidWarn = true
					//					break
					//				}
					//			}
					//		}
					//	}
					//}
					//if !hasValidWarn {
					//	////logger.Debugf(eventAlarmLog, "报警流程(%s)类型未对应当前信息", eventID)
					//	continue eventloop
					//}
				}
			}

			if !hasValidWarn || (actualCount != needCount) {
				////logger.Debugf(eventAlarmLog, "报警流程(%s)类型未对应当前信息", eventID)
				continue flowloop
			}

			//生成发送消息
			dataMappingType, err := setting.GetByWarnKindID(ctx, redisClient, mongoClient, projectName, data.Type)

			if err != nil {
				//logger.Errorf(eventAlarmLog, fmt.Sprintf("获取当前模型(%s)的报警类型中文失败:%s", modelID, err.Error()))
				continue
			}
			//生成报警对象并发送
			sendMap := bson.M{
				"time": nowTimeString,
				"type": dataMappingType,
				//"status":         data.Status,
				//"processed":      data.Processed,
				"desc":  data.Desc,
				"level": data.Level,
				//"departmentName": formatx.FormatKeyInfoListMap(deptInfoList, "name"),
				"modelName": formatx.FormatKeyInfo(modelInfo, "name"),
				"userName":  data.HandleUserName,
				"action":    actionType,
				//"nodeName":       formatx.FormatKeyInfo(nodeInfo, "name"),
				//"nodeUid":        formatx.FormatKeyInfo(nodeInfo, "uid"),
				//"tagInfo":        formatx.FormatDataInfoList(data.Fields),
				//"fields":         fieldsMap,
				//"isWarning": true,
			}
			err = flowx.StartFlow(zbClient,flowInfo.FlowXml,projectName,sendMap)
			if err != nil {
				logger.Errorf("流程(%s)推进到下一阶段失败:%s", flowID,err.Error())
				continue
			}
			hasExecute = true
		}

		//对只能执行一次的流程进行失效
		if validTime == "timeLimit" {
			if rangeDefine == "once" && hasExecute {
				//logger.Warnln(eventAlarmLog, "流程(%s)为只执行一次的流程", eventID)
				//修改流程为失效
				updateMap := bson.M{"settings.invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap
				var r = make(map[string]interface{})
				err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
				if err != nil {
					//logger.Errorf(eventAlarmLog, "失效流程(%s)失败:%s", eventID, err.Error())
					return fmt.Errorf("失效流程(%s)失败:%s", flowID, err.Error())
				}
			}
		}
	}

	////logger.Debugf(eventAlarmLog, "报警规则触发器执行结束")
	return nil
}

func TriggerWarningDisableNodeRulesFlow(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, mq mq.MQ, apiClient api.Client, zbClient zbc.Client, projectName string, data entity.WarningMessage, actionType string) error {
	////logger.Debugf(eventAlarmLog, "开始执行计算流程触发器")
	////logger.Debugf(eventAlarmLog, "传入参数为:%+v", data)
	headerMap := map[string]string{ginx.XRequestProject: projectName}
	nodeID := data.NodeID
	if nodeID == "" {
		//logger.Errorf(eventAlarmLog, fmt.Sprintf("数据消息中nodeId字段不存在或类型错误"))
		return fmt.Errorf("数据消息中nodeId字段不存在或类型错误")
	}

	modelID := data.ModelID
	if modelID == "" {
		//logger.Errorf(eventAlarmLog, fmt.Sprintf("数据消息中modelId字段不存在或类型错误"))
		return fmt.Errorf("数据消息中modelId字段不存在或类型错误")
	}

	nowTimeString := timex.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05")

	//fieldsMap := data.Fields
	//if len(fieldsMap) == 0 {
	//	//logger.Errorf(eventAlarmLog, fmt.Sprintf("数据消息中fields字段不存在或类型错误"))
	//	return fmt.Errorf("数据消息中fields字段不存在或类型错误")
	//}
	////logger.Debugf(eventAlarmLog, "开始获取当前模型的计算逻辑流程")
	//获取当前模型的报警规则逻辑流程=============================================
	flowInfoList := new([]entity.Flow)
	err := flow.GetByType(ctx, redisClient, mongoClient, projectName, string(Alarm), flowInfoList)
	if err != nil {
		//logger.Debugf(flowAlarmLog, fmt.Sprintf("获取当前模型(%s)的报警流程失败:%s", modelID, err.Error()))
		return fmt.Errorf("获取当前模型(%s)的报警流程失败:%s", modelID, err.Error())
	}
	////logger.Debugf(flowAlarmLog, "开始获取当前模型对应资产ID的资产")
	//获取当前模型对应资产ID的资产
	nodeInfo := map[string]interface{}{}
	err = node.Get(ctx, redisClient, mongoClient, projectName, nodeID, &nodeInfo)
	if err != nil {
		//logger.Errorf(flowAlarmLog, fmt.Sprintf("获取当前模型(%s)的资产(%s)失败:%s", modelID, nodeID, err.Error()))
		return fmt.Errorf("获取当前模型(%s)的资产(%s)失败:%s", modelID, nodeID, err.Error())
	}

	modelInfo := map[string]interface{}{}
	err = model.Get(ctx, redisClient, mongoClient, projectName, modelID, &modelInfo)
	if err != nil {
		//logger.Errorf(flowAlarmLog, fmt.Sprintf("获取当前模型(%s)详情失败:%s", modelID, err.Error()))
		return fmt.Errorf("获取当前模型(%s)详情失败:%s", modelID, err.Error())
	}

	////logger.Debugf(flowAlarmLog, "开始遍历流程列表")
flowloop:
	for _, flowInfo := range *flowInfoList {
		//logger.Debugf(flowAlarmLog, "开始分析流程")
		flowID := flowInfo.ID
		settings := flowInfo.Settings

		//判断是否已经失效
		if invalid, ok := settings["invalid"].(bool); ok {
			if invalid {
				//logger.Warnln(flowLog, "流程(%s)已经失效", flowID)
				continue
			}
		}

		//判断禁用
		if disable, ok := settings["disable"].(bool); ok {
			if disable {
				//logger.Warnln(flowAlarmLog, "流程(%s)已经被禁用", flowID)
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
						if startTime, ok := settings["startTime"].(string); ok {
							formatLayout := timex.FormatTimeFormat(startTime)
							formatStartTime, err := timex.ConvertStringToTime(formatLayout, startTime, time.Local)
							if err != nil {
								logger.Errorf( "开始时间范围字段值格式错误:%s", err.Error())
								continue
							}
							if timex.GetLocalTimeNow(time.Now()).Unix() < formatStartTime.Unix() {
								//logger.Debugf(flowAlarmLog, "流程(%s)的定时任务开始时间未到，不执行", flowID)
								continue
							}
						}

						if endTime, ok := settings["endTime"].(string); ok {
							formatLayout := timex.FormatTimeFormat(endTime)
							formatEndTime, err := timex.ConvertStringToTime(formatLayout, endTime, time.Local)
							if err != nil {
								logger.Errorf( "时间范围字段值格式错误:%s", err.Error())
								continue
							}
							if timex.GetLocalTimeNow(time.Now()).Unix() >= formatEndTime.Unix() {
								//logger.Debugf(flowAlarmLog, "流程(%s)的定时任务结束时间已到，不执行", flowID)
								//修改流程为失效
								updateMap := bson.M{"settings.invalid": true}
								//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("flow"), flowID, updateMap)
								var r = make(map[string]interface{})
								err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
								if err != nil {
									//logger.Errorf(flowAlarmLog, "失效流程(%s)失败:%s", flowID, err.Error())
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
		hasValidAction := false

		if rangeTypeRawList, ok := settings["eventRange"].([]interface{}); ok {
			rangeTypeList := formatx.InterfaceListToStringList(rangeTypeRawList)
			if actionList, ok := settings["action"].([]interface{}); ok {
				actionStringList := formatx.InterfaceListToStringList(actionList)
				for _, action := range actionStringList {
					if action == actionType {
						hasValidAction = true
						break
					}
				}
			}
			if !hasValidAction {
				////logger.Debugf(flowAlarmLog, "报警流程(%s)类型未对应当前信息", flowID)
				continue
			}

			hasValidWarn := false
			needCount := len(rangeTypeList)
			actualCount := 0

			for _, rangeType := range rangeTypeList {
				switch rangeType {
				case "warnType":
					hasValidWarn = false
					if warnTypeList, ok := settings["warnType"].([]interface{}); ok {
						warnTypeStringList := formatx.InterfaceListToStringList(warnTypeList)
						for _, warnType := range warnTypeStringList {
							if warnType == data.Type {
								hasValidWarn = true
								break
							}
						}
					}
					if hasValidWarn {
						actualCount++
					}
				case "warnLevel":
					hasValidWarn = false
					if warnLevelList, ok := settings["level"].([]interface{}); ok {
						warnLevelStringList := formatx.InterfaceListToStringList(warnLevelList)
						for _, warnLevel := range warnLevelStringList {
							if warnLevel == data.Level {
								hasValidWarn = true
								break
							}
						}
					}
					if hasValidWarn {
						actualCount++
					}
				case "department":
					hasValidWarn = false
					deptIDInDataList := data.Department
					if departmentList, ok := settings["department"].([]interface{}); ok {
					deptLoop:
						for _, dept := range departmentList {
							if deptMap, ok := dept.(map[string]interface{}); ok {
								if deptID, ok := deptMap["id"].(string); ok {
									for _, deptIDInData := range deptIDInDataList {
										if deptIDInData == deptID {
											hasValidWarn = true
											break deptLoop
										}
									}
								}
							}
						}
					}
					if hasValidWarn {
						actualCount++
					}
				case "model":
					hasValidWarn = false
					//判断该流程是否指定了特定报警规则
					if modelList, ok := settings["model"].([]interface{}); ok {
						for _, model := range modelList {
							if modelMap, ok := model.(map[string]interface{}); ok {
								if modelIDInSettings, ok := modelMap["id"].(string); ok {
									if modelID == modelIDInSettings {
										if ruleList, ok := settings["rule"].([]interface{}); ok {
											if len(ruleList) != 0 {
												for _, rule := range ruleList {
													if ruleMap, ok := rule.(map[string]interface{}); ok {
														if ruleIDInSettings, ok := ruleMap["id"].(string); ok {
															if data.RuleID == ruleIDInSettings {
																hasValidWarn = true
																break
															}
														}
													}
												}
											} else {
												hasValidWarn = true
												break
											}
										} else {
											hasValidWarn = true
											break
										}
									}
								}
							}
						}
					} else if ruleList, ok := settings["rule"].([]interface{}); ok {
						for _, rule := range ruleList {
							if ruleMap, ok := rule.(map[string]interface{}); ok {
								if ruleIDInSettings, ok := ruleMap["id"].(string); ok {
									if data.RuleID == ruleIDInSettings {
										hasValidWarn = true
										break
									}
								}
							}
						}
					}
					if hasValidWarn {
						actualCount++
					}
				case "node":
					hasValidWarn = false
					//判断该流程是否指定了特定报警规则
					if nodeList, ok := settings["node"].([]interface{}); ok {
						for _, node := range nodeList {
							if nodeMap, ok := node.(map[string]interface{}); ok {
								if nodeIDInSettings, ok := nodeMap["id"].(string); ok {
									if nodeID == nodeIDInSettings {
										if ruleList, ok := settings["rule"].([]interface{}); ok {
											if len(ruleList) != 0 {
												for _, rule := range ruleList {
													if ruleMap, ok := rule.(map[string]interface{}); ok {
														if ruleIDInSettings, ok := ruleMap["id"].(string); ok {
															if data.RuleID == ruleIDInSettings {
																hasValidWarn = true
																break
															}
														}
													}
												}
											} else {
												hasValidWarn = true
												break
											}
										} else {
											hasValidWarn = true
											break
										}
									}
								}
							}
						}
					} else if ruleList, ok := settings["rule"].([]interface{}); ok {
						for _, rule := range ruleList {
							if ruleMap, ok := rule.(map[string]interface{}); ok {
								if ruleIDInSettings, ok := ruleMap["id"].(string); ok {
									if data.RuleID == ruleIDInSettings {
										hasValidWarn = true
										break
									}
								}
							}
						}
					}
					if hasValidWarn {
						actualCount++
					}
				}
			}

			if !hasValidWarn || (actualCount != needCount) {
				////logger.Debugf(flowAlarmLog, "报警流程(%s)类型未对应当前信息", flowID)
				continue flowloop
			}

			//生成发送消息
			departmentStringIDList := make([]string, 0)
			//var departmentObjectList primitive.A
			if departmentIDList, ok := nodeInfo["department"].([]interface{}); ok {
				departmentStringIDList = formatx.InterfaceListToStringList(departmentIDList)
			} else {
				//logger.Warnf(flowAlarmLog, "资产(%s)的部门字段不存在或类型错误", nodeID)
			}

			deptInfoList := make([]map[string]interface{}, 0)
			if len(departmentStringIDList) != 0 {
				err := department.GetByList(ctx, redisClient, mongoClient, projectName, departmentStringIDList, &deptInfoList)
				if err != nil {
					//logger.Errorf(flowAlarmLog, "获取当前资产(%s)所属部门失败:%s", nodeID, err.Error())
					continue
				}
			}
			dataMappingType, err := setting.GetByWarnKindID(ctx, redisClient, mongoClient, projectName, data.Type)

			if err != nil {
				//logger.Errorf(flowAlarmLog, fmt.Sprintf("获取当前资产(%s)的报警类型中文失败:%s", nodeID, err.Error()))
				continue
			}
			//生成报警对象并发送
			sendMap := bson.M{
				"time": nowTimeString,
				"type": dataMappingType,
				//"status":         data.Status,
				//"processed":      data.Processed,
				"desc":           data.Desc,
				"level":          data.Level,
				"departmentName": formatx.FormatKeyInfoListMap(deptInfoList, "name"),
				"modelName":      formatx.FormatKeyInfo(modelInfo, "name"),
				"nodeName":       formatx.FormatKeyInfo(nodeInfo, "name"),
				"nodeUid":        formatx.FormatKeyInfo(nodeInfo, "uid"),
				"userName":       data.HandleUserName,
				"action":         actionType,
				//"tagInfo":        formatx.FormatDataInfoList(data.Fields),
				////"fields":         fieldsMap,
				//"isWarning": true,
			}
			err = flowx.StartFlow(zbClient,flowInfo.FlowXml,projectName,sendMap)
			if err != nil {
				logger.Errorf("流程(%s)推进到下一阶段失败:%s", flowID,err.Error())
				continue
			}
			hasExecute = true
		}

		//对只能执行一次的流程进行失效
		if validTime == "timeLimit" {
			if rangeDefine == "once" && hasExecute {
				//logger.Warnln(flowAlarmLog, "流程(%s)为只执行一次的流程", flowID)
				//修改流程为失效
				updateMap := bson.M{"settings.invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("flow"), flowID, updateMap)
				var r = make(map[string]interface{})
				err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
				if err != nil {
					//logger.Errorf(flowAlarmLog, "失效流程(%s)失败:%s", flowID, err.Error())
					continue
				}
			}
		}
	}

	////logger.Debugf(eventAlarmLog, "报警规则触发器执行结束")
	return nil
}
