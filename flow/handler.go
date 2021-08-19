package flow

import (
	"context"
	"github.com/air-iot/service/init/cache/flow"
	"github.com/air-iot/service/util/flowx"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/gin/ginx"
	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/mq"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/logger"
	"github.com/air-iot/service/util/timex"
	"github.com/camunda-cloud/zeebe/clients/go/pkg/zbc"
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
	ExtModify EventType = "工作表事件"

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

var flowLog = map[string]interface{}{"name": "通用流程触发"}

// TriggerFlow 流程触发
func TriggerFlow(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, mq mq.MQ, apiClient api.Client, zbClient zbc.Client, projectName string, flowType EventType, data map[string]interface{}) error {
	headerMap := map[string]string{ginx.XRequestProject: projectName}
	result := new([]entity.Flow)
	err := flow.GetByType(ctx, redisClient, mongoClient, projectName, string(flowType), result)
	if err != nil {
		//logger.Errorf(flowLog, "查询数据库错误:%s", err.Error())
		return err
	}

	for _, flowInfo := range *result {
		//logger.Debugf(flowAlarmLog, "开始分析流程")
		flowID := flowInfo.ID
		settings := flowInfo.Settings

		//判断是否已经失效
		if invalid, ok := settings["invalid"].(bool); ok {
			if invalid {
				logger.Warnln("流程(%s)已经失效", flowID)
				continue
			}
		}

		//判断禁用
		if disable, ok := settings["disable"].(bool); ok {
			if disable {
				logger.Warnln("流程(%s)已经被禁用", flowID)
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
								logger.Errorf("开始时间范围字段值格式错误:%s", err.Error())
								continue
							}
							if timex.GetLocalTimeNow(time.Now()).Unix() < formatStartTime.Unix() {
								//logger.Debugf(flowLog, "流程(%s)的定时任务开始时间未到，不执行", flowID)
								continue
							}
						}

						if endTime, ok := settings["endTime"].(string); ok {
							formatLayout := timex.FormatTimeFormat(endTime)
							formatEndTime, err := timex.ConvertStringToTime(formatLayout, endTime, time.Local)
							if err != nil {
								logger.Errorf("时间范围字段值格式错误:%s", err.Error())
								continue
							}
							if timex.GetLocalTimeNow(time.Now()).Unix() >= formatEndTime.Unix() {
								//logger.Debugf(flowLog, "流程(%s)的定时任务结束时间已到，不执行", flowID)
								//修改流程为失效
								updateMap := bson.M{"settings.invalid": true}
								//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("flow"), flowID, updateMap)
								var r = make(map[string]interface{})
								err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
								if err != nil {
									//logger.Errorf(flowLog, "失效流程(%s)失败:%s", flowID, err.Error())
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

		if loginTimeRaw, ok := data["time"].(string); ok {
			loginTime, err := timex.ConvertStringToTime("2006-01-02 15:04:05", loginTimeRaw, time.Local)
			if err != nil {
				continue
			}
			data["time"] = loginTime.UnixNano() / 1e6
		}

		err = flowx.StartFlow(zbClient,flowInfo.FlowXml,projectName,data)
		if err != nil {
			logger.Errorf("流程(%s)推进到下一阶段失败:%s", flowID,err.Error())
			continue
		}

		hasExecute = true

		//对只能执行一次的流程进行失效
		if validTime == "timeLimit" {
			if rangeDefine == "once" && hasExecute {
				logger.Warnln("流程(%s)为只执行一次的流程", flowID)
				//修改流程为失效
				updateMap := bson.M{"settings.invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("flow"), flowID, updateMap)
				var r = make(map[string]interface{})
				err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
				if err != nil {
					//logger.Errorf(flowLog, "失效流程(%s)失败:%s", eventID, err.Error())
					continue
				}
			}
		}
	}
	return nil
}
