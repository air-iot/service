package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	idb "github.com/air-iot/service/db/mongo"
	"github.com/air-iot/service/logger"
	imqtt "github.com/air-iot/service/mq/mqtt"
	"github.com/air-iot/service/restful-api"
	"github.com/air-iot/service/tools"
)

var eventScheduleLog = map[string]interface{}{"name": "计划事件触发"}

//var c *cron.Cron
//
//func init() {
//	c = cron.New()
//	c.Start()
//}

func TriggerAddSchedule(data map[string]interface{}, c *cron.Cron) error {
	//logger.Debugf(eventScheduleLog, "开始执行计划事件触发器（添加计划事件）")
	//logger.Debugf(eventScheduleLog, "传入参数为:%+v", data)

	eventID, ok := data["id"].(string)
	if !ok {
		return fmt.Errorf("传入参数中事件ID不存在或类型错误")
	}
	eventName, ok := data["name"].(string)
	if !ok {
		return fmt.Errorf("传入参数中事件名称不存在或类型错误")
	}
	cronExpression := ""
	if settings, ok := data["settings"].(map[string]interface{}); ok {
		if scheduleType, ok := settings["type"].(string); ok {
			if scheduleType == "once" {
				//if endTime, ok := settings["endTime"].(time.Time); ok {
				//	if tools.GetLocalTimeNow(time.Now()).Unix() >= endTime.Unix() {
				//		logger.Debugf(eventComputeLogicLog, "事件(%s)的定时任务结束事件已到，不再执行", eventID)
				//		return nil
				//	}
				//}
			}
			if startTime, ok := settings["startTime"].(map[string]interface{}); ok {
				cronExpression = tools.GetCronExpression(scheduleType, startTime)
			}
		}
		if endTime, ok := settings["endTime"].(time.Time); ok {
			if tools.GetLocalTimeNow(time.Now()).Unix() >= endTime.Unix() {
				logger.Debugf(eventScheduleLog, "事件(%s)的定时任务结束事件已到，不再执行", eventID)
				return nil
			}
		}
	}
	if cronExpression == "" {
		return fmt.Errorf("事件(%s)的定时表达式解析失败", eventID)
	}
	logger.Debugf(eventScheduleLog, "事件(%s)的定时cron为:(%s)",eventID, cronExpression)
	_,_ = c.AddFunc(cronExpression, func() {
		fmt.Printf("事件(%s)的定时任务开始",eventID)
		fmt.Println("============")
		scheduleType := ""
		if settings, ok := data["settings"].(map[string]interface{}); ok {
			scheduleType, ok = settings["type"].(string)
			if ok {
				if scheduleType == "once" {
					if startTime, ok := settings["startTime"].(map[string]interface{}); ok {
						if year, ok := startTime["year"]; ok {
							yearString := tools.InterfaceTypeToString(year)
							if time.Now().Format("2006") != yearString {
								logger.Debugf(eventScheduleLog, "事件(%s)的定时任务开始时间未到或已经超过，不执行", eventID)
								return
							}
						}
					}
				}
			}
			if endTime, ok := settings["endTime"].(time.Time); ok {
				if tools.GetLocalTimeNow(time.Now()).Unix() >= endTime.Unix() {
					logger.Debugf(eventScheduleLog, "事件(%s)的定时任务结束时间已到，不再执行", eventID)
					return
				}
			}
		}
		scheduleTypeMap := map[string]string{
			"hour":  "每小时",
			"day":   "每天",
			"week":  "每周",
			"month": "每月",
			"year":  "每年",
			"once":  "仅一次",
		}
		sendMap := map[string]interface{}{
			"time":      tools.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05"),
			"interval":  scheduleTypeMap[scheduleType],
			"eventName": eventName,
		}
		b, err := json.Marshal(sendMap)
		if err != nil {
			logger.Debugf(eventScheduleLog, "事件(%s)的发送map序列化失败:%s", err.Error())
			return
		}
		//imqtt.SendMsg(emqttConn, "event"+"/"+eventID, string(b))
		err = imqtt.Send(fmt.Sprintf("event/%s", eventID), b)
		if err != nil {
			logger.Warnf(eventScheduleLog, "发送事件(%s)错误:%s", eventID, err.Error())
		} else {
			logger.Debugf(eventScheduleLog, "发送事件成功:%s,数据为:%+v", eventID, sendMap)
		}
	})

	c.Start()

	//logger.Debugf(eventScheduleLog, "计划事件触发器（添加计划事件）执行结束")

	return nil
}

func TriggerEditOrDeleteSchedule(data map[string]interface{}, c *cron.Cron) error {
	//logger.Debugf(eventScheduleLog, "开始执行计划事件事件(修改或删除计划事件)")
	//logger.Debugf(eventScheduleLog, "传入参数为:%+v", data)
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(30)*time.Second)
	defer cancel()

	c.Stop()
	c = cron.New(cron.WithSeconds(), cron.WithChain(cron.DelayIfStillRunning(cron.DefaultLogger)))
	c.Start()

	paramMatch := bson.D{
		bson.E{
			Key: "$match",
			Value: bson.M{
				"type": Schedule,
			},
		},
	}
	paramLookup := bson.D{
		bson.E{
			Key: "$lookup",
			Value: bson.M{
				"from":         "eventhandler",
				"localField":   "_id",
				"foreignField": "event",
				"as":           "handlers",
			},
		},
	}

	pipeline := mongo.Pipeline{}
	pipeline = append(pipeline, paramMatch, paramLookup)
	eventInfoList := make([]bson.M, 0)
	err := restfulapi.FindPipeline(ctx, idb.Database.Collection("event"), &eventInfoList, pipeline, nil)
	if err != nil {
		return fmt.Errorf("获取所有计划事件失败:%s", err.Error())
	}

	//logger.Debugf(eventScheduleLog, "开始遍历事件列表")
	for i, eventInfo := range eventInfoList {
		j := i
		eventInfo = eventInfoList[j]
		logger.Debugf(eventScheduleLog, "事件信息为:%+v", eventInfo)
		handlers, ok := eventInfo["handlers"].(primitive.A)
		if !ok {
			logger.Warnln(eventScheduleLog, "handlers字段不存在或类型错误")
			continue
		}
		if len(handlers) == 0 {
			logger.Warnln(eventScheduleLog, "handlers字段数组长度为0")
			continue
		}
		eventName, ok := eventInfo["name"].(string)
		if !ok {
			logger.Errorf(eventScheduleLog, "传入参数中事件名称不存在或类型错误")
			continue
		}
		logger.Debugf(eventScheduleLog, "开始分析事件")
		if eventID, ok := eventInfo["id"].(primitive.ObjectID); ok {
			if settings, ok := eventInfo["settings"].(primitive.M); ok {
				cronExpression := ""
				if scheduleType, ok := settings["type"].(string); ok {
					if scheduleType == "once" {
						//if endTime, ok := settings["endTime"].(time.Time); ok {
						//	if tools.GetLocalTimeNow(time.Now()).Unix() >= endTime.Unix() {
						//		logger.Debugf(eventComputeLogicLog, "事件(%s)的定时任务结束事件已到，不再执行", eventID)
						//		return nil
						//	}
						//}
					}
					if startTime, ok := settings["startTime"].(primitive.M); ok {
						cronExpression = tools.GetCronExpression(scheduleType, startTime)
					}
				}
				if endTime, ok := settings["endTime"].(primitive.DateTime); ok {
					if tools.GetLocalTimeNow(time.Now()).Unix() >= int64(endTime)/1000 {
						logger.Debugf(eventScheduleLog, "事件(%s)的定时任务结束事件已到，不再执行", eventID)
						continue
					}
				}
				if cronExpression == "" {
					logger.Errorf(eventScheduleLog, "事件(%s)的定时表达式解析失败", eventID)
					continue
				}
				logger.Debugf(eventScheduleLog, "事件(%s)的定时cron为:(%s)",eventID, cronExpression)
				_,_ = c.AddFunc(cronExpression, func() {
					fmt.Printf("事件(%s)的定时任务开始",eventID)
					fmt.Println("============")
					scheduleType := ""
					if settings, ok := eventInfo["settings"].(primitive.M); ok {
						scheduleType, ok := settings["type"].(string)
						if ok {
							if scheduleType == "once" {
								if startTime, ok := settings["startTime"].(primitive.M); ok {
									if year, ok := startTime["year"]; ok {
										yearString := tools.InterfaceTypeToString(year)
										if time.Now().Format("2006") != yearString {
											logger.Debugf(eventScheduleLog, "事件(%s)的定时任务开始时间未到或已经超过，不执行", eventID)
											return
										}
									}
								}
							}
						}
						if endTime, ok := settings["endTime"].(primitive.DateTime); ok {
							if tools.GetLocalTimeNow(time.Now()).Unix() >= int64(endTime)/1000 {
								logger.Debugf(eventScheduleLog, "事件(%s)的定时任务结束事件已到，不再执行", eventID)
								return
							}
						}
					}
					scheduleTypeMap := map[string]string{
						"hour":  "每小时",
						"day":   "每天",
						"week":  "每周",
						"month": "每月",
						"year":  "每年",
						"once":  "仅一次",
					}
					sendMap := map[string]interface{}{
						"time":      tools.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05"),
						"interval":  scheduleTypeMap[scheduleType],
						"eventName": eventName,
					}
					//b, err := json.Marshal(sendMap)
					//if err != nil {
					//	return
					//}
					//imqtt.SendMsg(emqttConn, "event"+"/"+eventID.Hex(), string(b))
					b, err := json.Marshal(sendMap)
					if err != nil {
						logger.Debugf(eventScheduleLog, "事件(%s)的发送map序列化失败:%s", err.Error())
						return
					}
					err = imqtt.Send(fmt.Sprintf("event/%s", eventID.Hex()), b)
					if err != nil {
						logger.Warnf(eventScheduleLog, "发送事件(%s)错误:%s", eventID.Hex(), err.Error())
					} else {
						logger.Debugf(eventScheduleLog, "发送事件成功:%s,数据为:%+v", eventID.Hex(), sendMap)
					}
				})

				c.Start()
			}
		}
	}

	//logger.Debugf(eventScheduleLog, "计划事件触发器(修改或删除计划事件)执行结束")
	return nil
}
