package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/air-iot/service/api"
	"github.com/air-iot/service/gin/ginx"
	"github.com/air-iot/service/init/cache/event"
	"github.com/air-iot/service/init/mq"
	"github.com/air-iot/service/util/formatx"
	"github.com/air-iot/service/util/timex"
	"github.com/go-redis/redis/v8"
	"go.mongodb.org/mongo-driver/mongo"
	"time"

	"github.com/robfig/cron/v3"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var eventScheduleLog = map[string]interface{}{"name": "计划事件触发"}

func TriggerAddSchedule(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client,mq mq.MQ, apiClient api.Client,projectName string,data map[string]interface{}, c *cron.Cron) error {
	headerMap := map[string]string{ginx.XRequestProject:projectName}
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
				//logger.Warnln(eventLog, "事件(%s)已经失效", eventID)
				return fmt.Errorf("事件(%s)已经失效", eventID)
			}
		}
		if scheduleType, ok := settings["type"].(string); ok {
			if scheduleType == "once" {
				if startTime, ok := settings["startTime"].(string); ok {
					formatStartTime, err := timex.ConvertStringToTime("2006-01-02 15:04:05", startTime, time.Local)
					if err != nil {
						////logger.Errorf(logFieldsMap, "时间范围字段值格式错误:%s", err.Error())
						formatStartTime, err = timex.ConvertStringToTime("2006-01-02T15:04:05+08:00", startTime, time.Local)
						if err != nil {
							return fmt.Errorf("时间范围字段值格式错误:%s", err.Error())
						}
						//return restfulapi.NewHTTPError(http.StatusBadRequest, "startTime", fmt.Sprintf("时间范围字段格式错误:%s", err.Error()))
					}
					cronExpression = formatx.GetCronExpressionOnce(scheduleType, formatStartTime)
				}
			} else {
				if startTime, ok := settings["startTime"].(map[string]interface{}); ok {
					cronExpression = formatx.GetCronExpression(scheduleType, startTime)
				}
			}
		}
		if endTime, ok := settings["endTime"].(time.Time); ok {
			if timex.GetLocalTimeNow(time.Now()).Unix() >= endTime.Unix() {
				//修改事件为失效
				updateMap := bson.M{"settings.invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
				var r = make(map[string]interface{})
				err := apiClient.UpdateEventById(headerMap,eventID, updateMap, &r)

				err = apiClient.UpdateEventById(headerMap,eventID, updateMap, &r)
				if err != nil {
					return fmt.Errorf("失效事件(%s)失败:%s", eventID, err.Error())
				}
				return nil
			}
		}
	}
	if cronExpression == "" {
		return fmt.Errorf("事件(%s)的定时表达式解析失败", eventID)
	}
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
							yearString := formatx.InterfaceTypeToString(year)
							if time.Now().Format("2006") != yearString {
								//logger.Debugf(eventScheduleLog, "事件(%s)的定时任务开始时间未到或已经超过，不执行", eventID)
								return
							}
						}
					} else if startTime, ok := settings["startTime"].(string); ok {
						formatStartTime, err := timex.ConvertStringToTime("2006-01-02 15:04:05", startTime, time.Local)
						if err != nil {
							////logger.Errorf(logFieldsMap, "时间范围字段值格式错误:%s", err.Error())
							formatStartTime, err = timex.ConvertStringToTime("2006-01-02T15:04:05+08:00", startTime, time.Local)
							if err != nil {
								//logger.Errorf(eventScheduleLog, "时间范围字段值格式错误:%s", err.Error())
								return
							}
							//return restfulapi.NewHTTPError(http.StatusBadRequest, "startTime", fmt.Sprintf("时间范围字段格式错误:%s", err.Error()))
						}
						yearString := formatx.InterfaceTypeToString(formatStartTime.Year())
						if time.Now().Format("2006") != yearString {
							//logger.Debugf(eventScheduleLog, "事件(%s)的定时任务开始时间未到或已经超过，不执行", eventID)
							return
						}
					}
				}
			}
			if endTime, ok := settings["endTime"].(time.Time); ok {
				if timex.GetLocalTimeNow(time.Now()).Unix() >= endTime.Unix() {
					//logger.Debugf(eventScheduleLog, "事件(%s)的定时任务结束时间已到，不再执行", eventID)
					//修改事件为失效
					updateMap := bson.M{"settings.invalid": true}
					//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
					var r = make(map[string]interface{})
					err := apiClient.UpdateEventById(headerMap,eventID, updateMap, &r)
					if err != nil {
						//logger.Errorf(eventAlarmLog, "失效事件(%s)失败:%s", eventID, err.Error())
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
			"time":      timex.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05"),
			"interval":  scheduleTypeMap[scheduleType],
			"eventName": eventName,
		}
		b, err := json.Marshal(sendMap)
		if err != nil {
			//logger.Debugf(eventScheduleLog, "事件(%s)的发送map序列化失败:%s", err.Error())
			return
		}
		//imqtt.SendMsg(emqttConn, "event"+"/"+eventID, string(b))
		err = mq.Publish(ctx,[]string{"event",projectName,eventID}, b)
		if err != nil {
			//logger.Warnf(eventScheduleLog, "发送事件(%s)错误:%s", eventID, err.Error())
		} else {
			//logger.Debugf(eventScheduleLog, "发送事件成功:%s,数据为:%+v", eventID, sendMap)
		}
		if isOnce {
			//logger.Warnln(eventAlarmLog, "事件(%s)为只执行一次的事件", eventID)
			//修改事件为失效
			updateMap := bson.M{"settings.invalid": true}
			//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
			var r = make(map[string]interface{})
			err := apiClient.UpdateEventById(headerMap,eventID, updateMap, &r)
			if err != nil {
				//logger.Errorf(eventAlarmLog, "失效事件(%s)失败:%s", eventID, err.Error())
				return
			}
		}
	})

	c.Start()

	////logger.Debugf(eventScheduleLog, "计划事件触发器（添加计划事件）执行结束")

	return nil
}

func TriggerEditOrDeleteSchedule(ctx context.Context, redisClient *redis.Client, mongoClient *mongo.Client,mq mq.MQ, apiClient api.Client,projectName string,data map[string]interface{}, c *cron.Cron) error {
	headerMap := map[string]string{ginx.XRequestProject:projectName}
	c.Stop()
	//c = cron.New(cron.WithSeconds(), cron.WithChain(cron.DelayIfStillRunning(cron.DefaultLogger)))
	//c.Start()

	testBefore := c.Entries()
	for _, v := range testBefore {
		c.Remove(v.ID)
	}

	eventInfoList := make([]bson.M,0)
	err := event.GetByType(ctx,redisClient,mongoClient,projectName,string(Alarm),&eventInfoList)


	if err != nil {
		return fmt.Errorf("获取所有计划事件失败:%s", err.Error())
	}

	////logger.Debugf(eventScheduleLog, "开始遍历事件列表")
	//fmt.Println("len eventInfoList:",len(eventInfoList))
	for i, eventInfo := range eventInfoList {
		j := i
		eventInfo = eventInfoList[j]
		//logger.Debugf(eventScheduleLog, "事件信息为:%+v", eventInfo)
		handlers, ok := eventInfo["handlers"].(primitive.A)
		if !ok {
			//logger.Warnln(eventScheduleLog, "handlers字段不存在或类型错误")
			continue
		}
		if len(handlers) == 0 {
			//logger.Warnln(eventScheduleLog, "handlers字段数组长度为0")
			continue
		}
		eventName, ok := eventInfo["name"].(string)
		if !ok {
			//logger.Errorf(eventScheduleLog, "传入参数中事件名称不存在或类型错误")
			continue
		}
		//logger.Debugf(eventScheduleLog, "开始分析事件")
		if eventID, ok := eventInfo["id"].(string); ok {
			if settings, ok := eventInfo["settings"].(primitive.M); ok {
				//判断是否已经失效
				if invalid, ok := settings["invalid"].(bool); ok {
					if invalid {
						//logger.Warnln(eventLog, "事件(%s)已经失效", eventID)
						continue
					}
				}
				cronExpression := ""
				if scheduleType, ok := settings["type"].(string); ok {
					if scheduleType == "once" {
						if startTime, ok := settings["startTime"].(primitive.DateTime); ok {
							formatStartTime := time.Unix(int64(startTime/1e3), 0)
							if err != nil {
								//logger.Errorf(eventScheduleLog, "时间转time.Time失败:%s", err.Error())
								continue
							}
							cronExpression = formatx.GetCronExpressionOnce(scheduleType, &formatStartTime)
						}
					} else {
						if startTime, ok := settings["startTime"].(primitive.M); ok {
							cronExpression = formatx.GetCronExpression(scheduleType, startTime)
						}
					}
				}
				if endTime, ok := settings["endTime"].(primitive.DateTime); ok {
					if timex.GetLocalTimeNow(time.Now()).Unix() >= int64(endTime)/1000 {
						//logger.Debugf(eventScheduleLog, "事件(%s)的定时任务结束事件已到，不再执行", eventID)
						//修改事件为失效
						updateMap := bson.M{"settings.invalid": true}
						//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
						var r = make(map[string]interface{})
						err := apiClient.UpdateEventById(headerMap,eventID, updateMap, &r)
						if err != nil {
							//logger.Errorf(eventAlarmLog, "失效事件(%s)失败:%s", eventID, err.Error())
							continue
						}
						continue
					}
				}
				if cronExpression == "" {
					//logger.Errorf(eventScheduleLog, "事件(%s)的定时表达式解析失败", eventID)
					continue
				}
				//logger.Debugf(eventScheduleLog, "事件(%s)的定时cron为:(%s)", eventID, cronExpression)

				eventInfoCron := eventInfo
				eventIDCron := eventID
				_, _ = c.AddFunc(cronExpression, func() {
					scheduleType := ""
					isOnce := false
					if settings, ok := eventInfoCron["settings"].(primitive.M); ok {
						scheduleType, ok = settings["type"].(string)
						if ok {
							if scheduleType == "once" {
								isOnce = true
								if startTime, ok := settings["startTime"].(primitive.M); ok {
									if year, ok := startTime["year"]; ok {
										yearString := formatx.InterfaceTypeToString(year)
										if time.Now().Format("2006") != yearString {
											//logger.Debugf(eventScheduleLog, "事件(%s)的定时任务开始时间未到或已经超过，不执行", eventID)
											return
										}
									}
								} else if startTime, ok := settings["startTime"].(primitive.DateTime); ok {
									formatStartTime := time.Unix(int64(startTime/1e3), 0)
									if err != nil {
										//logger.Errorf(eventScheduleLog, "时间转time.Time失败:%s", err.Error())
										return
									}
									yearString := formatx.InterfaceTypeToString(formatStartTime.Year())
									if time.Now().Format("2006") != yearString {
										//logger.Debugf(eventScheduleLog, "事件(%s)的定时任务开始时间未到或已经超过，不执行", eventID)
										return
									}
								}
							}
						}
						if endTime, ok := settings["endTime"].(primitive.DateTime); ok {
							if timex.GetLocalTimeNow(time.Now()).Unix() >= int64(endTime)/1000 {
								//logger.Debugf(eventScheduleLog, "事件(%s)的定时任务结束事件已到，不再执行", eventID)

								//修改事件为失效
								updateMap := bson.M{"settings.invalid": true}
								//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
								var r = make(map[string]interface{})
								err := apiClient.UpdateEventById(headerMap,eventID, updateMap, &r)
								if err != nil {
									//logger.Errorf(eventAlarmLog, "失效事件(%s)失败:%s", eventID, err.Error())
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
						"time":      timex.GetLocalTimeNow(time.Now()).Format("2006-01-02 15:04:05"),
						"interval":  scheduleTypeMap[scheduleType],
						"eventName": eventName,
					}
					//b, err := json.Marshal(sendMap)
					//if err != nil {
					//	return
					//}
					//imqtt.SendMsg(emqttConn, "event"+"/"+eventID, string(b))
					b, err := json.Marshal(sendMap)
					if err != nil {
						//logger.Debugf(eventScheduleLog, "事件(%s)的发送map序列化失败:%s", err.Error())
						return
					}
					err = mq.Publish(ctx,[]string{"event",projectName,eventIDCron}, b)
					if err != nil {
						//logger.Warnf(eventScheduleLog, "发送事件(%s)错误:%s", eventIDCron, err.Error())
						//fmt.Println(eventScheduleLog, "发送事件(%s)错误:%s", eventIDCron, err.Error())
					} else {
						//logger.Debugf(eventScheduleLog, "发送事件成功:%s,数据为:%+v", eventIDCron, sendMap)
						//fmt.Println(eventScheduleLog, "发送事件成功:%s,数据为:%+v", eventIDCron, sendMap)
					}

					if isOnce {
						//logger.Warnln(eventAlarmLog, "事件(%s)为只执行一次的事件", eventID)
						//修改事件为失效
						updateMap := bson.M{"settings.invalid": true}
						//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
						var r = make(map[string]interface{})
						err := apiClient.UpdateEventById(headerMap,eventID, updateMap, &r)
						if err != nil {
							//logger.Errorf(eventAlarmLog, "失效事件(%s)失败:%s", eventID, err.Error())
							return
						}
					}
				})
				//if err != nil{
				//	fmt.Println("AddFunc err:",err.Error())
				//}

				//fmt.Println("AddFunc entryID:",entryID,"eventIDCron:",eventIDCron)
			}
		}
	}

	c.Start()

	//test := c.Entries()
	//fmt.Println("len entries:",len(test),"entries:",test)
	//for _,v :=  range test{
	//	fmt.Println("id:",v.ID,"prev:",v.Prev.Format("2006-01-02 15:04:05"),"next:",v.Next.Format("2006-01-02 15:04:05"))
	//}
	//logger.Debugf(eventScheduleLog, "计划事件触发器(修改或删除计划事件)执行结束")
	return nil
}
