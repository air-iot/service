package flow

import (
	"context"
	"fmt"
	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/logger"
	"github.com/air-iot/service/util/flowx"
	"github.com/air-iot/service/util/json"
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
		"资产修改":   "编辑资产属性",
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
			"type":                DeviceModify,
			"settings.eventType":  modifyTypeAfterMapping,
			"settings.eventRange": "node",
		},
		"project": map[string]interface{}{"flowXml":1,"settings": 1, "invalid": 1, "disable": 1, "startTime": 1, "endTime": 1},
	}
	err := apiClient.FindFlowQuery(headerMap, query, &flowInfoList)

	if err != nil {
		return fmt.Errorf("获取当前资产修改内容类型(%s)的资产修改逻辑流程失败:%s", modelID, err.Error())
	}

	logger.Debugf("开始遍历流程列表:%d",len(flowInfoList))
	variables := map[string]interface{}{"#project": projectName}
	variablesBytes, err := json.Marshal(variables)
	if err != nil {
		return err
	}
flowloop:
	for _, flowInfo := range flowInfoList {

		//logger.Debugf(eventDeviceModifyLog, "开始分析流程")
		if flowID, ok := flowInfo["id"].(string); ok {
			if settings, ok := flowInfo["settings"].(map[string]interface{}); ok {

				//判断是否已经失效
				if invalid, ok := flowInfo["invalid"].(bool); ok {
					if invalid {
						logger.Warnln( "流程(%s)已经失效", flowID)
						continue
					}
				}

				//判断禁用
				if disable, ok := flowInfo["disable"].(bool); ok {
					if disable {
						logger.Warnln("流程(%s)已经被禁用", flowID)
						continue
					}
				}

				rangeDefine := ""
				//validTime, ok := flowInfo["validTime"].(string)
				//if ok {
				//	if validTime == "timeLimit" {
				//		if rangeDefine, ok = flowInfo["range"].(string); ok {
				//			if rangeDefine != "once" {
				//判断有效期
				if startTime, ok := flowInfo["startTime"].(primitive.DateTime); ok {
					startTimeInt := int64(startTime / 1e3)
					if timex.GetLocalTimeNow(time.Now()).Unix() < startTimeInt {
						logger.Debugf( "流程(%s)的定时任务开始时间未到，不执行", flowID)
						continue
					}
				}

				if endTime, ok := flowInfo["endTime"].(primitive.DateTime); ok {
					endTimeInt := int64(endTime / 1e3)
					if timex.GetLocalTimeNow(time.Now()).Unix() >= endTimeInt {
						logger.Debugf("流程(%s)的定时任务结束时间已到，不执行", flowID)
						//修改流程为失效
						updateMap := bson.M{"invalid": true}
						//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), flowID, updateMap)
						var r = make(map[string]interface{})
						err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
						if err != nil {
							logger.Errorf("失效流程(%s)失败:%s", flowID, err.Error())
							continue
						}
						continue
					}
				}
				//}
				//		}
				//	}
				//}

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
				//		//logger.Warnln(eventDeviceModifyLog, "流程(%s)修改属性不是触发流程需要的:%+v", flowID, operateDataMap)
				//		continue
				//	}
				//}

				//departmentConditionList := make([]string, 0)
				//modelConditionList := make([]string, 0)

				isValid := false
				if rangType, ok := settings["eventRange"].(string); ok {
					switch rangType {
					case "node":
						departmentListInSettings := make([]string, 0)
						modelIDInSettings := ""
						//判断该流程是否指定了特定资产
						if nodeList, ok := settings["node"].([]interface{}); ok {
							for _, nodeEle := range nodeList {
								if nodeMap, ok := nodeEle.(map[string]interface{}); ok {
									if nodeIDInSettings, ok := nodeMap["id"].(string); ok {
										if nodeID == nodeIDInSettings {
											if modifyTypeAfterMapping == "编辑资产属性" {
												if props, ok := settings["prop"].([]interface{}); ok {
													if len(props) != 0 {
														counter := 0
														if dataMap, ok := data["data"].(map[string]interface{}); ok {
															for _, prop := range props {
																b, err := json.Marshal(prop)
																if err != nil {
																	logger.Errorf("流程(%s)的资产属性配置数组序列化失败:%s", flowID, err.Error())
																	continue
																}
																logic := entity.Logic{}
																err = json.Unmarshal(b, &logic)
																if err != nil {
																	logger.Errorf("流程(%s)的资产属性配置数组解序列化失败:%s", flowID, err.Error())
																	continue
																}
																compare := logic.Compare
																if compare.Value != "" {
																	formatVal, err := ConvertVariable(ctx, apiClient, variablesBytes, compare.Value)
																	if err != nil {
																		logger.Errorf("流程(%s)中替换模板Value变量失败:%s", flowID, err.Error())
																		continue
																	}
																	logic.Compare.Value = formatVal
																}
																if compare.StartTime.Value != "" {
																	formatVal, err := ConvertVariable(ctx, apiClient, variablesBytes, compare.StartTime.Value)
																	if err != nil {
																		logger.Errorf("流程(%s)中替换模板StartTime变量失败:%s", flowID, err.Error())
																		continue
																	}
																	logic.Compare.StartTime.Value = formatVal
																}
																if compare.EndTime.Value != "" {
																	formatVal, err := ConvertVariable(ctx, apiClient, variablesBytes, compare.EndTime.Value)
																	if err != nil {
																		logger.Errorf("流程(%s)中替换模板EndTime变量失败:%s", flowID, err.Error())
																		continue
																	}
																	logic.Compare.EndTime.Value = formatVal
																}
																if compare.StartValue.Value != "" {
																	formatVal, err := ConvertVariable(ctx, apiClient, variablesBytes, compare.StartValue.Value)
																	if err != nil {
																		logger.Errorf("流程(%s)中替换模板StartValue变量失败:%s", flowID, err.Error())
																		continue
																	}
																	logic.Compare.StartValue.Value = formatVal
																}
																if compare.EndValue.Value != "" {
																	formatVal, err := ConvertVariable(ctx, apiClient, variablesBytes, compare.EndValue.Value)
																	if err != nil {
																		logger.Errorf("流程(%s)中替换模板EndValue变量失败:%s", flowID, err.Error())
																		continue
																	}
																	logic.Compare.EndValue.Value = formatVal
																}
																compare = logic.Compare
																switch logic.PropType {
																case "自定义属性":
																	if custom, ok := dataMap["custom"].(map[string]interface{}); ok {
																		switch logic.DataType {
																		case "字符串":
																			if custom[logic.ID] == compare.Value {
																				counter++
																			}
																		case "选择器":
																			if custom[logic.ID] == compare.Value {
																				counter++
																			}
																		case "数字":
																			if custom[logic.ID] == compare.Value {
																				counter++
																			}
																		case "时间":
																			compareInValue := int64(0)
																			if ele, ok := compare.Value.(string); ok {
																				if ele != "" {
																					eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
																					if err != nil {
																						continue
																					}
																					compareInValue = eleTime.Unix()
																				}
																			}
																			dataVal := int64(0)
																			eleRaw, ok := custom[logic.ID].(string)
																			if !ok {
																				continue
																			} else {
																				if eleRaw != "" {
																					formatLayout := timex.FormatTimeFormat(eleRaw)
																					timeConvert, err := timex.ConvertStringToTime(formatLayout, eleRaw, time.Local)
																					if err != nil {
																						logger.Errorf("传入的时间转换失败:%s", err.Error())
																						continue
																					}
																					dataVal = timeConvert.Unix()
																				}
																			}
																			if compare.TimeType != "" {
																				startPoint := int64(0)
																				endPoint := int64(0)
																				switch compare.TimeType {
																				case "今天":
																					startPoint = timex.GetUnixToNewTimeDay(0).Unix()
																					endPoint = timex.GetUnixToNewTimeDay(1).Unix()
																				case "昨天":
																					startPoint = timex.GetUnixToNewTimeDay(-1).Unix()
																					endPoint = timex.GetUnixToNewTimeDay(0).Unix()
																				case "明天":
																					startPoint = timex.GetUnixToNewTimeDay(1).Unix()
																					endPoint = timex.GetUnixToNewTimeDay(2).Unix()
																				case "本周":
																					week := int(time.Now().Weekday())
																					if week == 0 {
																						week = 7
																					}
																					startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Unix()
																					endPoint = timex.GetUnixToNewTimeDay(8 - week).Unix()
																				case "上周":
																					week := int(time.Now().Weekday())
																					if week == 0 {
																						week = 7
																					}
																					startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Unix()
																					endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Unix()
																				case "今年":
																					startPoint = timex.GetUnixToOldYearTime(0, 0).Unix()
																					endPoint = timex.GetUnixToOldYearTime(-1, 0).Unix()
																				case "去年":
																					startPoint = timex.GetUnixToOldYearTime(1, 0).Unix()
																					endPoint = timex.GetUnixToOldYearTime(0, 0).Unix()
																				case "明年":

																					startPoint = timex.GetUnixToOldYearTime(-1, 0).Unix()
																					endPoint = timex.GetUnixToOldYearTime(-2, 0).Unix()
																				case "指定时间":
																					if compare.SpecificTime != "" {
																						eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.SpecificTime), compare.SpecificTime, time.Local)
																						if err != nil {
																							continue
																						}
																						compareInValue = eleTime.Unix()
																					}
																					if dataVal == compareInValue {
																						counter++
																					}
																					continue
																				}
																				if dataVal >= startPoint && dataVal < endPoint {
																					counter++
																				}
																			}
																		case "布尔", "附件", "定位":
																			if custom[logic.ID] == compare.Value {
																				counter++
																			}
																		case "数组":
																			if valueList, ok := compare.Value.([]interface{}); ok {
																				valueStringList := formatx.InterfaceListToStringList(valueList)
																				if dataVal, ok := custom[logic.ID]; ok {
																					if _, ok := dataVal.([]interface{}); ok {
																						err := formatx.FormatObjectToIDListMap(&custom, logic.ID, "id")
																						if err != nil {
																							return fmt.Errorf("传入参数(%s)对象中没有id字段:%s", logic.ID, err.Error())
																						}
																						if dataValStringList, ok := custom[logic.ID].([]string); ok {
																							if len(valueStringList) != len(dataValStringList) {
																								continue
																							} else {
																								matchMap := map[string]int{}
																								for _, ele := range valueStringList {
																									matchMap[ele] = 1
																								}
																								for _, ele := range dataValStringList {
																									if _, ok := matchMap[ele]; ok {
																										delete(matchMap, ele)
																									} else {
																										matchMap[ele] = -1
																									}
																								}
																								if len(matchMap) == 0 {
																									counter++
																								}
																							}
																						}
																					}
																				}
																			}
																		}
																	}
																case "内置属性":
																	switch logic.DataType {
																	case "字符串":
																		if dataMap[logic.ID] == compare.Value {
																			counter++
																		}
																	case "选择器":
																		if dataMap[logic.ID] == compare.Value {
																			counter++
																		}
																	case "数字":
																		if dataMap[logic.ID] == compare.Value {
																			counter++
																		}
																	case "时间":
																		compareInValue := int64(0)
																		if ele, ok := compare.Value.(string); ok {
																			if ele != "" {
																				eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
																				if err != nil {
																					continue
																				}
																				compareInValue = eleTime.Unix()
																			}
																		}
																		dataVal := int64(0)
																		eleRaw, ok := data[logic.ID].(string)
																		if !ok {
																			continue
																		} else {
																			if eleRaw != "" {
																				formatLayout := timex.FormatTimeFormat(eleRaw)
																				timeConvert, err := timex.ConvertStringToTime(formatLayout, eleRaw, time.Local)
																				if err != nil {
																					logger.Errorf("传入的时间转换失败:%s", err.Error())
																					continue
																				}
																				dataVal = timeConvert.Unix()
																			}
																		}
																		if compare.TimeType != "" {
																			startPoint := int64(0)
																			endPoint := int64(0)
																			switch compare.TimeType {
																			case "今天":
																				startPoint = timex.GetUnixToNewTimeDay(0).Unix()
																				endPoint = timex.GetUnixToNewTimeDay(1).Unix()
																			case "昨天":
																				startPoint = timex.GetUnixToNewTimeDay(-1).Unix()
																				endPoint = timex.GetUnixToNewTimeDay(0).Unix()
																			case "明天":
																				startPoint = timex.GetUnixToNewTimeDay(1).Unix()
																				endPoint = timex.GetUnixToNewTimeDay(2).Unix()
																			case "本周":
																				week := int(time.Now().Weekday())
																				if week == 0 {
																					week = 7
																				}
																				startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Unix()
																				endPoint = timex.GetUnixToNewTimeDay(8 - week).Unix()
																			case "上周":
																				week := int(time.Now().Weekday())
																				if week == 0 {
																					week = 7
																				}
																				startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Unix()
																				endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Unix()
																			case "今年":
																				startPoint = timex.GetUnixToOldYearTime(0, 0).Unix()
																				endPoint = timex.GetUnixToOldYearTime(-1, 0).Unix()
																			case "去年":
																				startPoint = timex.GetUnixToOldYearTime(1, 0).Unix()
																				endPoint = timex.GetUnixToOldYearTime(0, 0).Unix()
																			case "明年":

																				startPoint = timex.GetUnixToOldYearTime(-1, 0).Unix()
																				endPoint = timex.GetUnixToOldYearTime(-2, 0).Unix()
																			case "指定时间":
																				if compare.SpecificTime != "" {
																					eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.SpecificTime), compare.SpecificTime, time.Local)
																					if err != nil {
																						continue
																					}
																					compareInValue = eleTime.Unix()
																				}
																				if dataVal == compareInValue {
																					counter++
																				}
																				continue
																			}
																			if dataVal >= startPoint && dataVal < endPoint {
																				counter++
																			}
																		}
																	case "布尔", "附件", "定位":
																		if dataMap[logic.ID] == compare.Value {
																			counter++
																		}
																	case "数组":
																		if valueList, ok := compare.Value.([]interface{}); ok {
																			if dataVal, ok := dataMap[logic.ID]; ok {
																				if _, ok := dataVal.([]interface{}); ok {
																					valueStringList := make([]string, 0)
																					for _, ele := range valueList {
																						if valueListM, ok := ele.(map[string]interface{}); ok {
																							if idRaw, ok := valueListM["id"]; ok {
																								if id, ok := idRaw.(string); ok {
																									valueStringList = append(valueStringList, id)
																								}
																							}
																						}
																					}
																					err := formatx.FormatObjectToIDListMap(&dataMap, logic.ID, "id")
																					if err != nil {
																						return fmt.Errorf("传入参数(%s)对象中没有id字段:%s", logic.ID, err.Error())
																					}
																					if dataValStringList, ok := dataMap[logic.ID].([]string); ok {
																						if len(valueStringList) != len(dataValStringList) {
																							continue
																						} else {
																							matchMap := map[string]int{}
																							for _, ele := range valueStringList {
																								matchMap[ele] = 1
																							}
																							for _, ele := range dataValStringList {
																								if _, ok := matchMap[ele]; ok {
																									delete(matchMap, ele)
																								} else {
																									matchMap[ele] = -1
																								}
																							}
																							if len(matchMap) == 0 {
																								counter++
																							}
																						}
																					}else{
																						logger.Debugf("修改的变量不是字符串数组(%s)",flowID)
																					}
																				}else{
																					logger.Debugf("修改的变量不是数组(%s)",flowID)
																				}
																			}else{
																				logger.Debugf("修改的变量中没有配置里需要的(%s)",flowID)
																			}
																		}else{
																			logger.Debugf("数组型的prop值不是数组(%s)",flowID)
																		}
																	}
																}
															}
														} else {
															logger.Debugf("传入的修改资产数据没有data字段或格式不正确")
															continue
														}
														if counter == 0 {
															logger.Debugf("传入的修改资产数据的修改字段未满足条件(资产)")
															continue
														}
													}
												}
											}

											nodeInfo := bson.M{}
											if initNode, ok := data["initNode"].(map[string]interface{}); ok {
												nodeInfo = initNode
											} else {
												err = node.Get(ctx, redisClient, mongoClient, projectName, nodeID, &nodeInfo)
												if err != nil {
													//logger.Errorf(eventDeviceModifyLog, "流程(%s)的资产缓存(%+v)查询失败", flowID, nodeID)
													nodeInfo = bson.M{}
													continue
												}
											}

											departmentStringIDList := departmentObjectIDList
											departmentMList := make([]map[string]interface{}, 0)
											err := department.GetByList(ctx, redisClient, mongoClient, projectName, departmentStringIDList, &departmentMList)
											if err != nil {
												//logger.Errorf(eventDeviceModifyLog, "流程(%s)的部门缓存(%+v)查询失败", flowID, departmentStringIDList)
												departmentMList = make([]map[string]interface{}, 0)
											}

											data["departmentName"] = formatx.FormatKeyInfoMapList(departmentMList, "name")

											modelEle := bson.M{}
											err = model.Get(ctx, redisClient, mongoClient, projectName, modelID, &modelEle)
											if err != nil {
												logger.Errorf("流程(%s)的模型缓存(%+v)查询失败", flowID, departmentStringIDList)
												continue
											}

											modelMList := make([]bson.M, 0)
											modelMList = append(modelMList, modelEle)

											data["modelName"] = formatx.FormatKeyInfoList(modelMList, "name")
											data["nodeName"] = formatx.FormatKeyInfo(nodeInfo, "name")
											data["nodeUid"] = formatx.FormatKeyInfo(nodeInfo, "uid")
											data["time"] = timex.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05")

											switch modifyTypeAfterMapping {
											case "编辑资产画面", "删除资产画面":
												dashboardInfo, ok := data["dashboard"].(map[string]interface{})
												if !ok {
													if dashboardInfoID, ok := data["dashboard"].(string); ok {
														data["#$dashboard"] = bson.M{dashboardInfoID: bson.M{"id": dashboardInfoID, "_tableName": "dashboard"}}
													} else {
														logger.Warnf("数据消息中dashboard字段类型不是字符串")
														continue
													}
													//logger.Errorf(eventDeviceModifyLog, "数据消息中dashboard字段不存在或类型错误")
												} else {
													data["dashboardName"] = formatx.FormatKeyInfo(dashboardInfo, "name")
													if dashboardInfoID, ok := dashboardInfo["id"].(string); ok {
														data["#$dashboard"] = bson.M{dashboardInfoID: bson.M{"id": dashboardInfoID, "_tableName": "dashboard"}}
													}
												}
											}

											deptMap := bson.M{}
											for _, id := range departmentStringIDList {
												deptMap[id] = bson.M{"id": id, "_tableName": "dept"}
											}
											data["#$department"] = deptMap
											data["#$model"] = bson.M{modelID: bson.M{"id": modelID, "_tableName": "model"}}
											data["#$node"] = bson.M{nodeID: bson.M{"id": nodeID, "_tableName": "node", "uid": nodeID}}
											//if loginTimeRaw, ok := data["time"].(string); ok {
											//	loginTime, err := timex.ConvertStringToTime("2006-01-02 15:04:05", loginTimeRaw, time.Local)
											//	if err != nil {
											//		continue
											//	}
											//	data["time"] = loginTime.UnixNano() / 1e6
											//}

											if contentInSettings, ok := settings["content"].(string); ok {
												b, err := json.Marshal(data)
												if err != nil {
													logger.Errorf("资产修改事件的内容序列化失败:%s", err.Error())
													continue
												}
												templateContent, err := TemplateVariableMappingFlow(ctx, apiClient, contentInSettings, string(b))
												if err != nil {
													logger.Errorf(fmt.Sprintf("资产修改事件的内容模板替换失败:%s", err.Error()))
													continue
												}
												data["content"] = templateContent
											}

											if flowXml, ok := flowInfo["flowXml"].(string); ok {
												err = flowx.StartFlow(zbClient, flowXml, projectName, data)
												if err != nil {
													logger.Errorf("流程(%s)推进到下一阶段失败:%s",flowID, err.Error())
												}
											} else {
												logger.Errorf("流程(%s)的xml不存在或类型错误", flowID)
											}
											logger.Errorf("流程(%s)推进到下一阶段成功",flowID)
											continue flowloop
										} else {
											logger.Warnf("流程(%s)中未匹配到要修改的资产",flowID)
											continue
										}
									}
								}
							}
						}
						//没有指定特定资产时，结合部门与模型进行判断
						err = formatx.FormatObjectToIDListMap(&settings, "department", "id")
						if err != nil {
							logger.Warnln( "流程配置的部门对象中id字段不存在或类型错误")
							continue
						}
						if ele, ok := settings["customModel"].(map[string]interface{}); ok {
							if len(ele) != 0 {
								err = formatx.FormatObjectToIDListMap(&settings, "customModel", "id")
								if err != nil {
									logger.Warnln("流程配置的部门对象中id字段不存在或类型错误")
									continue
								}
							}
						}
						departmentListInSettings, ok = settings["department"].([]string)
						if !ok {
							departmentListInSettings = make([]string, 0)
						}
						modelIDInSettings, ok = settings["customModel"].(string)
						if !ok {
							modelIDInSettings = ""
						}
						//modelListInSettings, ok = settings["model"].([]string)
						//if !ok {
						//	modelListInSettings = make([]string, 0)
						//}
						if len(departmentListInSettings) != 0 && len(modelIDInSettings) != 0 {
							if modelObjectID == modelIDInSettings {
							loop1:
								for _, departmentIDInSettings := range departmentListInSettings {
									for _, departmentID := range departmentObjectIDList {
										if departmentIDInSettings == departmentID {
											isValid = true
											break loop1
										}
									}
								}
							}
						} else if len(departmentListInSettings) != 0 && len(modelIDInSettings) == 0 {
						loop2:
							for _, departmentIDInSettings := range departmentListInSettings {
								for _, departmentID := range departmentObjectIDList {
									if departmentIDInSettings == departmentID {
										isValid = true
										break loop2
									}
								}
							}
						} else if len(departmentListInSettings) == 0 && len(modelIDInSettings) != 0 {
							if modelObjectID == modelIDInSettings {
								isValid = true
							}
						} else {
							logger.Warnf("流程(%s)中未匹配到要修改的资产(根据部门模型过滤):%s",flowID)
							continue
						}
					default:
						logger.Errorf("流程(%s)的流程范围字段值(%s)未匹配到", flowID, rangType)
						continue
					}
				}
				if isValid {
					if modifyTypeAfterMapping == "编辑资产属性" {
						if props, ok := settings["prop"].([]interface{}); ok {
							if len(props) != 0 {
								counter := 0
								if dataMap, ok := data["data"].(map[string]interface{}); ok {
									for _, prop := range props {
										b, err := json.Marshal(prop)
										if err != nil {
											logger.Errorf("流程(%s)的资产属性配置数组序列化失败:%s", flowID, err.Error())
											continue
										}
										logic := entity.Logic{}
										err = json.Unmarshal(b, &logic)
										if err != nil {
											logger.Errorf("流程(%s)的资产属性配置数组解序列化失败:%s", flowID, err.Error())
											continue
										}
										compare := logic.Compare
										if compare.Value != "" {
											formatVal, err := ConvertVariable(ctx, apiClient, variablesBytes, compare.Value)
											if err != nil {
												logger.Errorf("流程(%s)中替换模板Value变量失败:%s", flowID, err.Error())
												continue
											}
											logic.Compare.Value = formatVal
										}
										if compare.StartTime.Value != "" {
											formatVal, err := ConvertVariable(ctx, apiClient, variablesBytes, compare.StartTime.Value)
											if err != nil {
												logger.Errorf("流程(%s)中替换模板StartTime变量失败:%s", flowID, err.Error())
												continue
											}
											logic.Compare.StartTime.Value = formatVal
										}
										if compare.EndTime.Value != "" {
											formatVal, err := ConvertVariable(ctx, apiClient, variablesBytes, compare.EndTime.Value)
											if err != nil {
												logger.Errorf("流程(%s)中替换模板EndTime变量失败:%s", flowID, err.Error())
												continue
											}
											logic.Compare.EndTime.Value = formatVal
										}
										if compare.StartValue.Value != "" {
											formatVal, err := ConvertVariable(ctx, apiClient, variablesBytes, compare.StartValue.Value)
											if err != nil {
												logger.Errorf("流程(%s)中替换模板StartValue变量失败:%s", flowID, err.Error())
												continue
											}
											logic.Compare.StartValue.Value = formatVal
										}
										if compare.EndValue.Value != "" {
											formatVal, err := ConvertVariable(ctx, apiClient, variablesBytes, compare.EndValue.Value)
											if err != nil {
												logger.Errorf("流程(%s)中替换模板EndValue变量失败:%s", flowID, err.Error())
												continue
											}
											logic.Compare.EndValue.Value = formatVal
										}
										compare = logic.Compare
										switch logic.PropType {
										case "自定义属性":
											if custom, ok := dataMap["custom"].(map[string]interface{}); ok {
												switch logic.DataType {
												case "字符串":
													if custom[logic.ID] == compare.Value {
														counter++
													}
												case "选择器":
													if custom[logic.ID] == compare.Value {
														counter++
													}
												case "数字":
													if custom[logic.ID] == compare.Value {
														counter++
													}
												case "时间":
													compareInValue := int64(0)
													if ele, ok := compare.Value.(string); ok {
														if ele != "" {
															eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
															if err != nil {
																continue
															}
															compareInValue = eleTime.Unix()
														}
													}
													dataVal := int64(0)
													eleRaw, ok := custom[logic.ID].(string)
													if !ok {
														continue
													} else {
														if eleRaw != "" {
															formatLayout := timex.FormatTimeFormat(eleRaw)
															timeConvert, err := timex.ConvertStringToTime(formatLayout, eleRaw, time.Local)
															if err != nil {
																logger.Errorf("传入的时间转换失败:%s", err.Error())
																continue
															}
															dataVal = timeConvert.Unix()
														}
													}
													if compare.TimeType != "" {
														startPoint := int64(0)
														endPoint := int64(0)
														switch compare.TimeType {
														case "今天":
															startPoint = timex.GetUnixToNewTimeDay(0).Unix()
															endPoint = timex.GetUnixToNewTimeDay(1).Unix()
														case "昨天":
															startPoint = timex.GetUnixToNewTimeDay(-1).Unix()
															endPoint = timex.GetUnixToNewTimeDay(0).Unix()
														case "明天":
															startPoint = timex.GetUnixToNewTimeDay(1).Unix()
															endPoint = timex.GetUnixToNewTimeDay(2).Unix()
														case "本周":
															week := int(time.Now().Weekday())
															if week == 0 {
																week = 7
															}
															startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Unix()
															endPoint = timex.GetUnixToNewTimeDay(8 - week).Unix()
														case "上周":
															week := int(time.Now().Weekday())
															if week == 0 {
																week = 7
															}
															startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Unix()
															endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Unix()
														case "今年":
															startPoint = timex.GetUnixToOldYearTime(0, 0).Unix()
															endPoint = timex.GetUnixToOldYearTime(-1, 0).Unix()
														case "去年":
															startPoint = timex.GetUnixToOldYearTime(1, 0).Unix()
															endPoint = timex.GetUnixToOldYearTime(0, 0).Unix()
														case "明年":

															startPoint = timex.GetUnixToOldYearTime(-1, 0).Unix()
															endPoint = timex.GetUnixToOldYearTime(-2, 0).Unix()
														case "指定时间":
															if compare.SpecificTime != "" {
																eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.SpecificTime), compare.SpecificTime, time.Local)
																if err != nil {
																	continue
																}
																compareInValue = eleTime.Unix()
															}
															if dataVal == compareInValue {
																counter++
															}
															continue
														}
														if dataVal >= startPoint && dataVal < endPoint {
															counter++
														}
													}
												case "布尔", "附件", "定位":
													if custom[logic.ID] == compare.Value {
														counter++
													}
												case "数组":
													if valueList, ok := compare.Value.([]interface{}); ok {
														valueStringList := formatx.InterfaceListToStringList(valueList)
														if dataVal, ok := custom[logic.ID]; ok {
															if _, ok := dataVal.([]interface{}); ok {
																err := formatx.FormatObjectToIDListMap(&custom, logic.ID, "id")
																if err != nil {
																	return fmt.Errorf("传入参数(%s)对象中没有id字段:%s", logic.ID, err.Error())
																}
																if dataValStringList, ok := custom[logic.ID].([]string); ok {
																	if len(valueStringList) != len(dataValStringList) {
																		continue
																	} else {
																		matchMap := map[string]int{}
																		for _, ele := range valueStringList {
																			matchMap[ele] = 1
																		}
																		for _, ele := range dataValStringList {
																			if _, ok := matchMap[ele]; ok {
																				delete(matchMap, ele)
																			} else {
																				matchMap[ele] = -1
																			}
																		}
																		if len(matchMap) == 0 {
																			counter++
																		}
																	}
																}
															}
														}
													}
												}
											}
										case "内置属性":
											switch logic.DataType {
											case "字符串":
												if dataMap[logic.ID] == compare.Value {
													counter++
												}
											case "选择器":
												if dataMap[logic.ID] == compare.Value {
													counter++
												}
											case "数字":
												if dataMap[logic.ID] == compare.Value {
													counter++
												}
											case "时间":
												compareInValue := int64(0)
												if ele, ok := compare.Value.(string); ok {
													if ele != "" {
														eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
														if err != nil {
															continue
														}
														compareInValue = eleTime.Unix()
													}
												}
												dataVal := int64(0)
												eleRaw, ok := data[logic.ID].(string)
												if !ok {
													continue
												} else {
													if eleRaw != "" {
														formatLayout := timex.FormatTimeFormat(eleRaw)
														timeConvert, err := timex.ConvertStringToTime(formatLayout, eleRaw, time.Local)
														if err != nil {
															logger.Errorf("传入的时间转换失败:%s", err.Error())
															continue
														}
														dataVal = timeConvert.Unix()
													}
												}
												if compare.TimeType != "" {
													startPoint := int64(0)
													endPoint := int64(0)
													switch compare.TimeType {
													case "今天":
														startPoint = timex.GetUnixToNewTimeDay(0).Unix()
														endPoint = timex.GetUnixToNewTimeDay(1).Unix()
													case "昨天":
														startPoint = timex.GetUnixToNewTimeDay(-1).Unix()
														endPoint = timex.GetUnixToNewTimeDay(0).Unix()
													case "明天":
														startPoint = timex.GetUnixToNewTimeDay(1).Unix()
														endPoint = timex.GetUnixToNewTimeDay(2).Unix()
													case "本周":
														week := int(time.Now().Weekday())
														if week == 0 {
															week = 7
														}
														startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Unix()
														endPoint = timex.GetUnixToNewTimeDay(8 - week).Unix()
													case "上周":
														week := int(time.Now().Weekday())
														if week == 0 {
															week = 7
														}
														startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Unix()
														endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Unix()
													case "今年":
														startPoint = timex.GetUnixToOldYearTime(0, 0).Unix()
														endPoint = timex.GetUnixToOldYearTime(-1, 0).Unix()
													case "去年":
														startPoint = timex.GetUnixToOldYearTime(1, 0).Unix()
														endPoint = timex.GetUnixToOldYearTime(0, 0).Unix()
													case "明年":

														startPoint = timex.GetUnixToOldYearTime(-1, 0).Unix()
														endPoint = timex.GetUnixToOldYearTime(-2, 0).Unix()
													case "指定时间":
														if compare.SpecificTime != "" {
															eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.SpecificTime), compare.SpecificTime, time.Local)
															if err != nil {
																continue
															}
															compareInValue = eleTime.Unix()
														}
														if dataVal == compareInValue {
															counter++
														}
														continue
													}
													if dataVal >= startPoint && dataVal < endPoint {
														counter++
													}
												}
											case "布尔", "附件", "定位":
												if dataMap[logic.ID] == compare.Value {
													counter++
												}
											case "数组":
												if valueList, ok := compare.Value.([]interface{}); ok {
													if dataVal, ok := dataMap[logic.ID]; ok {
														if _, ok := dataVal.([]interface{}); ok {
															valueStringList := make([]string, 0)
															for _, ele := range valueList {
																if valueListM, ok := ele.(map[string]interface{}); ok {
																	if idRaw, ok := valueListM["id"]; ok {
																		if id, ok := idRaw.(string); ok {
																			valueStringList = append(valueStringList, id)
																		}
																	}
																}
															}
															err := formatx.FormatObjectToIDListMap(&dataMap, logic.ID, "id")
															if err != nil {
																return fmt.Errorf("传入参数(%s)对象中没有id字段:%s", logic.ID, err.Error())
															}
															if dataValStringList, ok := dataMap[logic.ID].([]string); ok {
																if len(valueStringList) != len(dataValStringList) {
																	continue
																} else {
																	matchMap := map[string]int{}
																	for _, ele := range valueStringList {
																		matchMap[ele] = 1
																	}
																	for _, ele := range dataValStringList {
																		if _, ok := matchMap[ele]; ok {
																			delete(matchMap, ele)
																		} else {
																			matchMap[ele] = -1
																		}
																	}
																	if len(matchMap) == 0 {
																		counter++
																	}
																}
															}else{
																logger.Debugf("修改的变量不是字符串数组(%s)",flowID)
															}
														}else{
															logger.Debugf("修改的变量不是数组(%s)",flowID)
														}
													}else{
														logger.Debugf("修改的变量中没有配置里需要的(%s)",flowID)
													}
												}else{
													logger.Debugf("数组型的prop值不是数组(%s)",flowID)
												}
											}
										}
									}
								} else {
									logger.Debugf("传入的修改资产数据没有data字段或格式不正确")
									continue
								}
								if counter == 0 {
									logger.Debugf("传入的修改资产数据的修改字段未满足条件(资产)")
									continue
								}
							}
						}
					}

					nodeInfo := bson.M{}
					if initNode, ok := data["initNode"].(map[string]interface{}); ok {
						nodeInfo = initNode
					} else {
						err = node.Get(ctx, redisClient, mongoClient, projectName, nodeID, &nodeInfo)
						if err != nil {
							logger.Errorf("流程(%s)的资产缓存(%+v)查询失败", flowID, nodeID)
							nodeInfo = bson.M{}
							continue
						}
					}

					departmentStringIDList := departmentObjectIDList
					departmentMList := make([]map[string]interface{}, 0)
					err := department.GetByList(ctx, redisClient, mongoClient, projectName, departmentStringIDList, &departmentMList)
					if err != nil {
						//logger.Errorf(eventDeviceModifyLog, "流程(%s)的部门缓存(%+v)查询失败", flowID, departmentStringIDList)
						departmentMList = make([]map[string]interface{}, 0)
					}

					data["departmentName"] = formatx.FormatKeyInfoMapList(departmentMList, "name")

					modelEle := bson.M{}
					err = model.Get(ctx, redisClient, mongoClient, projectName, modelID, &modelEle)
					if err != nil {
						logger.Errorf( "流程(%s)的模型缓存(%+v)查询失败", flowID, departmentStringIDList)
						continue
					}

					modelMList := make([]bson.M, 0)
					modelMList = append(modelMList, modelEle)

					data["modelName"] = formatx.FormatKeyInfoList(modelMList, "name")
					data["nodeName"] = formatx.FormatKeyInfo(nodeInfo, "name")
					data["nodeUid"] = formatx.FormatKeyInfo(nodeInfo, "uid")
					data["time"] = timex.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05")

					if contentInSettings, ok := settings["content"].(string); ok {
						b, err := json.Marshal(data)
						if err != nil {
							logger.Errorf(fmt.Sprintf("资产修改事件的内容序列化失败:%s", err.Error()))
							continue
						}
						templateContent, err := TemplateVariableMappingFlow(ctx, apiClient, contentInSettings, string(b))
						if err != nil {
							logger.Errorf(fmt.Sprintf("资产修改事件的内容模板替换失败:%s", err.Error()))
							continue
						}
						data["content"] = templateContent
					}

					switch modifyTypeAfterMapping {
					case "编辑资产画面", "删除资产画面":
						dashboardInfo, ok := data["dashboard"].(map[string]interface{})
						if !ok {
							if dashboardInfoID, ok := data["dashboard"].(string); ok {
								data["#$dashboard"] = bson.M{dashboardInfoID: bson.M{"id": dashboardInfoID, "_tableName": "dashboard"}}
							} else {
								logger.Errorf("数据消息中dashboard字段类型不是字符串")
								continue
							}
							//logger.Errorf(eventDeviceModifyLog, "数据消息中dashboard字段不存在或类型错误")
						} else {
							data["dashboardName"] = formatx.FormatKeyInfo(dashboardInfo, "name")
							if dashboardInfoID, ok := dashboardInfo["id"].(string); ok {
								data["#$dashboard"] = bson.M{dashboardInfoID: bson.M{"id": dashboardInfoID, "_tableName": "dashboard"}}
							}
						}
					}

					deptMap := bson.M{}
					for _, id := range departmentStringIDList {
						deptMap[id] = bson.M{"id": id, "_tableName": "dept"}
					}
					data["#$department"] = deptMap
					data["#$model"] = bson.M{modelID: bson.M{"id": modelID, "_tableName": "model"}}
					data["#$node"] = bson.M{nodeID: bson.M{"id": nodeID, "_tableName": "node", "uid": nodeID}}
					//if loginTimeRaw, ok := data["time"].(string); ok {
					//	loginTime, err := timex.ConvertStringToTime("2006-01-02 15:04:05", loginTimeRaw, time.Local)
					//	if err != nil {
					//		continue
					//	}
					//	data["time"] = loginTime.UnixNano() / 1e6
					//}

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
					logger.Errorf("流程(%s)推进到下一阶段成功:%s", flowID, err.Error())
					hasExecute = true
				}

				//对只能执行一次的流程进行失效
				//if validTime == "timeLimit" {
				if rangeDefine == "once" && hasExecute {
					//logger.Warnln(eventDeviceModifyLog, "流程(%s)为只执行一次的流程", flowID)
					//修改流程为失效
					updateMap := bson.M{"invalid": true}
					//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), flowID, updateMap)
					var r = make(map[string]interface{})
					err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
					if err != nil {
						//logger.Errorf(eventDeviceModifyLog, "失效流程(%s)失败:%s", flowID, err.Error())
						continue
					}
				}
				//}
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
			"type":                DeviceModify,
			"settings.eventType":  modifyTypeAfterMapping,
			"settings.eventRange": "model",
		},
		"project": map[string]interface{}{"flowXml":1,"settings": 1, "invalid": 1, "disable": 1, "startTime": 1, "endTime": 1},
	}

	err := apiClient.FindFlowQuery(headerMap, query, &flowInfoList)
	if err != nil {
		//logger.Errorf(eventDeviceModifyLog, "获取当前资产修改内容类型(%s)的资产修改逻辑流程失败:%s", modelID, err.Error())
		return fmt.Errorf("获取当前资产修改内容类型(%s)的资产修改逻辑流程失败:%s", modelID, err.Error())
	}

	for _, flowInfo := range flowInfoList {

		//fmt.Println("data:", data)
		//break
		//logger.Debugf(eventDeviceModifyLog, "开始分析流程")
		if flowID, ok := flowInfo["id"].(string); ok {
			if settings, ok := flowInfo["settings"].(map[string]interface{}); ok {

				//判断是否已经失效
				if invalid, ok := flowInfo["invalid"].(bool); ok {
					if invalid {
						logger.Warnln("流程(%s)已经失效", flowID)
						continue
					}
				}

				//判断禁用
				if disable, ok := flowInfo["disable"].(bool); ok {
					if disable {
						logger.Warnln("流程(%s)已经被禁用", flowID)
						continue
					}
				}

				rangeDefine := ""
				//validTime, ok := flowInfo["validTime"].(string)
				//if ok {
				//	if validTime == "timeLimit" {
				//		if rangeDefine, ok = flowInfo["range"].(string); ok {
				//			if rangeDefine != "once" {
				//判断有效期
				if startTime, ok := flowInfo["startTime"].(primitive.DateTime); ok {
					startTimeInt := int64(startTime / 1e3)
					if timex.GetLocalTimeNow(time.Now()).Unix() < startTimeInt {
						logger.Debugf("流程(%s)的定时任务开始时间未到，不执行", flowID)
						continue
					}
				}

				if endTime, ok := flowInfo["endTime"].(primitive.DateTime); ok {
					endTimeInt := int64(endTime / 1e3)
					if timex.GetLocalTimeNow(time.Now()).Unix() >= endTimeInt {
						logger.Debugf("流程(%s)的定时任务结束时间已到，不执行", flowID)
						//修改流程为失效
						updateMap := bson.M{"invalid": true}
						//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), flowID, updateMap)
						var r = make(map[string]interface{})
						err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
						if err != nil {
							logger.Errorf("失效流程(%s)失败:%s", flowID, err.Error())
							continue
						}
						continue
					}
				}
				//			}
				//		}
				//	}
				//}

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
						err = formatx.FormatObjectToIDListMap(&settings, "model", "id")
						if err != nil {
							logger.Warnln("流程配置的模型对象中id字段不存在或类型错误")
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
						logger.Errorf("流程(%s)的流程范围字段值(%s)未匹配到", flowID, rangType)
						continue
					}
				}
				//fmt.Println("isValid:", isValid)
				if isValid {
					departmentStringIDList := departmentObjectIDList
					departmentMList := make([]map[string]interface{}, 0)
					err := department.GetByList(ctx, redisClient, mongoClient, projectName, departmentStringIDList, &departmentMList)
					if err != nil {
						logger.Errorf("流程(%s)的部门缓存(%+v)查询失败", flowID, departmentStringIDList)
						departmentMList = make([]map[string]interface{}, 0)
					}

					data["departmentName"] = formatx.FormatKeyInfoMapList(departmentMList, "name")

					modelEle := bson.M{}
					err = model.Get(ctx, redisClient, mongoClient, projectName, modelID, &modelEle)
					if err != nil {
						logger.Errorf("流程(%s)的模型缓存(%+v)查询失败", flowID, departmentStringIDList)
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
								data["#$dashboard"] = bson.M{dashboardInfoID: bson.M{"id": dashboardInfoID, "_tableName": "dashboard"}}
							} else {
								logger.Errorf("数据消息中dashboard字段类型不是字符串")
								continue
							}
							//logger.Errorf(eventDeviceModifyLog, "数据消息中dashboard字段不存在或类型错误")
						} else {
							data["dashboardName"] = formatx.FormatKeyInfo(dashboardInfo, "name")
							if dashboardInfoID, ok := dashboardInfo["id"].(string); ok {
								data["#$dashboard"] = bson.M{dashboardInfoID: bson.M{"id": dashboardInfoID, "_tableName": "dashboard"}}
							}
						}
					}

					deptMap := bson.M{}
					for _, id := range departmentStringIDList {
						deptMap[id] = bson.M{"id": id, "_tableName": "dept"}
					}
					data["#$department"] = deptMap
					data["#$model"] = bson.M{modelID: bson.M{"id": modelID, "_tableName": "model"}}
					//if loginTimeRaw, ok := data["time"].(string); ok {
					//	loginTime, err := timex.ConvertStringToTime("2006-01-02 15:04:05", loginTimeRaw, time.Local)
					//	if err != nil {
					//		continue
					//	}
					//	data["time"] = loginTime.UnixNano() / 1e6
					//}

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
					logger.Errorf("流程(%s)推进到下一阶段成功", flowID)
					hasExecute = true
				}

				//对只能执行一次的流程进行失效
				//if validTime == "timeLimit" {
				if rangeDefine == "once" && hasExecute {
					//logger.Warnln(eventDeviceModifyLog, "流程(%s)为只执行一次的流程", flowID)
					//修改流程为失效
					updateMap := bson.M{"invalid": true}
					//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), flowID, updateMap)
					var r = make(map[string]interface{})
					err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
					if err != nil {
						logger.Errorf("失效流程(%s)失败:%s", flowID, err.Error())
						continue
					}
				}
				//}
			}
		}
	}

	////logger.Debugf(eventDeviceModifyLog, "资产修改流程触发器执行结束")
	return nil
}
