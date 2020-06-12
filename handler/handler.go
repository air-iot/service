package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/air-iot/service/restful-api"
	"github.com/air-iot/service/tools"
	"go.mongodb.org/mongo-driver/bson"
	"time"

	"github.com/air-iot/service/logger"
	imqtt "github.com/air-iot/service/mq/mqtt"
	idb "github.com/air-iot/service/db/mongo"
	clogic "github.com/air-iot/service/logic"
)

type EventType string
type EventHandlerType string

const (
	ComputeLogic EventType = "数据事件"
	Alarm        EventType = "报警事件"
	Startup      EventType = "启动系统"
	ShutDown     EventType = "关闭系统"
	Schedule     EventType = "计划事件"
	Login        EventType = "用户登录"
	Log          EventType = "日志事件"
	ExecCmd      EventType = "执行指令"
	DeviceModify EventType = "模型资产事件"

	//HandlerEmail     EventHandlerType = "email"
	//HandlerWechat    EventHandlerType = "wechat"
	//HandlerSMS       EventHandlerType = "sms"
	//HandlerMessage   EventHandlerType = "message"
	//HandlerCmd       EventHandlerType = "command"
	//HandlerScript    EventHandlerType = "script"
	//HandlerInputData EventHandlerType = "data"
	//HandlerDirect    EventHandlerType = "direct"

	HandlerEmail     EventHandlerType = "发送邮件"
	HandlerWechat    EventHandlerType = "发送微信"
	HandlerSMS       EventHandlerType = "发送短信"
	HandlerMessage   EventHandlerType = "发送站内信"
	HandlerCmd       EventHandlerType = "系统指令"
	HandlerScript    EventHandlerType = "执行脚本"
	HandlerInputData EventHandlerType = "录入数据"
	HandlerSystemCmd EventHandlerType = ""
)

var eventLog = map[string]interface{}{"name": "通用事件触发"}

// Trigger 事件触发
func Trigger(eventType EventType, data map[string]interface{}) error {
	result, err := clogic.EventLogic.FindLocalCacheByType(string(eventType))
	if err != nil {
		logger.Errorf(eventLog, "查询数据库错误:%s", err.Error())
		return err
	}

	for _, eventInfo := range *result {
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

		sendMap := data
		b, err := json.Marshal(sendMap)
		if err != nil {
			continue
		}
		err = imqtt.Send(fmt.Sprintf("event/%s", eventID), b)
		if err != nil {
			logger.Warnf(eventLog, "发送事件(%s)错误:%s", eventID, err.Error())
		} else {
			logger.Debugf(eventLog, "发送事件成功:%s,数据为:%+v", eventID, sendMap)
		}

		hasExecute = true

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
	return nil
}
