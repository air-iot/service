package flow

import (
	"context"
	"fmt"
	"github.com/air-iot/service/logger"
	"github.com/air-iot/service/util/flowx"
	"github.com/air-iot/service/util/numberx"
	"github.com/camunda-cloud/zeebe/clients/go/pkg/zbc"
	"time"

	"github.com/robfig/cron/v3"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/init/cache/entity"
	mongoOps "github.com/air-iot/service/init/mongodb"
	"github.com/air-iot/service/init/mq"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/util/formatx"
	"github.com/air-iot/service/util/timex"
)

var flowScheduleLog = map[string]interface{}{"name": "计划流程触发"}

func TriggerAddScheduleFlow(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, mq mq.MQ, apiClient api.Client, zbClient zbc.Client, projectName string, data map[string]interface{}, c *cron.Cron) error {
	//headerMap := map[string]string{ginx.XRequestProject: projectName}
	flowID, ok := data["id"].(string)
	if !ok {
		return fmt.Errorf("传入参数中流程ID不存在或类型错误")
	}
	flowName, ok := data["name"].(string)
	if !ok {
		return fmt.Errorf("传入参数中流程名称不存在或类型错误")
	}
	cronExpression := ""
	if settings, ok := data["settings"].(map[string]interface{}); ok {
		//判断是否已经失效
		if invalid, ok := data["invalid"].(bool); ok {
			if invalid {
				//logger.Warnln(eventLog, "流程(%s)已经失效", eventID)
				return fmt.Errorf("流程(%s)已经失效", flowID)
			}
		}

		//判断禁用
		if disable, ok := data["disable"].(bool); ok {
			if disable {
				//logger.Warnln(eventDeviceModifyLog, "流程(%s)已经被禁用", eventID)
				return fmt.Errorf("流程(%s)已经被禁用", flowID)
			}
		}


		if scheduleType, ok := settings["selectType"].(string); ok {
			if scheduleType == "once" {
				if startTime, ok := settings["startTime"].(string); ok {
					formatLayout := timex.FormatTimeFormat(startTime)
					formatStartTime, err := timex.ConvertStringToTime(formatLayout, startTime, time.Local)
					if err != nil {
						logger.Errorf("开始时间范围字段值格式错误:%s", err.Error())
						return fmt.Errorf("时间范围字段值格式错误:%s", err.Error())
					}
					cronExpression = formatx.GetCronExpressionOnce(scheduleType, formatStartTime)
				}
			} else {
				if scheduleType, ok := settings["type"].(string); ok {
					if startTime, ok := settings["startTime"].(map[string]interface{}); ok {
						cronExpression = formatx.GetCronExpression(scheduleType, startTime)
					}
				}
			}
		} else if scheduleType, ok := settings["type"].(string); ok {
			if startTime, ok := settings["startTime"].(map[string]interface{}); ok {
				cronExpression = formatx.GetCronExpression(scheduleType, startTime)
			}
		}
		if endTime, ok := settings["endTime"].(time.Time); ok {
			if timex.GetLocalTimeNow(time.Now()).Unix() >= endTime.Unix() {
				//修改流程为失效
				updateMap := bson.M{"invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("flow"), flowID, updateMap)
				//var r = make(map[string]interface{})
				_, err := mongoOps.UpdateByID(ctx, mongoClient.Database(projectName).Collection("flow"), flowID, updateMap)
				//err := apiClient.UpdateEventById(headerMap, flowID, updateMap, &r)
				if err != nil {
					return fmt.Errorf("失效流程(%s)失败:%s", flowID, err.Error())
				}
				return nil
			}
		}
	}
	if cronExpression == "" {
		return fmt.Errorf("流程(%s)的定时表达式解析失败", flowID)
	}
	_, _ = c.AddFunc(cronExpression, func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(30)*time.Second)
		defer cancel()
		scheduleType := ""
		isOnce := false
		if settings, ok := data["settings"].(map[string]interface{}); ok {
			var ok bool
			if scheduleType, ok = settings["selectType"].(string); ok {
				if scheduleType == "once" {
					isOnce = true
					if startTime, ok := settings["startTime"].(map[string]interface{}); ok {
						if year, ok := startTime["year"]; ok {
							yearString := formatx.InterfaceTypeToString(year)
							if time.Now().Format("2006") != yearString {
								//logger.Debugf(eventScheduleLog, "流程(%s)的定时任务开始时间未到或已经超过，不执行", eventID)
								return
							}
						}
					} else if startTime, ok := settings["startTime"].(string); ok {
						formatLayout := timex.FormatTimeFormat(startTime)
						formatStartTime, err := timex.ConvertStringToTime(formatLayout, startTime, time.Local)
						if err != nil {
							logger.Errorf("开始时间范围字段值格式错误:%s", err.Error())
							return
						}

						yearString := formatx.InterfaceTypeToString(formatStartTime.Year())
						if time.Now().Format("2006") != yearString {
							//logger.Debugf(eventScheduleLog, "流程(%s)的定时任务开始时间未到或已经超过，不执行", eventID)
							return
						}
					}
				} else {
					scheduleType, ok = settings["type"].(string)
					if startTime, ok := settings["startTime"].(map[string]interface{}); ok {
						if year, ok := startTime["year"]; ok {
							if year != nil {
								yearFloat, err := numberx.GetFloatNumber(year)
								if err != nil {
									//logger.Debugf(eventScheduleLog, "流程(%s)的年份转数字失败:%s", err.Error())
									return
								}
								if int(yearFloat) != 0 {
									if time.Now().Year()%int(yearFloat) != 0 {
										//logger.Debugf(eventScheduleLog, "流程(%s)的定时任务开始时间未到或已经超过，不执行", eventID)
										return
									}
								}
							}
						}
					}
				}
			} else {
				scheduleType, ok = settings["type"].(string)
				if startTime, ok := settings["startTime"].(map[string]interface{}); ok {
					if year, ok := startTime["year"]; ok {
						if year != nil {
							yearFloat, err := numberx.GetFloatNumber(year)
							if err != nil {
								//logger.Debugf(eventScheduleLog, "流程(%s)的年份转数字失败:%s", err.Error())
								return
							}
							if int(yearFloat) != 0 {
								if time.Now().Year()%int(yearFloat) != 0 {
									//logger.Debugf(eventScheduleLog, "流程(%s)的定时任务开始时间未到或已经超过，不执行", eventID)
									return
								}
							}
						}
					}
				}
			}
			if startValidTime, ok := settings["startValidTime"].(time.Time); ok {
				if timex.GetLocalTimeNow(time.Now()).Unix() < startValidTime.Unix() {
					logger.Debugf("流程(%s)的定时任务开始时间未到:%s", flowID)
					return
				}
			}
			if endTime, ok := settings["endTime"].(time.Time); ok {
				if timex.GetLocalTimeNow(time.Now()).Unix() >= endTime.Unix() {
					//logger.Debugf(eventScheduleLog, "流程(%s)的定时任务结束时间已到，不再执行", eventID)
					//修改流程为失效
					updateMap := bson.M{"invalid": true}
					//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
					//var r = make(map[string]interface{})
					_, err := mongoOps.UpdateByID(ctx, mongoClient.Database(projectName).Collection("flow"), flowID, updateMap)
					//err := apiClient.UpdateEventById(headerMap, eventID, updateMap, &r)
					if err != nil {
						//logger.Errorf(eventAlarmLog, "失效流程(%s)失败:%s", eventID, err.Error())
						return
					}
					return
				}
			}
		}
		scheduleTypeMap := map[string]string{
			"second": "每秒",
			"minute": "每分钟",
			"hour":   "每小时",
			"day":    "每天",
			"week":   "每周",
			"month":  "每月",
			"year":   "每年",
			"once":   "仅一次",
		}
		sendMap := map[string]interface{}{
			"time":     timex.GetLocalTimeNow(time.Now()).UnixNano() / 1e6,
			"interval": scheduleTypeMap[scheduleType],
			"flowName": flowName,
		}
		if flowXml, ok := data["flowXml"].(string); ok {
			err := flowx.StartFlow(zbClient, flowXml, projectName, sendMap)
			if err != nil {
				logger.Errorf("流程(%s)推进到下一阶段失败:%s", flowID, err.Error())
				return
			}
		} else {
			logger.Errorf("流程(%s)的xml不存在或类型错误", flowID)
			return
		}
		if isOnce {
			//logger.Warnln(eventAlarmLog, "流程(%s)为只执行一次的流程", eventID)
			//修改流程为失效
			updateMap := bson.M{"invalid": true}
			//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
			//var r = make(map[string]interface{})
			_, err := mongoOps.UpdateByID(ctx, mongoClient.Database(projectName).Collection("flow"), flowID, updateMap)
			//err := apiClient.UpdateEventById(headerMap, eventID, updateMap, &r)
			if err != nil {
				//logger.Errorf(eventAlarmLog, "失效流程(%s)失败:%s", eventID, err.Error())
				return
			}
		}
	})

	c.Start()

	////logger.Debugf(eventScheduleLog, "计划流程触发器（添加计划流程）执行结束")

	return nil
}

func TriggerEditOrDeleteScheduleFlow(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, mq mq.MQ, apiClient api.Client, zbClient zbc.Client, projectName string, data map[string]interface{}, c *cron.Cron) error {
	//headerMap := map[string]string{ginx.XRequestProject: projectName}

	pipeline := mongo.Pipeline{}
	paramMatch := bson.D{bson.E{Key: "$match", Value: bson.M{"type": Schedule}}}
	pipeline = append(pipeline, paramMatch)
	flowInfoList := make([]entity.FlowMongo, 0)
	err := mongoOps.FindPipeline(ctx, mongoClient.Database(projectName).Collection("flow"), &flowInfoList, pipeline)
	if err != nil {
		return fmt.Errorf("获取所有计划流程失败:%s", err.Error())
	}

	////logger.Debugf(eventScheduleLog, "开始遍历流程列表")
	//fmt.Println("len eventInfoList:",len(eventInfoList))
	if len(flowInfoList) == 0 {
		return nil
	}
	c.Stop()

	testBefore := c.Entries()
	for _, v := range testBefore {
		c.Remove(v.ID)
	}
	for i, flowInfo := range flowInfoList {
		j := i
		flowInfo = flowInfoList[j]
		//logger.Debugf(eventScheduleLog, "流程信息为:%+v", eventInfo)
		flowName := flowInfo.Name
		if flowName == "" {
			//logger.Errorf(eventScheduleLog, "传入参数中流程名称不存在或类型错误")
			continue
		}
		//logger.Debugf(eventScheduleLog, "开始分析流程")
		flowID := flowInfo.ID
		settings := flowInfo.Settings
		if settings == nil || len(settings) == 0 {
			//logger.Errorf(eventScheduleLog, "流程的配置不存在")
			continue
		}
		//判断是否已经失效
		if flowInfo.Invalid {
			logger.Warnln("流程(%s)已经失效", flowID)
			continue
		}

		//判断禁用
		if flowInfo.Disable {
			logger.Warnln("流程(%s)已经被禁用", flowID)
			continue
		}

		cronExpression := ""
		if scheduleType, ok := settings["selectType"].(string); ok {
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
				if scheduleType, ok := settings["type"].(string); ok {
					if startTime, ok := settings["startTime"].(map[string]interface{}); ok {
						cronExpression = formatx.GetCronExpression(scheduleType, startTime)
					}
				}
			}
		} else if scheduleType, ok := settings["type"].(string); ok {
			if startTime, ok := settings["startTime"].(map[string]interface{}); ok {
				cronExpression = formatx.GetCronExpression(scheduleType, startTime)
			}
		}
		if endTime, ok := settings["endTime"].(primitive.DateTime); ok {
			if timex.GetLocalTimeNow(time.Now()).Unix() >= int64(endTime)/1000 {
				//logger.Debugf(eventScheduleLog, "流程(%s)的定时任务结束流程已到，不再执行", eventID)
				//修改流程为失效
				updateMap := bson.M{"invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
				//var r = make(map[string]interface{})
				_, err := mongoOps.UpdateByID(ctx, mongoClient.Database(projectName).Collection("flow"), flowID, updateMap)
				//err := apiClient.UpdateEventById(headerMap, eventID, updateMap, &r)
				if err != nil {
					//logger.Errorf(eventAlarmLog, "失效流程(%s)失败:%s", eventID, err.Error())
					continue
				}
				continue
			}
		}
		if cronExpression == "" {
			//logger.Errorf(eventScheduleLog, "流程(%s)的定时表达式解析失败", eventID)
			continue
		}
		//logger.Debugf(eventScheduleLog, "流程(%s)的定时cron为:(%s)", eventID, cronExpression)

		flowInfoCron := flowInfo
		flowIDCron := flowID
		_, _ = c.AddFunc(cronExpression, func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Duration(30)*time.Second)
			defer cancel()
			scheduleType := ""
			isOnce := false
			settings := flowInfoCron.Settings
			if settings != nil && len(settings) != 0 {
				var ok bool
				if scheduleType, ok = settings["selectType"].(string); ok {
					if scheduleType == "once" {
						isOnce = true
						if startTime, ok := settings["startTime"].(primitive.M); ok {
							if year, ok := startTime["year"]; ok {
								yearString := formatx.InterfaceTypeToString(year)
								if time.Now().Format("2006") != yearString {
									//logger.Debugf(eventScheduleLog, "流程(%s)的定时任务开始时间未到或已经超过，不执行", eventID)
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
								//logger.Debugf(eventScheduleLog, "流程(%s)的定时任务开始时间未到或已经超过，不执行", eventID)
								return
							}
						}
					} else {
						scheduleType, ok = settings["type"].(string)
						if startTime, ok := settings["startTime"].(map[string]interface{}); ok {
							if year, ok := startTime["year"]; ok {
								if year != nil {
									yearFloat, err := numberx.GetFloatNumber(year)
									if err != nil {
										//logger.Debugf(eventScheduleLog, "流程(%s)的年份转数字失败:%s", err.Error())
										return
									}
									if int(yearFloat) != 0 {
										if time.Now().Year()%int(yearFloat) != 0 {
											//logger.Debugf(eventScheduleLog, "流程(%s)的定时任务开始时间未到或已经超过，不执行", eventID)
											return
										}
									}
								}
							}
						}
					}
				} else {
					scheduleType, ok = settings["type"].(string)
					if startTime, ok := settings["startTime"].(map[string]interface{}); ok {
						if year, ok := startTime["year"]; ok {
							if year != nil {
								yearFloat, err := numberx.GetFloatNumber(year)
								if err != nil {
									//logger.Debugf(eventScheduleLog, "流程(%s)的年份转数字失败:%s", err.Error())
									return
								}
								if int(yearFloat) != 0 {
									if time.Now().Year()%int(yearFloat) != 0 {
										//logger.Debugf(eventScheduleLog, "流程(%s)的定时任务开始时间未到或已经超过，不执行", eventID)
										return
									}
								}
							}
						}
					}
				}
				if startValidTime, ok := settings["startValidTime"].(primitive.DateTime); ok {
					if timex.GetLocalTimeNow(time.Now()).Unix() < int64(startValidTime)/1000 {
						logger.Debugf("流程(%s)的定时任务开始时间未到:%s", flowID)
						return
					}
				}
				if endTime, ok := settings["endTime"].(primitive.DateTime); ok {
					if timex.GetLocalTimeNow(time.Now()).Unix() >= int64(endTime)/1000 {
						//logger.Debugf(eventScheduleLog, "流程(%s)的定时任务结束流程已到，不再执行", eventID)

						//修改流程为失效
						updateMap := bson.M{"invalid": true}
						//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
						//var r = make(map[string]interface{})
						_, err := mongoOps.UpdateByID(ctx, mongoClient.Database(projectName).Collection("flow"), flowID, updateMap)
						//err := apiClient.UpdateEventById(headerMap, eventID, updateMap, &r)
						if err != nil {
							//logger.Errorf(eventAlarmLog, "失效流程(%s)失败:%s", eventID, err.Error())
							return
						}
						return
					}
				}
			}
			scheduleTypeMap := map[string]string{
				"second": "每秒",
				"minute": "每分钟",
				"hour":   "每小时",
				"day":    "每天",
				"week":   "每周",
				"month":  "每月",
				"year":   "每年",
				"once":   "仅一次",
			}
			sendMap := map[string]interface{}{
				"time":     timex.GetLocalTimeNow(time.Now()).UnixNano() / 1e6,
				"interval": scheduleTypeMap[scheduleType],
				"flowName": flowName,
			}
			err = flowx.StartFlow(zbClient, flowInfo.FlowXml, projectName, sendMap)
			if err != nil {
				logger.Errorf("流程(%s)推进到下一阶段失败:%s", flowIDCron, err.Error())
				return
			}

			if isOnce {
				//logger.Warnln(eventAlarmLog, "流程(%s)为只执行一次的流程", eventID)
				//修改流程为失效
				updateMap := bson.M{"invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
				//var r = make(map[string]interface{})
				_, err := mongoOps.UpdateByID(ctx, mongoClient.Database(projectName).Collection("flow"), flowID, updateMap)
				//err := apiClient.UpdateEventById(headerMap, eventID, updateMap, &r)
				if err != nil {
					//logger.Errorf(eventAlarmLog, "失效流程(%s)失败:%s", eventID, err.Error())
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
	//logger.Debugf(eventScheduleLog, "计划流程触发器(修改或删除计划流程)执行结束")
	return nil
}
