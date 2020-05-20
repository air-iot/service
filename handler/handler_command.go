package handler

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/air-iot/service/logger"
	imqtt "github.com/air-iot/service/mq/mqtt"
	"github.com/air-iot/service/tools"
	"go.mongodb.org/mongo-driver/bson"

	clogic "github.com/air-iot/service/logic"
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

	//modelObjectID, err := primitive.ObjectIDFromHex(modelID)
	//if err != nil {
	//	return fmt.Errorf("数据消息中modelId转ObjectID失败")
	//}

	//logger.Debugf(eventExecCmdLog, "开始获取当前模型的执行指令逻辑事件")
	//获取当前模型的执行指令逻辑事件=============================================
	eventInfoList, err := clogic.EventLogic.FindLocalCache(modelID + "|" + string(ExecCmd))
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
		logger.Warnf(eventComputeLogicLog, "资产(%s)的部门字段不存在或类型错误", nodeID)
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

		logger.Debugf(eventExecCmdLog, "开始分析事件")
		if command, ok := settings["command"].(map[string]interface{}); ok {
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
				}
			}
		}
	}

	//logger.Debugf(eventExecCmdLog, "执行指令事件触发器执行结束")
	return nil
}
