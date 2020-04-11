package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	idb "github.com/air-iot/service/db/mongo"
	"github.com/air-iot/service/logger"
	imqtt "github.com/air-iot/service/mq/mqtt"
	"github.com/air-iot/service/restful-api"
)

type EventType string
type EventHandlerType string

const (
	ComputeLogic EventType = "计算逻辑事件"
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
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*10)
	defer cancel()
	pipeLine := mongo.Pipeline{
		bson.D{
			bson.E{
				Key: "$match",
				Value: bson.M{
					"type": eventType,
				},
			},
		},
		bson.D{
			bson.E{
				Key: "$lookup",
				Value: bson.M{
					"from":         "eventhandler",
					"localField":   "_id",
					"foreignField": "event",
					"as":           "handlers",
				},
			},
		},
	}

	result := make([]bson.M, 0)
	err := restfulapi.FindPipeline(ctx, idb.Database.Collection("event"), &result, pipeLine, nil)

	if err != nil {
		logger.Errorf(eventLog, "查询数据库错误:%s", err.Error())
		return err
	}


	for _, event := range result {
		id, ok := event["id"]
		if !ok {
			logger.Warnln(eventLog, "未找到id字段")
			continue
		}
		oid, ok := id.(primitive.ObjectID)
		if !ok {
			logger.Warnln(eventLog, "id字段非字符串")
			continue
		}
		handlers, ok := event["handlers"]
		if !ok {
			logger.Warnln(eventLog, "未找到handlers字段")
			continue
		}
		handler, ok := handlers.(primitive.A)
		if !ok {
			logger.Warnln(eventLog, "handlers字段转数组错误")
			continue
		}
		if len(handler) == 0 {
			logger.Warnln(eventLog, "handlers字段非数组")
			continue
		}
		sendMap := map[string]interface{}{
			"data": data,
		}
		b, err := json.Marshal(sendMap)
		if err != nil {
			continue
		}
		err = imqtt.Send(fmt.Sprintf("event/%s", oid.Hex()), b)
		if err != nil {
			logger.Warnf(eventLog, "发送事件(%s)错误:%s", id, err.Error())
		} else {
			logger.Debugf(eventLog, "发送事件成功:%s,数据为:%+v", id, sendMap)
		}
	}
	return nil
}
