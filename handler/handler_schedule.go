package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/air-iot/service/logic"
	"github.com/air-iot/service/restful-api"
	"go.mongodb.org/mongo-driver/mongo"
	"time"

	idb "github.com/air-iot/service/db/mongo"
	"github.com/air-iot/service/logger"
	imqtt "github.com/air-iot/service/mq/mqtt"
	"github.com/air-iot/service/tools"
	"github.com/robfig/cron/v3"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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
		//判断是否已经失效
		if invalid, ok := settings["invalid"].(bool); ok {
			if invalid {
				logger.Warnln(eventLog, "事件(%s)已经失效", eventID)
				return fmt.Errorf("事件(%s)已经失效", eventID)
			}
		}
		if scheduleType, ok := settings["type"].(string); ok {
			if scheduleType == "once" {
				if startTime, ok := settings["startTime"].(string); ok {
					formatStartTime, err := tools.ConvertStringToTime("2006-01-02 15:04:05", startTime, time.Local)
					if err != nil {
						//logger.Errorf(logFieldsMap, "时间范围字段值格式错误:%s", err.Error())
						formatStartTime, err = tools.ConvertStringToTime("2006-01-02T15:04:05+08:00", startTime, time.Local)
						if err != nil {
							logger.Errorf(eventScheduleLog, "时间范围字段值格式错误:%s", err.Error())
							return fmt.Errorf("时间范围字段值格式错误:%s", err.Error())
						}
						//return restfulapi.NewHTTPError(http.StatusBadRequest, "startTime", fmt.Sprintf("时间范围字段格式错误:%s", err.Error()))
					}
					cronExpression = tools.GetCronExpressionOnce(scheduleType, formatStartTime)
				}
			} else {
				if startTime, ok := settings["startTime"].(map[string]interface{}); ok {
					cronExpression = tools.GetCronExpression(scheduleType, startTime)
				}
			}
		}
		if endTime, ok := settings["endTime"].(time.Time); ok {
			if tools.GetLocalTimeNow(time.Now()).Unix() >= endTime.Unix() {
				logger.Debugf(eventScheduleLog, "事件(%s)的定时任务结束事件已到，不再执行", eventID)
				//修改事件为失效
				updateMap := bson.M{"settings.invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
				_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
				if err != nil {
					logger.Errorf(eventAlarmLog, "失效事件(%s)失败:%s", eventID, err.Error())
					return fmt.Errorf("失效事件(%s)失败:%s", eventID, err.Error())
				}
				return nil
			}
		}
	}
	if cronExpression == "" {
		return fmt.Errorf("事件(%s)的定时表达式解析失败", eventID)
	}
	logger.Debugf(eventScheduleLog, "事件(%s)的定时cron为:(%s)", eventID, cronExpression)
	_, _ = c.AddFunc(cronExpression, func() {
		scheduleType := ""
		isOnce := false
		if settings, ok := data["settings"].(map[string]interface{}); ok {
			scheduleType, ok = settings["type"].(string)
			if ok {
				if scheduleType == "once" {
					isOnce = true
					if startTime, ok := settings["startTime"].(map[string]interface{}); ok {
						if year, ok := startTime["year"]; ok {
							yearString := tools.InterfaceTypeToString(year)
							if time.Now().Format("2006") != yearString {
								logger.Debugf(eventScheduleLog, "事件(%s)的定时任务开始时间未到或已经超过，不执行", eventID)
								return
							}
						}
					} else if startTime, ok := settings["startTime"].(string); ok {
						formatStartTime, err := tools.ConvertStringToTime("2006-01-02 15:04:05", startTime, time.Local)
						if err != nil {
							//logger.Errorf(logFieldsMap, "时间范围字段值格式错误:%s", err.Error())
							formatStartTime, err = tools.ConvertStringToTime("2006-01-02T15:04:05+08:00", startTime, time.Local)
							if err != nil {
								logger.Errorf(eventScheduleLog, "时间范围字段值格式错误:%s", err.Error())
								return
							}
							//return restfulapi.NewHTTPError(http.StatusBadRequest, "startTime", fmt.Sprintf("时间范围字段格式错误:%s", err.Error()))
						}
						yearString := tools.InterfaceTypeToString(formatStartTime.Year())
						if time.Now().Format("2006") != yearString {
							logger.Debugf(eventScheduleLog, "事件(%s)的定时任务开始时间未到或已经超过，不执行", eventID)
							return
						}
					}
				}else{
					if startTime, ok := settings["startTime"].(map[string]interface{}); ok {
						if year, ok := startTime["year"]; ok {
							yearFloat,err := tools.GetFloatNumber(year)
							if err != nil {
								logger.Debugf(eventScheduleLog, "事件(%s)的年份转数字失败:%s", err.Error())
								return
							}
							if time.Now().Year() % int(yearFloat) != 0 {
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
					//修改事件为失效
					updateMap := bson.M{"settings.invalid": true}
					//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
					_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
					if err != nil {
						logger.Errorf(eventAlarmLog, "失效事件(%s)失败:%s", eventID, err.Error())
						return
					}
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
		if isOnce {
			logger.Warnln(eventAlarmLog, "事件(%s)为只执行一次的事件", eventID)
			//修改事件为失效
			updateMap := bson.M{"settings.invalid": true}
			_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
			//var r = make(map[string]interface{})
			//err := api.Cli.UpdateEventById(eventID, updateMap, &r)
			if err != nil {
				logger.Errorf(eventAlarmLog, "失效事件(%s)失败:%s", eventID, err.Error())
				return
			}
		}
	})

	c.Start()

	//logger.Debugf(eventScheduleLog, "计划事件触发器（添加计划事件）执行结束")

	return nil
}

func TriggerEditOrDeleteSchedule(data map[string]interface{}, c *cron.Cron) error {
	//logger.Debugf(eventScheduleLog, "开始执行计划事件事件(修改或删除计划事件)")
	//logger.Debugf(eventScheduleLog, "传入参数为:%+v", data)
	//ctx, cancel := context.WithTimeout(context.Background(), time.Duration(30)*time.Second)
	//defer cancel()

	c.Stop()
	//c = cron.New(cron.WithSeconds(), cron.WithChain(cron.DelayIfStillRunning(cron.DefaultLogger)))
	//c.Start()

	testBefore := c.Entries()
	for _, v := range testBefore {
		c.Remove(v.ID)
	}

	//paramMatch := bson.D{
	//	bson.E{
	//		Key: "$match",
	//		Value: bson.M{
	//			"type": Schedule,
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
	//
	//pipeline := mongo.Pipeline{}
	//pipeline = append(pipeline, paramMatch, paramLookup)
	//err := restfulapi.FindPipeline(ctx, idb.Database.Collection("event"), &eventInfoList, pipeline, nil)

	//获取所有事件列表
	pipeline := mongo.Pipeline{}
	paramMatch := bson.D{bson.E{Key: "$match", Value: bson.M{"type": Schedule}}}
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
	pipeline = append(pipeline, paramMatch, paramLookup)
	eventInfoList, err := logic.EventLogic.FindByPipeline(pipeline)

	if err != nil {
		return fmt.Errorf("获取所有计划事件失败:%s", err.Error())
	}

	//logger.Debugf(eventScheduleLog, "开始遍历事件列表")
	//fmt.Println("len eventInfoList:",len(eventInfoList))
	for i, eventInfo := range eventInfoList {
		j := i
		eventInfo = eventInfoList[j]
		logger.Debugf(eventScheduleLog, "事件信息为:%+v", eventInfo)
		handlers := eventInfo.Handlers
		if len(handlers) == 0 {
			logger.Warnln(eventScheduleLog, "handlers字段数组长度为0")
			continue
		}
		eventName := eventInfo.Name
		if eventName == "" {
			logger.Errorf(eventScheduleLog, "传入参数中事件名称为空")
			continue
		}
		logger.Debugf(eventScheduleLog, "开始分析事件")
		eventID := eventInfo.ID
		settings := eventInfo.Settings
		if settings == nil || len(settings) == 0 {
			logger.Errorf(eventScheduleLog, "事件的配置不存在")
			continue
		}
		//判断是否已经失效
		if invalid, ok := settings["invalid"].(bool); ok {
			if invalid {
				logger.Warnln(eventLog, "事件(%s)已经失效", eventID)
				continue
			}
		}
		cronExpression := ""
		if scheduleType, ok := settings["type"].(string); ok {
			if scheduleType == "once" {
				if startTime, ok := settings["startTime"].(primitive.DateTime); ok {
					formatStartTime := time.Unix(int64(startTime/1e3), 0)
					if err != nil {
						logger.Errorf(eventScheduleLog, "时间转time.Time失败:%s", err.Error())
						continue
					}
					cronExpression = tools.GetCronExpressionOnce(scheduleType, &formatStartTime)
				}
			} else {
				if startTime, ok := settings["startTime"].(primitive.M); ok {
					cronExpression = tools.GetCronExpression(scheduleType, startTime)
				}
			}
		}
		if endTime, ok := settings["endTime"].(primitive.DateTime); ok {
			if tools.GetLocalTimeNow(time.Now()).Unix() >= int64(endTime)/1000 {
				logger.Debugf(eventScheduleLog, "事件(%s)的定时任务结束事件已到，不再执行", eventID.Hex())
				//修改事件为失效
				updateMap := bson.M{"settings.invalid": true}
				_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID.Hex(), updateMap)
				if err != nil {
					logger.Errorf(eventAlarmLog, "失效事件(%s)失败:%s", eventID, err.Error())
					continue
				}
				continue
			}
		}
		if cronExpression == "" {
			logger.Errorf(eventScheduleLog, "事件(%s)的定时表达式解析失败", eventID)
			continue
		}
		logger.Debugf(eventScheduleLog, "事件(%s)的定时cron为:(%s)", eventID, cronExpression)

		eventInfoCron := eventInfo
		eventIDCron := eventID
		_, _ = c.AddFunc(cronExpression, func() {
			scheduleType := ""
			isOnce := false
			settings := eventInfoCron.Settings
			if settings != nil && len(settings) != 0 {
				scheduleType, ok := settings["type"].(string)
				if ok {
					if scheduleType == "once" {
						isOnce = true
						if startTime, ok := settings["startTime"].(primitive.M); ok {
							if year, ok := startTime["year"]; ok {
								yearString := tools.InterfaceTypeToString(year)
								if time.Now().Format("2006") != yearString {
									logger.Debugf(eventScheduleLog, "事件(%s)的定时任务开始时间未到或已经超过，不执行", eventID)
									return
								}
							}
						} else if startTime, ok := settings["startTime"].(primitive.DateTime); ok {
							formatStartTime := time.Unix(int64(startTime/1e3), 0)
							if err != nil {
								logger.Errorf(eventScheduleLog, "时间转time.Time失败:%s", err.Error())
								return
							}
							yearString := tools.InterfaceTypeToString(formatStartTime.Year())
							if time.Now().Format("2006") != yearString {
								logger.Debugf(eventScheduleLog, "事件(%s)的定时任务开始时间未到或已经超过，不执行", eventID)
								return
							}
						}
					}
				}
				if endTime, ok := settings["endTime"].(primitive.DateTime); ok {
					if tools.GetLocalTimeNow(time.Now()).Unix() >= int64(endTime)/1000 {
						logger.Debugf(eventScheduleLog, "事件(%s)的定时任务结束事件已到，不再执行", eventID.Hex())

						//修改事件为失效
						updateMap := bson.M{"settings.invalid": true}
						//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID.Hex(), updateMap)
						_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID.Hex(), updateMap)
						if err != nil {
							logger.Errorf(eventAlarmLog, "失效事件(%s)失败:%s", eventID, err.Error())
							return
						}
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
			err = imqtt.Send(fmt.Sprintf("event/%s", eventIDCron.Hex()), b)
			if err != nil {
				logger.Warnf(eventScheduleLog, "发送事件(%s)错误:%s", eventIDCron.Hex(), err.Error())
				//fmt.Println(eventScheduleLog, "发送事件(%s)错误:%s", eventIDCron.Hex(), err.Error())
			} else {
				logger.Debugf(eventScheduleLog, "发送事件成功:%s,数据为:%+v", eventIDCron.Hex(), sendMap)
				//fmt.Println(eventScheduleLog, "发送事件成功:%s,数据为:%+v", eventIDCron.Hex(), sendMap)
			}

			if isOnce {
				logger.Warnln(eventAlarmLog, "事件(%s)为只执行一次的事件", eventID)
				//修改事件为失效
				updateMap := bson.M{"settings.invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID.Hex(), updateMap)
				_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID.Hex(), updateMap)
				if err != nil {
					logger.Errorf(eventAlarmLog, "失效事件(%s)失败:%s", eventID.Hex(), err.Error())
					return
				}
			}
		})
		//if err != nil{
		//	fmt.Println("AddFunc err:",err.Error())
		//}

		//fmt.Println("AddFunc entryID:",entryID,"eventIDCron:",eventIDCron)
	}

	c.Start()

	//test := c.Entries()
	//fmt.Println("len entries:",len(test),"entries:",test)
	//for _,v :=  range test{
	//	fmt.Println("id:",v.ID,"prev:",v.Prev.Format("2006-01-02 15:04:05"),"next:",v.Next.Format("2006-01-02 15:04:05"))
	//}
	logger.Debugf(eventScheduleLog, "计划事件触发器(修改或删除计划事件)执行结束")
	return nil
}
