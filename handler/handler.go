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
	DeviceModify EventType = "资产修改"

	//HandlerEmail     EventHandlerType = "email"
	//HandlerWechat    EventHandlerType = "wechat"
	//HandlerSMS       EventHandlerType = "sms"
	//HandlerMessage   EventHandlerType = "message"
	//HandlerCmd       EventHandlerType = "command"
	//HandlerScript    EventHandlerType = "script"
	//HandlerInputData EventHandlerType = "data"
	//HandlerDirect    EventHandlerType = "direct"

	HandlerDingtalk  EventHandlerType = "发送钉钉"
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
		if invalid, ok := settings["invalid"].(bool); ok {
			if invalid {
				logger.Warnln(eventLog, "事件(%s)已经失效", eventID)
				continue
			}
		}

		//判断禁用
		if disable, ok := settings["disable"].(bool); ok {
			if disable {
				logger.Warnln(eventLog, "事件(%s)已经被禁用", eventID)
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
									logger.Errorf(eventLog, "时间范围字段值格式错误:%s", err.Error())
									continue
								}
								//return restfulapi.NewHTTPError(http.StatusBadRequest, "startTime", fmt.Sprintf("时间范围字段格式错误:%s", err.Error()))
							}
							if tools.GetLocalTimeNow(time.Now()).Unix() < formatStartTime.Unix() {
								logger.Debugf(eventLog, "事件(%s)的定时任务开始时间未到，不执行", eventID)
								continue
							}
						}

						if endTime, ok := settings["endTime"].(string); ok {
							formatEndTime, err := tools.ConvertStringToTime("2006-01-02 15:04:05", endTime, time.Local)
							if err != nil {
								//logger.Errorf(logFieldsMap, "时间范围字段值格式错误:%s", err.Error())
								formatEndTime, err = tools.ConvertStringToTime("2006-01-02T15:04:05+08:00", endTime, time.Local)
								if err != nil {
									logger.Errorf(eventLog, "时间范围字段值格式错误:%s", err.Error())
									continue
								}
								//return restfulapi.NewHTTPError(http.StatusBadRequest, "startTime", fmt.Sprintf("时间范围字段格式错误:%s", err.Error()))
							}
							if tools.GetLocalTimeNow(time.Now()).Unix() >= formatEndTime.Unix() {
								logger.Debugf(eventLog, "事件(%s)的定时任务结束时间已到，不执行", eventID)
								//修改事件为失效
								updateMap := bson.M{"settings.invalid": true}
								//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
								var r = make(map[string]interface{})
								err := api.Cli.UpdateEventById(eventID, updateMap, &r)
								if err != nil {
									logger.Errorf(eventLog, "失效事件(%s)失败:%s", eventID, err.Error())
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
				logger.Warnln(eventLog, "事件(%s)为只执行一次的事件", eventID)
				//修改事件为失效
				updateMap := bson.M{"settings.invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
				var r = make(map[string]interface{})
				err := api.Cli.UpdateEventById(eventID, updateMap, &r)
				if err != nil {
					logger.Errorf(eventLog, "失效事件(%s)失败:%s", eventID, err.Error())
					continue
				}
			}
		}
	}
	return nil
}
