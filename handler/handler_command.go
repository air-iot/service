package handler

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/gin/ginx"
	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/cache/event"
	"github.com/air-iot/service/init/cache/model"
	"github.com/air-iot/service/init/cache/node"
	"github.com/air-iot/service/init/mq"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/logger"
	"github.com/air-iot/service/util/formatx"
	"github.com/air-iot/service/util/json"
	"github.com/air-iot/service/util/timex"
)

func TriggerExecCmd(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, mq mq.MQ, apiClient api.Client, projectName string,
	data map[string]interface{}) error {

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

	headerMap := map[string]string{ginx.XRequestProject: projectName}
	eventInfoList := new([]entity.Event)
	//logger.Debugf(eventExecCmdLog, "开始获取当前模型的执行指令逻辑事件")
	//获取当前模型的执行指令逻辑事件=============================================
	err := event.GetByType(ctx, redisClient, mongoClient, projectName, string(ExecCmd), eventInfoList)
	if err != nil {
		return fmt.Errorf("获取执行指令事件失败: %s", err.Error())
	}
	nodeInfo := make(map[string]interface{})
	err = node.Get(ctx, redisClient, mongoClient, projectName, nodeID, &nodeInfo)
	if err != nil {
		return fmt.Errorf("获取当前模型(%s)的资产(%s)失败:%s", modelID, nodeID, err.Error())
	}

	modelInfo := make(map[string]interface{})
	err = model.Get(ctx, redisClient, mongoClient, projectName, modelID, &modelInfo)
	if err != nil {
		return fmt.Errorf("获取当前模型(%s)详情失败:%s", modelID, err.Error())
	}

	departmentStringIDList := make([]string, 0)
	//var departmentObjectList primitive.A
	if departmentIDList, ok := nodeInfo["department"].([]interface{}); ok {
		departmentStringIDList = formatx.InterfaceListToStringList(departmentIDList)
	} else {
		logger.Warnf("资产(%s)的部门字段不存在或类型错误", nodeID)
	}

	//deptInfoList := make([]map[string]interface{}, 0)
	//if len(departmentStringIDList) != 0 {
	//	deptInfoList, err = clogic.DeptLogic.FindLocalCacheList(departmentStringIDList)
	//	if err != nil {
	//		return fmt.Errorf("获取当前资产(%s)所属部门失败:%s", nodeID, err.Error())
	//	}
	//}

	//logger.Debugf(eventExecCmdLog, "开始遍历事件列表")
	for _, eventInfo := range *eventInfoList {
		if eventInfo.Handlers == nil || len(eventInfo.Handlers) == 0 {
			logger.Warnln(eventAlarmLog, "handlers字段数组长度为0")
			continue
		}
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
				logger.Warnln("事件(%s)已经被禁用", eventID)
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
								logger.Debugf("事件(%s)的定时任务开始时间未到，不执行", eventID)
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
								logger.Debugf("事件(%s)的定时任务结束时间已到，不执行", eventID)
								//修改事件为失效
								updateMap := bson.M{"settings.invalid": true}
								//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
								var r = make(map[string]interface{})
								err := apiClient.UpdateEventById(headerMap, eventID, updateMap, &r)
								if err != nil {
									logger.Errorf("失效事件(%s)失败:%s", eventID, err.Error())
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
		logger.Debugf("开始分析事件")
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
								"time": timex.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05"),
								//"departmentName": formatx.FormatKeyInfoListMap(deptInfoList, "name"),
								"departmentName": departmentStringIDList,
								"modelName":      formatx.FormatKeyInfo(modelInfo, "name"),
								"nodeName":       formatx.FormatKeyInfo(nodeInfo, "name"),
								"nodeUid":        formatx.FormatKeyInfo(nodeInfo, "uid"),
								"commandName":    commandName,
							}
							b, err := json.Marshal(sendMap)
							if err != nil {
								continue
							}

							err = mq.Publish(ctx, []string{"event", eventID}, b)
							if err != nil {
								logger.Warnf("发送事件(%s)错误:%s", eventID, err.Error())
							} else {
								logger.Debugf("发送事件成功:%s,数据为:%+v", eventID, sendMap)
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
				logger.Warnln("事件(%s)为只执行一次的事件", eventID)
				//修改事件为失效
				updateMap := bson.M{"settings.invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
				var r = make(map[string]interface{})
				err := apiClient.UpdateEventById(headerMap, eventID, updateMap, &r)
				if err != nil {
					logger.Errorf("失效事件(%s)失败:%s", eventID, err.Error())
					continue
				}
			}
		}
	}

	//logger.Debugf(eventExecCmdLog, "执行指令事件触发器执行结束")
	return nil
}
