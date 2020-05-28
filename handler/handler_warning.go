package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"

	idb "github.com/air-iot/service/db/mongo"
	"github.com/air-iot/service/logger"
	clogic "github.com/air-iot/service/logic"
	cmodel "github.com/air-iot/service/model"
	imqtt "github.com/air-iot/service/mq/mqtt"
	"github.com/air-iot/service/restful-api"
	"github.com/air-iot/service/tools"
)

var eventAlarmLog = map[string]interface{}{"name": "报警事件触发"}

func TriggerWarningRules(data cmodel.WarningMessage, actionType string) error {
	//logger.Debugf(eventAlarmLog, "开始执行计算事件触发器")
	//logger.Debugf(eventAlarmLog, "传入参数为:%+v", data)

	nodeID := data.NodeID.Hex()
	if nodeID == "" {
		logger.Errorf(eventAlarmLog, fmt.Sprintf("数据消息中nodeId字段不存在或类型错误"))
		return fmt.Errorf("数据消息中nodeId字段不存在或类型错误")
	}

	modelID := data.ModelID.Hex()
	if modelID == "" {
		logger.Errorf(eventAlarmLog, fmt.Sprintf("数据消息中modelId字段不存在或类型错误"))
		return fmt.Errorf("数据消息中modelId字段不存在或类型错误")
	}

	nowTimeString := tools.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05")

	//fieldsMap := data.Fields
	//if len(fieldsMap) == 0 {
	//	logger.Errorf(eventAlarmLog, fmt.Sprintf("数据消息中fields字段不存在或类型错误"))
	//	return fmt.Errorf("数据消息中fields字段不存在或类型错误")
	//}
	//logger.Debugf(eventAlarmLog, "开始获取当前模型的计算逻辑事件")
	//获取当前模型的报警规则逻辑事件=============================================
	eventInfoList, err := clogic.EventLogic.FindLocalCacheByType(string(Alarm))
	if err != nil {
		logger.Debugf(eventAlarmLog, fmt.Sprintf("获取当前模型(%s)的报警事件失败:%s", modelID, err.Error()))
		return fmt.Errorf("获取当前模型(%s)的报警事件失败:%s", modelID, err.Error())
	}
	//logger.Debugf(eventAlarmLog, "开始获取当前模型对应资产ID的资产")
	//获取当前模型对应资产ID的资产
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

	//logger.Debugf(eventAlarmLog, "开始遍历事件列表")
	for _, eventInfo := range *eventInfoList {
		logger.Debugf(eventAlarmLog, "事件信息为:%+v", eventInfo)
		if eventInfo.Handlers == nil || len(eventInfo.Handlers) == 0 {
			logger.Warnln(eventAlarmLog, "handlers字段数组长度为0")
			continue
		}
		logger.Debugf(eventAlarmLog, "开始分析事件")
		eventID := eventInfo.ID
		settings := eventInfo.Settings

		//判断是否已经失效
		invalid := eventInfo.Invalid
		if invalid {
			logger.Warnln(eventAlarmLog, "事件(%s)已经失效", eventID)
			continue
		}

		//判断禁用
		if disable, ok := settings["disable"].(bool); ok {
			if disable {
				logger.Warnln(eventAlarmLog, "事件(%s)已经被禁用", eventID)
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
								logger.Debugf(eventAlarmLog, "事件(%s)的定时任务开始时间未到，不执行", eventID)
								continue
							}
						}

						if endTime, ok := settings["endTime"].(time.Time); ok {
							if tools.GetLocalTimeNow(time.Now()).Unix() >= endTime.Unix() {
								logger.Debugf(eventAlarmLog, "事件(%s)的定时任务结束时间已到，不执行", eventID)
								//修改事件为失效
								updateMap := bson.M{"invalid": true}
								_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
								if err != nil {
									logger.Errorf(eventAlarmLog, "失效事件(%s)失败:%s", eventID, err.Error())
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
		hasValidAction := false

		if rangeType, ok := settings["range"].(string); ok {
			switch rangeType {
			case "warnType":
				if actionList, ok := settings["action"].([]interface{}); ok {
					actionStringList := tools.InterfaceListToStringList(actionList)
					for _, action := range actionStringList {
						if action == actionType {
							hasValidAction = true
							break
						}
					}
				}
				if !hasValidAction {
					//logger.Debugf(eventAlarmLog, "报警事件(%s)类型未对应当前信息", eventID)
					continue
				}
				hasValidType := false
				if warnTypeList, ok := settings["type"].([]interface{}); ok {
					warnTypeStringList := tools.InterfaceListToStringList(warnTypeList)
					for _, warnType := range warnTypeStringList {
						if warnType == data.Type {
							hasValidType = true
							break
						}
					}
				}
				if !hasValidType {
					//logger.Debugf(eventAlarmLog, "报警事件(%s)类型未对应当前信息", eventID)
					continue
				}

				//生成发送消息
				departmentStringIDList := make([]string, 0)
				//var departmentObjectList primitive.A
				if departmentIDList, ok := nodeInfo["department"].([]interface{}); ok {
					departmentStringIDList = tools.InterfaceListToStringList(departmentIDList)
				} else {
					logger.Warnf(eventAlarmLog, "资产(%s)的部门字段不存在或类型错误", nodeID)
				}

				deptInfoList := make([]map[string]interface{}, 0)
				if len(departmentStringIDList) != 0 {
					deptInfoList, err = clogic.DeptLogic.FindLocalCacheList(departmentStringIDList)
					if err != nil {
						return fmt.Errorf("获取当前资产(%s)所属部门失败:%s", nodeID, err.Error())
					}
				}
				//生成报警对象并发送
				sendMap := bson.M{
					"time":           nowTimeString,
					"type":           data.Type,
					"status":         data.Status,
					"processed":      data.Processed,
					"desc":           data.Desc,
					"level":          data.Level,
					"departmentName": tools.FormatKeyInfoListMap(deptInfoList, "name"),
					"modelName":      tools.FormatKeyInfo(modelInfo, "name"),
					"nodeName":       tools.FormatKeyInfo(nodeInfo, "name"),
					"nodeUid":        tools.FormatKeyInfo(nodeInfo, "uid"),
					"tagInfo":        tools.FormatDataInfoList(data.Fields),
					//"fields":         fieldsMap,
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

				b, err := json.Marshal(sendMap)
				if err != nil {
					continue
				}
				err = imqtt.Send(fmt.Sprintf("event/%s", eventID), b)
				if err != nil {
					logger.Warnf(eventAlarmLog, "发送事件(%s)错误:%s", eventID, err.Error())
				} else {
					logger.Debugf(eventAlarmLog, "发送事件成功:%s,数据为:%+v", eventID, sendMap)
				}
				hasExecute = true
			case "warnLevel":
				if actionList, ok := settings["action"].([]interface{}); ok {
					actionStringList := tools.InterfaceListToStringList(actionList)
					for _, action := range actionStringList {
						if action == actionType {
							hasValidAction = true
							break
						}
					}
				}
				if !hasValidAction {
					//logger.Debugf(eventAlarmLog, "报警事件(%s)类型未对应当前信息", eventID)
					continue
				}
				hasValidLevel := false
				if warnLevelList, ok := settings["level"].([]interface{}); ok {
					warnLevelStringList := tools.InterfaceListToStringList(warnLevelList)
					for _, warnLevel := range warnLevelStringList {
						if warnLevel == data.Level {
							hasValidLevel = true
							break
						}
					}
				}
				if !hasValidLevel {
					//logger.Debugf(eventAlarmLog, "报警事件(%s)类型未对应当前信息", eventID)
					continue
				}

				//生成发送消息
				departmentStringIDList := make([]string, 0)
				//var departmentObjectList primitive.A
				if departmentIDList, ok := nodeInfo["department"].([]interface{}); ok {
					departmentStringIDList = tools.InterfaceListToStringList(departmentIDList)
				} else {
					logger.Warnf(eventAlarmLog, "资产(%s)的部门字段不存在或类型错误", nodeID)
				}

				deptInfoList := make([]map[string]interface{}, 0)
				if len(departmentStringIDList) != 0 {
					deptInfoList, err = clogic.DeptLogic.FindLocalCacheList(departmentStringIDList)
					if err != nil {
						return fmt.Errorf("获取当前资产(%s)所属部门失败:%s", nodeID, err.Error())
					}
				}
				//生成报警对象并发送
				sendMap := bson.M{
					"time":           nowTimeString,
					"type":           data.Type,
					"status":         data.Status,
					"processed":      data.Processed,
					"desc":           data.Desc,
					"level":          data.Level,
					"departmentName": tools.FormatKeyInfoListMap(deptInfoList, "name"),
					"modelName":      tools.FormatKeyInfo(modelInfo, "name"),
					"nodeName":       tools.FormatKeyInfo(nodeInfo, "name"),
					"nodeUid":        tools.FormatKeyInfo(nodeInfo, "uid"),
					"tagInfo":        tools.FormatDataInfoList(data.Fields),
					//"fields":         fieldsMap,
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

				b, err := json.Marshal(sendMap)
				if err != nil {
					continue
				}
				err = imqtt.Send(fmt.Sprintf("event/%s", eventID), b)
				if err != nil {
					logger.Warnf(eventAlarmLog, "发送事件(%s)错误:%s", eventID, err.Error())
				} else {
					logger.Debugf(eventAlarmLog, "发送事件成功:%s,数据为:%+v", eventID, sendMap)
				}
				hasExecute = true
			case "department":
				if actionList, ok := settings["action"].([]interface{}); ok {
					actionStringList := tools.InterfaceListToStringList(actionList)
					for _, action := range actionStringList {
						if action == actionType {
							hasValidAction = true
							break
						}
					}
				}
				if !hasValidAction {
					//logger.Debugf(eventAlarmLog, "报警事件(%s)类型未对应当前信息", eventID)
					continue
				}
				hasValidWarn := false
				deptIDInDataList, err := tools.ObjectIdListToStringList(data.Department)
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
				if !hasValidWarn {
					//logger.Debugf(eventAlarmLog, "报警事件(%s)类型未对应当前信息", eventID)
					continue
				}

				//生成发送消息
				departmentStringIDList := make([]string, 0)
				//var departmentObjectList primitive.A
				if departmentIDList, ok := nodeInfo["department"].([]interface{}); ok {
					departmentStringIDList = tools.InterfaceListToStringList(departmentIDList)
				} else {
					logger.Warnf(eventAlarmLog, "资产(%s)的部门字段不存在或类型错误", nodeID)
				}

				deptInfoList := make([]map[string]interface{}, 0)
				if len(departmentStringIDList) != 0 {
					deptInfoList, err = clogic.DeptLogic.FindLocalCacheList(departmentStringIDList)
					if err != nil {
						return fmt.Errorf("获取当前资产(%s)所属部门失败:%s", nodeID, err.Error())
					}
				}
				//生成报警对象并发送
				sendMap := bson.M{
					"time":           nowTimeString,
					"type":           data.Type,
					"status":         data.Status,
					"processed":      data.Processed,
					"desc":           data.Desc,
					"level":          data.Level,
					"departmentName": tools.FormatKeyInfoListMap(deptInfoList, "name"),
					"modelName":      tools.FormatKeyInfo(modelInfo, "name"),
					"nodeName":       tools.FormatKeyInfo(nodeInfo, "name"),
					"nodeUid":        tools.FormatKeyInfo(nodeInfo, "uid"),
					"tagInfo":        tools.FormatDataInfoList(data.Fields),
					//"fields":         fieldsMap,
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

				b, err := json.Marshal(sendMap)
				if err != nil {
					continue
				}
				err = imqtt.Send(fmt.Sprintf("event/%s", eventID), b)
				if err != nil {
					logger.Warnf(eventAlarmLog, "发送事件(%s)错误:%s", eventID, err.Error())
				} else {
					logger.Debugf(eventAlarmLog, "发送事件成功:%s,数据为:%+v", eventID, sendMap)
				}
				hasExecute = true
			case "modelNode":
				if actionList, ok := settings["action"].([]interface{}); ok {
					actionStringList := tools.InterfaceListToStringList(actionList)
					for _, action := range actionStringList {
						if action == actionType {
							hasValidAction = true
							break
						}
					}
				}
				if !hasValidAction {
					//logger.Debugf(eventAlarmLog, "报警事件(%s)类型未对应当前信息", eventID)
					continue
				}
				hasValidWarn := false
				//判断该事件是否指定了特定报警规则
				if ruleList, ok := settings["rule"].([]interface{}); ok {
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
				} else if nodeList, ok := settings["node"].([]interface{}); ok {
					for _, node := range nodeList {
						if nodeMap, ok := node.(map[string]interface{}); ok {
							if nodeIDInSettings, ok := nodeMap["id"].(string); ok {
								if nodeID == nodeIDInSettings {
									hasValidWarn = true
									break
								}
							}
						}
					}
				} else if modelList, ok := settings["model"].([]interface{}); ok {
					for _, model := range modelList {
						if modelMap, ok := model.(map[string]interface{}); ok {
							if modelIDInSettings, ok := modelMap["id"].(string); ok {
								if modelID == modelIDInSettings {
									hasValidWarn = true
									break
								}
							}
						}
					}
				}
				if !hasValidWarn {
					//logger.Debugf(eventAlarmLog, "报警事件(%s)类型未对应当前信息", eventID)
					continue
				}

				//生成发送消息
				departmentStringIDList := make([]string, 0)
				//var departmentObjectList primitive.A
				if departmentIDList, ok := nodeInfo["department"].([]interface{}); ok {
					departmentStringIDList = tools.InterfaceListToStringList(departmentIDList)
				} else {
					logger.Warnf(eventAlarmLog, "资产(%s)的部门字段不存在或类型错误", nodeID)
				}

				deptInfoList := make([]map[string]interface{}, 0)
				if len(departmentStringIDList) != 0 {
					deptInfoList, err = clogic.DeptLogic.FindLocalCacheList(departmentStringIDList)
					if err != nil {
						return fmt.Errorf("获取当前资产(%s)所属部门失败:%s", nodeID, err.Error())
					}
				}
				//生成报警对象并发送
				sendMap := bson.M{
					"time":           nowTimeString,
					"type":           data.Type,
					"status":         data.Status,
					"processed":      data.Processed,
					"desc":           data.Desc,
					"level":          data.Level,
					"departmentName": tools.FormatKeyInfoListMap(deptInfoList, "name"),
					"modelName":      tools.FormatKeyInfo(modelInfo, "name"),
					"nodeName":       tools.FormatKeyInfo(nodeInfo, "name"),
					"nodeUid":        tools.FormatKeyInfo(nodeInfo, "uid"),
					"tagInfo":        tools.FormatDataInfoList(data.Fields),
					//"fields":         fieldsMap,
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

				b, err := json.Marshal(sendMap)
				if err != nil {
					continue
				}
				err = imqtt.Send(fmt.Sprintf("event/%s", eventID), b)
				if err != nil {
					logger.Warnf(eventAlarmLog, "发送事件(%s)错误:%s", eventID, err.Error())
				} else {
					logger.Debugf(eventAlarmLog, "发送事件成功:%s,数据为:%+v", eventID, sendMap)
				}
				hasExecute = true
			}
		}

		//对只能执行一次的事件进行失效
		if validTime == "timeLimit" {
			if rangeDefine == "once" && hasExecute {
				logger.Warnln(eventAlarmLog, "事件(%s)为只执行一次的事件", eventID)
				//修改事件为失效
				updateMap := bson.M{"invalid": true}
				_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
				if err != nil {
					logger.Errorf(eventAlarmLog, "失效事件(%s)失败:%s", eventID, err.Error())
					return fmt.Errorf("失效事件(%s)失败:%s", eventID, err.Error())
				}
			}
		}
	}

	//logger.Debugf(eventAlarmLog, "报警规则触发器执行结束")
	return nil
}
