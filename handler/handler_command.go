package handler

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/air-iot/service/api/v2"
	"github.com/air-iot/service/logger"
	clogic "github.com/air-iot/service/logic"
	imqtt "github.com/air-iot/service/mq/mqtt"
	"github.com/air-iot/service/tools"
	"go.mongodb.org/mongo-driver/bson"
)

var eventExecCmdLog = map[string]interface{}{"name": "执行指令事件触发"}

func TriggerExecCmd(data map[string]interface{}) error {
	//logger.Debugf(eventExecCmdLog, "开始执行执行指令事件触发器")
	//logger.Debugf(eventExecCmdLog, "传入参数为:%+v", data)

	nodeID, ok := data["nodeId"].(string)
	if !ok {
		return fmt.Errorf("数据消息中nodeId字段不存在或类型错误")
	}

	modelID, ok := data["modelId"].(string)
	if !ok {
		return fmt.Errorf("数据消息中modelId字段不存在或类型错误")
	}

	command, ok := data["command"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("数据消息中command字段不存在或类型错误")
	}

	commandName, ok := command["name"].(string)
	if !ok {
		return fmt.Errorf("数据消息中command对象中name字段不存在或类型错误")
	}

	cmdStatus, ok := data["cmdStatus"].(string)
	if !ok {
		return fmt.Errorf("数据消息中cmdStatus字段不存在或类型错误")
	}

	//modelObjectID, err := primitive.ObjectIDFromHex(modelID)
	//if err != nil {
	//	return fmt.Errorf("数据消息中modelId转ObjectID失败")
	//}

	//logger.Debugf(eventExecCmdLog, "开始获取当前模型的执行指令逻辑事件")
	//获取当前模型的执行指令逻辑事件=============================================
	eventInfoList, err := clogic.EventLogic.FindLocalCache(string(ExecCmd))
	if err != nil {
		return fmt.Errorf("获取当前模型(%s)的执行指令逻辑事件失败:%s", modelID, err.Error())
	}

	nodeInfo, err := clogic.NodeLogic.FindLocalMapCache(nodeID)
	if err != nil {
		logger.Errorf(eventAlarmLog, fmt.Sprintf("获取当前模型(%s)的资产(%s)失败:%s", modelID, nodeID, err.Error()))
		return fmt.Errorf("获取当前模型(%s)的资产(%s)失败:%s", modelID, nodeID, err.Error())
	}

	modelInfo, err := clogic.ModelLogic.FindLocalMapCache(modelID)
	if err != nil {
		logger.Errorf(eventAlarmLog, fmt.Sprintf("获取当前模型(%s)详情失败:%s", modelID, err.Error()))
		return fmt.Errorf("获取当前模型(%s)详情失败:%s", modelID, err.Error())
	}

	departmentStringIDList := make([]string, 0)
	//var departmentObjectList primitive.A
	if departmentIDList, ok := nodeInfo["department"].([]interface{}); ok {
		departmentStringIDList = tools.InterfaceListToStringList(departmentIDList)
	} else {
		logger.Warnf(eventExecCmdLog, "资产(%s)的部门字段不存在或类型错误", nodeID)
	}

	deptInfoList := make([]map[string]interface{}, 0)
	if len(departmentStringIDList) != 0 {
		deptInfoList, err = clogic.DeptLogic.FindLocalCacheList(departmentStringIDList)
		if err != nil {
			return fmt.Errorf("获取当前资产(%s)所属部门失败:%s", nodeID, err.Error())
		}
	}

	//logger.Debugf(eventExecCmdLog, "开始遍历事件列表")
	for _, eventInfo := range *eventInfoList {
		logger.Debugf(eventExecCmdLog, "事件信息为:%+v", eventInfo)
		if eventInfo.Handlers == nil || len(eventInfo.Handlers) == 0 {
			logger.Warnln(eventAlarmLog, "handlers字段数组长度为0")
			continue
		}
		logger.Debugf(eventAlarmLog, "开始分析事件")
		eventID := eventInfo.ID
		settings := eventInfo.Settings

		//判断是否已经失效
		if invalid, ok := settings["invalid"].(bool); ok {
			if invalid {
				logger.Warnln(eventLog, "事件(%s)已经失效", eventID)
				continue
			}
		}

		//判断禁用
		if disable, ok := settings["disable"].(bool); ok {
			if disable {
				logger.Warnln(eventExecCmdLog, "事件(%s)已经被禁用", eventID)
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
							formatStartTime, err := tools.ConvertStringToTime("2006-01-02 15:04:05", startTime, time.Local)
							if err != nil {
								//logger.Errorf(logFieldsMap, "时间范围字段值格式错误:%s", err.Error())
								formatStartTime, err = tools.ConvertStringToTime("2006-01-02T15:04:05+08:00", startTime, time.Local)
								if err != nil {
									logger.Errorf(eventExecCmdLog, "时间范围字段值格式错误:%s", err.Error())
									continue
								}
								//return restfulapi.NewHTTPError(http.StatusBadRequest, "startTime", fmt.Sprintf("时间范围字段格式错误:%s", err.Error()))
							}
							if tools.GetLocalTimeNow(time.Now()).Unix() < formatStartTime.Unix() {
								logger.Debugf(eventExecCmdLog, "事件(%s)的定时任务开始时间未到，不执行", eventID)
								continue
							}
						}

						if endTime, ok := settings["endTime"].(string); ok {
							formatEndTime, err := tools.ConvertStringToTime("2006-01-02 15:04:05", endTime, time.Local)
							if err != nil {
								//logger.Errorf(logFieldsMap, "时间范围字段值格式错误:%s", err.Error())
								formatEndTime, err = tools.ConvertStringToTime("2006-01-02T15:04:05+08:00", endTime, time.Local)
								if err != nil {
									logger.Errorf(eventExecCmdLog, "时间范围字段值格式错误:%s", err.Error())
									continue
								}
								//return restfulapi.NewHTTPError(http.StatusBadRequest, "startTime", fmt.Sprintf("时间范围字段格式错误:%s", err.Error()))
							}
							if tools.GetLocalTimeNow(time.Now()).Unix() >= formatEndTime.Unix() {
								logger.Debugf(eventExecCmdLog, "事件(%s)的定时任务结束时间已到，不执行", eventID)
								//修改事件为失效
								updateMap := bson.M{"settings.invalid": true}
								//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
								var r = make(map[string]interface{})
								err := api.Cli.UpdateEventById(eventID, updateMap, &r)
								if err != nil {
									logger.Errorf(eventExecCmdLog, "失效事件(%s)失败:%s", eventID, err.Error())
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
		logger.Debugf(eventExecCmdLog, "开始分析事件")
		isValidCmd := false
		//判断该事件是否指定了特定资产
		if nodeMap, ok := settings["node"].(map[string]interface{}); ok {
			if nodeIDInSettings, ok := nodeMap["id"].(string); ok {
				if nodeID == nodeIDInSettings {
					isValidCmd = true
				}
			}
		} else if modelMap, ok := settings["model"].(map[string]interface{}); ok {
			if modelIDInSettings, ok := modelMap["id"].(string); ok {
				if modelID == modelIDInSettings {
					isValidCmd = true
				}
			}
		}

		if !isValidCmd {
			continue
		}

		isValidStatus := false
		if cmdStatusInSetting, ok := settings["cmdStatus"].(string); ok {
			if cmdStatus == cmdStatusInSetting {
				isValidStatus = true
			}
		}

		if !isValidStatus {
			continue
		}

		if commandList, ok := settings["command"].([]interface{}); ok {
			for _, commandRaw := range commandList {
				if command, ok := commandRaw.(map[string]interface{}); ok {
					if name, ok := command["name"].(string); ok {
						if commandName == name {
							sendMap := bson.M{
								"time":           tools.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05"),
								"departmentName": tools.FormatKeyInfoListMap(deptInfoList, "name"),
								"modelName":      tools.FormatKeyInfo(modelInfo, "name"),
								"nodeName":       tools.FormatKeyInfo(nodeInfo, "name"),
								"nodeUid":        tools.FormatKeyInfo(nodeInfo, "uid"),
								"commandName":    commandName,
							}
							b, err := json.Marshal(sendMap)
							if err != nil {
								continue
							}
							err = imqtt.Send(fmt.Sprintf("event/%s", eventID), b)
							if err != nil {
								logger.Warnf(eventExecCmdLog, "发送事件(%s)错误:%s", eventID, err.Error())
							} else {
								logger.Debugf(eventExecCmdLog, "发送事件成功:%s,数据为:%+v", eventID, sendMap)
							}

							hasExecute = true
							break
						}
					}
				}
			}
		}

		//对只能执行一次的事件进行失效
		if validTime == "timeLimit" {
			if rangeDefine == "once" && hasExecute {
				logger.Warnln(eventExecCmdLog, "事件(%s)为只执行一次的事件", eventID)
				//修改事件为失效
				updateMap := bson.M{"settings.invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
				var r = make(map[string]interface{})
				err := api.Cli.UpdateEventById(eventID, updateMap, &r)
				if err != nil {
					logger.Errorf(eventExecCmdLog, "失效事件(%s)失败:%s", eventID, err.Error())
					continue
				}
			}
		}
	}

	//logger.Debugf(eventExecCmdLog, "执行指令事件触发器执行结束")
	return nil
}
