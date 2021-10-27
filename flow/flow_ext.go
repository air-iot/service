package flow

import (
	"context"
	"fmt"
	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/cache/table"
	"github.com/air-iot/service/logger"
	"github.com/air-iot/service/util/flowx"
	"github.com/air-iot/service/util/json"
	"github.com/air-iot/service/util/numberx"
	"github.com/camunda-cloud/zeebe/clients/go/pkg/zbc"
	"reflect"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/gin/ginx"
	"github.com/air-iot/service/init/mq"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/util/timex"
	"github.com/air-iot/service/util/uuid"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

var flowExtModifyLog = map[string]interface{}{"name": "工作表流程触发"}

func TriggerExtModifyFlow(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, mq mq.MQ, apiClient api.Client, zbClient zbc.Client, projectName, tableName string, data map[string]interface{}, oldInfoRaw bson.M, operateType string) error {
	////logger.Debugf(eventDeviceModifyLog, "开始执行资产修改流程触发器")
	////logger.Debugf(eventDeviceModifyLog, "传入参数为:%+v", data)
	//ctx, cancel := context.WithTimeout(context.Background(), time.Duration(30)*time.Second)
	//defer cancel()
	headerMap := map[string]string{ginx.XRequestProject: projectName}

	modifyType, ok := data["extFlowType"].(string)
	if !ok {
		return nil
	}

	modifyTypeMapping := map[string]string{
		"工作表记录增加":  "新增记录时",
		"工作表记录修改":  "更新记录时",
		"新增或更新记录时": "添加或更新记录时",
		"工作表记录删除":  "删除记录时",
		//"编辑模型":   "编辑模型",
		//"删除模型":   "删除模型",
		//"编辑模型画面": "编辑模型画面",
		//"删除模型画面": "删除模型画面",
		//"新增模型画面": "新增模型画面",
	}

	modifyTypeAfterMapping := modifyTypeMapping[modifyType]

	flowInfoList := make([]entity.ExtFlow, 0)
	//err = restfulapi.FindPipeline(ctx, idb.Database.Collection("event"), &eventInfoList, pipeline, nil)

	query := map[string]interface{}{
		"filter": map[string]interface{}{
			"type":                ExtModify,
			"settings.eventType":  modifyTypeAfterMapping,
			"settings.table.name": tableName,
			//"settings.eventRange": "node",
		},
		"project": map[string]interface{}{
			"name":     1,
			"settings": 1,
			"type":     1,
			"flowJson": 1,
			"invalid":  1,
			"flowXml":  1,
			"disable":  1,
		},
	}
	err := apiClient.FindFlowQuery(headerMap, query, &flowInfoList)
	if err != nil {
		logger.Warnf("获取工作表流程失败:%s", err.Error())
		return nil
	}

	variables := map[string]interface{}{"#project": projectName}
	variablesBytes, err := json.Marshal(variables)
	if err != nil {
		return err
	}
	//////fmt.Println("flowInfoList:", len(flowInfoList))
	////logger.Debugf(eventDeviceModifyLog, "开始遍历流程列表")
	timeNowRaw := time.Now()
	timeNow := &timeNowRaw
	for _, flowInfo := range flowInfoList {

		//logger.Debugf(eventDeviceModifyLog, "开始分析流程")
		flowID := flowInfo.ID
		settings := flowInfo.Settings

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

		//if flowInfo.ValidTime == "timeLimit" {
		//	if flowInfo.Range != "once" {
		//判断有效期
		startTime := flowInfo.StartTime
		formatLayout := timex.FormatTimeFormat(startTime)
		if startTime != "" {
			formatStartTime, err := timex.ConvertStringToTime(formatLayout, startTime, time.Local)
			if err != nil {
				logger.Errorf("开始时间范围字段值格式错误:%s", err.Error())
				continue
			}
			if timex.GetLocalTimeNow(time.Now()).Unix() < formatStartTime.Unix() {
				logger.Debugf("流程(%s)的定时任务开始时间未到，不执行", flowID)
				continue
			}
		}

		endTime := flowInfo.EndTime
		formatLayout = timex.FormatTimeFormat(endTime)
		if endTime != "" {
			formatEndTime, err := timex.ConvertStringToTime(formatLayout, endTime, time.Local)
			if err != nil {
				logger.Errorf("时间范围字段值格式错误:%s", err.Error())
				continue
			}
			if timex.GetLocalTimeNow(time.Now()).Unix() >= formatEndTime.Unix() {
				logger.Debugf("流程(%s)的定时任务结束时间已到，不执行", flowID)
				//修改流程为失效
				updateMap := bson.M{"invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("flow"), eventID, updateMap)
				var r = make(map[string]interface{})
				err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
				if err != nil {
					logger.Errorf("失效流程(%s)失败:%s", flowID, err.Error())
					continue
				}
				continue
			}
		}
		//	}
		//}

		if tableName != settings.Table.Name {
			logger.Debugf("不是流程(%s)触发需要的工作表", flowID)
			continue
		}

		//判断流程是否已经触发
		hasExecute := false
		logger.Debugf("开始分析流程")
		isValid := false

		excelColNameTypeExtMap, err := getTableSchemaColsNameMap(ctx, redisClient, mongoClient, projectName, settings.Table.ID)
		if err != nil {
			logger.Errorf("流程(%s)中解析工作表字段失败:%s", flowID, err.Error())
			continue
		}
		//////fmt.Println("excelColNameTypeExtMap：", excelColNameTypeExtMap)
		b, err := json.Marshal(settings.Query)
		if err != nil {
			logger.Errorf("流程(%s)中序列化过滤参数失败:%s", flowID, err.Error())
			continue
		}
		formatQueryString := strings.ReplaceAll(string(b), "@#$", "$")
		formatQuery := map[string]interface{}{}
		err = json.Unmarshal([]byte(formatQueryString), &formatQuery)
		if err != nil {
			logger.Errorf("流程(%s)中解序列化过滤参数失败:%s", flowID, err.Error())
			continue
		}
		settings.Query = formatQuery

		//=================
		//////fmt.Println("settings:", settings)
		//////fmt.Println("settings.EventType:", settings.EventType)
		formatVal, err := ConvertVariable(ctx, apiClient, variablesBytes, settings.Query)
		if err != nil {
			logger.Errorf("流程(%s)中替换模板变量失败:%s", flowID, err.Error())
			continue
		}
		if formatMap, ok := formatVal.(map[string]interface{}); ok {
			settings.Query = formatMap
		} else {
			logger.Errorf("流程(%s)中替换后的模板变量类型不是对象", flowID)
			continue
		}
		oldInfo := map[string]interface{}{}
		if oldInfoRaw != nil && len(oldInfoRaw) != 0 {
			oldByte, err := json.Marshal(oldInfoRaw)
			if err != nil {
				logger.Errorf("流程(%s)中序列化旧值失败:%s", flowID, err.Error())
				continue
			}
			err = json.Unmarshal(oldByte, &oldInfo)
			if err != nil {
				logger.Errorf("流程(%s)中解序列化旧值失败:%s", flowID, err.Error())
				continue
			}
		}
		////fmt.Println("after settings.EventType")
		switch settings.EventType {
		case "新增记录时":
			if filter, ok := settings.Query["filter"]; ok {
				if filterM, ok := filter.(map[string]interface{}); ok {
					for key, val := range filterM {
						if extRaw, ok := excelColNameTypeExtMap[key]; ok {
							//fmt.Println("excelColNameTypeExtMap[key] in:", key)
							if extRaw.Format != "" {
								if timeMapRaw, ok := val.(map[string]interface{}); ok {
									for k, v := range timeMapRaw {
										if originTime, ok := v.(string); ok {
											var queryTime string
											var startPoint string
											var endPoint string
											timeMap := map[string]interface{}{}
											switch originTime {
											case "今天":
												startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											case "昨天":
												startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											case "明天":
												startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											case "本周":
												week := int(time.Now().Weekday())
												if week == 0 {
													week = 7
												}
												startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											case "上周":
												week := int(time.Now().Weekday())
												if week == 0 {
													week = 7
												}
												startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											case "今年":
												startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											case "去年":
												startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											case "明年":
												startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											default:
												if originTime != "" {
													fromNow := false
													if strings.HasPrefix(originTime, "前") {
														//fmt.Println("originTime:", originTime)
														if strings.HasSuffix(originTime, "当前") {
															fromNow = true
															originTime = strings.ReplaceAll(originTime, "当前", "")
															//fmt.Println("originTime after 当前:", originTime)
														}
														intervalNumberString := numberx.GetNumberExp(originTime)
														intervalNumber, err := strconv.Atoi(intervalNumberString)
														if err != nil {
															logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
															continue
														}
														interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
														//fmt.Println("intervalNumber:", intervalNumber)
														//fmt.Println("interval:", interval)
														switch interval {
														case "天":
															if fromNow {
																//fmt.Println("fromNow")
																startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
															}
														case "周":
															if fromNow {
																startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																weekDay := int(timeNow.Weekday())
																if weekDay == 0 {
																	weekDay = 7
																}
																startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
															}
														case "月":
															if fromNow {
																startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
															}
														case "年":
															if fromNow {
																startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
															}
														case "季度":
															if fromNow {
																startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																timeNowMonth := int(timeNow.Month())
																quarter1 := timeNowMonth - 1
																quarter2 := timeNowMonth - 4
																quarter3 := timeNowMonth - 7
																quarter4 := timeNowMonth - 10
																quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																min := quarter1
																for _, v := range quarterList {
																	if v < min {
																		min = v
																	}
																}
																startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
															}
														case "时":
															if fromNow {
																startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
															}
														case "分":
															if fromNow {
																startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
															}
														case "秒":
															startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														}
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													} else if strings.HasPrefix(originTime, "后") {
														if strings.HasSuffix(originTime, "当前") {
															fromNow = true
															originTime = strings.ReplaceAll(originTime, "当前", "")
														}
														intervalNumberString := numberx.GetNumberExp(originTime)
														intervalNumber, err := strconv.Atoi(intervalNumberString)
														if err != nil {
															logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
															continue
														}
														interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
														switch interval {
														case "天":
															if fromNow {
																endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
															}
														case "周":
															if fromNow {
																endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																weekDay := int(timeNow.Weekday())
																if weekDay == 0 {
																	weekDay = 7
																}
																endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
															}
														case "月":
															if fromNow {
																endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
															}
														case "年":
															if fromNow {
																endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
															}
														case "季度":
															if fromNow {
																endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																timeNowMonth := int(timeNow.Month())
																quarter1 := timeNowMonth - 1
																quarter2 := timeNowMonth - 4
																quarter3 := timeNowMonth - 7
																quarter4 := timeNowMonth - 10
																quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																min := quarter1
																for _, v := range quarterList {
																	if v < min {
																		min = v
																	}
																}
																endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
															}
														case "时":
															if fromNow {
																endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
															}
														case "分":
															if fromNow {
																endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
															}
														case "秒":
															endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														}
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													} else if strings.HasPrefix(originTime, "当前") {
														interval := strings.ReplaceAll(originTime, "当前", "")
														switch interval {
														case "天":
															startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
														case "周":
															weekDay := int(timeNow.Weekday())
															if weekDay == 0 {
																weekDay = 7
															}
															startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
															endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
														case "月":
															startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
														case "年":
															startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
														case "季度":
															timeNowMonth := int(timeNow.Month())
															quarter1 := timeNowMonth - 1
															quarter2 := timeNowMonth - 4
															quarter3 := timeNowMonth - 7
															quarter4 := timeNowMonth - 10
															quarterList := []int{quarter1, quarter2, quarter3, quarter4}
															min := quarter1
															for _, v := range quarterList {
																if v < min {
																	min = v
																}
															}
															startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
															endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
														case "时":
															startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
														case "分":
															startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
														case "秒":
															endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														}
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													} else {
														eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
														if err != nil {
															continue
														}
														queryTime = eleTime.Format("2006-01-02 15:04:05")
													}
												}
											}
											if len(timeMap) != 0 {
												timeMapRaw[k] = timeMap
											} else {
												timeMapRaw[k] = queryTime
											}
										}
									}
								} else if originTime, ok := val.(string); ok {
									var queryTime string
									var startPoint string
									var endPoint string
									timeMap := map[string]interface{}{}
									switch originTime {
									case "今天":
										startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									case "昨天":
										startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									case "明天":
										startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									case "本周":
										week := int(time.Now().Weekday())
										if week == 0 {
											week = 7
										}
										startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									case "上周":
										week := int(time.Now().Weekday())
										if week == 0 {
											week = 7
										}
										startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									case "今年":
										startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									case "去年":
										startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									case "明年":
										startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									default:
										if originTime != "" {
											fromNow := false
											if strings.HasPrefix(originTime, "前") {
												//fmt.Println("originTime:", originTime)
												if strings.HasSuffix(originTime, "当前") {
													fromNow = true
													originTime = strings.ReplaceAll(originTime, "当前", "")
													//fmt.Println("originTime after 当前:", originTime)
												}
												intervalNumberString := numberx.GetNumberExp(originTime)
												intervalNumber, err := strconv.Atoi(intervalNumberString)
												if err != nil {
													logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
													continue
												}
												interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
												//fmt.Println("intervalNumber:", intervalNumber)
												//fmt.Println("interval:", interval)
												switch interval {
												case "天":
													if fromNow {
														//fmt.Println("fromNow")
														startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
													}
												case "周":
													if fromNow {
														startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														weekDay := int(timeNow.Weekday())
														if weekDay == 0 {
															weekDay = 7
														}
														startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
													}
												case "月":
													if fromNow {
														startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
													}
												case "年":
													if fromNow {
														startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
													}
												case "季度":
													if fromNow {
														startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														timeNowMonth := int(timeNow.Month())
														quarter1 := timeNowMonth - 1
														quarter2 := timeNowMonth - 4
														quarter3 := timeNowMonth - 7
														quarter4 := timeNowMonth - 10
														quarterList := []int{quarter1, quarter2, quarter3, quarter4}
														min := quarter1
														for _, v := range quarterList {
															if v < min {
																min = v
															}
														}
														startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
													}
												case "时":
													if fromNow {
														startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
														endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
													}
												case "分":
													if fromNow {
														startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
														endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
													}
												case "秒":
													startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
													endPoint = timeNow.Format("2006-01-02 15:04:05")
												}
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											} else if strings.HasPrefix(originTime, "后") {
												if strings.HasSuffix(originTime, "当前") {
													fromNow = true
													originTime = strings.ReplaceAll(originTime, "当前", "")
												}
												intervalNumberString := numberx.GetNumberExp(originTime)
												intervalNumber, err := strconv.Atoi(intervalNumberString)
												if err != nil {
													logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
													continue
												}
												interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
												switch interval {
												case "天":
													if fromNow {
														endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
														startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
													}
												case "周":
													if fromNow {
														endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														weekDay := int(timeNow.Weekday())
														if weekDay == 0 {
															weekDay = 7
														}
														endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
														startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
													}
												case "月":
													if fromNow {
														endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
														startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
													}
												case "年":
													if fromNow {
														endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
														startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
													}
												case "季度":
													if fromNow {
														endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														timeNowMonth := int(timeNow.Month())
														quarter1 := timeNowMonth - 1
														quarter2 := timeNowMonth - 4
														quarter3 := timeNowMonth - 7
														quarter4 := timeNowMonth - 10
														quarterList := []int{quarter1, quarter2, quarter3, quarter4}
														min := quarter1
														for _, v := range quarterList {
															if v < min {
																min = v
															}
														}
														endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
														startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
													}
												case "时":
													if fromNow {
														endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
														startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
													}
												case "分":
													if fromNow {
														endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
														startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
													}
												case "秒":
													endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
													startPoint = timeNow.Format("2006-01-02 15:04:05")
												}
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											} else if strings.HasPrefix(originTime, "当前") {
												interval := strings.ReplaceAll(originTime, "当前", "")
												switch interval {
												case "天":
													startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
													endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
												case "周":
													weekDay := int(timeNow.Weekday())
													if weekDay == 0 {
														weekDay = 7
													}
													startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
													endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
												case "月":
													startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
													endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
												case "年":
													startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
													endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
												case "季度":
													timeNowMonth := int(timeNow.Month())
													quarter1 := timeNowMonth - 1
													quarter2 := timeNowMonth - 4
													quarter3 := timeNowMonth - 7
													quarter4 := timeNowMonth - 10
													quarterList := []int{quarter1, quarter2, quarter3, quarter4}
													min := quarter1
													for _, v := range quarterList {
														if v < min {
															min = v
														}
													}
													startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
													endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
												case "时":
													startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
													endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
												case "分":
													startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
													endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
												case "秒":
													endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
													startPoint = timeNow.Format("2006-01-02 15:04:05")
												}
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											} else {
												eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
												if err != nil {
													continue
												}
												queryTime = eleTime.Format("2006-01-02 15:04:05")
											}
										}
									}
									if len(timeMap) != 0 {
										filterM[key] = timeMap
									} else {
										filterM[key] = queryTime
									}
								}
							}
						} else if key == "createTime" || key == "modifyTime" {
							//fmt.Println("createTime[key] in:", key, "val:", val)
							if timeMapRaw, ok := val.(map[string]interface{}); ok {
								for k, v := range timeMapRaw {
									if originTime, ok := v.(string); ok {
										var queryTime string
										var startPoint string
										var endPoint string
										timeMap := map[string]interface{}{}
										switch originTime {
										case "今天":
											startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										case "昨天":
											startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										case "明天":
											startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										case "本周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										case "上周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										case "今年":
											startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										case "去年":
											startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										case "明年":
											startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										default:
											if originTime != "" {
												fromNow := false
												if strings.HasPrefix(originTime, "前") {
													//fmt.Println("originTime:", originTime)
													if strings.HasSuffix(originTime, "当前") {
														fromNow = true
														originTime = strings.ReplaceAll(originTime, "当前", "")
														//fmt.Println("originTime after 当前:", originTime)
													}
													intervalNumberString := numberx.GetNumberExp(originTime)
													intervalNumber, err := strconv.Atoi(intervalNumberString)
													if err != nil {
														logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
														continue
													}
													interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
													//fmt.Println("intervalNumber:", intervalNumber)
													//fmt.Println("interval:", interval)
													switch interval {
													case "天":
														if fromNow {
															//fmt.Println("fromNow")
															startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
														}
													case "周":
														if fromNow {
															startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															weekDay := int(timeNow.Weekday())
															if weekDay == 0 {
																weekDay = 7
															}
															startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
														}
													case "月":
														if fromNow {
															startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
														}
													case "年":
														if fromNow {
															startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
														}
													case "季度":
														if fromNow {
															startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															timeNowMonth := int(timeNow.Month())
															quarter1 := timeNowMonth - 1
															quarter2 := timeNowMonth - 4
															quarter3 := timeNowMonth - 7
															quarter4 := timeNowMonth - 10
															quarterList := []int{quarter1, quarter2, quarter3, quarter4}
															min := quarter1
															for _, v := range quarterList {
																if v < min {
																	min = v
																}
															}
															startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
														}
													case "时":
														if fromNow {
															startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
															endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
														}
													case "分":
														if fromNow {
															startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
															endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
														}
													case "秒":
														startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													}
													timeMap = map[string]interface{}{
														"$gte": startPoint,
														"$lt":  endPoint,
													}
												} else if strings.HasPrefix(originTime, "后") {
													if strings.HasSuffix(originTime, "当前") {
														fromNow = true
														originTime = strings.ReplaceAll(originTime, "当前", "")
													}
													intervalNumberString := numberx.GetNumberExp(originTime)
													intervalNumber, err := strconv.Atoi(intervalNumberString)
													if err != nil {
														logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
														continue
													}
													interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
													switch interval {
													case "天":
														if fromNow {
															endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
															startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
														}
													case "周":
														if fromNow {
															endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															weekDay := int(timeNow.Weekday())
															if weekDay == 0 {
																weekDay = 7
															}
															endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
															startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
														}
													case "月":
														if fromNow {
															endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
															startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
														}
													case "年":
														if fromNow {
															endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
															startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
														}
													case "季度":
														if fromNow {
															endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															timeNowMonth := int(timeNow.Month())
															quarter1 := timeNowMonth - 1
															quarter2 := timeNowMonth - 4
															quarter3 := timeNowMonth - 7
															quarter4 := timeNowMonth - 10
															quarterList := []int{quarter1, quarter2, quarter3, quarter4}
															min := quarter1
															for _, v := range quarterList {
																if v < min {
																	min = v
																}
															}
															endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
															startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
														}
													case "时":
														if fromNow {
															endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
															startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
														}
													case "分":
														if fromNow {
															endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
															startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
														}
													case "秒":
														endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													}
													timeMap = map[string]interface{}{
														"$gte": startPoint,
														"$lt":  endPoint,
													}
												} else if strings.HasPrefix(originTime, "当前") {
													interval := strings.ReplaceAll(originTime, "当前", "")
													switch interval {
													case "天":
														startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
													case "周":
														weekDay := int(timeNow.Weekday())
														if weekDay == 0 {
															weekDay = 7
														}
														startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
														endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
													case "月":
														startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
													case "年":
														startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
													case "季度":
														timeNowMonth := int(timeNow.Month())
														quarter1 := timeNowMonth - 1
														quarter2 := timeNowMonth - 4
														quarter3 := timeNowMonth - 7
														quarter4 := timeNowMonth - 10
														quarterList := []int{quarter1, quarter2, quarter3, quarter4}
														min := quarter1
														for _, v := range quarterList {
															if v < min {
																min = v
															}
														}
														startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
														endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
													case "时":
														startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
													case "分":
														startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
													case "秒":
														endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													}
													timeMap = map[string]interface{}{
														"$gte": startPoint,
														"$lt":  endPoint,
													}
												} else {
													eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
													if err != nil {
														continue
													}
													queryTime = eleTime.Format("2006-01-02 15:04:05")
												}
											}
										}
										if len(timeMap) != 0 {
											timeMapRaw[k] = timeMap
										} else {
											timeMapRaw[k] = queryTime
										}
									}
								}
							} else if originTime, ok := val.(string); ok {
								//fmt.Println("val.(string):", val.(string))
								var queryTime string
								var startPoint string
								var endPoint string
								timeMap := map[string]interface{}{}
								switch originTime {
								case "今天":
									startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
									endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
									timeMap = map[string]interface{}{
										"$gte": startPoint,
										"$lt":  endPoint,
									}
								case "昨天":
									startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
									endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
									timeMap = map[string]interface{}{
										"$gte": startPoint,
										"$lt":  endPoint,
									}
								case "明天":
									startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
									endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
									timeMap = map[string]interface{}{
										"$gte": startPoint,
										"$lt":  endPoint,
									}
								case "本周":
									week := int(time.Now().Weekday())
									if week == 0 {
										week = 7
									}
									startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
									endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
									timeMap = map[string]interface{}{
										"$gte": startPoint,
										"$lt":  endPoint,
									}
								case "上周":
									week := int(time.Now().Weekday())
									if week == 0 {
										week = 7
									}
									startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
									endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
									timeMap = map[string]interface{}{
										"$gte": startPoint,
										"$lt":  endPoint,
									}
								case "今年":
									startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
									endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
									timeMap = map[string]interface{}{
										"$gte": startPoint,
										"$lt":  endPoint,
									}
								case "去年":
									startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
									endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
									timeMap = map[string]interface{}{
										"$gte": startPoint,
										"$lt":  endPoint,
									}
								case "明年":
									startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
									endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
									timeMap = map[string]interface{}{
										"$gte": startPoint,
										"$lt":  endPoint,
									}
								default:
									if originTime != "" {
										fromNow := false
										if strings.HasPrefix(originTime, "前") {
											//fmt.Println("originTime:", originTime)
											if strings.HasSuffix(originTime, "当前") {
												fromNow = true
												originTime = strings.ReplaceAll(originTime, "当前", "")
												//fmt.Println("originTime after 当前:", originTime)
											}
											intervalNumberString := numberx.GetNumberExp(originTime)
											intervalNumber, err := strconv.Atoi(intervalNumberString)
											if err != nil {
												logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
												continue
											}
											interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
											//fmt.Println("intervalNumber:", intervalNumber)
											//fmt.Println("interval:", interval)
											switch interval {
											case "天":
												if fromNow {
													//fmt.Println("fromNow")
													startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													endPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
													endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
												}
											case "周":
												if fromNow {
													startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													endPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													weekDay := int(timeNow.Weekday())
													if weekDay == 0 {
														weekDay = 7
													}
													startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
													endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
												}
											case "月":
												if fromNow {
													startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													endPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
													endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
												}
											case "年":
												if fromNow {
													startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													endPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
													endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
												}
											case "季度":
												if fromNow {
													startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													endPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													timeNowMonth := int(timeNow.Month())
													quarter1 := timeNowMonth - 1
													quarter2 := timeNowMonth - 4
													quarter3 := timeNowMonth - 7
													quarter4 := timeNowMonth - 10
													quarterList := []int{quarter1, quarter2, quarter3, quarter4}
													min := quarter1
													for _, v := range quarterList {
														if v < min {
															min = v
														}
													}
													startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
													endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
												}
											case "时":
												if fromNow {
													startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													endPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
													endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
												}
											case "分":
												if fromNow {
													startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
													endPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
													endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
												}
											case "秒":
												startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
												endPoint = timeNow.Format("2006-01-02 15:04:05")
											}
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										} else if strings.HasPrefix(originTime, "后") {
											if strings.HasSuffix(originTime, "当前") {
												fromNow = true
												originTime = strings.ReplaceAll(originTime, "当前", "")
											}
											intervalNumberString := numberx.GetNumberExp(originTime)
											intervalNumber, err := strconv.Atoi(intervalNumberString)
											if err != nil {
												logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
												continue
											}
											interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
											switch interval {
											case "天":
												if fromNow {
													endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													startPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
													startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
												}
											case "周":
												if fromNow {
													endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													startPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													weekDay := int(timeNow.Weekday())
													if weekDay == 0 {
														weekDay = 7
													}
													endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
													startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
												}
											case "月":
												if fromNow {
													endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													startPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
													startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
												}
											case "年":
												if fromNow {
													endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													startPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
													startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
												}
											case "季度":
												if fromNow {
													endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													startPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													timeNowMonth := int(timeNow.Month())
													quarter1 := timeNowMonth - 1
													quarter2 := timeNowMonth - 4
													quarter3 := timeNowMonth - 7
													quarter4 := timeNowMonth - 10
													quarterList := []int{quarter1, quarter2, quarter3, quarter4}
													min := quarter1
													for _, v := range quarterList {
														if v < min {
															min = v
														}
													}
													endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
													startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
												}
											case "时":
												if fromNow {
													endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													startPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
													startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
												}
											case "分":
												if fromNow {
													endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
													startPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
													startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
												}
											case "秒":
												endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
												startPoint = timeNow.Format("2006-01-02 15:04:05")
											}
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										} else if strings.HasPrefix(originTime, "当前") {
											interval := strings.ReplaceAll(originTime, "当前", "")
											switch interval {
											case "天":
												startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
											case "周":
												weekDay := int(timeNow.Weekday())
												if weekDay == 0 {
													weekDay = 7
												}
												startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
												endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
											case "月":
												startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
											case "年":
												startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
											case "季度":
												timeNowMonth := int(timeNow.Month())
												quarter1 := timeNowMonth - 1
												quarter2 := timeNowMonth - 4
												quarter3 := timeNowMonth - 7
												quarter4 := timeNowMonth - 10
												quarterList := []int{quarter1, quarter2, quarter3, quarter4}
												min := quarter1
												for _, v := range quarterList {
													if v < min {
														min = v
													}
												}
												startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
												endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
											case "时":
												startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
											case "分":
												startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
											case "秒":
												endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
												startPoint = timeNow.Format("2006-01-02 15:04:05")
											}
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										} else {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
											if err != nil {
												continue
											}
											queryTime = eleTime.Format("2006-01-02 15:04:05")
										}
									}
								}
								if len(timeMap) != 0 {
									filterM[key] = timeMap
								} else {
									filterM[key] = queryTime
								}
							}
						}
						if key == "$or" {
							if orList, ok := val.([]interface{}); ok {
								for _, orEle := range orList {
									if orEleMap, ok := orEle.(map[string]interface{}); ok {
										for orKey, orVal := range orEleMap {
											if orKey == "$and" {
												if orList, ok := orVal.([]interface{}); ok {
													for _, orEle := range orList {
														if orEleMap, ok := orEle.(map[string]interface{}); ok {
															for orKey, orVal := range orEleMap {
																if extRaw, ok := excelColNameTypeExtMap[orKey]; ok {
																	if extRaw.Format != "" {
																		if timeMapRaw, ok := orVal.(map[string]interface{}); ok {
																			for k, v := range timeMapRaw {
																				if originTime, ok := v.(string); ok {
																					var queryTime string
																					var startPoint string
																					var endPoint string
																					timeMap := map[string]interface{}{}
																					switch originTime {
																					case "今天":
																						startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					case "昨天":
																						startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					case "明天":
																						startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					case "本周":
																						week := int(time.Now().Weekday())
																						if week == 0 {
																							week = 7
																						}
																						startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					case "上周":
																						week := int(time.Now().Weekday())
																						if week == 0 {
																							week = 7
																						}
																						startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					case "今年":
																						startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					case "去年":
																						startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					case "明年":
																						startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					default:
																						if originTime != "" {
																							fromNow := false
																							if strings.HasPrefix(originTime, "前") {
																								//fmt.Println("originTime:", originTime)
																								if strings.HasSuffix(originTime, "当前") {
																									fromNow = true
																									originTime = strings.ReplaceAll(originTime, "当前", "")
																									//fmt.Println("originTime after 当前:", originTime)
																								}
																								intervalNumberString := numberx.GetNumberExp(originTime)
																								intervalNumber, err := strconv.Atoi(intervalNumberString)
																								if err != nil {
																									logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																									continue
																								}
																								interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																								//fmt.Println("intervalNumber:", intervalNumber)
																								//fmt.Println("interval:", interval)
																								switch interval {
																								case "天":
																									if fromNow {
																										//fmt.Println("fromNow")
																										startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																									}
																								case "周":
																									if fromNow {
																										startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										weekDay := int(timeNow.Weekday())
																										if weekDay == 0 {
																											weekDay = 7
																										}
																										startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																									}
																								case "月":
																									if fromNow {
																										startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																									}
																								case "年":
																									if fromNow {
																										startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																									}
																								case "季度":
																									if fromNow {
																										startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										timeNowMonth := int(timeNow.Month())
																										quarter1 := timeNowMonth - 1
																										quarter2 := timeNowMonth - 4
																										quarter3 := timeNowMonth - 7
																										quarter4 := timeNowMonth - 10
																										quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																										min := quarter1
																										for _, v := range quarterList {
																											if v < min {
																												min = v
																											}
																										}
																										startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																									}
																								case "时":
																									if fromNow {
																										startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																									}
																								case "分":
																									if fromNow {
																										startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																									}
																								case "秒":
																									startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								}
																								timeMap = map[string]interface{}{
																									"$gte": startPoint,
																									"$lt":  endPoint,
																								}
																							} else if strings.HasPrefix(originTime, "后") {
																								if strings.HasSuffix(originTime, "当前") {
																									fromNow = true
																									originTime = strings.ReplaceAll(originTime, "当前", "")
																								}
																								intervalNumberString := numberx.GetNumberExp(originTime)
																								intervalNumber, err := strconv.Atoi(intervalNumberString)
																								if err != nil {
																									logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																									continue
																								}
																								interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																								switch interval {
																								case "天":
																									if fromNow {
																										endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																										startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																									}
																								case "周":
																									if fromNow {
																										endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										weekDay := int(timeNow.Weekday())
																										if weekDay == 0 {
																											weekDay = 7
																										}
																										endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																										startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																									}
																								case "月":
																									if fromNow {
																										endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																										startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																									}
																								case "年":
																									if fromNow {
																										endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																										startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																									}
																								case "季度":
																									if fromNow {
																										endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										timeNowMonth := int(timeNow.Month())
																										quarter1 := timeNowMonth - 1
																										quarter2 := timeNowMonth - 4
																										quarter3 := timeNowMonth - 7
																										quarter4 := timeNowMonth - 10
																										quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																										min := quarter1
																										for _, v := range quarterList {
																											if v < min {
																												min = v
																											}
																										}
																										endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																										startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																									}
																								case "时":
																									if fromNow {
																										endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																										startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																									}
																								case "分":
																									if fromNow {
																										endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																										startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																									}
																								case "秒":
																									endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								}
																								timeMap = map[string]interface{}{
																									"$gte": startPoint,
																									"$lt":  endPoint,
																								}
																							} else if strings.HasPrefix(originTime, "当前") {
																								interval := strings.ReplaceAll(originTime, "当前", "")
																								switch interval {
																								case "天":
																									startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																								case "周":
																									weekDay := int(timeNow.Weekday())
																									if weekDay == 0 {
																										weekDay = 7
																									}
																									startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																								case "月":
																									startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																								case "年":
																									startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																								case "季度":
																									timeNowMonth := int(timeNow.Month())
																									quarter1 := timeNowMonth - 1
																									quarter2 := timeNowMonth - 4
																									quarter3 := timeNowMonth - 7
																									quarter4 := timeNowMonth - 10
																									quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																									min := quarter1
																									for _, v := range quarterList {
																										if v < min {
																											min = v
																										}
																									}
																									startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																								case "时":
																									startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																								case "分":
																									startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																								case "秒":
																									endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								}
																								timeMap = map[string]interface{}{
																									"$gte": startPoint,
																									"$lt":  endPoint,
																								}
																							} else {
																								eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																								if err != nil {
																									continue
																								}
																								queryTime = eleTime.Format("2006-01-02 15:04:05")
																							}
																						}
																					}
																					if len(timeMap) != 0 {
																						timeMapRaw[k] = timeMap
																					} else {
																						timeMapRaw[k] = queryTime
																					}
																				}
																			}
																		} else if originTime, ok := orVal.(string); ok {
																			var queryTime string
																			var startPoint string
																			var endPoint string
																			timeMap := map[string]interface{}{}
																			switch originTime {
																			case "今天":
																				startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			case "昨天":
																				startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			case "明天":
																				startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			case "本周":
																				week := int(time.Now().Weekday())
																				if week == 0 {
																					week = 7
																				}
																				startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			case "上周":
																				week := int(time.Now().Weekday())
																				if week == 0 {
																					week = 7
																				}
																				startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			case "今年":
																				startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			case "去年":
																				startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			case "明年":
																				startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			default:
																				if originTime != "" {
																					fromNow := false
																					if strings.HasPrefix(originTime, "前") {
																						//fmt.Println("originTime:", originTime)
																						if strings.HasSuffix(originTime, "当前") {
																							fromNow = true
																							originTime = strings.ReplaceAll(originTime, "当前", "")
																							//fmt.Println("originTime after 当前:", originTime)
																						}
																						intervalNumberString := numberx.GetNumberExp(originTime)
																						intervalNumber, err := strconv.Atoi(intervalNumberString)
																						if err != nil {
																							logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																							continue
																						}
																						interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																						//fmt.Println("intervalNumber:", intervalNumber)
																						//fmt.Println("interval:", interval)
																						switch interval {
																						case "天":
																							if fromNow {
																								//fmt.Println("fromNow")
																								startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																							}
																						case "周":
																							if fromNow {
																								startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								weekDay := int(timeNow.Weekday())
																								if weekDay == 0 {
																									weekDay = 7
																								}
																								startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																							}
																						case "月":
																							if fromNow {
																								startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																							}
																						case "年":
																							if fromNow {
																								startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																							}
																						case "季度":
																							if fromNow {
																								startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								timeNowMonth := int(timeNow.Month())
																								quarter1 := timeNowMonth - 1
																								quarter2 := timeNowMonth - 4
																								quarter3 := timeNowMonth - 7
																								quarter4 := timeNowMonth - 10
																								quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																								min := quarter1
																								for _, v := range quarterList {
																									if v < min {
																										min = v
																									}
																								}
																								startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																							}
																						case "时":
																							if fromNow {
																								startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																							}
																						case "分":
																							if fromNow {
																								startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																							}
																						case "秒":
																							startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																							endPoint = timeNow.Format("2006-01-02 15:04:05")
																						}
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					} else if strings.HasPrefix(originTime, "后") {
																						if strings.HasSuffix(originTime, "当前") {
																							fromNow = true
																							originTime = strings.ReplaceAll(originTime, "当前", "")
																						}
																						intervalNumberString := numberx.GetNumberExp(originTime)
																						intervalNumber, err := strconv.Atoi(intervalNumberString)
																						if err != nil {
																							logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																							continue
																						}
																						interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																						switch interval {
																						case "天":
																							if fromNow {
																								endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																								startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																							}
																						case "周":
																							if fromNow {
																								endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								weekDay := int(timeNow.Weekday())
																								if weekDay == 0 {
																									weekDay = 7
																								}
																								endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																								startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																							}
																						case "月":
																							if fromNow {
																								endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																								startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																							}
																						case "年":
																							if fromNow {
																								endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																								startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																							}
																						case "季度":
																							if fromNow {
																								endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								timeNowMonth := int(timeNow.Month())
																								quarter1 := timeNowMonth - 1
																								quarter2 := timeNowMonth - 4
																								quarter3 := timeNowMonth - 7
																								quarter4 := timeNowMonth - 10
																								quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																								min := quarter1
																								for _, v := range quarterList {
																									if v < min {
																										min = v
																									}
																								}
																								endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																								startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																							}
																						case "时":
																							if fromNow {
																								endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																								startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																							}
																						case "分":
																							if fromNow {
																								endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																								startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																							}
																						case "秒":
																							endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																							startPoint = timeNow.Format("2006-01-02 15:04:05")
																						}
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					} else if strings.HasPrefix(originTime, "当前") {
																						interval := strings.ReplaceAll(originTime, "当前", "")
																						switch interval {
																						case "天":
																							startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																						case "周":
																							weekDay := int(timeNow.Weekday())
																							if weekDay == 0 {
																								weekDay = 7
																							}
																							startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																						case "月":
																							startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																						case "年":
																							startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																						case "季度":
																							timeNowMonth := int(timeNow.Month())
																							quarter1 := timeNowMonth - 1
																							quarter2 := timeNowMonth - 4
																							quarter3 := timeNowMonth - 7
																							quarter4 := timeNowMonth - 10
																							quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																							min := quarter1
																							for _, v := range quarterList {
																								if v < min {
																									min = v
																								}
																							}
																							startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																						case "时":
																							startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																						case "分":
																							startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																						case "秒":
																							endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																							startPoint = timeNow.Format("2006-01-02 15:04:05")
																						}
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					} else {
																						eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																						if err != nil {
																							continue
																						}
																						queryTime = eleTime.Format("2006-01-02 15:04:05")
																					}
																				}
																			}
																			if len(timeMap) != 0 {
																				orEleMap[orKey] = timeMap
																			} else {
																				orEleMap[orKey] = queryTime
																			}
																		}
																	}
																} else if orKey == "createTime" || orKey == "modifyTime" {
																	if timeMapRaw, ok := orVal.(map[string]interface{}); ok {
																		for k, v := range timeMapRaw {
																			if originTime, ok := v.(string); ok {
																				var queryTime string
																				var startPoint string
																				var endPoint string
																				timeMap := map[string]interface{}{}
																				switch originTime {
																				case "今天":
																					startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				case "昨天":
																					startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				case "明天":
																					startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				case "本周":
																					week := int(time.Now().Weekday())
																					if week == 0 {
																						week = 7
																					}
																					startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				case "上周":
																					week := int(time.Now().Weekday())
																					if week == 0 {
																						week = 7
																					}
																					startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				case "今年":
																					startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				case "去年":
																					startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				case "明年":
																					startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				default:
																					if originTime != "" {
																						fromNow := false
																						if strings.HasPrefix(originTime, "前") {
																							//fmt.Println("originTime:", originTime)
																							if strings.HasSuffix(originTime, "当前") {
																								fromNow = true
																								originTime = strings.ReplaceAll(originTime, "当前", "")
																								//fmt.Println("originTime after 当前:", originTime)
																							}
																							intervalNumberString := numberx.GetNumberExp(originTime)
																							intervalNumber, err := strconv.Atoi(intervalNumberString)
																							if err != nil {
																								logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																								continue
																							}
																							interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																							//fmt.Println("intervalNumber:", intervalNumber)
																							//fmt.Println("interval:", interval)
																							switch interval {
																							case "天":
																								if fromNow {
																									//fmt.Println("fromNow")
																									startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																								}
																							case "周":
																								if fromNow {
																									startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									weekDay := int(timeNow.Weekday())
																									if weekDay == 0 {
																										weekDay = 7
																									}
																									startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																								}
																							case "月":
																								if fromNow {
																									startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																								}
																							case "年":
																								if fromNow {
																									startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																								}
																							case "季度":
																								if fromNow {
																									startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									timeNowMonth := int(timeNow.Month())
																									quarter1 := timeNowMonth - 1
																									quarter2 := timeNowMonth - 4
																									quarter3 := timeNowMonth - 7
																									quarter4 := timeNowMonth - 10
																									quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																									min := quarter1
																									for _, v := range quarterList {
																										if v < min {
																											min = v
																										}
																									}
																									startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																								}
																							case "时":
																								if fromNow {
																									startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																								}
																							case "分":
																								if fromNow {
																									startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																								}
																							case "秒":
																								startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							}
																							timeMap = map[string]interface{}{
																								"$gte": startPoint,
																								"$lt":  endPoint,
																							}
																						} else if strings.HasPrefix(originTime, "后") {
																							if strings.HasSuffix(originTime, "当前") {
																								fromNow = true
																								originTime = strings.ReplaceAll(originTime, "当前", "")
																							}
																							intervalNumberString := numberx.GetNumberExp(originTime)
																							intervalNumber, err := strconv.Atoi(intervalNumberString)
																							if err != nil {
																								logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																								continue
																							}
																							interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																							switch interval {
																							case "天":
																								if fromNow {
																									endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																									startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																								}
																							case "周":
																								if fromNow {
																									endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									weekDay := int(timeNow.Weekday())
																									if weekDay == 0 {
																										weekDay = 7
																									}
																									endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																									startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																								}
																							case "月":
																								if fromNow {
																									endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																									startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																								}
																							case "年":
																								if fromNow {
																									endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																									startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																								}
																							case "季度":
																								if fromNow {
																									endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									timeNowMonth := int(timeNow.Month())
																									quarter1 := timeNowMonth - 1
																									quarter2 := timeNowMonth - 4
																									quarter3 := timeNowMonth - 7
																									quarter4 := timeNowMonth - 10
																									quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																									min := quarter1
																									for _, v := range quarterList {
																										if v < min {
																											min = v
																										}
																									}
																									endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																									startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																								}
																							case "时":
																								if fromNow {
																									endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																									startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																								}
																							case "分":
																								if fromNow {
																									endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																									startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																								}
																							case "秒":
																								endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							}
																							timeMap = map[string]interface{}{
																								"$gte": startPoint,
																								"$lt":  endPoint,
																							}
																						} else if strings.HasPrefix(originTime, "当前") {
																							interval := strings.ReplaceAll(originTime, "当前", "")
																							switch interval {
																							case "天":
																								startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																							case "周":
																								weekDay := int(timeNow.Weekday())
																								if weekDay == 0 {
																									weekDay = 7
																								}
																								startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																							case "月":
																								startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																							case "年":
																								startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																							case "季度":
																								timeNowMonth := int(timeNow.Month())
																								quarter1 := timeNowMonth - 1
																								quarter2 := timeNowMonth - 4
																								quarter3 := timeNowMonth - 7
																								quarter4 := timeNowMonth - 10
																								quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																								min := quarter1
																								for _, v := range quarterList {
																									if v < min {
																										min = v
																									}
																								}
																								startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																							case "时":
																								startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																							case "分":
																								startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																							case "秒":
																								endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							}
																							timeMap = map[string]interface{}{
																								"$gte": startPoint,
																								"$lt":  endPoint,
																							}
																						} else {
																							eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																							if err != nil {
																								continue
																							}
																							queryTime = eleTime.Format("2006-01-02 15:04:05")
																						}
																					}
																				}
																				if len(timeMap) != 0 {
																					timeMapRaw[k] = timeMap
																				} else {
																					timeMapRaw[k] = queryTime
																				}
																			}
																		}
																	} else if originTime, ok := orVal.(string); ok {
																		var queryTime string
																		var startPoint string
																		var endPoint string
																		timeMap := map[string]interface{}{}
																		switch originTime {
																		case "今天":
																			startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		case "昨天":
																			startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		case "明天":
																			startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		case "本周":
																			week := int(time.Now().Weekday())
																			if week == 0 {
																				week = 7
																			}
																			startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		case "上周":
																			week := int(time.Now().Weekday())
																			if week == 0 {
																				week = 7
																			}
																			startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		case "今年":
																			startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		case "去年":
																			startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		case "明年":
																			startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		default:
																			if originTime != "" {
																				fromNow := false
																				if strings.HasPrefix(originTime, "前") {
																					//fmt.Println("originTime:", originTime)
																					if strings.HasSuffix(originTime, "当前") {
																						fromNow = true
																						originTime = strings.ReplaceAll(originTime, "当前", "")
																						//fmt.Println("originTime after 当前:", originTime)
																					}
																					intervalNumberString := numberx.GetNumberExp(originTime)
																					intervalNumber, err := strconv.Atoi(intervalNumberString)
																					if err != nil {
																						logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																						continue
																					}
																					interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																					//fmt.Println("intervalNumber:", intervalNumber)
																					//fmt.Println("interval:", interval)
																					switch interval {
																					case "天":
																						if fromNow {
																							//fmt.Println("fromNow")
																							startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							endPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																						}
																					case "周":
																						if fromNow {
																							startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							endPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							weekDay := int(timeNow.Weekday())
																							if weekDay == 0 {
																								weekDay = 7
																							}
																							startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																						}
																					case "月":
																						if fromNow {
																							startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							endPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																						}
																					case "年":
																						if fromNow {
																							startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							endPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																						}
																					case "季度":
																						if fromNow {
																							startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							endPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							timeNowMonth := int(timeNow.Month())
																							quarter1 := timeNowMonth - 1
																							quarter2 := timeNowMonth - 4
																							quarter3 := timeNowMonth - 7
																							quarter4 := timeNowMonth - 10
																							quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																							min := quarter1
																							for _, v := range quarterList {
																								if v < min {
																									min = v
																								}
																							}
																							startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																						}
																					case "时":
																						if fromNow {
																							startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							endPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																						}
																					case "分":
																						if fromNow {
																							startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																							endPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																						}
																					case "秒":
																						startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																						endPoint = timeNow.Format("2006-01-02 15:04:05")
																					}
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				} else if strings.HasPrefix(originTime, "后") {
																					if strings.HasSuffix(originTime, "当前") {
																						fromNow = true
																						originTime = strings.ReplaceAll(originTime, "当前", "")
																					}
																					intervalNumberString := numberx.GetNumberExp(originTime)
																					intervalNumber, err := strconv.Atoi(intervalNumberString)
																					if err != nil {
																						logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																						continue
																					}
																					interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																					switch interval {
																					case "天":
																						if fromNow {
																							endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							startPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																							startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																						}
																					case "周":
																						if fromNow {
																							endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							startPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							weekDay := int(timeNow.Weekday())
																							if weekDay == 0 {
																								weekDay = 7
																							}
																							endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																							startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																						}
																					case "月":
																						if fromNow {
																							endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							startPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																							startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																						}
																					case "年":
																						if fromNow {
																							endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							startPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																							startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																						}
																					case "季度":
																						if fromNow {
																							endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							startPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							timeNowMonth := int(timeNow.Month())
																							quarter1 := timeNowMonth - 1
																							quarter2 := timeNowMonth - 4
																							quarter3 := timeNowMonth - 7
																							quarter4 := timeNowMonth - 10
																							quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																							min := quarter1
																							for _, v := range quarterList {
																								if v < min {
																									min = v
																								}
																							}
																							endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																							startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																						}
																					case "时":
																						if fromNow {
																							endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							startPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																							startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																						}
																					case "分":
																						if fromNow {
																							endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																							startPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																							startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																						}
																					case "秒":
																						endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																						startPoint = timeNow.Format("2006-01-02 15:04:05")
																					}
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				} else if strings.HasPrefix(originTime, "当前") {
																					interval := strings.ReplaceAll(originTime, "当前", "")
																					switch interval {
																					case "天":
																						startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																					case "周":
																						weekDay := int(timeNow.Weekday())
																						if weekDay == 0 {
																							weekDay = 7
																						}
																						startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																					case "月":
																						startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																					case "年":
																						startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																					case "季度":
																						timeNowMonth := int(timeNow.Month())
																						quarter1 := timeNowMonth - 1
																						quarter2 := timeNowMonth - 4
																						quarter3 := timeNowMonth - 7
																						quarter4 := timeNowMonth - 10
																						quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																						min := quarter1
																						for _, v := range quarterList {
																							if v < min {
																								min = v
																							}
																						}
																						startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																					case "时":
																						startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																					case "分":
																						startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																					case "秒":
																						endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																						startPoint = timeNow.Format("2006-01-02 15:04:05")
																					}
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				} else {
																					eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																					if err != nil {
																						continue
																					}
																					queryTime = eleTime.Format("2006-01-02 15:04:05")
																				}
																			}
																		}
																		if len(timeMap) != 0 {
																			orEleMap[orKey] = timeMap
																		} else {
																			orEleMap[orKey] = queryTime
																		}
																	}
																}
															}
														}
													}
												}
											} else if extRaw, ok := excelColNameTypeExtMap[orKey]; ok {
												if extRaw.Format != "" {
													if timeMapRaw, ok := orVal.(map[string]interface{}); ok {
														for k, v := range timeMapRaw {
															if originTime, ok := v.(string); ok {
																var queryTime string
																var startPoint string
																var endPoint string
																timeMap := map[string]interface{}{}
																switch originTime {
																case "今天":
																	startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																case "昨天":
																	startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																case "明天":
																	startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																case "本周":
																	week := int(time.Now().Weekday())
																	if week == 0 {
																		week = 7
																	}
																	startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																case "上周":
																	week := int(time.Now().Weekday())
																	if week == 0 {
																		week = 7
																	}
																	startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																case "今年":
																	startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																case "去年":
																	startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																case "明年":
																	startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																default:
																	if originTime != "" {
																		fromNow := false
																		if strings.HasPrefix(originTime, "前") {
																			//fmt.Println("originTime:", originTime)
																			if strings.HasSuffix(originTime, "当前") {
																				fromNow = true
																				originTime = strings.ReplaceAll(originTime, "当前", "")
																				//fmt.Println("originTime after 当前:", originTime)
																			}
																			intervalNumberString := numberx.GetNumberExp(originTime)
																			intervalNumber, err := strconv.Atoi(intervalNumberString)
																			if err != nil {
																				logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																				continue
																			}
																			interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																			//fmt.Println("intervalNumber:", intervalNumber)
																			//fmt.Println("interval:", interval)
																			switch interval {
																			case "天":
																				if fromNow {
																					//fmt.Println("fromNow")
																					startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																				}
																			case "周":
																				if fromNow {
																					startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					weekDay := int(timeNow.Weekday())
																					if weekDay == 0 {
																						weekDay = 7
																					}
																					startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																				}
																			case "月":
																				if fromNow {
																					startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																				}
																			case "年":
																				if fromNow {
																					startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																				}
																			case "季度":
																				if fromNow {
																					startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					timeNowMonth := int(timeNow.Month())
																					quarter1 := timeNowMonth - 1
																					quarter2 := timeNowMonth - 4
																					quarter3 := timeNowMonth - 7
																					quarter4 := timeNowMonth - 10
																					quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																					min := quarter1
																					for _, v := range quarterList {
																						if v < min {
																							min = v
																						}
																					}
																					startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																				}
																			case "时":
																				if fromNow {
																					startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																				}
																			case "分":
																				if fromNow {
																					startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																				}
																			case "秒":
																				startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			}
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		} else if strings.HasPrefix(originTime, "后") {
																			if strings.HasSuffix(originTime, "当前") {
																				fromNow = true
																				originTime = strings.ReplaceAll(originTime, "当前", "")
																			}
																			intervalNumberString := numberx.GetNumberExp(originTime)
																			intervalNumber, err := strconv.Atoi(intervalNumberString)
																			if err != nil {
																				logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																				continue
																			}
																			interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																			switch interval {
																			case "天":
																				if fromNow {
																					endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																					startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																				}
																			case "周":
																				if fromNow {
																					endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					weekDay := int(timeNow.Weekday())
																					if weekDay == 0 {
																						weekDay = 7
																					}
																					endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																					startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																				}
																			case "月":
																				if fromNow {
																					endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																					startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																				}
																			case "年":
																				if fromNow {
																					endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																					startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																				}
																			case "季度":
																				if fromNow {
																					endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					timeNowMonth := int(timeNow.Month())
																					quarter1 := timeNowMonth - 1
																					quarter2 := timeNowMonth - 4
																					quarter3 := timeNowMonth - 7
																					quarter4 := timeNowMonth - 10
																					quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																					min := quarter1
																					for _, v := range quarterList {
																						if v < min {
																							min = v
																						}
																					}
																					endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																					startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																				}
																			case "时":
																				if fromNow {
																					endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																					startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																				}
																			case "分":
																				if fromNow {
																					endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																					startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																				}
																			case "秒":
																				endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			}
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		} else if strings.HasPrefix(originTime, "当前") {
																			interval := strings.ReplaceAll(originTime, "当前", "")
																			switch interval {
																			case "天":
																				startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																			case "周":
																				weekDay := int(timeNow.Weekday())
																				if weekDay == 0 {
																					weekDay = 7
																				}
																				startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																			case "月":
																				startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																			case "年":
																				startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																			case "季度":
																				timeNowMonth := int(timeNow.Month())
																				quarter1 := timeNowMonth - 1
																				quarter2 := timeNowMonth - 4
																				quarter3 := timeNowMonth - 7
																				quarter4 := timeNowMonth - 10
																				quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																				min := quarter1
																				for _, v := range quarterList {
																					if v < min {
																						min = v
																					}
																				}
																				startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																			case "时":
																				startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																			case "分":
																				startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																			case "秒":
																				endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			}
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		} else {
																			eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																			if err != nil {
																				continue
																			}
																			queryTime = eleTime.Format("2006-01-02 15:04:05")
																		}
																	}
																}
																if len(timeMap) != 0 {
																	timeMapRaw[k] = timeMap
																} else {
																	timeMapRaw[k] = queryTime
																}
															}
														}
													} else if originTime, ok := orVal.(string); ok {
														var queryTime string
														var startPoint string
														var endPoint string
														timeMap := map[string]interface{}{}
														switch originTime {
														case "今天":
															startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														case "昨天":
															startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														case "明天":
															startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														case "本周":
															week := int(time.Now().Weekday())
															if week == 0 {
																week = 7
															}
															startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														case "上周":
															week := int(time.Now().Weekday())
															if week == 0 {
																week = 7
															}
															startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														case "今年":
															startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														case "去年":
															startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														case "明年":
															startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														default:
															if originTime != "" {
																fromNow := false
																if strings.HasPrefix(originTime, "前") {
																	//fmt.Println("originTime:", originTime)
																	if strings.HasSuffix(originTime, "当前") {
																		fromNow = true
																		originTime = strings.ReplaceAll(originTime, "当前", "")
																		//fmt.Println("originTime after 当前:", originTime)
																	}
																	intervalNumberString := numberx.GetNumberExp(originTime)
																	intervalNumber, err := strconv.Atoi(intervalNumberString)
																	if err != nil {
																		logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																		continue
																	}
																	interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																	//fmt.Println("intervalNumber:", intervalNumber)
																	//fmt.Println("interval:", interval)
																	switch interval {
																	case "天":
																		if fromNow {
																			//fmt.Println("fromNow")
																			startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																		}
																	case "周":
																		if fromNow {
																			startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			weekDay := int(timeNow.Weekday())
																			if weekDay == 0 {
																				weekDay = 7
																			}
																			startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																		}
																	case "月":
																		if fromNow {
																			startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																		}
																	case "年":
																		if fromNow {
																			startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																		}
																	case "季度":
																		if fromNow {
																			startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			timeNowMonth := int(timeNow.Month())
																			quarter1 := timeNowMonth - 1
																			quarter2 := timeNowMonth - 4
																			quarter3 := timeNowMonth - 7
																			quarter4 := timeNowMonth - 10
																			quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																			min := quarter1
																			for _, v := range quarterList {
																				if v < min {
																					min = v
																				}
																			}
																			startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																		}
																	case "时":
																		if fromNow {
																			startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																		}
																	case "分":
																		if fromNow {
																			startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																		}
																	case "秒":
																		startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																		endPoint = timeNow.Format("2006-01-02 15:04:05")
																	}
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																} else if strings.HasPrefix(originTime, "后") {
																	if strings.HasSuffix(originTime, "当前") {
																		fromNow = true
																		originTime = strings.ReplaceAll(originTime, "当前", "")
																	}
																	intervalNumberString := numberx.GetNumberExp(originTime)
																	intervalNumber, err := strconv.Atoi(intervalNumberString)
																	if err != nil {
																		logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																		continue
																	}
																	interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																	switch interval {
																	case "天":
																		if fromNow {
																			endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																			startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																		}
																	case "周":
																		if fromNow {
																			endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			weekDay := int(timeNow.Weekday())
																			if weekDay == 0 {
																				weekDay = 7
																			}
																			endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																			startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																		}
																	case "月":
																		if fromNow {
																			endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																			startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																		}
																	case "年":
																		if fromNow {
																			endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																			startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																		}
																	case "季度":
																		if fromNow {
																			endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			timeNowMonth := int(timeNow.Month())
																			quarter1 := timeNowMonth - 1
																			quarter2 := timeNowMonth - 4
																			quarter3 := timeNowMonth - 7
																			quarter4 := timeNowMonth - 10
																			quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																			min := quarter1
																			for _, v := range quarterList {
																				if v < min {
																					min = v
																				}
																			}
																			endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																			startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																		}
																	case "时":
																		if fromNow {
																			endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																			startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																		}
																	case "分":
																		if fromNow {
																			endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																			startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																		}
																	case "秒":
																		endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																		startPoint = timeNow.Format("2006-01-02 15:04:05")
																	}
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																} else if strings.HasPrefix(originTime, "当前") {
																	interval := strings.ReplaceAll(originTime, "当前", "")
																	switch interval {
																	case "天":
																		startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																	case "周":
																		weekDay := int(timeNow.Weekday())
																		if weekDay == 0 {
																			weekDay = 7
																		}
																		startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																	case "月":
																		startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																	case "年":
																		startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																	case "季度":
																		timeNowMonth := int(timeNow.Month())
																		quarter1 := timeNowMonth - 1
																		quarter2 := timeNowMonth - 4
																		quarter3 := timeNowMonth - 7
																		quarter4 := timeNowMonth - 10
																		quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																		min := quarter1
																		for _, v := range quarterList {
																			if v < min {
																				min = v
																			}
																		}
																		startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																	case "时":
																		startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																	case "分":
																		startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																	case "秒":
																		endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																		startPoint = timeNow.Format("2006-01-02 15:04:05")
																	}
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																} else {
																	eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																	if err != nil {
																		continue
																	}
																	queryTime = eleTime.Format("2006-01-02 15:04:05")
																}
															}
														}
														if len(timeMap) != 0 {
															orEleMap[orKey] = timeMap
														} else {
															orEleMap[orKey] = queryTime
														}
													}
												}
											} else if orKey == "createTime" || orKey == "modifyTime" {
												if timeMapRaw, ok := orVal.(map[string]interface{}); ok {
													for k, v := range timeMapRaw {
														if originTime, ok := v.(string); ok {
															var queryTime string
															var startPoint string
															var endPoint string
															timeMap := map[string]interface{}{}
															switch originTime {
															case "今天":
																startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															case "昨天":
																startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															case "明天":
																startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															case "本周":
																week := int(time.Now().Weekday())
																if week == 0 {
																	week = 7
																}
																startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															case "上周":
																week := int(time.Now().Weekday())
																if week == 0 {
																	week = 7
																}
																startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															case "今年":
																startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															case "去年":
																startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															case "明年":
																startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															default:
																if originTime != "" {
																	fromNow := false
																	if strings.HasPrefix(originTime, "前") {
																		//fmt.Println("originTime:", originTime)
																		if strings.HasSuffix(originTime, "当前") {
																			fromNow = true
																			originTime = strings.ReplaceAll(originTime, "当前", "")
																			//fmt.Println("originTime after 当前:", originTime)
																		}
																		intervalNumberString := numberx.GetNumberExp(originTime)
																		intervalNumber, err := strconv.Atoi(intervalNumberString)
																		if err != nil {
																			logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																			continue
																		}
																		interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																		//fmt.Println("intervalNumber:", intervalNumber)
																		//fmt.Println("interval:", interval)
																		switch interval {
																		case "天":
																			if fromNow {
																				//fmt.Println("fromNow")
																				startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																			}
																		case "周":
																			if fromNow {
																				startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				weekDay := int(timeNow.Weekday())
																				if weekDay == 0 {
																					weekDay = 7
																				}
																				startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																			}
																		case "月":
																			if fromNow {
																				startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																			}
																		case "年":
																			if fromNow {
																				startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																			}
																		case "季度":
																			if fromNow {
																				startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				timeNowMonth := int(timeNow.Month())
																				quarter1 := timeNowMonth - 1
																				quarter2 := timeNowMonth - 4
																				quarter3 := timeNowMonth - 7
																				quarter4 := timeNowMonth - 10
																				quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																				min := quarter1
																				for _, v := range quarterList {
																					if v < min {
																						min = v
																					}
																				}
																				startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																			}
																		case "时":
																			if fromNow {
																				startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																			}
																		case "分":
																			if fromNow {
																				startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																			}
																		case "秒":
																			startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		}
																		timeMap = map[string]interface{}{
																			"$gte": startPoint,
																			"$lt":  endPoint,
																		}
																	} else if strings.HasPrefix(originTime, "后") {
																		if strings.HasSuffix(originTime, "当前") {
																			fromNow = true
																			originTime = strings.ReplaceAll(originTime, "当前", "")
																		}
																		intervalNumberString := numberx.GetNumberExp(originTime)
																		intervalNumber, err := strconv.Atoi(intervalNumberString)
																		if err != nil {
																			logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																			continue
																		}
																		interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																		switch interval {
																		case "天":
																			if fromNow {
																				endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																				startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																			}
																		case "周":
																			if fromNow {
																				endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				weekDay := int(timeNow.Weekday())
																				if weekDay == 0 {
																					weekDay = 7
																				}
																				endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																				startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																			}
																		case "月":
																			if fromNow {
																				endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																				startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																			}
																		case "年":
																			if fromNow {
																				endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																				startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																			}
																		case "季度":
																			if fromNow {
																				endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				timeNowMonth := int(timeNow.Month())
																				quarter1 := timeNowMonth - 1
																				quarter2 := timeNowMonth - 4
																				quarter3 := timeNowMonth - 7
																				quarter4 := timeNowMonth - 10
																				quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																				min := quarter1
																				for _, v := range quarterList {
																					if v < min {
																						min = v
																					}
																				}
																				endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																				startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																			}
																		case "时":
																			if fromNow {
																				endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																				startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																			}
																		case "分":
																			if fromNow {
																				endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																				startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																			}
																		case "秒":
																			endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		}
																		timeMap = map[string]interface{}{
																			"$gte": startPoint,
																			"$lt":  endPoint,
																		}
																	} else if strings.HasPrefix(originTime, "当前") {
																		interval := strings.ReplaceAll(originTime, "当前", "")
																		switch interval {
																		case "天":
																			startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																		case "周":
																			weekDay := int(timeNow.Weekday())
																			if weekDay == 0 {
																				weekDay = 7
																			}
																			startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																		case "月":
																			startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																		case "年":
																			startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																		case "季度":
																			timeNowMonth := int(timeNow.Month())
																			quarter1 := timeNowMonth - 1
																			quarter2 := timeNowMonth - 4
																			quarter3 := timeNowMonth - 7
																			quarter4 := timeNowMonth - 10
																			quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																			min := quarter1
																			for _, v := range quarterList {
																				if v < min {
																					min = v
																				}
																			}
																			startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																		case "时":
																			startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																		case "分":
																			startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																		case "秒":
																			endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		}
																		timeMap = map[string]interface{}{
																			"$gte": startPoint,
																			"$lt":  endPoint,
																		}
																	} else {
																		eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																		if err != nil {
																			continue
																		}
																		queryTime = eleTime.Format("2006-01-02 15:04:05")
																	}
																}
															}
															if len(timeMap) != 0 {
																timeMapRaw[k] = timeMap
															} else {
																timeMapRaw[k] = queryTime
															}
														}
													}
												} else if originTime, ok := orVal.(string); ok {
													var queryTime string
													var startPoint string
													var endPoint string
													timeMap := map[string]interface{}{}
													switch originTime {
													case "今天":
														startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													case "昨天":
														startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													case "明天":
														startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													case "本周":
														week := int(time.Now().Weekday())
														if week == 0 {
															week = 7
														}
														startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													case "上周":
														week := int(time.Now().Weekday())
														if week == 0 {
															week = 7
														}
														startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													case "今年":
														startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													case "去年":
														startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													case "明年":
														startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													default:
														if originTime != "" {
															fromNow := false
															if strings.HasPrefix(originTime, "前") {
																//fmt.Println("originTime:", originTime)
																if strings.HasSuffix(originTime, "当前") {
																	fromNow = true
																	originTime = strings.ReplaceAll(originTime, "当前", "")
																	//fmt.Println("originTime after 当前:", originTime)
																}
																intervalNumberString := numberx.GetNumberExp(originTime)
																intervalNumber, err := strconv.Atoi(intervalNumberString)
																if err != nil {
																	logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																	continue
																}
																interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																//fmt.Println("intervalNumber:", intervalNumber)
																//fmt.Println("interval:", interval)
																switch interval {
																case "天":
																	if fromNow {
																		//fmt.Println("fromNow")
																		startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		endPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																	}
																case "周":
																	if fromNow {
																		startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		endPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		weekDay := int(timeNow.Weekday())
																		if weekDay == 0 {
																			weekDay = 7
																		}
																		startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																	}
																case "月":
																	if fromNow {
																		startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		endPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																	}
																case "年":
																	if fromNow {
																		startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		endPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																	}
																case "季度":
																	if fromNow {
																		startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		endPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		timeNowMonth := int(timeNow.Month())
																		quarter1 := timeNowMonth - 1
																		quarter2 := timeNowMonth - 4
																		quarter3 := timeNowMonth - 7
																		quarter4 := timeNowMonth - 10
																		quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																		min := quarter1
																		for _, v := range quarterList {
																			if v < min {
																				min = v
																			}
																		}
																		startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																	}
																case "时":
																	if fromNow {
																		startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		endPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																	}
																case "分":
																	if fromNow {
																		startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																		endPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																	}
																case "秒":
																	startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																	endPoint = timeNow.Format("2006-01-02 15:04:05")
																}
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															} else if strings.HasPrefix(originTime, "后") {
																if strings.HasSuffix(originTime, "当前") {
																	fromNow = true
																	originTime = strings.ReplaceAll(originTime, "当前", "")
																}
																intervalNumberString := numberx.GetNumberExp(originTime)
																intervalNumber, err := strconv.Atoi(intervalNumberString)
																if err != nil {
																	logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																	continue
																}
																interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																switch interval {
																case "天":
																	if fromNow {
																		endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		startPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																		startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																	}
																case "周":
																	if fromNow {
																		endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		startPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		weekDay := int(timeNow.Weekday())
																		if weekDay == 0 {
																			weekDay = 7
																		}
																		endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																		startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																	}
																case "月":
																	if fromNow {
																		endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		startPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																		startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																	}
																case "年":
																	if fromNow {
																		endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		startPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																		startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																	}
																case "季度":
																	if fromNow {
																		endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		startPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		timeNowMonth := int(timeNow.Month())
																		quarter1 := timeNowMonth - 1
																		quarter2 := timeNowMonth - 4
																		quarter3 := timeNowMonth - 7
																		quarter4 := timeNowMonth - 10
																		quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																		min := quarter1
																		for _, v := range quarterList {
																			if v < min {
																				min = v
																			}
																		}
																		endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																		startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																	}
																case "时":
																	if fromNow {
																		endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		startPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																		startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																	}
																case "分":
																	if fromNow {
																		endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																		startPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																		startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																	}
																case "秒":
																	endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																	startPoint = timeNow.Format("2006-01-02 15:04:05")
																}
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															} else if strings.HasPrefix(originTime, "当前") {
																interval := strings.ReplaceAll(originTime, "当前", "")
																switch interval {
																case "天":
																	startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																case "周":
																	weekDay := int(timeNow.Weekday())
																	if weekDay == 0 {
																		weekDay = 7
																	}
																	startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																case "月":
																	startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																case "年":
																	startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																case "季度":
																	timeNowMonth := int(timeNow.Month())
																	quarter1 := timeNowMonth - 1
																	quarter2 := timeNowMonth - 4
																	quarter3 := timeNowMonth - 7
																	quarter4 := timeNowMonth - 10
																	quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																	min := quarter1
																	for _, v := range quarterList {
																		if v < min {
																			min = v
																		}
																	}
																	startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																case "时":
																	startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																case "分":
																	startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																case "秒":
																	endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																	startPoint = timeNow.Format("2006-01-02 15:04:05")
																}
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															} else {
																eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																if err != nil {
																	continue
																}
																queryTime = eleTime.Format("2006-01-02 15:04:05")
															}
														}
													}
													if len(timeMap) != 0 {
														orEleMap[orKey] = timeMap
													} else {
														orEleMap[orKey] = queryTime
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		case "删除记录时":
			switch settings.SelectTyp {
			case "记录选择":
				deleteID, ok := data["id"].(string)
				if !ok {
					continue
				}
				canDelete := false
				for _, record := range settings.SelectRecord {
					if deleteID == record.ID {
						canDelete = true
						break
					}
				}
				if !canDelete {
					continue
				}
			case "范围定义":
				if filter, ok := settings.Query["filter"]; ok {
					if filterM, ok := filter.(map[string]interface{}); ok {
						for key, val := range filterM {
							if extRaw, ok := excelColNameTypeExtMap[key]; ok {
								//fmt.Println("excelColNameTypeExtMap[key] in:", key)
								if extRaw.Format != "" {
									if timeMapRaw, ok := val.(map[string]interface{}); ok {
										for k, v := range timeMapRaw {
											if originTime, ok := v.(string); ok {
												var queryTime string
												var startPoint string
												var endPoint string
												timeMap := map[string]interface{}{}
												switch originTime {
												case "今天":
													startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
													endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
													timeMap = map[string]interface{}{
														"$gte": startPoint,
														"$lt":  endPoint,
													}
												case "昨天":
													startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
													endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
													timeMap = map[string]interface{}{
														"$gte": startPoint,
														"$lt":  endPoint,
													}
												case "明天":
													startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
													endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
													timeMap = map[string]interface{}{
														"$gte": startPoint,
														"$lt":  endPoint,
													}
												case "本周":
													week := int(time.Now().Weekday())
													if week == 0 {
														week = 7
													}
													startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
													endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
													timeMap = map[string]interface{}{
														"$gte": startPoint,
														"$lt":  endPoint,
													}
												case "上周":
													week := int(time.Now().Weekday())
													if week == 0 {
														week = 7
													}
													startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
													endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
													timeMap = map[string]interface{}{
														"$gte": startPoint,
														"$lt":  endPoint,
													}
												case "今年":
													startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
													endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
													timeMap = map[string]interface{}{
														"$gte": startPoint,
														"$lt":  endPoint,
													}
												case "去年":
													startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
													endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
													timeMap = map[string]interface{}{
														"$gte": startPoint,
														"$lt":  endPoint,
													}
												case "明年":
													startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
													endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
													timeMap = map[string]interface{}{
														"$gte": startPoint,
														"$lt":  endPoint,
													}
												default:
													if originTime != "" {
														fromNow := false
														if strings.HasPrefix(originTime, "前") {
															//fmt.Println("originTime:", originTime)
															if strings.HasSuffix(originTime, "当前") {
																fromNow = true
																originTime = strings.ReplaceAll(originTime, "当前", "")
																//fmt.Println("originTime after 当前:", originTime)
															}
															intervalNumberString := numberx.GetNumberExp(originTime)
															intervalNumber, err := strconv.Atoi(intervalNumberString)
															if err != nil {
																logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																continue
															}
															interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
															//fmt.Println("intervalNumber:", intervalNumber)
															//fmt.Println("interval:", interval)
															switch interval {
															case "天":
																if fromNow {
																	//fmt.Println("fromNow")
																	startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																	endPoint = timeNow.Format("2006-01-02 15:04:05")
																} else {
																	startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																}
															case "周":
																if fromNow {
																	startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																	endPoint = timeNow.Format("2006-01-02 15:04:05")
																} else {
																	weekDay := int(timeNow.Weekday())
																	if weekDay == 0 {
																		weekDay = 7
																	}
																	startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																}
															case "月":
																if fromNow {
																	startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																	endPoint = timeNow.Format("2006-01-02 15:04:05")
																} else {
																	startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																}
															case "年":
																if fromNow {
																	startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																	endPoint = timeNow.Format("2006-01-02 15:04:05")
																} else {
																	startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																}
															case "季度":
																if fromNow {
																	startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																	endPoint = timeNow.Format("2006-01-02 15:04:05")
																} else {
																	timeNowMonth := int(timeNow.Month())
																	quarter1 := timeNowMonth - 1
																	quarter2 := timeNowMonth - 4
																	quarter3 := timeNowMonth - 7
																	quarter4 := timeNowMonth - 10
																	quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																	min := quarter1
																	for _, v := range quarterList {
																		if v < min {
																			min = v
																		}
																	}
																	startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																}
															case "时":
																if fromNow {
																	startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																	endPoint = timeNow.Format("2006-01-02 15:04:05")
																} else {
																	startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																}
															case "分":
																if fromNow {
																	startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																	endPoint = timeNow.Format("2006-01-02 15:04:05")
																} else {
																	startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																}
															case "秒":
																startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															}
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														} else if strings.HasPrefix(originTime, "后") {
															if strings.HasSuffix(originTime, "当前") {
																fromNow = true
																originTime = strings.ReplaceAll(originTime, "当前", "")
															}
															intervalNumberString := numberx.GetNumberExp(originTime)
															intervalNumber, err := strconv.Atoi(intervalNumberString)
															if err != nil {
																logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																continue
															}
															interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
															switch interval {
															case "天":
																if fromNow {
																	endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																	startPoint = timeNow.Format("2006-01-02 15:04:05")
																} else {
																	endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																	startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																}
															case "周":
																if fromNow {
																	endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																	startPoint = timeNow.Format("2006-01-02 15:04:05")
																} else {
																	weekDay := int(timeNow.Weekday())
																	if weekDay == 0 {
																		weekDay = 7
																	}
																	endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																	startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																}
															case "月":
																if fromNow {
																	endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																	startPoint = timeNow.Format("2006-01-02 15:04:05")
																} else {
																	endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																	startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																}
															case "年":
																if fromNow {
																	endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																	startPoint = timeNow.Format("2006-01-02 15:04:05")
																} else {
																	endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																	startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																}
															case "季度":
																if fromNow {
																	endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																	startPoint = timeNow.Format("2006-01-02 15:04:05")
																} else {
																	timeNowMonth := int(timeNow.Month())
																	quarter1 := timeNowMonth - 1
																	quarter2 := timeNowMonth - 4
																	quarter3 := timeNowMonth - 7
																	quarter4 := timeNowMonth - 10
																	quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																	min := quarter1
																	for _, v := range quarterList {
																		if v < min {
																			min = v
																		}
																	}
																	endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																	startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																}
															case "时":
																if fromNow {
																	endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																	startPoint = timeNow.Format("2006-01-02 15:04:05")
																} else {
																	endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																	startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																}
															case "分":
																if fromNow {
																	endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																	startPoint = timeNow.Format("2006-01-02 15:04:05")
																} else {
																	endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																	startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																}
															case "秒":
																endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															}
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														} else if strings.HasPrefix(originTime, "当前") {
															interval := strings.ReplaceAll(originTime, "当前", "")
															switch interval {
															case "天":
																startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
															case "周":
																weekDay := int(timeNow.Weekday())
																if weekDay == 0 {
																	weekDay = 7
																}
																startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
															case "月":
																startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
															case "年":
																startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
															case "季度":
																timeNowMonth := int(timeNow.Month())
																quarter1 := timeNowMonth - 1
																quarter2 := timeNowMonth - 4
																quarter3 := timeNowMonth - 7
																quarter4 := timeNowMonth - 10
																quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																min := quarter1
																for _, v := range quarterList {
																	if v < min {
																		min = v
																	}
																}
																startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
															case "时":
																startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
															case "分":
																startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
															case "秒":
																endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															}
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														} else {
															eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
															if err != nil {
																continue
															}
															queryTime = eleTime.Format("2006-01-02 15:04:05")
														}
													}
												}
												if len(timeMap) != 0 {
													timeMapRaw[k] = timeMap
												} else {
													timeMapRaw[k] = queryTime
												}
											}
										}
									} else if originTime, ok := val.(string); ok {
										var queryTime string
										var startPoint string
										var endPoint string
										timeMap := map[string]interface{}{}
										switch originTime {
										case "今天":
											startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										case "昨天":
											startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										case "明天":
											startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										case "本周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										case "上周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										case "今年":
											startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										case "去年":
											startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										case "明年":
											startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										default:
											if originTime != "" {
												fromNow := false
												if strings.HasPrefix(originTime, "前") {
													//fmt.Println("originTime:", originTime)
													if strings.HasSuffix(originTime, "当前") {
														fromNow = true
														originTime = strings.ReplaceAll(originTime, "当前", "")
														//fmt.Println("originTime after 当前:", originTime)
													}
													intervalNumberString := numberx.GetNumberExp(originTime)
													intervalNumber, err := strconv.Atoi(intervalNumberString)
													if err != nil {
														logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
														continue
													}
													interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
													//fmt.Println("intervalNumber:", intervalNumber)
													//fmt.Println("interval:", interval)
													switch interval {
													case "天":
														if fromNow {
															//fmt.Println("fromNow")
															startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
														}
													case "周":
														if fromNow {
															startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															weekDay := int(timeNow.Weekday())
															if weekDay == 0 {
																weekDay = 7
															}
															startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
														}
													case "月":
														if fromNow {
															startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
														}
													case "年":
														if fromNow {
															startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
														}
													case "季度":
														if fromNow {
															startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															timeNowMonth := int(timeNow.Month())
															quarter1 := timeNowMonth - 1
															quarter2 := timeNowMonth - 4
															quarter3 := timeNowMonth - 7
															quarter4 := timeNowMonth - 10
															quarterList := []int{quarter1, quarter2, quarter3, quarter4}
															min := quarter1
															for _, v := range quarterList {
																if v < min {
																	min = v
																}
															}
															startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
														}
													case "时":
														if fromNow {
															startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
															endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
														}
													case "分":
														if fromNow {
															startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
															endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
														}
													case "秒":
														startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													}
													timeMap = map[string]interface{}{
														"$gte": startPoint,
														"$lt":  endPoint,
													}
												} else if strings.HasPrefix(originTime, "后") {
													if strings.HasSuffix(originTime, "当前") {
														fromNow = true
														originTime = strings.ReplaceAll(originTime, "当前", "")
													}
													intervalNumberString := numberx.GetNumberExp(originTime)
													intervalNumber, err := strconv.Atoi(intervalNumberString)
													if err != nil {
														logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
														continue
													}
													interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
													switch interval {
													case "天":
														if fromNow {
															endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
															startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
														}
													case "周":
														if fromNow {
															endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															weekDay := int(timeNow.Weekday())
															if weekDay == 0 {
																weekDay = 7
															}
															endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
															startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
														}
													case "月":
														if fromNow {
															endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
															startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
														}
													case "年":
														if fromNow {
															endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
															startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
														}
													case "季度":
														if fromNow {
															endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															timeNowMonth := int(timeNow.Month())
															quarter1 := timeNowMonth - 1
															quarter2 := timeNowMonth - 4
															quarter3 := timeNowMonth - 7
															quarter4 := timeNowMonth - 10
															quarterList := []int{quarter1, quarter2, quarter3, quarter4}
															min := quarter1
															for _, v := range quarterList {
																if v < min {
																	min = v
																}
															}
															endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
															startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
														}
													case "时":
														if fromNow {
															endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
															startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
														}
													case "分":
														if fromNow {
															endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
															startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
														}
													case "秒":
														endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													}
													timeMap = map[string]interface{}{
														"$gte": startPoint,
														"$lt":  endPoint,
													}
												} else if strings.HasPrefix(originTime, "当前") {
													interval := strings.ReplaceAll(originTime, "当前", "")
													switch interval {
													case "天":
														startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
													case "周":
														weekDay := int(timeNow.Weekday())
														if weekDay == 0 {
															weekDay = 7
														}
														startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
														endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
													case "月":
														startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
													case "年":
														startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
													case "季度":
														timeNowMonth := int(timeNow.Month())
														quarter1 := timeNowMonth - 1
														quarter2 := timeNowMonth - 4
														quarter3 := timeNowMonth - 7
														quarter4 := timeNowMonth - 10
														quarterList := []int{quarter1, quarter2, quarter3, quarter4}
														min := quarter1
														for _, v := range quarterList {
															if v < min {
																min = v
															}
														}
														startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
														endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
													case "时":
														startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
													case "分":
														startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
													case "秒":
														endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													}
													timeMap = map[string]interface{}{
														"$gte": startPoint,
														"$lt":  endPoint,
													}
												} else {
													eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
													if err != nil {
														continue
													}
													queryTime = eleTime.Format("2006-01-02 15:04:05")
												}
											}
										}
										if len(timeMap) != 0 {
											filterM[key] = timeMap
										} else {
											filterM[key] = queryTime
										}
									}
								}
							} else if key == "createTime" || key == "modifyTime" {
								//fmt.Println("createTime[key] in:", key, "val:", val)
								if timeMapRaw, ok := val.(map[string]interface{}); ok {
									for k, v := range timeMapRaw {
										if originTime, ok := v.(string); ok {
											var queryTime string
											var startPoint string
											var endPoint string
											timeMap := map[string]interface{}{}
											switch originTime {
											case "今天":
												startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											case "昨天":
												startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											case "明天":
												startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											case "本周":
												week := int(time.Now().Weekday())
												if week == 0 {
													week = 7
												}
												startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											case "上周":
												week := int(time.Now().Weekday())
												if week == 0 {
													week = 7
												}
												startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											case "今年":
												startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											case "去年":
												startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											case "明年":
												startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											default:
												if originTime != "" {
													fromNow := false
													if strings.HasPrefix(originTime, "前") {
														//fmt.Println("originTime:", originTime)
														if strings.HasSuffix(originTime, "当前") {
															fromNow = true
															originTime = strings.ReplaceAll(originTime, "当前", "")
															//fmt.Println("originTime after 当前:", originTime)
														}
														intervalNumberString := numberx.GetNumberExp(originTime)
														intervalNumber, err := strconv.Atoi(intervalNumberString)
														if err != nil {
															logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
															continue
														}
														interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
														//fmt.Println("intervalNumber:", intervalNumber)
														//fmt.Println("interval:", interval)
														switch interval {
														case "天":
															if fromNow {
																//fmt.Println("fromNow")
																startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
															}
														case "周":
															if fromNow {
																startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																weekDay := int(timeNow.Weekday())
																if weekDay == 0 {
																	weekDay = 7
																}
																startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
															}
														case "月":
															if fromNow {
																startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
															}
														case "年":
															if fromNow {
																startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
															}
														case "季度":
															if fromNow {
																startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																timeNowMonth := int(timeNow.Month())
																quarter1 := timeNowMonth - 1
																quarter2 := timeNowMonth - 4
																quarter3 := timeNowMonth - 7
																quarter4 := timeNowMonth - 10
																quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																min := quarter1
																for _, v := range quarterList {
																	if v < min {
																		min = v
																	}
																}
																startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
															}
														case "时":
															if fromNow {
																startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
															}
														case "分":
															if fromNow {
																startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
															}
														case "秒":
															startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														}
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													} else if strings.HasPrefix(originTime, "后") {
														if strings.HasSuffix(originTime, "当前") {
															fromNow = true
															originTime = strings.ReplaceAll(originTime, "当前", "")
														}
														intervalNumberString := numberx.GetNumberExp(originTime)
														intervalNumber, err := strconv.Atoi(intervalNumberString)
														if err != nil {
															logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
															continue
														}
														interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
														switch interval {
														case "天":
															if fromNow {
																endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
															}
														case "周":
															if fromNow {
																endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																weekDay := int(timeNow.Weekday())
																if weekDay == 0 {
																	weekDay = 7
																}
																endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
															}
														case "月":
															if fromNow {
																endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
															}
														case "年":
															if fromNow {
																endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
															}
														case "季度":
															if fromNow {
																endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																timeNowMonth := int(timeNow.Month())
																quarter1 := timeNowMonth - 1
																quarter2 := timeNowMonth - 4
																quarter3 := timeNowMonth - 7
																quarter4 := timeNowMonth - 10
																quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																min := quarter1
																for _, v := range quarterList {
																	if v < min {
																		min = v
																	}
																}
																endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
															}
														case "时":
															if fromNow {
																endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
															}
														case "分":
															if fromNow {
																endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
															}
														case "秒":
															endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														}
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													} else if strings.HasPrefix(originTime, "当前") {
														interval := strings.ReplaceAll(originTime, "当前", "")
														switch interval {
														case "天":
															startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
														case "周":
															weekDay := int(timeNow.Weekday())
															if weekDay == 0 {
																weekDay = 7
															}
															startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
															endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
														case "月":
															startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
														case "年":
															startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
														case "季度":
															timeNowMonth := int(timeNow.Month())
															quarter1 := timeNowMonth - 1
															quarter2 := timeNowMonth - 4
															quarter3 := timeNowMonth - 7
															quarter4 := timeNowMonth - 10
															quarterList := []int{quarter1, quarter2, quarter3, quarter4}
															min := quarter1
															for _, v := range quarterList {
																if v < min {
																	min = v
																}
															}
															startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
															endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
														case "时":
															startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
														case "分":
															startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
														case "秒":
															endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														}
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													} else {
														eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
														if err != nil {
															continue
														}
														queryTime = eleTime.Format("2006-01-02 15:04:05")
													}
												}
											}
											if len(timeMap) != 0 {
												timeMapRaw[k] = timeMap
											} else {
												timeMapRaw[k] = queryTime
											}
										}
									}
								} else if originTime, ok := val.(string); ok {
									//fmt.Println("val.(string):", val.(string))
									var queryTime string
									var startPoint string
									var endPoint string
									timeMap := map[string]interface{}{}
									switch originTime {
									case "今天":
										startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									case "昨天":
										startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									case "明天":
										startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									case "本周":
										week := int(time.Now().Weekday())
										if week == 0 {
											week = 7
										}
										startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									case "上周":
										week := int(time.Now().Weekday())
										if week == 0 {
											week = 7
										}
										startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									case "今年":
										startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									case "去年":
										startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									case "明年":
										startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									default:
										if originTime != "" {
											fromNow := false
											if strings.HasPrefix(originTime, "前") {
												//fmt.Println("originTime:", originTime)
												if strings.HasSuffix(originTime, "当前") {
													fromNow = true
													originTime = strings.ReplaceAll(originTime, "当前", "")
													//fmt.Println("originTime after 当前:", originTime)
												}
												intervalNumberString := numberx.GetNumberExp(originTime)
												intervalNumber, err := strconv.Atoi(intervalNumberString)
												if err != nil {
													logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
													continue
												}
												interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
												//fmt.Println("intervalNumber:", intervalNumber)
												//fmt.Println("interval:", interval)
												switch interval {
												case "天":
													if fromNow {
														//fmt.Println("fromNow")
														startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
													}
												case "周":
													if fromNow {
														startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														weekDay := int(timeNow.Weekday())
														if weekDay == 0 {
															weekDay = 7
														}
														startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
													}
												case "月":
													if fromNow {
														startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
													}
												case "年":
													if fromNow {
														startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
													}
												case "季度":
													if fromNow {
														startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														timeNowMonth := int(timeNow.Month())
														quarter1 := timeNowMonth - 1
														quarter2 := timeNowMonth - 4
														quarter3 := timeNowMonth - 7
														quarter4 := timeNowMonth - 10
														quarterList := []int{quarter1, quarter2, quarter3, quarter4}
														min := quarter1
														for _, v := range quarterList {
															if v < min {
																min = v
															}
														}
														startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
													}
												case "时":
													if fromNow {
														startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
														endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
													}
												case "分":
													if fromNow {
														startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
														endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
													}
												case "秒":
													startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
													endPoint = timeNow.Format("2006-01-02 15:04:05")
												}
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											} else if strings.HasPrefix(originTime, "后") {
												if strings.HasSuffix(originTime, "当前") {
													fromNow = true
													originTime = strings.ReplaceAll(originTime, "当前", "")
												}
												intervalNumberString := numberx.GetNumberExp(originTime)
												intervalNumber, err := strconv.Atoi(intervalNumberString)
												if err != nil {
													logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
													continue
												}
												interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
												switch interval {
												case "天":
													if fromNow {
														endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
														startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
													}
												case "周":
													if fromNow {
														endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														weekDay := int(timeNow.Weekday())
														if weekDay == 0 {
															weekDay = 7
														}
														endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
														startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
													}
												case "月":
													if fromNow {
														endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
														startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
													}
												case "年":
													if fromNow {
														endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
														startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
													}
												case "季度":
													if fromNow {
														endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														timeNowMonth := int(timeNow.Month())
														quarter1 := timeNowMonth - 1
														quarter2 := timeNowMonth - 4
														quarter3 := timeNowMonth - 7
														quarter4 := timeNowMonth - 10
														quarterList := []int{quarter1, quarter2, quarter3, quarter4}
														min := quarter1
														for _, v := range quarterList {
															if v < min {
																min = v
															}
														}
														endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
														startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
													}
												case "时":
													if fromNow {
														endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
														startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
													}
												case "分":
													if fromNow {
														endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
														startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
													}
												case "秒":
													endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
													startPoint = timeNow.Format("2006-01-02 15:04:05")
												}
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											} else if strings.HasPrefix(originTime, "当前") {
												interval := strings.ReplaceAll(originTime, "当前", "")
												switch interval {
												case "天":
													startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
													endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
												case "周":
													weekDay := int(timeNow.Weekday())
													if weekDay == 0 {
														weekDay = 7
													}
													startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
													endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
												case "月":
													startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
													endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
												case "年":
													startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
													endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
												case "季度":
													timeNowMonth := int(timeNow.Month())
													quarter1 := timeNowMonth - 1
													quarter2 := timeNowMonth - 4
													quarter3 := timeNowMonth - 7
													quarter4 := timeNowMonth - 10
													quarterList := []int{quarter1, quarter2, quarter3, quarter4}
													min := quarter1
													for _, v := range quarterList {
														if v < min {
															min = v
														}
													}
													startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
													endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
												case "时":
													startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
													endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
												case "分":
													startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
													endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
												case "秒":
													endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
													startPoint = timeNow.Format("2006-01-02 15:04:05")
												}
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											} else {
												eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
												if err != nil {
													continue
												}
												queryTime = eleTime.Format("2006-01-02 15:04:05")
											}
										}
									}
									if len(timeMap) != 0 {
										filterM[key] = timeMap
									} else {
										filterM[key] = queryTime
									}
								}
							}
							if key == "$or" {
								if orList, ok := val.([]interface{}); ok {
									for _, orEle := range orList {
										if orEleMap, ok := orEle.(map[string]interface{}); ok {
											for orKey, orVal := range orEleMap {
												if orKey == "$and" {
													if orList, ok := orVal.([]interface{}); ok {
														for _, orEle := range orList {
															if orEleMap, ok := orEle.(map[string]interface{}); ok {
																for orKey, orVal := range orEleMap {
																	if extRaw, ok := excelColNameTypeExtMap[orKey]; ok {
																		if extRaw.Format != "" {
																			if timeMapRaw, ok := orVal.(map[string]interface{}); ok {
																				for k, v := range timeMapRaw {
																					if originTime, ok := v.(string); ok {
																						var queryTime string
																						var startPoint string
																						var endPoint string
																						timeMap := map[string]interface{}{}
																						switch originTime {
																						case "今天":
																							startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																							timeMap = map[string]interface{}{
																								"$gte": startPoint,
																								"$lt":  endPoint,
																							}
																						case "昨天":
																							startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																							timeMap = map[string]interface{}{
																								"$gte": startPoint,
																								"$lt":  endPoint,
																							}
																						case "明天":
																							startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
																							timeMap = map[string]interface{}{
																								"$gte": startPoint,
																								"$lt":  endPoint,
																							}
																						case "本周":
																							week := int(time.Now().Weekday())
																							if week == 0 {
																								week = 7
																							}
																							startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
																							timeMap = map[string]interface{}{
																								"$gte": startPoint,
																								"$lt":  endPoint,
																							}
																						case "上周":
																							week := int(time.Now().Weekday())
																							if week == 0 {
																								week = 7
																							}
																							startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																							timeMap = map[string]interface{}{
																								"$gte": startPoint,
																								"$lt":  endPoint,
																							}
																						case "今年":
																							startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																							timeMap = map[string]interface{}{
																								"$gte": startPoint,
																								"$lt":  endPoint,
																							}
																						case "去年":
																							startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																							timeMap = map[string]interface{}{
																								"$gte": startPoint,
																								"$lt":  endPoint,
																							}
																						case "明年":
																							startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
																							timeMap = map[string]interface{}{
																								"$gte": startPoint,
																								"$lt":  endPoint,
																							}
																						default:
																							if originTime != "" {
																								fromNow := false
																								if strings.HasPrefix(originTime, "前") {
																									//fmt.Println("originTime:", originTime)
																									if strings.HasSuffix(originTime, "当前") {
																										fromNow = true
																										originTime = strings.ReplaceAll(originTime, "当前", "")
																										//fmt.Println("originTime after 当前:", originTime)
																									}
																									intervalNumberString := numberx.GetNumberExp(originTime)
																									intervalNumber, err := strconv.Atoi(intervalNumberString)
																									if err != nil {
																										logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																										continue
																									}
																									interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																									//fmt.Println("intervalNumber:", intervalNumber)
																									//fmt.Println("interval:", interval)
																									switch interval {
																									case "天":
																										if fromNow {
																											//fmt.Println("fromNow")
																											startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																											endPoint = timeNow.Format("2006-01-02 15:04:05")
																										} else {
																											startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																											endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																										}
																									case "周":
																										if fromNow {
																											startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																											endPoint = timeNow.Format("2006-01-02 15:04:05")
																										} else {
																											weekDay := int(timeNow.Weekday())
																											if weekDay == 0 {
																												weekDay = 7
																											}
																											startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																											endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																										}
																									case "月":
																										if fromNow {
																											startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																											endPoint = timeNow.Format("2006-01-02 15:04:05")
																										} else {
																											startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																											endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																										}
																									case "年":
																										if fromNow {
																											startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																											endPoint = timeNow.Format("2006-01-02 15:04:05")
																										} else {
																											startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																											endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																										}
																									case "季度":
																										if fromNow {
																											startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																											endPoint = timeNow.Format("2006-01-02 15:04:05")
																										} else {
																											timeNowMonth := int(timeNow.Month())
																											quarter1 := timeNowMonth - 1
																											quarter2 := timeNowMonth - 4
																											quarter3 := timeNowMonth - 7
																											quarter4 := timeNowMonth - 10
																											quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																											min := quarter1
																											for _, v := range quarterList {
																												if v < min {
																													min = v
																												}
																											}
																											startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																											endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																										}
																									case "时":
																										if fromNow {
																											startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																											endPoint = timeNow.Format("2006-01-02 15:04:05")
																										} else {
																											startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																											endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																										}
																									case "分":
																										if fromNow {
																											startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																											endPoint = timeNow.Format("2006-01-02 15:04:05")
																										} else {
																											startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																											endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																										}
																									case "秒":
																										startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									}
																									timeMap = map[string]interface{}{
																										"$gte": startPoint,
																										"$lt":  endPoint,
																									}
																								} else if strings.HasPrefix(originTime, "后") {
																									if strings.HasSuffix(originTime, "当前") {
																										fromNow = true
																										originTime = strings.ReplaceAll(originTime, "当前", "")
																									}
																									intervalNumberString := numberx.GetNumberExp(originTime)
																									intervalNumber, err := strconv.Atoi(intervalNumberString)
																									if err != nil {
																										logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																										continue
																									}
																									interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																									switch interval {
																									case "天":
																										if fromNow {
																											endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																											startPoint = timeNow.Format("2006-01-02 15:04:05")
																										} else {
																											endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																											startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																										}
																									case "周":
																										if fromNow {
																											endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																											startPoint = timeNow.Format("2006-01-02 15:04:05")
																										} else {
																											weekDay := int(timeNow.Weekday())
																											if weekDay == 0 {
																												weekDay = 7
																											}
																											endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																											startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																										}
																									case "月":
																										if fromNow {
																											endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																											startPoint = timeNow.Format("2006-01-02 15:04:05")
																										} else {
																											endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																											startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																										}
																									case "年":
																										if fromNow {
																											endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																											startPoint = timeNow.Format("2006-01-02 15:04:05")
																										} else {
																											endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																											startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																										}
																									case "季度":
																										if fromNow {
																											endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																											startPoint = timeNow.Format("2006-01-02 15:04:05")
																										} else {
																											timeNowMonth := int(timeNow.Month())
																											quarter1 := timeNowMonth - 1
																											quarter2 := timeNowMonth - 4
																											quarter3 := timeNowMonth - 7
																											quarter4 := timeNowMonth - 10
																											quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																											min := quarter1
																											for _, v := range quarterList {
																												if v < min {
																													min = v
																												}
																											}
																											endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																											startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																										}
																									case "时":
																										if fromNow {
																											endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																											startPoint = timeNow.Format("2006-01-02 15:04:05")
																										} else {
																											endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																											startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																										}
																									case "分":
																										if fromNow {
																											endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																											startPoint = timeNow.Format("2006-01-02 15:04:05")
																										} else {
																											endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																											startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																										}
																									case "秒":
																										endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									}
																									timeMap = map[string]interface{}{
																										"$gte": startPoint,
																										"$lt":  endPoint,
																									}
																								} else if strings.HasPrefix(originTime, "当前") {
																									interval := strings.ReplaceAll(originTime, "当前", "")
																									switch interval {
																									case "天":
																										startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																									case "周":
																										weekDay := int(timeNow.Weekday())
																										if weekDay == 0 {
																											weekDay = 7
																										}
																										startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																									case "月":
																										startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																									case "年":
																										startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																									case "季度":
																										timeNowMonth := int(timeNow.Month())
																										quarter1 := timeNowMonth - 1
																										quarter2 := timeNowMonth - 4
																										quarter3 := timeNowMonth - 7
																										quarter4 := timeNowMonth - 10
																										quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																										min := quarter1
																										for _, v := range quarterList {
																											if v < min {
																												min = v
																											}
																										}
																										startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																									case "时":
																										startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																									case "分":
																										startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																									case "秒":
																										endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									}
																									timeMap = map[string]interface{}{
																										"$gte": startPoint,
																										"$lt":  endPoint,
																									}
																								} else {
																									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																									if err != nil {
																										continue
																									}
																									queryTime = eleTime.Format("2006-01-02 15:04:05")
																								}
																							}
																						}
																						if len(timeMap) != 0 {
																							timeMapRaw[k] = timeMap
																						} else {
																							timeMapRaw[k] = queryTime
																						}
																					}
																				}
																			} else if originTime, ok := orVal.(string); ok {
																				var queryTime string
																				var startPoint string
																				var endPoint string
																				timeMap := map[string]interface{}{}
																				switch originTime {
																				case "今天":
																					startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				case "昨天":
																					startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				case "明天":
																					startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				case "本周":
																					week := int(time.Now().Weekday())
																					if week == 0 {
																						week = 7
																					}
																					startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				case "上周":
																					week := int(time.Now().Weekday())
																					if week == 0 {
																						week = 7
																					}
																					startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				case "今年":
																					startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				case "去年":
																					startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				case "明年":
																					startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				default:
																					if originTime != "" {
																						fromNow := false
																						if strings.HasPrefix(originTime, "前") {
																							//fmt.Println("originTime:", originTime)
																							if strings.HasSuffix(originTime, "当前") {
																								fromNow = true
																								originTime = strings.ReplaceAll(originTime, "当前", "")
																								//fmt.Println("originTime after 当前:", originTime)
																							}
																							intervalNumberString := numberx.GetNumberExp(originTime)
																							intervalNumber, err := strconv.Atoi(intervalNumberString)
																							if err != nil {
																								logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																								continue
																							}
																							interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																							//fmt.Println("intervalNumber:", intervalNumber)
																							//fmt.Println("interval:", interval)
																							switch interval {
																							case "天":
																								if fromNow {
																									//fmt.Println("fromNow")
																									startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																								}
																							case "周":
																								if fromNow {
																									startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									weekDay := int(timeNow.Weekday())
																									if weekDay == 0 {
																										weekDay = 7
																									}
																									startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																								}
																							case "月":
																								if fromNow {
																									startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																								}
																							case "年":
																								if fromNow {
																									startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																								}
																							case "季度":
																								if fromNow {
																									startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									timeNowMonth := int(timeNow.Month())
																									quarter1 := timeNowMonth - 1
																									quarter2 := timeNowMonth - 4
																									quarter3 := timeNowMonth - 7
																									quarter4 := timeNowMonth - 10
																									quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																									min := quarter1
																									for _, v := range quarterList {
																										if v < min {
																											min = v
																										}
																									}
																									startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																								}
																							case "时":
																								if fromNow {
																									startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																								}
																							case "分":
																								if fromNow {
																									startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																								}
																							case "秒":
																								startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							}
																							timeMap = map[string]interface{}{
																								"$gte": startPoint,
																								"$lt":  endPoint,
																							}
																						} else if strings.HasPrefix(originTime, "后") {
																							if strings.HasSuffix(originTime, "当前") {
																								fromNow = true
																								originTime = strings.ReplaceAll(originTime, "当前", "")
																							}
																							intervalNumberString := numberx.GetNumberExp(originTime)
																							intervalNumber, err := strconv.Atoi(intervalNumberString)
																							if err != nil {
																								logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																								continue
																							}
																							interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																							switch interval {
																							case "天":
																								if fromNow {
																									endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																									startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																								}
																							case "周":
																								if fromNow {
																									endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									weekDay := int(timeNow.Weekday())
																									if weekDay == 0 {
																										weekDay = 7
																									}
																									endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																									startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																								}
																							case "月":
																								if fromNow {
																									endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																									startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																								}
																							case "年":
																								if fromNow {
																									endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																									startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																								}
																							case "季度":
																								if fromNow {
																									endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									timeNowMonth := int(timeNow.Month())
																									quarter1 := timeNowMonth - 1
																									quarter2 := timeNowMonth - 4
																									quarter3 := timeNowMonth - 7
																									quarter4 := timeNowMonth - 10
																									quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																									min := quarter1
																									for _, v := range quarterList {
																										if v < min {
																											min = v
																										}
																									}
																									endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																									startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																								}
																							case "时":
																								if fromNow {
																									endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																									startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																								}
																							case "分":
																								if fromNow {
																									endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																									startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																								}
																							case "秒":
																								endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							}
																							timeMap = map[string]interface{}{
																								"$gte": startPoint,
																								"$lt":  endPoint,
																							}
																						} else if strings.HasPrefix(originTime, "当前") {
																							interval := strings.ReplaceAll(originTime, "当前", "")
																							switch interval {
																							case "天":
																								startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																							case "周":
																								weekDay := int(timeNow.Weekday())
																								if weekDay == 0 {
																									weekDay = 7
																								}
																								startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																							case "月":
																								startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																							case "年":
																								startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																							case "季度":
																								timeNowMonth := int(timeNow.Month())
																								quarter1 := timeNowMonth - 1
																								quarter2 := timeNowMonth - 4
																								quarter3 := timeNowMonth - 7
																								quarter4 := timeNowMonth - 10
																								quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																								min := quarter1
																								for _, v := range quarterList {
																									if v < min {
																										min = v
																									}
																								}
																								startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																							case "时":
																								startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																							case "分":
																								startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																							case "秒":
																								endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							}
																							timeMap = map[string]interface{}{
																								"$gte": startPoint,
																								"$lt":  endPoint,
																							}
																						} else {
																							eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																							if err != nil {
																								continue
																							}
																							queryTime = eleTime.Format("2006-01-02 15:04:05")
																						}
																					}
																				}
																				if len(timeMap) != 0 {
																					orEleMap[orKey] = timeMap
																				} else {
																					orEleMap[orKey] = queryTime
																				}
																			}
																		}
																	} else if orKey == "createTime" || orKey == "modifyTime" {
																		if timeMapRaw, ok := orVal.(map[string]interface{}); ok {
																			for k, v := range timeMapRaw {
																				if originTime, ok := v.(string); ok {
																					var queryTime string
																					var startPoint string
																					var endPoint string
																					timeMap := map[string]interface{}{}
																					switch originTime {
																					case "今天":
																						startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					case "昨天":
																						startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					case "明天":
																						startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					case "本周":
																						week := int(time.Now().Weekday())
																						if week == 0 {
																							week = 7
																						}
																						startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					case "上周":
																						week := int(time.Now().Weekday())
																						if week == 0 {
																							week = 7
																						}
																						startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					case "今年":
																						startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					case "去年":
																						startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					case "明年":
																						startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					default:
																						if originTime != "" {
																							fromNow := false
																							if strings.HasPrefix(originTime, "前") {
																								//fmt.Println("originTime:", originTime)
																								if strings.HasSuffix(originTime, "当前") {
																									fromNow = true
																									originTime = strings.ReplaceAll(originTime, "当前", "")
																									//fmt.Println("originTime after 当前:", originTime)
																								}
																								intervalNumberString := numberx.GetNumberExp(originTime)
																								intervalNumber, err := strconv.Atoi(intervalNumberString)
																								if err != nil {
																									logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																									continue
																								}
																								interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																								//fmt.Println("intervalNumber:", intervalNumber)
																								//fmt.Println("interval:", interval)
																								switch interval {
																								case "天":
																									if fromNow {
																										//fmt.Println("fromNow")
																										startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																									}
																								case "周":
																									if fromNow {
																										startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										weekDay := int(timeNow.Weekday())
																										if weekDay == 0 {
																											weekDay = 7
																										}
																										startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																									}
																								case "月":
																									if fromNow {
																										startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																									}
																								case "年":
																									if fromNow {
																										startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																									}
																								case "季度":
																									if fromNow {
																										startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										timeNowMonth := int(timeNow.Month())
																										quarter1 := timeNowMonth - 1
																										quarter2 := timeNowMonth - 4
																										quarter3 := timeNowMonth - 7
																										quarter4 := timeNowMonth - 10
																										quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																										min := quarter1
																										for _, v := range quarterList {
																											if v < min {
																												min = v
																											}
																										}
																										startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																									}
																								case "时":
																									if fromNow {
																										startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																									}
																								case "分":
																									if fromNow {
																										startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																									}
																								case "秒":
																									startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								}
																								timeMap = map[string]interface{}{
																									"$gte": startPoint,
																									"$lt":  endPoint,
																								}
																							} else if strings.HasPrefix(originTime, "后") {
																								if strings.HasSuffix(originTime, "当前") {
																									fromNow = true
																									originTime = strings.ReplaceAll(originTime, "当前", "")
																								}
																								intervalNumberString := numberx.GetNumberExp(originTime)
																								intervalNumber, err := strconv.Atoi(intervalNumberString)
																								if err != nil {
																									logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																									continue
																								}
																								interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																								switch interval {
																								case "天":
																									if fromNow {
																										endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																										startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																									}
																								case "周":
																									if fromNow {
																										endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										weekDay := int(timeNow.Weekday())
																										if weekDay == 0 {
																											weekDay = 7
																										}
																										endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																										startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																									}
																								case "月":
																									if fromNow {
																										endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																										startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																									}
																								case "年":
																									if fromNow {
																										endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																										startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																									}
																								case "季度":
																									if fromNow {
																										endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										timeNowMonth := int(timeNow.Month())
																										quarter1 := timeNowMonth - 1
																										quarter2 := timeNowMonth - 4
																										quarter3 := timeNowMonth - 7
																										quarter4 := timeNowMonth - 10
																										quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																										min := quarter1
																										for _, v := range quarterList {
																											if v < min {
																												min = v
																											}
																										}
																										endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																										startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																									}
																								case "时":
																									if fromNow {
																										endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																										startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																									}
																								case "分":
																									if fromNow {
																										endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																										startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																									}
																								case "秒":
																									endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								}
																								timeMap = map[string]interface{}{
																									"$gte": startPoint,
																									"$lt":  endPoint,
																								}
																							} else if strings.HasPrefix(originTime, "当前") {
																								interval := strings.ReplaceAll(originTime, "当前", "")
																								switch interval {
																								case "天":
																									startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																								case "周":
																									weekDay := int(timeNow.Weekday())
																									if weekDay == 0 {
																										weekDay = 7
																									}
																									startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																								case "月":
																									startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																								case "年":
																									startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																								case "季度":
																									timeNowMonth := int(timeNow.Month())
																									quarter1 := timeNowMonth - 1
																									quarter2 := timeNowMonth - 4
																									quarter3 := timeNowMonth - 7
																									quarter4 := timeNowMonth - 10
																									quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																									min := quarter1
																									for _, v := range quarterList {
																										if v < min {
																											min = v
																										}
																									}
																									startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																								case "时":
																									startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																								case "分":
																									startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																								case "秒":
																									endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								}
																								timeMap = map[string]interface{}{
																									"$gte": startPoint,
																									"$lt":  endPoint,
																								}
																							} else {
																								eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																								if err != nil {
																									continue
																								}
																								queryTime = eleTime.Format("2006-01-02 15:04:05")
																							}
																						}
																					}
																					if len(timeMap) != 0 {
																						timeMapRaw[k] = timeMap
																					} else {
																						timeMapRaw[k] = queryTime
																					}
																				}
																			}
																		} else if originTime, ok := orVal.(string); ok {
																			var queryTime string
																			var startPoint string
																			var endPoint string
																			timeMap := map[string]interface{}{}
																			switch originTime {
																			case "今天":
																				startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			case "昨天":
																				startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			case "明天":
																				startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			case "本周":
																				week := int(time.Now().Weekday())
																				if week == 0 {
																					week = 7
																				}
																				startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			case "上周":
																				week := int(time.Now().Weekday())
																				if week == 0 {
																					week = 7
																				}
																				startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			case "今年":
																				startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			case "去年":
																				startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			case "明年":
																				startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			default:
																				if originTime != "" {
																					fromNow := false
																					if strings.HasPrefix(originTime, "前") {
																						//fmt.Println("originTime:", originTime)
																						if strings.HasSuffix(originTime, "当前") {
																							fromNow = true
																							originTime = strings.ReplaceAll(originTime, "当前", "")
																							//fmt.Println("originTime after 当前:", originTime)
																						}
																						intervalNumberString := numberx.GetNumberExp(originTime)
																						intervalNumber, err := strconv.Atoi(intervalNumberString)
																						if err != nil {
																							logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																							continue
																						}
																						interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																						//fmt.Println("intervalNumber:", intervalNumber)
																						//fmt.Println("interval:", interval)
																						switch interval {
																						case "天":
																							if fromNow {
																								//fmt.Println("fromNow")
																								startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																							}
																						case "周":
																							if fromNow {
																								startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								weekDay := int(timeNow.Weekday())
																								if weekDay == 0 {
																									weekDay = 7
																								}
																								startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																							}
																						case "月":
																							if fromNow {
																								startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																							}
																						case "年":
																							if fromNow {
																								startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																							}
																						case "季度":
																							if fromNow {
																								startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								timeNowMonth := int(timeNow.Month())
																								quarter1 := timeNowMonth - 1
																								quarter2 := timeNowMonth - 4
																								quarter3 := timeNowMonth - 7
																								quarter4 := timeNowMonth - 10
																								quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																								min := quarter1
																								for _, v := range quarterList {
																									if v < min {
																										min = v
																									}
																								}
																								startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																							}
																						case "时":
																							if fromNow {
																								startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																							}
																						case "分":
																							if fromNow {
																								startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																							}
																						case "秒":
																							startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																							endPoint = timeNow.Format("2006-01-02 15:04:05")
																						}
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					} else if strings.HasPrefix(originTime, "后") {
																						if strings.HasSuffix(originTime, "当前") {
																							fromNow = true
																							originTime = strings.ReplaceAll(originTime, "当前", "")
																						}
																						intervalNumberString := numberx.GetNumberExp(originTime)
																						intervalNumber, err := strconv.Atoi(intervalNumberString)
																						if err != nil {
																							logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																							continue
																						}
																						interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																						switch interval {
																						case "天":
																							if fromNow {
																								endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																								startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																							}
																						case "周":
																							if fromNow {
																								endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								weekDay := int(timeNow.Weekday())
																								if weekDay == 0 {
																									weekDay = 7
																								}
																								endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																								startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																							}
																						case "月":
																							if fromNow {
																								endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																								startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																							}
																						case "年":
																							if fromNow {
																								endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																								startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																							}
																						case "季度":
																							if fromNow {
																								endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								timeNowMonth := int(timeNow.Month())
																								quarter1 := timeNowMonth - 1
																								quarter2 := timeNowMonth - 4
																								quarter3 := timeNowMonth - 7
																								quarter4 := timeNowMonth - 10
																								quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																								min := quarter1
																								for _, v := range quarterList {
																									if v < min {
																										min = v
																									}
																								}
																								endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																								startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																							}
																						case "时":
																							if fromNow {
																								endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																								startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																							}
																						case "分":
																							if fromNow {
																								endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																								startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																							}
																						case "秒":
																							endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																							startPoint = timeNow.Format("2006-01-02 15:04:05")
																						}
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					} else if strings.HasPrefix(originTime, "当前") {
																						interval := strings.ReplaceAll(originTime, "当前", "")
																						switch interval {
																						case "天":
																							startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																						case "周":
																							weekDay := int(timeNow.Weekday())
																							if weekDay == 0 {
																								weekDay = 7
																							}
																							startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																						case "月":
																							startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																						case "年":
																							startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																						case "季度":
																							timeNowMonth := int(timeNow.Month())
																							quarter1 := timeNowMonth - 1
																							quarter2 := timeNowMonth - 4
																							quarter3 := timeNowMonth - 7
																							quarter4 := timeNowMonth - 10
																							quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																							min := quarter1
																							for _, v := range quarterList {
																								if v < min {
																									min = v
																								}
																							}
																							startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																						case "时":
																							startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																						case "分":
																							startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																						case "秒":
																							endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																							startPoint = timeNow.Format("2006-01-02 15:04:05")
																						}
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					} else {
																						eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																						if err != nil {
																							continue
																						}
																						queryTime = eleTime.Format("2006-01-02 15:04:05")
																					}
																				}
																			}
																			if len(timeMap) != 0 {
																				orEleMap[orKey] = timeMap
																			} else {
																				orEleMap[orKey] = queryTime
																			}
																		}
																	}
																}
															}
														}
													}
												} else if extRaw, ok := excelColNameTypeExtMap[orKey]; ok {
													if extRaw.Format != "" {
														if timeMapRaw, ok := orVal.(map[string]interface{}); ok {
															for k, v := range timeMapRaw {
																if originTime, ok := v.(string); ok {
																	var queryTime string
																	var startPoint string
																	var endPoint string
																	timeMap := map[string]interface{}{}
																	switch originTime {
																	case "今天":
																		startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																		timeMap = map[string]interface{}{
																			"$gte": startPoint,
																			"$lt":  endPoint,
																		}
																	case "昨天":
																		startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																		timeMap = map[string]interface{}{
																			"$gte": startPoint,
																			"$lt":  endPoint,
																		}
																	case "明天":
																		startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
																		timeMap = map[string]interface{}{
																			"$gte": startPoint,
																			"$lt":  endPoint,
																		}
																	case "本周":
																		week := int(time.Now().Weekday())
																		if week == 0 {
																			week = 7
																		}
																		startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
																		timeMap = map[string]interface{}{
																			"$gte": startPoint,
																			"$lt":  endPoint,
																		}
																	case "上周":
																		week := int(time.Now().Weekday())
																		if week == 0 {
																			week = 7
																		}
																		startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																		timeMap = map[string]interface{}{
																			"$gte": startPoint,
																			"$lt":  endPoint,
																		}
																	case "今年":
																		startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																		timeMap = map[string]interface{}{
																			"$gte": startPoint,
																			"$lt":  endPoint,
																		}
																	case "去年":
																		startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																		timeMap = map[string]interface{}{
																			"$gte": startPoint,
																			"$lt":  endPoint,
																		}
																	case "明年":
																		startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
																		timeMap = map[string]interface{}{
																			"$gte": startPoint,
																			"$lt":  endPoint,
																		}
																	default:
																		if originTime != "" {
																			fromNow := false
																			if strings.HasPrefix(originTime, "前") {
																				//fmt.Println("originTime:", originTime)
																				if strings.HasSuffix(originTime, "当前") {
																					fromNow = true
																					originTime = strings.ReplaceAll(originTime, "当前", "")
																					//fmt.Println("originTime after 当前:", originTime)
																				}
																				intervalNumberString := numberx.GetNumberExp(originTime)
																				intervalNumber, err := strconv.Atoi(intervalNumberString)
																				if err != nil {
																					logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																					continue
																				}
																				interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																				//fmt.Println("intervalNumber:", intervalNumber)
																				//fmt.Println("interval:", interval)
																				switch interval {
																				case "天":
																					if fromNow {
																						//fmt.Println("fromNow")
																						startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																						endPoint = timeNow.Format("2006-01-02 15:04:05")
																					} else {
																						startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																					}
																				case "周":
																					if fromNow {
																						startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																						endPoint = timeNow.Format("2006-01-02 15:04:05")
																					} else {
																						weekDay := int(timeNow.Weekday())
																						if weekDay == 0 {
																							weekDay = 7
																						}
																						startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																					}
																				case "月":
																					if fromNow {
																						startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																						endPoint = timeNow.Format("2006-01-02 15:04:05")
																					} else {
																						startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																					}
																				case "年":
																					if fromNow {
																						startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																						endPoint = timeNow.Format("2006-01-02 15:04:05")
																					} else {
																						startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																					}
																				case "季度":
																					if fromNow {
																						startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																						endPoint = timeNow.Format("2006-01-02 15:04:05")
																					} else {
																						timeNowMonth := int(timeNow.Month())
																						quarter1 := timeNowMonth - 1
																						quarter2 := timeNowMonth - 4
																						quarter3 := timeNowMonth - 7
																						quarter4 := timeNowMonth - 10
																						quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																						min := quarter1
																						for _, v := range quarterList {
																							if v < min {
																								min = v
																							}
																						}
																						startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																					}
																				case "时":
																					if fromNow {
																						startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																						endPoint = timeNow.Format("2006-01-02 15:04:05")
																					} else {
																						startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																					}
																				case "分":
																					if fromNow {
																						startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																						endPoint = timeNow.Format("2006-01-02 15:04:05")
																					} else {
																						startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																					}
																				case "秒":
																					startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				}
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			} else if strings.HasPrefix(originTime, "后") {
																				if strings.HasSuffix(originTime, "当前") {
																					fromNow = true
																					originTime = strings.ReplaceAll(originTime, "当前", "")
																				}
																				intervalNumberString := numberx.GetNumberExp(originTime)
																				intervalNumber, err := strconv.Atoi(intervalNumberString)
																				if err != nil {
																					logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																					continue
																				}
																				interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																				switch interval {
																				case "天":
																					if fromNow {
																						endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																						startPoint = timeNow.Format("2006-01-02 15:04:05")
																					} else {
																						endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																						startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																					}
																				case "周":
																					if fromNow {
																						endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																						startPoint = timeNow.Format("2006-01-02 15:04:05")
																					} else {
																						weekDay := int(timeNow.Weekday())
																						if weekDay == 0 {
																							weekDay = 7
																						}
																						endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																						startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																					}
																				case "月":
																					if fromNow {
																						endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																						startPoint = timeNow.Format("2006-01-02 15:04:05")
																					} else {
																						endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																						startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																					}
																				case "年":
																					if fromNow {
																						endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																						startPoint = timeNow.Format("2006-01-02 15:04:05")
																					} else {
																						endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																						startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																					}
																				case "季度":
																					if fromNow {
																						endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																						startPoint = timeNow.Format("2006-01-02 15:04:05")
																					} else {
																						timeNowMonth := int(timeNow.Month())
																						quarter1 := timeNowMonth - 1
																						quarter2 := timeNowMonth - 4
																						quarter3 := timeNowMonth - 7
																						quarter4 := timeNowMonth - 10
																						quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																						min := quarter1
																						for _, v := range quarterList {
																							if v < min {
																								min = v
																							}
																						}
																						endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																						startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																					}
																				case "时":
																					if fromNow {
																						endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																						startPoint = timeNow.Format("2006-01-02 15:04:05")
																					} else {
																						endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																						startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																					}
																				case "分":
																					if fromNow {
																						endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																						startPoint = timeNow.Format("2006-01-02 15:04:05")
																					} else {
																						endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																						startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																					}
																				case "秒":
																					endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				}
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			} else if strings.HasPrefix(originTime, "当前") {
																				interval := strings.ReplaceAll(originTime, "当前", "")
																				switch interval {
																				case "天":
																					startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																				case "周":
																					weekDay := int(timeNow.Weekday())
																					if weekDay == 0 {
																						weekDay = 7
																					}
																					startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																				case "月":
																					startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																				case "年":
																					startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																				case "季度":
																					timeNowMonth := int(timeNow.Month())
																					quarter1 := timeNowMonth - 1
																					quarter2 := timeNowMonth - 4
																					quarter3 := timeNowMonth - 7
																					quarter4 := timeNowMonth - 10
																					quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																					min := quarter1
																					for _, v := range quarterList {
																						if v < min {
																							min = v
																						}
																					}
																					startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																				case "时":
																					startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																				case "分":
																					startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																				case "秒":
																					endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				}
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			} else {
																				eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																				if err != nil {
																					continue
																				}
																				queryTime = eleTime.Format("2006-01-02 15:04:05")
																			}
																		}
																	}
																	if len(timeMap) != 0 {
																		timeMapRaw[k] = timeMap
																	} else {
																		timeMapRaw[k] = queryTime
																	}
																}
															}
														} else if originTime, ok := orVal.(string); ok {
															var queryTime string
															var startPoint string
															var endPoint string
															timeMap := map[string]interface{}{}
															switch originTime {
															case "今天":
																startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															case "昨天":
																startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															case "明天":
																startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															case "本周":
																week := int(time.Now().Weekday())
																if week == 0 {
																	week = 7
																}
																startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															case "上周":
																week := int(time.Now().Weekday())
																if week == 0 {
																	week = 7
																}
																startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															case "今年":
																startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															case "去年":
																startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															case "明年":
																startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															default:
																if originTime != "" {
																	fromNow := false
																	if strings.HasPrefix(originTime, "前") {
																		//fmt.Println("originTime:", originTime)
																		if strings.HasSuffix(originTime, "当前") {
																			fromNow = true
																			originTime = strings.ReplaceAll(originTime, "当前", "")
																			//fmt.Println("originTime after 当前:", originTime)
																		}
																		intervalNumberString := numberx.GetNumberExp(originTime)
																		intervalNumber, err := strconv.Atoi(intervalNumberString)
																		if err != nil {
																			logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																			continue
																		}
																		interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																		//fmt.Println("intervalNumber:", intervalNumber)
																		//fmt.Println("interval:", interval)
																		switch interval {
																		case "天":
																			if fromNow {
																				//fmt.Println("fromNow")
																				startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																			}
																		case "周":
																			if fromNow {
																				startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				weekDay := int(timeNow.Weekday())
																				if weekDay == 0 {
																					weekDay = 7
																				}
																				startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																			}
																		case "月":
																			if fromNow {
																				startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																			}
																		case "年":
																			if fromNow {
																				startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																			}
																		case "季度":
																			if fromNow {
																				startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				timeNowMonth := int(timeNow.Month())
																				quarter1 := timeNowMonth - 1
																				quarter2 := timeNowMonth - 4
																				quarter3 := timeNowMonth - 7
																				quarter4 := timeNowMonth - 10
																				quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																				min := quarter1
																				for _, v := range quarterList {
																					if v < min {
																						min = v
																					}
																				}
																				startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																			}
																		case "时":
																			if fromNow {
																				startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																			}
																		case "分":
																			if fromNow {
																				startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																			}
																		case "秒":
																			startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		}
																		timeMap = map[string]interface{}{
																			"$gte": startPoint,
																			"$lt":  endPoint,
																		}
																	} else if strings.HasPrefix(originTime, "后") {
																		if strings.HasSuffix(originTime, "当前") {
																			fromNow = true
																			originTime = strings.ReplaceAll(originTime, "当前", "")
																		}
																		intervalNumberString := numberx.GetNumberExp(originTime)
																		intervalNumber, err := strconv.Atoi(intervalNumberString)
																		if err != nil {
																			logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																			continue
																		}
																		interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																		switch interval {
																		case "天":
																			if fromNow {
																				endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																				startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																			}
																		case "周":
																			if fromNow {
																				endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				weekDay := int(timeNow.Weekday())
																				if weekDay == 0 {
																					weekDay = 7
																				}
																				endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																				startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																			}
																		case "月":
																			if fromNow {
																				endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																				startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																			}
																		case "年":
																			if fromNow {
																				endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																				startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																			}
																		case "季度":
																			if fromNow {
																				endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				timeNowMonth := int(timeNow.Month())
																				quarter1 := timeNowMonth - 1
																				quarter2 := timeNowMonth - 4
																				quarter3 := timeNowMonth - 7
																				quarter4 := timeNowMonth - 10
																				quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																				min := quarter1
																				for _, v := range quarterList {
																					if v < min {
																						min = v
																					}
																				}
																				endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																				startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																			}
																		case "时":
																			if fromNow {
																				endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																				startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																			}
																		case "分":
																			if fromNow {
																				endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																				startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																			}
																		case "秒":
																			endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		}
																		timeMap = map[string]interface{}{
																			"$gte": startPoint,
																			"$lt":  endPoint,
																		}
																	} else if strings.HasPrefix(originTime, "当前") {
																		interval := strings.ReplaceAll(originTime, "当前", "")
																		switch interval {
																		case "天":
																			startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																		case "周":
																			weekDay := int(timeNow.Weekday())
																			if weekDay == 0 {
																				weekDay = 7
																			}
																			startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																		case "月":
																			startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																		case "年":
																			startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																		case "季度":
																			timeNowMonth := int(timeNow.Month())
																			quarter1 := timeNowMonth - 1
																			quarter2 := timeNowMonth - 4
																			quarter3 := timeNowMonth - 7
																			quarter4 := timeNowMonth - 10
																			quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																			min := quarter1
																			for _, v := range quarterList {
																				if v < min {
																					min = v
																				}
																			}
																			startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																		case "时":
																			startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																		case "分":
																			startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																		case "秒":
																			endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		}
																		timeMap = map[string]interface{}{
																			"$gte": startPoint,
																			"$lt":  endPoint,
																		}
																	} else {
																		eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																		if err != nil {
																			continue
																		}
																		queryTime = eleTime.Format("2006-01-02 15:04:05")
																	}
																}
															}
															if len(timeMap) != 0 {
																orEleMap[orKey] = timeMap
															} else {
																orEleMap[orKey] = queryTime
															}
														}
													}
												} else if orKey == "createTime" || orKey == "modifyTime" {
													if timeMapRaw, ok := orVal.(map[string]interface{}); ok {
														for k, v := range timeMapRaw {
															if originTime, ok := v.(string); ok {
																var queryTime string
																var startPoint string
																var endPoint string
																timeMap := map[string]interface{}{}
																switch originTime {
																case "今天":
																	startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																case "昨天":
																	startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																case "明天":
																	startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																case "本周":
																	week := int(time.Now().Weekday())
																	if week == 0 {
																		week = 7
																	}
																	startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																case "上周":
																	week := int(time.Now().Weekday())
																	if week == 0 {
																		week = 7
																	}
																	startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																case "今年":
																	startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																case "去年":
																	startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																case "明年":
																	startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																default:
																	if originTime != "" {
																		fromNow := false
																		if strings.HasPrefix(originTime, "前") {
																			//fmt.Println("originTime:", originTime)
																			if strings.HasSuffix(originTime, "当前") {
																				fromNow = true
																				originTime = strings.ReplaceAll(originTime, "当前", "")
																				//fmt.Println("originTime after 当前:", originTime)
																			}
																			intervalNumberString := numberx.GetNumberExp(originTime)
																			intervalNumber, err := strconv.Atoi(intervalNumberString)
																			if err != nil {
																				logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																				continue
																			}
																			interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																			//fmt.Println("intervalNumber:", intervalNumber)
																			//fmt.Println("interval:", interval)
																			switch interval {
																			case "天":
																				if fromNow {
																					//fmt.Println("fromNow")
																					startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																				}
																			case "周":
																				if fromNow {
																					startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					weekDay := int(timeNow.Weekday())
																					if weekDay == 0 {
																						weekDay = 7
																					}
																					startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																				}
																			case "月":
																				if fromNow {
																					startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																				}
																			case "年":
																				if fromNow {
																					startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																				}
																			case "季度":
																				if fromNow {
																					startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					timeNowMonth := int(timeNow.Month())
																					quarter1 := timeNowMonth - 1
																					quarter2 := timeNowMonth - 4
																					quarter3 := timeNowMonth - 7
																					quarter4 := timeNowMonth - 10
																					quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																					min := quarter1
																					for _, v := range quarterList {
																						if v < min {
																							min = v
																						}
																					}
																					startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																				}
																			case "时":
																				if fromNow {
																					startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																				}
																			case "分":
																				if fromNow {
																					startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																				}
																			case "秒":
																				startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			}
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		} else if strings.HasPrefix(originTime, "后") {
																			if strings.HasSuffix(originTime, "当前") {
																				fromNow = true
																				originTime = strings.ReplaceAll(originTime, "当前", "")
																			}
																			intervalNumberString := numberx.GetNumberExp(originTime)
																			intervalNumber, err := strconv.Atoi(intervalNumberString)
																			if err != nil {
																				logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																				continue
																			}
																			interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																			switch interval {
																			case "天":
																				if fromNow {
																					endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																					startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																				}
																			case "周":
																				if fromNow {
																					endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					weekDay := int(timeNow.Weekday())
																					if weekDay == 0 {
																						weekDay = 7
																					}
																					endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																					startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																				}
																			case "月":
																				if fromNow {
																					endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																					startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																				}
																			case "年":
																				if fromNow {
																					endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																					startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																				}
																			case "季度":
																				if fromNow {
																					endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					timeNowMonth := int(timeNow.Month())
																					quarter1 := timeNowMonth - 1
																					quarter2 := timeNowMonth - 4
																					quarter3 := timeNowMonth - 7
																					quarter4 := timeNowMonth - 10
																					quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																					min := quarter1
																					for _, v := range quarterList {
																						if v < min {
																							min = v
																						}
																					}
																					endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																					startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																				}
																			case "时":
																				if fromNow {
																					endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																					startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																				}
																			case "分":
																				if fromNow {
																					endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																					startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																				}
																			case "秒":
																				endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			}
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		} else if strings.HasPrefix(originTime, "当前") {
																			interval := strings.ReplaceAll(originTime, "当前", "")
																			switch interval {
																			case "天":
																				startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																			case "周":
																				weekDay := int(timeNow.Weekday())
																				if weekDay == 0 {
																					weekDay = 7
																				}
																				startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																			case "月":
																				startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																			case "年":
																				startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																			case "季度":
																				timeNowMonth := int(timeNow.Month())
																				quarter1 := timeNowMonth - 1
																				quarter2 := timeNowMonth - 4
																				quarter3 := timeNowMonth - 7
																				quarter4 := timeNowMonth - 10
																				quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																				min := quarter1
																				for _, v := range quarterList {
																					if v < min {
																						min = v
																					}
																				}
																				startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																			case "时":
																				startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																			case "分":
																				startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																			case "秒":
																				endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			}
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		} else {
																			eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																			if err != nil {
																				continue
																			}
																			queryTime = eleTime.Format("2006-01-02 15:04:05")
																		}
																	}
																}
																if len(timeMap) != 0 {
																	timeMapRaw[k] = timeMap
																} else {
																	timeMapRaw[k] = queryTime
																}
															}
														}
													} else if originTime, ok := orVal.(string); ok {
														var queryTime string
														var startPoint string
														var endPoint string
														timeMap := map[string]interface{}{}
														switch originTime {
														case "今天":
															startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														case "昨天":
															startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														case "明天":
															startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														case "本周":
															week := int(time.Now().Weekday())
															if week == 0 {
																week = 7
															}
															startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														case "上周":
															week := int(time.Now().Weekday())
															if week == 0 {
																week = 7
															}
															startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														case "今年":
															startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														case "去年":
															startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														case "明年":
															startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														default:
															if originTime != "" {
																fromNow := false
																if strings.HasPrefix(originTime, "前") {
																	//fmt.Println("originTime:", originTime)
																	if strings.HasSuffix(originTime, "当前") {
																		fromNow = true
																		originTime = strings.ReplaceAll(originTime, "当前", "")
																		//fmt.Println("originTime after 当前:", originTime)
																	}
																	intervalNumberString := numberx.GetNumberExp(originTime)
																	intervalNumber, err := strconv.Atoi(intervalNumberString)
																	if err != nil {
																		logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																		continue
																	}
																	interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																	//fmt.Println("intervalNumber:", intervalNumber)
																	//fmt.Println("interval:", interval)
																	switch interval {
																	case "天":
																		if fromNow {
																			//fmt.Println("fromNow")
																			startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																		}
																	case "周":
																		if fromNow {
																			startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			weekDay := int(timeNow.Weekday())
																			if weekDay == 0 {
																				weekDay = 7
																			}
																			startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																		}
																	case "月":
																		if fromNow {
																			startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																		}
																	case "年":
																		if fromNow {
																			startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																		}
																	case "季度":
																		if fromNow {
																			startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			timeNowMonth := int(timeNow.Month())
																			quarter1 := timeNowMonth - 1
																			quarter2 := timeNowMonth - 4
																			quarter3 := timeNowMonth - 7
																			quarter4 := timeNowMonth - 10
																			quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																			min := quarter1
																			for _, v := range quarterList {
																				if v < min {
																					min = v
																				}
																			}
																			startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																		}
																	case "时":
																		if fromNow {
																			startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																		}
																	case "分":
																		if fromNow {
																			startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																		}
																	case "秒":
																		startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																		endPoint = timeNow.Format("2006-01-02 15:04:05")
																	}
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																} else if strings.HasPrefix(originTime, "后") {
																	if strings.HasSuffix(originTime, "当前") {
																		fromNow = true
																		originTime = strings.ReplaceAll(originTime, "当前", "")
																	}
																	intervalNumberString := numberx.GetNumberExp(originTime)
																	intervalNumber, err := strconv.Atoi(intervalNumberString)
																	if err != nil {
																		logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																		continue
																	}
																	interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																	switch interval {
																	case "天":
																		if fromNow {
																			endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																			startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																		}
																	case "周":
																		if fromNow {
																			endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			weekDay := int(timeNow.Weekday())
																			if weekDay == 0 {
																				weekDay = 7
																			}
																			endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																			startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																		}
																	case "月":
																		if fromNow {
																			endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																			startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																		}
																	case "年":
																		if fromNow {
																			endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																			startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																		}
																	case "季度":
																		if fromNow {
																			endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			timeNowMonth := int(timeNow.Month())
																			quarter1 := timeNowMonth - 1
																			quarter2 := timeNowMonth - 4
																			quarter3 := timeNowMonth - 7
																			quarter4 := timeNowMonth - 10
																			quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																			min := quarter1
																			for _, v := range quarterList {
																				if v < min {
																					min = v
																				}
																			}
																			endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																			startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																		}
																	case "时":
																		if fromNow {
																			endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																			startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																		}
																	case "分":
																		if fromNow {
																			endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																			startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																		}
																	case "秒":
																		endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																		startPoint = timeNow.Format("2006-01-02 15:04:05")
																	}
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																} else if strings.HasPrefix(originTime, "当前") {
																	interval := strings.ReplaceAll(originTime, "当前", "")
																	switch interval {
																	case "天":
																		startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																	case "周":
																		weekDay := int(timeNow.Weekday())
																		if weekDay == 0 {
																			weekDay = 7
																		}
																		startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																	case "月":
																		startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																	case "年":
																		startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																	case "季度":
																		timeNowMonth := int(timeNow.Month())
																		quarter1 := timeNowMonth - 1
																		quarter2 := timeNowMonth - 4
																		quarter3 := timeNowMonth - 7
																		quarter4 := timeNowMonth - 10
																		quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																		min := quarter1
																		for _, v := range quarterList {
																			if v < min {
																				min = v
																			}
																		}
																		startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																	case "时":
																		startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																	case "分":
																		startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																	case "秒":
																		endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																		startPoint = timeNow.Format("2006-01-02 15:04:05")
																	}
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																} else {
																	eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																	if err != nil {
																		continue
																	}
																	queryTime = eleTime.Format("2006-01-02 15:04:05")
																}
															}
														}
														if len(timeMap) != 0 {
															orEleMap[orKey] = timeMap
														} else {
															orEleMap[orKey] = queryTime
														}
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		case "更新记录时", "添加或更新记录时":
			hasValidField := false
			if operateType == "add" {
				hasValidField = true
			} else {
				for _, ele := range settings.Field {
					if val, ok := data[ele]; ok {
						if !reflect.DeepEqual(val, oldInfo[ele]) {
							hasValidField = true
							break
						}
					}
				}
			}
			////fmt.Println("hasValidField:",hasValidField)
			if !hasValidField {
				continue
			}
			if filter, ok := settings.Query["filter"]; ok {
				if filterM, ok := filter.(map[string]interface{}); ok {
					for key, val := range filterM {
						if extRaw, ok := excelColNameTypeExtMap[key]; ok {
							//fmt.Println("excelColNameTypeExtMap[key] in:", key)
							if extRaw.Format != "" {
								if timeMapRaw, ok := val.(map[string]interface{}); ok {
									for k, v := range timeMapRaw {
										if originTime, ok := v.(string); ok {
											var queryTime string
											var startPoint string
											var endPoint string
											timeMap := map[string]interface{}{}
											switch originTime {
											case "今天":
												startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											case "昨天":
												startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											case "明天":
												startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											case "本周":
												week := int(time.Now().Weekday())
												if week == 0 {
													week = 7
												}
												startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											case "上周":
												week := int(time.Now().Weekday())
												if week == 0 {
													week = 7
												}
												startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											case "今年":
												startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											case "去年":
												startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											case "明年":
												startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											default:
												if originTime != "" {
													fromNow := false
													if strings.HasPrefix(originTime, "前") {
														//fmt.Println("originTime:", originTime)
														if strings.HasSuffix(originTime, "当前") {
															fromNow = true
															originTime = strings.ReplaceAll(originTime, "当前", "")
															//fmt.Println("originTime after 当前:", originTime)
														}
														intervalNumberString := numberx.GetNumberExp(originTime)
														intervalNumber, err := strconv.Atoi(intervalNumberString)
														if err != nil {
															logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
															continue
														}
														interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
														//fmt.Println("intervalNumber:", intervalNumber)
														//fmt.Println("interval:", interval)
														switch interval {
														case "天":
															if fromNow {
																//fmt.Println("fromNow")
																startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
															}
														case "周":
															if fromNow {
																startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																weekDay := int(timeNow.Weekday())
																if weekDay == 0 {
																	weekDay = 7
																}
																startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
															}
														case "月":
															if fromNow {
																startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
															}
														case "年":
															if fromNow {
																startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
															}
														case "季度":
															if fromNow {
																startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																timeNowMonth := int(timeNow.Month())
																quarter1 := timeNowMonth - 1
																quarter2 := timeNowMonth - 4
																quarter3 := timeNowMonth - 7
																quarter4 := timeNowMonth - 10
																quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																min := quarter1
																for _, v := range quarterList {
																	if v < min {
																		min = v
																	}
																}
																startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
															}
														case "时":
															if fromNow {
																startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
															}
														case "分":
															if fromNow {
																startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																endPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
															}
														case "秒":
															startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														}
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													} else if strings.HasPrefix(originTime, "后") {
														if strings.HasSuffix(originTime, "当前") {
															fromNow = true
															originTime = strings.ReplaceAll(originTime, "当前", "")
														}
														intervalNumberString := numberx.GetNumberExp(originTime)
														intervalNumber, err := strconv.Atoi(intervalNumberString)
														if err != nil {
															logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
															continue
														}
														interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
														switch interval {
														case "天":
															if fromNow {
																endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
															}
														case "周":
															if fromNow {
																endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																weekDay := int(timeNow.Weekday())
																if weekDay == 0 {
																	weekDay = 7
																}
																endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
															}
														case "月":
															if fromNow {
																endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
															}
														case "年":
															if fromNow {
																endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
															}
														case "季度":
															if fromNow {
																endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																timeNowMonth := int(timeNow.Month())
																quarter1 := timeNowMonth - 1
																quarter2 := timeNowMonth - 4
																quarter3 := timeNowMonth - 7
																quarter4 := timeNowMonth - 10
																quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																min := quarter1
																for _, v := range quarterList {
																	if v < min {
																		min = v
																	}
																}
																endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
															}
														case "时":
															if fromNow {
																endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
															}
														case "分":
															if fromNow {
																endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																startPoint = timeNow.Format("2006-01-02 15:04:05")
															} else {
																endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
															}
														case "秒":
															endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														}
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													} else if strings.HasPrefix(originTime, "当前") {
														interval := strings.ReplaceAll(originTime, "当前", "")
														switch interval {
														case "天":
															startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
														case "周":
															weekDay := int(timeNow.Weekday())
															if weekDay == 0 {
																weekDay = 7
															}
															startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
															endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
														case "月":
															startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
														case "年":
															startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
														case "季度":
															timeNowMonth := int(timeNow.Month())
															quarter1 := timeNowMonth - 1
															quarter2 := timeNowMonth - 4
															quarter3 := timeNowMonth - 7
															quarter4 := timeNowMonth - 10
															quarterList := []int{quarter1, quarter2, quarter3, quarter4}
															min := quarter1
															for _, v := range quarterList {
																if v < min {
																	min = v
																}
															}
															startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
															endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
														case "时":
															startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
														case "分":
															startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
														case "秒":
															endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														}
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													} else {
														eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
														if err != nil {
															continue
														}
														queryTime = eleTime.Format("2006-01-02 15:04:05")
													}
												}
											}
											if len(timeMap) != 0 {
												timeMapRaw[k] = timeMap
											} else {
												timeMapRaw[k] = queryTime
											}
										}
									}
								} else if originTime, ok := val.(string); ok {
									var queryTime string
									var startPoint string
									var endPoint string
									timeMap := map[string]interface{}{}
									switch originTime {
									case "今天":
										startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									case "昨天":
										startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									case "明天":
										startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									case "本周":
										week := int(time.Now().Weekday())
										if week == 0 {
											week = 7
										}
										startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									case "上周":
										week := int(time.Now().Weekday())
										if week == 0 {
											week = 7
										}
										startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									case "今年":
										startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									case "去年":
										startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									case "明年":
										startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
										endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
										timeMap = map[string]interface{}{
											"$gte": startPoint,
											"$lt":  endPoint,
										}
									default:
										if originTime != "" {
											fromNow := false
											if strings.HasPrefix(originTime, "前") {
												//fmt.Println("originTime:", originTime)
												if strings.HasSuffix(originTime, "当前") {
													fromNow = true
													originTime = strings.ReplaceAll(originTime, "当前", "")
													//fmt.Println("originTime after 当前:", originTime)
												}
												intervalNumberString := numberx.GetNumberExp(originTime)
												intervalNumber, err := strconv.Atoi(intervalNumberString)
												if err != nil {
													logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
													continue
												}
												interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
												//fmt.Println("intervalNumber:", intervalNumber)
												//fmt.Println("interval:", interval)
												switch interval {
												case "天":
													if fromNow {
														//fmt.Println("fromNow")
														startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
													}
												case "周":
													if fromNow {
														startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														weekDay := int(timeNow.Weekday())
														if weekDay == 0 {
															weekDay = 7
														}
														startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
													}
												case "月":
													if fromNow {
														startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
													}
												case "年":
													if fromNow {
														startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
													}
												case "季度":
													if fromNow {
														startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														timeNowMonth := int(timeNow.Month())
														quarter1 := timeNowMonth - 1
														quarter2 := timeNowMonth - 4
														quarter3 := timeNowMonth - 7
														quarter4 := timeNowMonth - 10
														quarterList := []int{quarter1, quarter2, quarter3, quarter4}
														min := quarter1
														for _, v := range quarterList {
															if v < min {
																min = v
															}
														}
														startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
													}
												case "时":
													if fromNow {
														startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
														endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
													}
												case "分":
													if fromNow {
														startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
														endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
													}
												case "秒":
													startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
													endPoint = timeNow.Format("2006-01-02 15:04:05")
												}
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											} else if strings.HasPrefix(originTime, "后") {
												if strings.HasSuffix(originTime, "当前") {
													fromNow = true
													originTime = strings.ReplaceAll(originTime, "当前", "")
												}
												intervalNumberString := numberx.GetNumberExp(originTime)
												intervalNumber, err := strconv.Atoi(intervalNumberString)
												if err != nil {
													logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
													continue
												}
												interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
												switch interval {
												case "天":
													if fromNow {
														endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
														startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
													}
												case "周":
													if fromNow {
														endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														weekDay := int(timeNow.Weekday())
														if weekDay == 0 {
															weekDay = 7
														}
														endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
														startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
													}
												case "月":
													if fromNow {
														endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
														startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
													}
												case "年":
													if fromNow {
														endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
														startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
													}
												case "季度":
													if fromNow {
														endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														timeNowMonth := int(timeNow.Month())
														quarter1 := timeNowMonth - 1
														quarter2 := timeNowMonth - 4
														quarter3 := timeNowMonth - 7
														quarter4 := timeNowMonth - 10
														quarterList := []int{quarter1, quarter2, quarter3, quarter4}
														min := quarter1
														for _, v := range quarterList {
															if v < min {
																min = v
															}
														}
														endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
														startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
													}
												case "时":
													if fromNow {
														endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
														startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
													}
												case "分":
													if fromNow {
														endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													} else {
														endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
														startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
													}
												case "秒":
													endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
													startPoint = timeNow.Format("2006-01-02 15:04:05")
												}
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											} else if strings.HasPrefix(originTime, "当前") {
												interval := strings.ReplaceAll(originTime, "当前", "")
												switch interval {
												case "天":
													startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
													endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
												case "周":
													weekDay := int(timeNow.Weekday())
													if weekDay == 0 {
														weekDay = 7
													}
													startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
													endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
												case "月":
													startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
													endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
												case "年":
													startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
													endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
												case "季度":
													timeNowMonth := int(timeNow.Month())
													quarter1 := timeNowMonth - 1
													quarter2 := timeNowMonth - 4
													quarter3 := timeNowMonth - 7
													quarter4 := timeNowMonth - 10
													quarterList := []int{quarter1, quarter2, quarter3, quarter4}
													min := quarter1
													for _, v := range quarterList {
														if v < min {
															min = v
														}
													}
													startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
													endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
												case "时":
													startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
													endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
												case "分":
													startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
													endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
												case "秒":
													endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
													startPoint = timeNow.Format("2006-01-02 15:04:05")
												}
												timeMap = map[string]interface{}{
													"$gte": startPoint,
													"$lt":  endPoint,
												}
											} else {
												eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
												if err != nil {
													continue
												}
												queryTime = eleTime.Format("2006-01-02 15:04:05")
											}
										}
									}
									if len(timeMap) != 0 {
										filterM[key] = timeMap
									} else {
										filterM[key] = queryTime
									}
								}
							}
						} else if key == "createTime" || key == "modifyTime" {
							//fmt.Println("createTime[key] in:", key, "val:", val)
							if timeMapRaw, ok := val.(map[string]interface{}); ok {
								for k, v := range timeMapRaw {
									if originTime, ok := v.(string); ok {
										var queryTime string
										var startPoint string
										var endPoint string
										timeMap := map[string]interface{}{}
										switch originTime {
										case "今天":
											startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										case "昨天":
											startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										case "明天":
											startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										case "本周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										case "上周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										case "今年":
											startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										case "去年":
											startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										case "明年":
											startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
											endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										default:
											if originTime != "" {
												fromNow := false
												if strings.HasPrefix(originTime, "前") {
													//fmt.Println("originTime:", originTime)
													if strings.HasSuffix(originTime, "当前") {
														fromNow = true
														originTime = strings.ReplaceAll(originTime, "当前", "")
														//fmt.Println("originTime after 当前:", originTime)
													}
													intervalNumberString := numberx.GetNumberExp(originTime)
													intervalNumber, err := strconv.Atoi(intervalNumberString)
													if err != nil {
														logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
														continue
													}
													interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
													//fmt.Println("intervalNumber:", intervalNumber)
													//fmt.Println("interval:", interval)
													switch interval {
													case "天":
														if fromNow {
															//fmt.Println("fromNow")
															startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
														}
													case "周":
														if fromNow {
															startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															weekDay := int(timeNow.Weekday())
															if weekDay == 0 {
																weekDay = 7
															}
															startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
														}
													case "月":
														if fromNow {
															startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
														}
													case "年":
														if fromNow {
															startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
														}
													case "季度":
														if fromNow {
															startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															timeNowMonth := int(timeNow.Month())
															quarter1 := timeNowMonth - 1
															quarter2 := timeNowMonth - 4
															quarter3 := timeNowMonth - 7
															quarter4 := timeNowMonth - 10
															quarterList := []int{quarter1, quarter2, quarter3, quarter4}
															min := quarter1
															for _, v := range quarterList {
																if v < min {
																	min = v
																}
															}
															startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
														}
													case "时":
														if fromNow {
															startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
															endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
														}
													case "分":
														if fromNow {
															startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
															endPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
															endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
														}
													case "秒":
														startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
														endPoint = timeNow.Format("2006-01-02 15:04:05")
													}
													timeMap = map[string]interface{}{
														"$gte": startPoint,
														"$lt":  endPoint,
													}
												} else if strings.HasPrefix(originTime, "后") {
													if strings.HasSuffix(originTime, "当前") {
														fromNow = true
														originTime = strings.ReplaceAll(originTime, "当前", "")
													}
													intervalNumberString := numberx.GetNumberExp(originTime)
													intervalNumber, err := strconv.Atoi(intervalNumberString)
													if err != nil {
														logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
														continue
													}
													interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
													switch interval {
													case "天":
														if fromNow {
															endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
															startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
														}
													case "周":
														if fromNow {
															endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															weekDay := int(timeNow.Weekday())
															if weekDay == 0 {
																weekDay = 7
															}
															endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
															startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
														}
													case "月":
														if fromNow {
															endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
															startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
														}
													case "年":
														if fromNow {
															endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
															startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
														}
													case "季度":
														if fromNow {
															endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															timeNowMonth := int(timeNow.Month())
															quarter1 := timeNowMonth - 1
															quarter2 := timeNowMonth - 4
															quarter3 := timeNowMonth - 7
															quarter4 := timeNowMonth - 10
															quarterList := []int{quarter1, quarter2, quarter3, quarter4}
															min := quarter1
															for _, v := range quarterList {
																if v < min {
																	min = v
																}
															}
															endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
															startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
														}
													case "时":
														if fromNow {
															endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
															startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
														}
													case "分":
														if fromNow {
															endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
															startPoint = timeNow.Format("2006-01-02 15:04:05")
														} else {
															endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
															startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
														}
													case "秒":
														endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													}
													timeMap = map[string]interface{}{
														"$gte": startPoint,
														"$lt":  endPoint,
													}
												} else if strings.HasPrefix(originTime, "当前") {
													interval := strings.ReplaceAll(originTime, "当前", "")
													switch interval {
													case "天":
														startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
													case "周":
														weekDay := int(timeNow.Weekday())
														if weekDay == 0 {
															weekDay = 7
														}
														startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
														endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
													case "月":
														startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
													case "年":
														startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
													case "季度":
														timeNowMonth := int(timeNow.Month())
														quarter1 := timeNowMonth - 1
														quarter2 := timeNowMonth - 4
														quarter3 := timeNowMonth - 7
														quarter4 := timeNowMonth - 10
														quarterList := []int{quarter1, quarter2, quarter3, quarter4}
														min := quarter1
														for _, v := range quarterList {
															if v < min {
																min = v
															}
														}
														startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
														endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
													case "时":
														startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
													case "分":
														startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
													case "秒":
														endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
														startPoint = timeNow.Format("2006-01-02 15:04:05")
													}
													timeMap = map[string]interface{}{
														"$gte": startPoint,
														"$lt":  endPoint,
													}
												} else {
													eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
													if err != nil {
														continue
													}
													queryTime = eleTime.Format("2006-01-02 15:04:05")
												}
											}
										}
										if len(timeMap) != 0 {
											timeMapRaw[k] = timeMap
										} else {
											timeMapRaw[k] = queryTime
										}
									}
								}
							} else if originTime, ok := val.(string); ok {
								//fmt.Println("val.(string):", val.(string))
								var queryTime string
								var startPoint string
								var endPoint string
								timeMap := map[string]interface{}{}
								switch originTime {
								case "今天":
									startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
									endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
									timeMap = map[string]interface{}{
										"$gte": startPoint,
										"$lt":  endPoint,
									}
								case "昨天":
									startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
									endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
									timeMap = map[string]interface{}{
										"$gte": startPoint,
										"$lt":  endPoint,
									}
								case "明天":
									startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
									endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
									timeMap = map[string]interface{}{
										"$gte": startPoint,
										"$lt":  endPoint,
									}
								case "本周":
									week := int(time.Now().Weekday())
									if week == 0 {
										week = 7
									}
									startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
									endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
									timeMap = map[string]interface{}{
										"$gte": startPoint,
										"$lt":  endPoint,
									}
								case "上周":
									week := int(time.Now().Weekday())
									if week == 0 {
										week = 7
									}
									startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
									endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
									timeMap = map[string]interface{}{
										"$gte": startPoint,
										"$lt":  endPoint,
									}
								case "今年":
									startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
									endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
									timeMap = map[string]interface{}{
										"$gte": startPoint,
										"$lt":  endPoint,
									}
								case "去年":
									startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
									endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
									timeMap = map[string]interface{}{
										"$gte": startPoint,
										"$lt":  endPoint,
									}
								case "明年":
									startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
									endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
									timeMap = map[string]interface{}{
										"$gte": startPoint,
										"$lt":  endPoint,
									}
								default:
									if originTime != "" {
										fromNow := false
										if strings.HasPrefix(originTime, "前") {
											//fmt.Println("originTime:", originTime)
											if strings.HasSuffix(originTime, "当前") {
												fromNow = true
												originTime = strings.ReplaceAll(originTime, "当前", "")
												//fmt.Println("originTime after 当前:", originTime)
											}
											intervalNumberString := numberx.GetNumberExp(originTime)
											intervalNumber, err := strconv.Atoi(intervalNumberString)
											if err != nil {
												logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
												continue
											}
											interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
											//fmt.Println("intervalNumber:", intervalNumber)
											//fmt.Println("interval:", interval)
											switch interval {
											case "天":
												if fromNow {
													//fmt.Println("fromNow")
													startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													endPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
													endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
												}
											case "周":
												if fromNow {
													startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													endPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													weekDay := int(timeNow.Weekday())
													if weekDay == 0 {
														weekDay = 7
													}
													startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
													endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
												}
											case "月":
												if fromNow {
													startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													endPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
													endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
												}
											case "年":
												if fromNow {
													startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													endPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
													endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
												}
											case "季度":
												if fromNow {
													startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													endPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													timeNowMonth := int(timeNow.Month())
													quarter1 := timeNowMonth - 1
													quarter2 := timeNowMonth - 4
													quarter3 := timeNowMonth - 7
													quarter4 := timeNowMonth - 10
													quarterList := []int{quarter1, quarter2, quarter3, quarter4}
													min := quarter1
													for _, v := range quarterList {
														if v < min {
															min = v
														}
													}
													startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
													endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
												}
											case "时":
												if fromNow {
													startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													endPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
													endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
												}
											case "分":
												if fromNow {
													startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
													endPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
													endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
												}
											case "秒":
												startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
												endPoint = timeNow.Format("2006-01-02 15:04:05")
											}
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										} else if strings.HasPrefix(originTime, "后") {
											if strings.HasSuffix(originTime, "当前") {
												fromNow = true
												originTime = strings.ReplaceAll(originTime, "当前", "")
											}
											intervalNumberString := numberx.GetNumberExp(originTime)
											intervalNumber, err := strconv.Atoi(intervalNumberString)
											if err != nil {
												logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
												continue
											}
											interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
											switch interval {
											case "天":
												if fromNow {
													endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													startPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
													startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
												}
											case "周":
												if fromNow {
													endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													startPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													weekDay := int(timeNow.Weekday())
													if weekDay == 0 {
														weekDay = 7
													}
													endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
													startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
												}
											case "月":
												if fromNow {
													endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													startPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
													startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
												}
											case "年":
												if fromNow {
													endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													startPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
													startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
												}
											case "季度":
												if fromNow {
													endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													startPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													timeNowMonth := int(timeNow.Month())
													quarter1 := timeNowMonth - 1
													quarter2 := timeNowMonth - 4
													quarter3 := timeNowMonth - 7
													quarter4 := timeNowMonth - 10
													quarterList := []int{quarter1, quarter2, quarter3, quarter4}
													min := quarter1
													for _, v := range quarterList {
														if v < min {
															min = v
														}
													}
													endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
													startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
												}
											case "时":
												if fromNow {
													endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
													startPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
													startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
												}
											case "分":
												if fromNow {
													endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
													startPoint = timeNow.Format("2006-01-02 15:04:05")
												} else {
													endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
													startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
												}
											case "秒":
												endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
												startPoint = timeNow.Format("2006-01-02 15:04:05")
											}
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										} else if strings.HasPrefix(originTime, "当前") {
											interval := strings.ReplaceAll(originTime, "当前", "")
											switch interval {
											case "天":
												startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
											case "周":
												weekDay := int(timeNow.Weekday())
												if weekDay == 0 {
													weekDay = 7
												}
												startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
												endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
											case "月":
												startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
											case "年":
												startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
											case "季度":
												timeNowMonth := int(timeNow.Month())
												quarter1 := timeNowMonth - 1
												quarter2 := timeNowMonth - 4
												quarter3 := timeNowMonth - 7
												quarter4 := timeNowMonth - 10
												quarterList := []int{quarter1, quarter2, quarter3, quarter4}
												min := quarter1
												for _, v := range quarterList {
													if v < min {
														min = v
													}
												}
												startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
												endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
											case "时":
												startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
											case "分":
												startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
												endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
											case "秒":
												endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
												startPoint = timeNow.Format("2006-01-02 15:04:05")
											}
											timeMap = map[string]interface{}{
												"$gte": startPoint,
												"$lt":  endPoint,
											}
										} else {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
											if err != nil {
												continue
											}
											queryTime = eleTime.Format("2006-01-02 15:04:05")
										}
									}
								}
								if len(timeMap) != 0 {
									filterM[key] = timeMap
								} else {
									filterM[key] = queryTime
								}
							}
						}
						if key == "$or" {
							if orList, ok := val.([]interface{}); ok {
								for _, orEle := range orList {
									if orEleMap, ok := orEle.(map[string]interface{}); ok {
										for orKey, orVal := range orEleMap {
											if orKey == "$and" {
												if orList, ok := orVal.([]interface{}); ok {
													for _, orEle := range orList {
														if orEleMap, ok := orEle.(map[string]interface{}); ok {
															for orKey, orVal := range orEleMap {
																if extRaw, ok := excelColNameTypeExtMap[orKey]; ok {
																	if extRaw.Format != "" {
																		if timeMapRaw, ok := orVal.(map[string]interface{}); ok {
																			for k, v := range timeMapRaw {
																				if originTime, ok := v.(string); ok {
																					var queryTime string
																					var startPoint string
																					var endPoint string
																					timeMap := map[string]interface{}{}
																					switch originTime {
																					case "今天":
																						startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					case "昨天":
																						startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					case "明天":
																						startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					case "本周":
																						week := int(time.Now().Weekday())
																						if week == 0 {
																							week = 7
																						}
																						startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					case "上周":
																						week := int(time.Now().Weekday())
																						if week == 0 {
																							week = 7
																						}
																						startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					case "今年":
																						startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					case "去年":
																						startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					case "明年":
																						startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					default:
																						if originTime != "" {
																							fromNow := false
																							if strings.HasPrefix(originTime, "前") {
																								//fmt.Println("originTime:", originTime)
																								if strings.HasSuffix(originTime, "当前") {
																									fromNow = true
																									originTime = strings.ReplaceAll(originTime, "当前", "")
																									//fmt.Println("originTime after 当前:", originTime)
																								}
																								intervalNumberString := numberx.GetNumberExp(originTime)
																								intervalNumber, err := strconv.Atoi(intervalNumberString)
																								if err != nil {
																									logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																									continue
																								}
																								interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																								//fmt.Println("intervalNumber:", intervalNumber)
																								//fmt.Println("interval:", interval)
																								switch interval {
																								case "天":
																									if fromNow {
																										//fmt.Println("fromNow")
																										startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																									}
																								case "周":
																									if fromNow {
																										startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										weekDay := int(timeNow.Weekday())
																										if weekDay == 0 {
																											weekDay = 7
																										}
																										startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																									}
																								case "月":
																									if fromNow {
																										startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																									}
																								case "年":
																									if fromNow {
																										startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																									}
																								case "季度":
																									if fromNow {
																										startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										timeNowMonth := int(timeNow.Month())
																										quarter1 := timeNowMonth - 1
																										quarter2 := timeNowMonth - 4
																										quarter3 := timeNowMonth - 7
																										quarter4 := timeNowMonth - 10
																										quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																										min := quarter1
																										for _, v := range quarterList {
																											if v < min {
																												min = v
																											}
																										}
																										startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																									}
																								case "时":
																									if fromNow {
																										startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																									}
																								case "分":
																									if fromNow {
																										startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																										endPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																										endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																									}
																								case "秒":
																									startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								}
																								timeMap = map[string]interface{}{
																									"$gte": startPoint,
																									"$lt":  endPoint,
																								}
																							} else if strings.HasPrefix(originTime, "后") {
																								if strings.HasSuffix(originTime, "当前") {
																									fromNow = true
																									originTime = strings.ReplaceAll(originTime, "当前", "")
																								}
																								intervalNumberString := numberx.GetNumberExp(originTime)
																								intervalNumber, err := strconv.Atoi(intervalNumberString)
																								if err != nil {
																									logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																									continue
																								}
																								interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																								switch interval {
																								case "天":
																									if fromNow {
																										endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																										startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																									}
																								case "周":
																									if fromNow {
																										endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										weekDay := int(timeNow.Weekday())
																										if weekDay == 0 {
																											weekDay = 7
																										}
																										endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																										startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																									}
																								case "月":
																									if fromNow {
																										endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																										startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																									}
																								case "年":
																									if fromNow {
																										endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																										startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																									}
																								case "季度":
																									if fromNow {
																										endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										timeNowMonth := int(timeNow.Month())
																										quarter1 := timeNowMonth - 1
																										quarter2 := timeNowMonth - 4
																										quarter3 := timeNowMonth - 7
																										quarter4 := timeNowMonth - 10
																										quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																										min := quarter1
																										for _, v := range quarterList {
																											if v < min {
																												min = v
																											}
																										}
																										endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																										startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																									}
																								case "时":
																									if fromNow {
																										endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																										startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																									}
																								case "分":
																									if fromNow {
																										endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																										startPoint = timeNow.Format("2006-01-02 15:04:05")
																									} else {
																										endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																										startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																									}
																								case "秒":
																									endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								}
																								timeMap = map[string]interface{}{
																									"$gte": startPoint,
																									"$lt":  endPoint,
																								}
																							} else if strings.HasPrefix(originTime, "当前") {
																								interval := strings.ReplaceAll(originTime, "当前", "")
																								switch interval {
																								case "天":
																									startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																								case "周":
																									weekDay := int(timeNow.Weekday())
																									if weekDay == 0 {
																										weekDay = 7
																									}
																									startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																								case "月":
																									startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																								case "年":
																									startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																								case "季度":
																									timeNowMonth := int(timeNow.Month())
																									quarter1 := timeNowMonth - 1
																									quarter2 := timeNowMonth - 4
																									quarter3 := timeNowMonth - 7
																									quarter4 := timeNowMonth - 10
																									quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																									min := quarter1
																									for _, v := range quarterList {
																										if v < min {
																											min = v
																										}
																									}
																									startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																								case "时":
																									startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																								case "分":
																									startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																								case "秒":
																									endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								}
																								timeMap = map[string]interface{}{
																									"$gte": startPoint,
																									"$lt":  endPoint,
																								}
																							} else {
																								eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																								if err != nil {
																									continue
																								}
																								queryTime = eleTime.Format("2006-01-02 15:04:05")
																							}
																						}
																					}
																					if len(timeMap) != 0 {
																						timeMapRaw[k] = timeMap
																					} else {
																						timeMapRaw[k] = queryTime
																					}
																				}
																			}
																		} else if originTime, ok := orVal.(string); ok {
																			var queryTime string
																			var startPoint string
																			var endPoint string
																			timeMap := map[string]interface{}{}
																			switch originTime {
																			case "今天":
																				startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			case "昨天":
																				startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			case "明天":
																				startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			case "本周":
																				week := int(time.Now().Weekday())
																				if week == 0 {
																					week = 7
																				}
																				startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			case "上周":
																				week := int(time.Now().Weekday())
																				if week == 0 {
																					week = 7
																				}
																				startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			case "今年":
																				startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			case "去年":
																				startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			case "明年":
																				startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
																				timeMap = map[string]interface{}{
																					"$gte": startPoint,
																					"$lt":  endPoint,
																				}
																			default:
																				if originTime != "" {
																					fromNow := false
																					if strings.HasPrefix(originTime, "前") {
																						//fmt.Println("originTime:", originTime)
																						if strings.HasSuffix(originTime, "当前") {
																							fromNow = true
																							originTime = strings.ReplaceAll(originTime, "当前", "")
																							//fmt.Println("originTime after 当前:", originTime)
																						}
																						intervalNumberString := numberx.GetNumberExp(originTime)
																						intervalNumber, err := strconv.Atoi(intervalNumberString)
																						if err != nil {
																							logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																							continue
																						}
																						interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																						//fmt.Println("intervalNumber:", intervalNumber)
																						//fmt.Println("interval:", interval)
																						switch interval {
																						case "天":
																							if fromNow {
																								//fmt.Println("fromNow")
																								startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																							}
																						case "周":
																							if fromNow {
																								startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								weekDay := int(timeNow.Weekday())
																								if weekDay == 0 {
																									weekDay = 7
																								}
																								startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																							}
																						case "月":
																							if fromNow {
																								startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																							}
																						case "年":
																							if fromNow {
																								startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																							}
																						case "季度":
																							if fromNow {
																								startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								timeNowMonth := int(timeNow.Month())
																								quarter1 := timeNowMonth - 1
																								quarter2 := timeNowMonth - 4
																								quarter3 := timeNowMonth - 7
																								quarter4 := timeNowMonth - 10
																								quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																								min := quarter1
																								for _, v := range quarterList {
																									if v < min {
																										min = v
																									}
																								}
																								startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																							}
																						case "时":
																							if fromNow {
																								startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																							}
																						case "分":
																							if fromNow {
																								startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																							}
																						case "秒":
																							startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																							endPoint = timeNow.Format("2006-01-02 15:04:05")
																						}
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					} else if strings.HasPrefix(originTime, "后") {
																						if strings.HasSuffix(originTime, "当前") {
																							fromNow = true
																							originTime = strings.ReplaceAll(originTime, "当前", "")
																						}
																						intervalNumberString := numberx.GetNumberExp(originTime)
																						intervalNumber, err := strconv.Atoi(intervalNumberString)
																						if err != nil {
																							logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																							continue
																						}
																						interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																						switch interval {
																						case "天":
																							if fromNow {
																								endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																								startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																							}
																						case "周":
																							if fromNow {
																								endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								weekDay := int(timeNow.Weekday())
																								if weekDay == 0 {
																									weekDay = 7
																								}
																								endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																								startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																							}
																						case "月":
																							if fromNow {
																								endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																								startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																							}
																						case "年":
																							if fromNow {
																								endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																								startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																							}
																						case "季度":
																							if fromNow {
																								endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								timeNowMonth := int(timeNow.Month())
																								quarter1 := timeNowMonth - 1
																								quarter2 := timeNowMonth - 4
																								quarter3 := timeNowMonth - 7
																								quarter4 := timeNowMonth - 10
																								quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																								min := quarter1
																								for _, v := range quarterList {
																									if v < min {
																										min = v
																									}
																								}
																								endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																								startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																							}
																						case "时":
																							if fromNow {
																								endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																								startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																							}
																						case "分":
																							if fromNow {
																								endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							} else {
																								endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																								startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																							}
																						case "秒":
																							endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																							startPoint = timeNow.Format("2006-01-02 15:04:05")
																						}
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					} else if strings.HasPrefix(originTime, "当前") {
																						interval := strings.ReplaceAll(originTime, "当前", "")
																						switch interval {
																						case "天":
																							startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																						case "周":
																							weekDay := int(timeNow.Weekday())
																							if weekDay == 0 {
																								weekDay = 7
																							}
																							startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																						case "月":
																							startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																						case "年":
																							startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																						case "季度":
																							timeNowMonth := int(timeNow.Month())
																							quarter1 := timeNowMonth - 1
																							quarter2 := timeNowMonth - 4
																							quarter3 := timeNowMonth - 7
																							quarter4 := timeNowMonth - 10
																							quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																							min := quarter1
																							for _, v := range quarterList {
																								if v < min {
																									min = v
																								}
																							}
																							startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																						case "时":
																							startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																						case "分":
																							startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																						case "秒":
																							endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																							startPoint = timeNow.Format("2006-01-02 15:04:05")
																						}
																						timeMap = map[string]interface{}{
																							"$gte": startPoint,
																							"$lt":  endPoint,
																						}
																					} else {
																						eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																						if err != nil {
																							continue
																						}
																						queryTime = eleTime.Format("2006-01-02 15:04:05")
																					}
																				}
																			}
																			if len(timeMap) != 0 {
																				orEleMap[orKey] = timeMap
																			} else {
																				orEleMap[orKey] = queryTime
																			}
																		}
																	}
																} else if orKey == "createTime" || orKey == "modifyTime" {
																	if timeMapRaw, ok := orVal.(map[string]interface{}); ok {
																		for k, v := range timeMapRaw {
																			if originTime, ok := v.(string); ok {
																				var queryTime string
																				var startPoint string
																				var endPoint string
																				timeMap := map[string]interface{}{}
																				switch originTime {
																				case "今天":
																					startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				case "昨天":
																					startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				case "明天":
																					startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				case "本周":
																					week := int(time.Now().Weekday())
																					if week == 0 {
																						week = 7
																					}
																					startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				case "上周":
																					week := int(time.Now().Weekday())
																					if week == 0 {
																						week = 7
																					}
																					startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				case "今年":
																					startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				case "去年":
																					startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				case "明年":
																					startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				default:
																					if originTime != "" {
																						fromNow := false
																						if strings.HasPrefix(originTime, "前") {
																							//fmt.Println("originTime:", originTime)
																							if strings.HasSuffix(originTime, "当前") {
																								fromNow = true
																								originTime = strings.ReplaceAll(originTime, "当前", "")
																								//fmt.Println("originTime after 当前:", originTime)
																							}
																							intervalNumberString := numberx.GetNumberExp(originTime)
																							intervalNumber, err := strconv.Atoi(intervalNumberString)
																							if err != nil {
																								logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																								continue
																							}
																							interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																							//fmt.Println("intervalNumber:", intervalNumber)
																							//fmt.Println("interval:", interval)
																							switch interval {
																							case "天":
																								if fromNow {
																									//fmt.Println("fromNow")
																									startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																								}
																							case "周":
																								if fromNow {
																									startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									weekDay := int(timeNow.Weekday())
																									if weekDay == 0 {
																										weekDay = 7
																									}
																									startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																								}
																							case "月":
																								if fromNow {
																									startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																								}
																							case "年":
																								if fromNow {
																									startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																								}
																							case "季度":
																								if fromNow {
																									startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									timeNowMonth := int(timeNow.Month())
																									quarter1 := timeNowMonth - 1
																									quarter2 := timeNowMonth - 4
																									quarter3 := timeNowMonth - 7
																									quarter4 := timeNowMonth - 10
																									quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																									min := quarter1
																									for _, v := range quarterList {
																										if v < min {
																											min = v
																										}
																									}
																									startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																								}
																							case "时":
																								if fromNow {
																									startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																								}
																							case "分":
																								if fromNow {
																									startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																									endPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																									endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																								}
																							case "秒":
																								startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																								endPoint = timeNow.Format("2006-01-02 15:04:05")
																							}
																							timeMap = map[string]interface{}{
																								"$gte": startPoint,
																								"$lt":  endPoint,
																							}
																						} else if strings.HasPrefix(originTime, "后") {
																							if strings.HasSuffix(originTime, "当前") {
																								fromNow = true
																								originTime = strings.ReplaceAll(originTime, "当前", "")
																							}
																							intervalNumberString := numberx.GetNumberExp(originTime)
																							intervalNumber, err := strconv.Atoi(intervalNumberString)
																							if err != nil {
																								logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																								continue
																							}
																							interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																							switch interval {
																							case "天":
																								if fromNow {
																									endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																									startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																								}
																							case "周":
																								if fromNow {
																									endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									weekDay := int(timeNow.Weekday())
																									if weekDay == 0 {
																										weekDay = 7
																									}
																									endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																									startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																								}
																							case "月":
																								if fromNow {
																									endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																									startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																								}
																							case "年":
																								if fromNow {
																									endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																									startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																								}
																							case "季度":
																								if fromNow {
																									endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									timeNowMonth := int(timeNow.Month())
																									quarter1 := timeNowMonth - 1
																									quarter2 := timeNowMonth - 4
																									quarter3 := timeNowMonth - 7
																									quarter4 := timeNowMonth - 10
																									quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																									min := quarter1
																									for _, v := range quarterList {
																										if v < min {
																											min = v
																										}
																									}
																									endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																									startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																								}
																							case "时":
																								if fromNow {
																									endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																									startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																								}
																							case "分":
																								if fromNow {
																									endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																									startPoint = timeNow.Format("2006-01-02 15:04:05")
																								} else {
																									endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																									startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																								}
																							case "秒":
																								endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							}
																							timeMap = map[string]interface{}{
																								"$gte": startPoint,
																								"$lt":  endPoint,
																							}
																						} else if strings.HasPrefix(originTime, "当前") {
																							interval := strings.ReplaceAll(originTime, "当前", "")
																							switch interval {
																							case "天":
																								startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																							case "周":
																								weekDay := int(timeNow.Weekday())
																								if weekDay == 0 {
																									weekDay = 7
																								}
																								startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																							case "月":
																								startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																							case "年":
																								startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																							case "季度":
																								timeNowMonth := int(timeNow.Month())
																								quarter1 := timeNowMonth - 1
																								quarter2 := timeNowMonth - 4
																								quarter3 := timeNowMonth - 7
																								quarter4 := timeNowMonth - 10
																								quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																								min := quarter1
																								for _, v := range quarterList {
																									if v < min {
																										min = v
																									}
																								}
																								startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																							case "时":
																								startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																							case "分":
																								startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																								endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																							case "秒":
																								endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																								startPoint = timeNow.Format("2006-01-02 15:04:05")
																							}
																							timeMap = map[string]interface{}{
																								"$gte": startPoint,
																								"$lt":  endPoint,
																							}
																						} else {
																							eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																							if err != nil {
																								continue
																							}
																							queryTime = eleTime.Format("2006-01-02 15:04:05")
																						}
																					}
																				}
																				if len(timeMap) != 0 {
																					timeMapRaw[k] = timeMap
																				} else {
																					timeMapRaw[k] = queryTime
																				}
																			}
																		}
																	} else if originTime, ok := orVal.(string); ok {
																		var queryTime string
																		var startPoint string
																		var endPoint string
																		timeMap := map[string]interface{}{}
																		switch originTime {
																		case "今天":
																			startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		case "昨天":
																			startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		case "明天":
																			startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		case "本周":
																			week := int(time.Now().Weekday())
																			if week == 0 {
																				week = 7
																			}
																			startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		case "上周":
																			week := int(time.Now().Weekday())
																			if week == 0 {
																				week = 7
																			}
																			startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		case "今年":
																			startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		case "去年":
																			startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		case "明年":
																			startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		default:
																			if originTime != "" {
																				fromNow := false
																				if strings.HasPrefix(originTime, "前") {
																					//fmt.Println("originTime:", originTime)
																					if strings.HasSuffix(originTime, "当前") {
																						fromNow = true
																						originTime = strings.ReplaceAll(originTime, "当前", "")
																						//fmt.Println("originTime after 当前:", originTime)
																					}
																					intervalNumberString := numberx.GetNumberExp(originTime)
																					intervalNumber, err := strconv.Atoi(intervalNumberString)
																					if err != nil {
																						logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																						continue
																					}
																					interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																					//fmt.Println("intervalNumber:", intervalNumber)
																					//fmt.Println("interval:", interval)
																					switch interval {
																					case "天":
																						if fromNow {
																							//fmt.Println("fromNow")
																							startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							endPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																						}
																					case "周":
																						if fromNow {
																							startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							endPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							weekDay := int(timeNow.Weekday())
																							if weekDay == 0 {
																								weekDay = 7
																							}
																							startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																						}
																					case "月":
																						if fromNow {
																							startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							endPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																						}
																					case "年":
																						if fromNow {
																							startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							endPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																						}
																					case "季度":
																						if fromNow {
																							startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							endPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							timeNowMonth := int(timeNow.Month())
																							quarter1 := timeNowMonth - 1
																							quarter2 := timeNowMonth - 4
																							quarter3 := timeNowMonth - 7
																							quarter4 := timeNowMonth - 10
																							quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																							min := quarter1
																							for _, v := range quarterList {
																								if v < min {
																									min = v
																								}
																							}
																							startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																						}
																					case "时":
																						if fromNow {
																							startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							endPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																						}
																					case "分":
																						if fromNow {
																							startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																							endPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																							endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																						}
																					case "秒":
																						startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																						endPoint = timeNow.Format("2006-01-02 15:04:05")
																					}
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				} else if strings.HasPrefix(originTime, "后") {
																					if strings.HasSuffix(originTime, "当前") {
																						fromNow = true
																						originTime = strings.ReplaceAll(originTime, "当前", "")
																					}
																					intervalNumberString := numberx.GetNumberExp(originTime)
																					intervalNumber, err := strconv.Atoi(intervalNumberString)
																					if err != nil {
																						logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																						continue
																					}
																					interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																					switch interval {
																					case "天":
																						if fromNow {
																							endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							startPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																							startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																						}
																					case "周":
																						if fromNow {
																							endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							startPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							weekDay := int(timeNow.Weekday())
																							if weekDay == 0 {
																								weekDay = 7
																							}
																							endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																							startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																						}
																					case "月":
																						if fromNow {
																							endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							startPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																							startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																						}
																					case "年":
																						if fromNow {
																							endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							startPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																							startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																						}
																					case "季度":
																						if fromNow {
																							endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							startPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							timeNowMonth := int(timeNow.Month())
																							quarter1 := timeNowMonth - 1
																							quarter2 := timeNowMonth - 4
																							quarter3 := timeNowMonth - 7
																							quarter4 := timeNowMonth - 10
																							quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																							min := quarter1
																							for _, v := range quarterList {
																								if v < min {
																									min = v
																								}
																							}
																							endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																							startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																						}
																					case "时":
																						if fromNow {
																							endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																							startPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																							startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																						}
																					case "分":
																						if fromNow {
																							endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																							startPoint = timeNow.Format("2006-01-02 15:04:05")
																						} else {
																							endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																							startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																						}
																					case "秒":
																						endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																						startPoint = timeNow.Format("2006-01-02 15:04:05")
																					}
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				} else if strings.HasPrefix(originTime, "当前") {
																					interval := strings.ReplaceAll(originTime, "当前", "")
																					switch interval {
																					case "天":
																						startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																					case "周":
																						weekDay := int(timeNow.Weekday())
																						if weekDay == 0 {
																							weekDay = 7
																						}
																						startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																					case "月":
																						startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																					case "年":
																						startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																					case "季度":
																						timeNowMonth := int(timeNow.Month())
																						quarter1 := timeNowMonth - 1
																						quarter2 := timeNowMonth - 4
																						quarter3 := timeNowMonth - 7
																						quarter4 := timeNowMonth - 10
																						quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																						min := quarter1
																						for _, v := range quarterList {
																							if v < min {
																								min = v
																							}
																						}
																						startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																					case "时":
																						startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																					case "分":
																						startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																						endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																					case "秒":
																						endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																						startPoint = timeNow.Format("2006-01-02 15:04:05")
																					}
																					timeMap = map[string]interface{}{
																						"$gte": startPoint,
																						"$lt":  endPoint,
																					}
																				} else {
																					eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																					if err != nil {
																						continue
																					}
																					queryTime = eleTime.Format("2006-01-02 15:04:05")
																				}
																			}
																		}
																		if len(timeMap) != 0 {
																			orEleMap[orKey] = timeMap
																		} else {
																			orEleMap[orKey] = queryTime
																		}
																	}
																}
															}
														}
													}
												}
											} else if extRaw, ok := excelColNameTypeExtMap[orKey]; ok {
												if extRaw.Format != "" {
													if timeMapRaw, ok := orVal.(map[string]interface{}); ok {
														for k, v := range timeMapRaw {
															if originTime, ok := v.(string); ok {
																var queryTime string
																var startPoint string
																var endPoint string
																timeMap := map[string]interface{}{}
																switch originTime {
																case "今天":
																	startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																case "昨天":
																	startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																case "明天":
																	startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																case "本周":
																	week := int(time.Now().Weekday())
																	if week == 0 {
																		week = 7
																	}
																	startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																case "上周":
																	week := int(time.Now().Weekday())
																	if week == 0 {
																		week = 7
																	}
																	startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																case "今年":
																	startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																case "去年":
																	startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																case "明年":
																	startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																default:
																	if originTime != "" {
																		fromNow := false
																		if strings.HasPrefix(originTime, "前") {
																			//fmt.Println("originTime:", originTime)
																			if strings.HasSuffix(originTime, "当前") {
																				fromNow = true
																				originTime = strings.ReplaceAll(originTime, "当前", "")
																				//fmt.Println("originTime after 当前:", originTime)
																			}
																			intervalNumberString := numberx.GetNumberExp(originTime)
																			intervalNumber, err := strconv.Atoi(intervalNumberString)
																			if err != nil {
																				logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																				continue
																			}
																			interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																			//fmt.Println("intervalNumber:", intervalNumber)
																			//fmt.Println("interval:", interval)
																			switch interval {
																			case "天":
																				if fromNow {
																					//fmt.Println("fromNow")
																					startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																				}
																			case "周":
																				if fromNow {
																					startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					weekDay := int(timeNow.Weekday())
																					if weekDay == 0 {
																						weekDay = 7
																					}
																					startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																				}
																			case "月":
																				if fromNow {
																					startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																				}
																			case "年":
																				if fromNow {
																					startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																				}
																			case "季度":
																				if fromNow {
																					startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					timeNowMonth := int(timeNow.Month())
																					quarter1 := timeNowMonth - 1
																					quarter2 := timeNowMonth - 4
																					quarter3 := timeNowMonth - 7
																					quarter4 := timeNowMonth - 10
																					quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																					min := quarter1
																					for _, v := range quarterList {
																						if v < min {
																							min = v
																						}
																					}
																					startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																				}
																			case "时":
																				if fromNow {
																					startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																				}
																			case "分":
																				if fromNow {
																					startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																					endPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																					endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																				}
																			case "秒":
																				startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			}
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		} else if strings.HasPrefix(originTime, "后") {
																			if strings.HasSuffix(originTime, "当前") {
																				fromNow = true
																				originTime = strings.ReplaceAll(originTime, "当前", "")
																			}
																			intervalNumberString := numberx.GetNumberExp(originTime)
																			intervalNumber, err := strconv.Atoi(intervalNumberString)
																			if err != nil {
																				logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																				continue
																			}
																			interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																			switch interval {
																			case "天":
																				if fromNow {
																					endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																					startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																				}
																			case "周":
																				if fromNow {
																					endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					weekDay := int(timeNow.Weekday())
																					if weekDay == 0 {
																						weekDay = 7
																					}
																					endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																					startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																				}
																			case "月":
																				if fromNow {
																					endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																					startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																				}
																			case "年":
																				if fromNow {
																					endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																					startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																				}
																			case "季度":
																				if fromNow {
																					endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					timeNowMonth := int(timeNow.Month())
																					quarter1 := timeNowMonth - 1
																					quarter2 := timeNowMonth - 4
																					quarter3 := timeNowMonth - 7
																					quarter4 := timeNowMonth - 10
																					quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																					min := quarter1
																					for _, v := range quarterList {
																						if v < min {
																							min = v
																						}
																					}
																					endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																					startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																				}
																			case "时":
																				if fromNow {
																					endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																					startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																				}
																			case "分":
																				if fromNow {
																					endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																					startPoint = timeNow.Format("2006-01-02 15:04:05")
																				} else {
																					endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																					startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																				}
																			case "秒":
																				endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			}
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		} else if strings.HasPrefix(originTime, "当前") {
																			interval := strings.ReplaceAll(originTime, "当前", "")
																			switch interval {
																			case "天":
																				startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																			case "周":
																				weekDay := int(timeNow.Weekday())
																				if weekDay == 0 {
																					weekDay = 7
																				}
																				startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																			case "月":
																				startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																			case "年":
																				startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																			case "季度":
																				timeNowMonth := int(timeNow.Month())
																				quarter1 := timeNowMonth - 1
																				quarter2 := timeNowMonth - 4
																				quarter3 := timeNowMonth - 7
																				quarter4 := timeNowMonth - 10
																				quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																				min := quarter1
																				for _, v := range quarterList {
																					if v < min {
																						min = v
																					}
																				}
																				startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																			case "时":
																				startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																			case "分":
																				startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																			case "秒":
																				endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			}
																			timeMap = map[string]interface{}{
																				"$gte": startPoint,
																				"$lt":  endPoint,
																			}
																		} else {
																			eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																			if err != nil {
																				continue
																			}
																			queryTime = eleTime.Format("2006-01-02 15:04:05")
																		}
																	}
																}
																if len(timeMap) != 0 {
																	timeMapRaw[k] = timeMap
																} else {
																	timeMapRaw[k] = queryTime
																}
															}
														}
													} else if originTime, ok := orVal.(string); ok {
														var queryTime string
														var startPoint string
														var endPoint string
														timeMap := map[string]interface{}{}
														switch originTime {
														case "今天":
															startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														case "昨天":
															startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														case "明天":
															startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														case "本周":
															week := int(time.Now().Weekday())
															if week == 0 {
																week = 7
															}
															startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														case "上周":
															week := int(time.Now().Weekday())
															if week == 0 {
																week = 7
															}
															startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														case "今年":
															startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														case "去年":
															startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														case "明年":
															startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
															endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
															timeMap = map[string]interface{}{
																"$gte": startPoint,
																"$lt":  endPoint,
															}
														default:
															if originTime != "" {
																fromNow := false
																if strings.HasPrefix(originTime, "前") {
																	//fmt.Println("originTime:", originTime)
																	if strings.HasSuffix(originTime, "当前") {
																		fromNow = true
																		originTime = strings.ReplaceAll(originTime, "当前", "")
																		//fmt.Println("originTime after 当前:", originTime)
																	}
																	intervalNumberString := numberx.GetNumberExp(originTime)
																	intervalNumber, err := strconv.Atoi(intervalNumberString)
																	if err != nil {
																		logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																		continue
																	}
																	interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																	//fmt.Println("intervalNumber:", intervalNumber)
																	//fmt.Println("interval:", interval)
																	switch interval {
																	case "天":
																		if fromNow {
																			//fmt.Println("fromNow")
																			startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																		}
																	case "周":
																		if fromNow {
																			startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			weekDay := int(timeNow.Weekday())
																			if weekDay == 0 {
																				weekDay = 7
																			}
																			startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																		}
																	case "月":
																		if fromNow {
																			startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																		}
																	case "年":
																		if fromNow {
																			startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																		}
																	case "季度":
																		if fromNow {
																			startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			timeNowMonth := int(timeNow.Month())
																			quarter1 := timeNowMonth - 1
																			quarter2 := timeNowMonth - 4
																			quarter3 := timeNowMonth - 7
																			quarter4 := timeNowMonth - 10
																			quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																			min := quarter1
																			for _, v := range quarterList {
																				if v < min {
																					min = v
																				}
																			}
																			startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																		}
																	case "时":
																		if fromNow {
																			startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																		}
																	case "分":
																		if fromNow {
																			startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																		}
																	case "秒":
																		startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																		endPoint = timeNow.Format("2006-01-02 15:04:05")
																	}
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																} else if strings.HasPrefix(originTime, "后") {
																	if strings.HasSuffix(originTime, "当前") {
																		fromNow = true
																		originTime = strings.ReplaceAll(originTime, "当前", "")
																	}
																	intervalNumberString := numberx.GetNumberExp(originTime)
																	intervalNumber, err := strconv.Atoi(intervalNumberString)
																	if err != nil {
																		logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																		continue
																	}
																	interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																	switch interval {
																	case "天":
																		if fromNow {
																			endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																			startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																		}
																	case "周":
																		if fromNow {
																			endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			weekDay := int(timeNow.Weekday())
																			if weekDay == 0 {
																				weekDay = 7
																			}
																			endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																			startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																		}
																	case "月":
																		if fromNow {
																			endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																			startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																		}
																	case "年":
																		if fromNow {
																			endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																			startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																		}
																	case "季度":
																		if fromNow {
																			endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			timeNowMonth := int(timeNow.Month())
																			quarter1 := timeNowMonth - 1
																			quarter2 := timeNowMonth - 4
																			quarter3 := timeNowMonth - 7
																			quarter4 := timeNowMonth - 10
																			quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																			min := quarter1
																			for _, v := range quarterList {
																				if v < min {
																					min = v
																				}
																			}
																			endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																			startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																		}
																	case "时":
																		if fromNow {
																			endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																			startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																		}
																	case "分":
																		if fromNow {
																			endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		} else {
																			endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																			startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																		}
																	case "秒":
																		endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																		startPoint = timeNow.Format("2006-01-02 15:04:05")
																	}
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																} else if strings.HasPrefix(originTime, "当前") {
																	interval := strings.ReplaceAll(originTime, "当前", "")
																	switch interval {
																	case "天":
																		startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																	case "周":
																		weekDay := int(timeNow.Weekday())
																		if weekDay == 0 {
																			weekDay = 7
																		}
																		startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																	case "月":
																		startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																	case "年":
																		startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																	case "季度":
																		timeNowMonth := int(timeNow.Month())
																		quarter1 := timeNowMonth - 1
																		quarter2 := timeNowMonth - 4
																		quarter3 := timeNowMonth - 7
																		quarter4 := timeNowMonth - 10
																		quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																		min := quarter1
																		for _, v := range quarterList {
																			if v < min {
																				min = v
																			}
																		}
																		startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																	case "时":
																		startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																	case "分":
																		startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																	case "秒":
																		endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																		startPoint = timeNow.Format("2006-01-02 15:04:05")
																	}
																	timeMap = map[string]interface{}{
																		"$gte": startPoint,
																		"$lt":  endPoint,
																	}
																} else {
																	eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																	if err != nil {
																		continue
																	}
																	queryTime = eleTime.Format("2006-01-02 15:04:05")
																}
															}
														}
														if len(timeMap) != 0 {
															orEleMap[orKey] = timeMap
														} else {
															orEleMap[orKey] = queryTime
														}
													}
												}
											} else if orKey == "createTime" || orKey == "modifyTime" {
												if timeMapRaw, ok := orVal.(map[string]interface{}); ok {
													for k, v := range timeMapRaw {
														if originTime, ok := v.(string); ok {
															var queryTime string
															var startPoint string
															var endPoint string
															timeMap := map[string]interface{}{}
															switch originTime {
															case "今天":
																startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															case "昨天":
																startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															case "明天":
																startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															case "本周":
																week := int(time.Now().Weekday())
																if week == 0 {
																	week = 7
																}
																startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															case "上周":
																week := int(time.Now().Weekday())
																if week == 0 {
																	week = 7
																}
																startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															case "今年":
																startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															case "去年":
																startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															case "明年":
																startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
																endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															default:
																if originTime != "" {
																	fromNow := false
																	if strings.HasPrefix(originTime, "前") {
																		//fmt.Println("originTime:", originTime)
																		if strings.HasSuffix(originTime, "当前") {
																			fromNow = true
																			originTime = strings.ReplaceAll(originTime, "当前", "")
																			//fmt.Println("originTime after 当前:", originTime)
																		}
																		intervalNumberString := numberx.GetNumberExp(originTime)
																		intervalNumber, err := strconv.Atoi(intervalNumberString)
																		if err != nil {
																			logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																			continue
																		}
																		interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																		//fmt.Println("intervalNumber:", intervalNumber)
																		//fmt.Println("interval:", interval)
																		switch interval {
																		case "天":
																			if fromNow {
																				//fmt.Println("fromNow")
																				startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																			}
																		case "周":
																			if fromNow {
																				startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				weekDay := int(timeNow.Weekday())
																				if weekDay == 0 {
																					weekDay = 7
																				}
																				startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																			}
																		case "月":
																			if fromNow {
																				startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																			}
																		case "年":
																			if fromNow {
																				startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																			}
																		case "季度":
																			if fromNow {
																				startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				timeNowMonth := int(timeNow.Month())
																				quarter1 := timeNowMonth - 1
																				quarter2 := timeNowMonth - 4
																				quarter3 := timeNowMonth - 7
																				quarter4 := timeNowMonth - 10
																				quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																				min := quarter1
																				for _, v := range quarterList {
																					if v < min {
																						min = v
																					}
																				}
																				startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																			}
																		case "时":
																			if fromNow {
																				startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																			}
																		case "分":
																			if fromNow {
																				startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																				endPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																				endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																			}
																		case "秒":
																			startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																			endPoint = timeNow.Format("2006-01-02 15:04:05")
																		}
																		timeMap = map[string]interface{}{
																			"$gte": startPoint,
																			"$lt":  endPoint,
																		}
																	} else if strings.HasPrefix(originTime, "后") {
																		if strings.HasSuffix(originTime, "当前") {
																			fromNow = true
																			originTime = strings.ReplaceAll(originTime, "当前", "")
																		}
																		intervalNumberString := numberx.GetNumberExp(originTime)
																		intervalNumber, err := strconv.Atoi(intervalNumberString)
																		if err != nil {
																			logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																			continue
																		}
																		interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																		switch interval {
																		case "天":
																			if fromNow {
																				endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																				startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																			}
																		case "周":
																			if fromNow {
																				endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				weekDay := int(timeNow.Weekday())
																				if weekDay == 0 {
																					weekDay = 7
																				}
																				endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																				startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																			}
																		case "月":
																			if fromNow {
																				endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																				startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																			}
																		case "年":
																			if fromNow {
																				endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																				startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																			}
																		case "季度":
																			if fromNow {
																				endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				timeNowMonth := int(timeNow.Month())
																				quarter1 := timeNowMonth - 1
																				quarter2 := timeNowMonth - 4
																				quarter3 := timeNowMonth - 7
																				quarter4 := timeNowMonth - 10
																				quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																				min := quarter1
																				for _, v := range quarterList {
																					if v < min {
																						min = v
																					}
																				}
																				endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																				startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																			}
																		case "时":
																			if fromNow {
																				endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																				startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																			}
																		case "分":
																			if fromNow {
																				endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																				startPoint = timeNow.Format("2006-01-02 15:04:05")
																			} else {
																				endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																				startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																			}
																		case "秒":
																			endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		}
																		timeMap = map[string]interface{}{
																			"$gte": startPoint,
																			"$lt":  endPoint,
																		}
																	} else if strings.HasPrefix(originTime, "当前") {
																		interval := strings.ReplaceAll(originTime, "当前", "")
																		switch interval {
																		case "天":
																			startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																		case "周":
																			weekDay := int(timeNow.Weekday())
																			if weekDay == 0 {
																				weekDay = 7
																			}
																			startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																		case "月":
																			startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																		case "年":
																			startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																		case "季度":
																			timeNowMonth := int(timeNow.Month())
																			quarter1 := timeNowMonth - 1
																			quarter2 := timeNowMonth - 4
																			quarter3 := timeNowMonth - 7
																			quarter4 := timeNowMonth - 10
																			quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																			min := quarter1
																			for _, v := range quarterList {
																				if v < min {
																					min = v
																				}
																			}
																			startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																		case "时":
																			startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																		case "分":
																			startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																			endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																		case "秒":
																			endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																			startPoint = timeNow.Format("2006-01-02 15:04:05")
																		}
																		timeMap = map[string]interface{}{
																			"$gte": startPoint,
																			"$lt":  endPoint,
																		}
																	} else {
																		eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																		if err != nil {
																			continue
																		}
																		queryTime = eleTime.Format("2006-01-02 15:04:05")
																	}
																}
															}
															if len(timeMap) != 0 {
																timeMapRaw[k] = timeMap
															} else {
																timeMapRaw[k] = queryTime
															}
														}
													}
												} else if originTime, ok := orVal.(string); ok {
													var queryTime string
													var startPoint string
													var endPoint string
													timeMap := map[string]interface{}{}
													switch originTime {
													case "今天":
														startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													case "昨天":
														startPoint = timex.GetUnixToNewTimeDay(-1).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													case "明天":
														startPoint = timex.GetUnixToNewTimeDay(1).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToNewTimeDay(2).Format("2006-01-02 15:04:05")
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													case "本周":
														week := int(time.Now().Weekday())
														if week == 0 {
															week = 7
														}
														startPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToNewTimeDay(8 - week).Format("2006-01-02 15:04:05")
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													case "上周":
														week := int(time.Now().Weekday())
														if week == 0 {
															week = 7
														}
														startPoint = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToNewTimeDay(-(week - 1)).Format("2006-01-02 15:04:05")
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													case "今年":
														startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													case "去年":
														startPoint = timex.GetUnixToOldYearTime(1, 0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													case "明年":
														startPoint = timex.GetUnixToOldYearTime(-1, 0).Format("2006-01-02 15:04:05")
														endPoint = timex.GetUnixToOldYearTime(-2, 0).Format("2006-01-02 15:04:05")
														timeMap = map[string]interface{}{
															"$gte": startPoint,
															"$lt":  endPoint,
														}
													default:
														if originTime != "" {
															fromNow := false
															if strings.HasPrefix(originTime, "前") {
																//fmt.Println("originTime:", originTime)
																if strings.HasSuffix(originTime, "当前") {
																	fromNow = true
																	originTime = strings.ReplaceAll(originTime, "当前", "")
																	//fmt.Println("originTime after 当前:", originTime)
																}
																intervalNumberString := numberx.GetNumberExp(originTime)
																intervalNumber, err := strconv.Atoi(intervalNumberString)
																if err != nil {
																	logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																	continue
																}
																interval := strings.ReplaceAll(originTime, "前"+intervalNumberString, "")
																//fmt.Println("intervalNumber:", intervalNumber)
																//fmt.Println("interval:", interval)
																switch interval {
																case "天":
																	if fromNow {
																		//fmt.Println("fromNow")
																		startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		endPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		startPoint = timex.GetUnixToNewTimeDay(-intervalNumber).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																	}
																case "周":
																	if fromNow {
																		startPoint = timex.GetFutureDayTimeSpecific(timeNow, -intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		endPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		weekDay := int(timeNow.Weekday())
																		if weekDay == 0 {
																			weekDay = 7
																		}
																		startPoint = timex.GetUnixToNewTimeDay(-intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																	}
																case "月":
																	if fromNow {
																		startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		endPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		startPoint = timex.GetUnixToOldYearTime(0, intervalNumber).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																	}
																case "年":
																	if fromNow {
																		startPoint = timex.GetFutureYearTimeSpecific(timeNow, -intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		endPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		startPoint = timex.GetUnixToOldYearTime(intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																	}
																case "季度":
																	if fromNow {
																		startPoint = timex.GetFutureMonthTimeSpecific(timeNow, -intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		endPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		timeNowMonth := int(timeNow.Month())
																		quarter1 := timeNowMonth - 1
																		quarter2 := timeNowMonth - 4
																		quarter3 := timeNowMonth - 7
																		quarter4 := timeNowMonth - 10
																		quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																		min := quarter1
																		for _, v := range quarterList {
																			if v < min {
																				min = v
																			}
																		}
																		startPoint = timex.GetUnixToOldYearTime(0, -(-intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetUnixToOldYearTime(0, min).Format("2006-01-02 15:04:05")
																	}
																case "时":
																	if fromNow {
																		startPoint = timex.GetFutureHourTimeSpecific(timeNow, -intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		endPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		startPoint = timex.GetLastServeralHoursFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																	}
																case "分":
																	if fromNow {
																		startPoint = timex.GetFutureMinuteTimeSpecific(timeNow, -intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																		endPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		startPoint = timex.GetLastServeralMinuteFromZero(intervalNumber).Format("2006-01-02 15:04:05")
																		endPoint = timex.GetLastServeralMinuteFromZero(0).Format("2006-01-02 15:04:05")
																	}
																case "秒":
																	startPoint = time.Unix(timeNow.Unix()-int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																	endPoint = timeNow.Format("2006-01-02 15:04:05")
																}
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															} else if strings.HasPrefix(originTime, "后") {
																if strings.HasSuffix(originTime, "当前") {
																	fromNow = true
																	originTime = strings.ReplaceAll(originTime, "当前", "")
																}
																intervalNumberString := numberx.GetNumberExp(originTime)
																intervalNumber, err := strconv.Atoi(intervalNumberString)
																if err != nil {
																	logger.Errorf("流程(%s)时间周期转数字失败:%s", flowID, err.Error())
																	continue
																}
																interval := strings.ReplaceAll(originTime, "后"+intervalNumberString, "")
																switch interval {
																case "天":
																	if fromNow {
																		endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		startPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		endPoint = timex.GetUnixToNewTimeDay(intervalNumber).Format("2006-01-02 15:04:05")
																		startPoint = timex.GetUnixToNewTimeDay(0).Format("2006-01-02 15:04:05")
																	}
																case "周":
																	if fromNow {
																		endPoint = timex.GetFutureDayTimeSpecific(timeNow, intervalNumber*7, timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		startPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		weekDay := int(timeNow.Weekday())
																		if weekDay == 0 {
																			weekDay = 7
																		}
																		endPoint = timex.GetUnixToNewTimeDay(intervalNumber*7 - weekDay + 1).Format("2006-01-02 15:04:05")
																		startPoint = timex.GetUnixToNewTimeDay(-weekDay + 1).Format("2006-01-02 15:04:05")
																	}
																case "月":
																	if fromNow {
																		endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		startPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		endPoint = timex.GetUnixToOldYearTime(0, -intervalNumber).Format("2006-01-02 15:04:05")
																		startPoint = timex.GetUnixToOldYearTime(0, 0).Format("2006-01-02 15:04:05")
																	}
																case "年":
																	if fromNow {
																		endPoint = timex.GetFutureYearTimeSpecific(timeNow, intervalNumber, int(timeNow.Month()), timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		startPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		endPoint = timex.GetUnixToOldYearTime(-intervalNumber,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																		startPoint = timex.GetUnixToOldYearTime(0,-(1-int(timeNow.Month()))).Format("2006-01-02 15:04:05")
																	}
																case "季度":
																	if fromNow {
																		endPoint = timex.GetFutureMonthTimeSpecific(timeNow, intervalNumber*3, timeNow.Day(), timeNow.Hour(), timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		startPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		timeNowMonth := int(timeNow.Month())
																		quarter1 := timeNowMonth - 1
																		quarter2 := timeNowMonth - 4
																		quarter3 := timeNowMonth - 7
																		quarter4 := timeNowMonth - 10
																		quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																		min := quarter1
																		for _, v := range quarterList {
																			if v < min {
																				min = v
																			}
																		}
																		endPoint = timex.GetUnixToOldYearTime(0,-(intervalNumber*3-min)).Format("2006-01-02 15:04:05")
																		startPoint = timex.GetUnixToOldYearTime(0,min).Format("2006-01-02 15:04:05")
																	}
																case "时":
																	if fromNow {
																		endPoint = timex.GetFutureHourTimeSpecific(timeNow, intervalNumber, timeNow.Minute(), timeNow.Second()).Format("2006-01-02 15:04:05")
																		startPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																		startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																	}
																case "分":
																	if fromNow {
																		endPoint = timex.GetFutureMinuteTimeSpecific(timeNow, intervalNumber, timeNow.Second()).Format("2006-01-02 15:04:05")
																		startPoint = timeNow.Format("2006-01-02 15:04:05")
																	} else {
																		endPoint = timex.GetLastServeralHoursFromZero(-intervalNumber).Format("2006-01-02 15:04:05")
																		startPoint = timex.GetLastServeralHoursFromZero(0).Format("2006-01-02 15:04:05")
																	}
																case "秒":
																	endPoint = time.Unix(timeNow.Unix()+int64(intervalNumber), 0).Format("2006-01-02 15:04:05")
																	startPoint = timeNow.Format("2006-01-02 15:04:05")
																}
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															} else if strings.HasPrefix(originTime, "当前") {
																interval := strings.ReplaceAll(originTime, "当前", "")
																switch interval {
																case "天":
																	startPoint = timex.GetFutureDayTime(timeNow, 0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetFutureDayTime(timeNow, 1).Format("2006-01-02 15:04:05")
																case "周":
																	weekDay := int(timeNow.Weekday())
																	if weekDay == 0 {
																		weekDay = 7
																	}
																	startPoint = timex.GetFutureDayTime(timeNow, -weekDay+1).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetFutureDayTime(timeNow, 7-weekDay+1).Format("2006-01-02 15:04:05")
																case "月":
																	startPoint = timex.GetFutureMonthTime(timeNow, 0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetFutureMonthTime(timeNow, 1).Format("2006-01-02 15:04:05")
																case "年":
																	startPoint = timex.GetFutureYearTime(timeNow, 0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetFutureYearTime(timeNow, 1).Format("2006-01-02 15:04:05")
																case "季度":
																	timeNowMonth := int(timeNow.Month())
																	quarter1 := timeNowMonth - 1
																	quarter2 := timeNowMonth - 4
																	quarter3 := timeNowMonth - 7
																	quarter4 := timeNowMonth - 10
																	quarterList := []int{quarter1, quarter2, quarter3, quarter4}
																	min := quarter1
																	for _, v := range quarterList {
																		if v < min {
																			min = v
																		}
																	}
																	startPoint = timex.GetFutureMonthTime(timeNow, -min).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetFutureMonthTime(timeNow, 3-min).Format("2006-01-02 15:04:05")
																case "时":
																	startPoint = timex.GetFutureHourTime(timeNow, 0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetFutureHourTime(timeNow, 1).Format("2006-01-02 15:04:05")
																case "分":
																	startPoint = timex.GetFutureMinuteTime(timeNow, 0).Format("2006-01-02 15:04:05")
																	endPoint = timex.GetFutureMinuteTime(timeNow, 1).Format("2006-01-02 15:04:05")
																case "秒":
																	endPoint = time.Unix(timeNow.Unix()+int64(1), 0).Format("2006-01-02 15:04:05")
																	startPoint = timeNow.Format("2006-01-02 15:04:05")
																}
																timeMap = map[string]interface{}{
																	"$gte": startPoint,
																	"$lt":  endPoint,
																}
															} else {
																eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
																if err != nil {
																	continue
																}
																queryTime = eleTime.Format("2006-01-02 15:04:05")
															}
														}
													}
													if len(timeMap) != 0 {
														orEleMap[orKey] = timeMap
													} else {
														orEleMap[orKey] = queryTime
													}
												}
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}

		////fmt.Println("after settings.EventType")

		tempTable, err := uuid.NewUUID()
		if err != nil {
			logger.Errorf("流程(%s)中生成临时表失败:%s", flowID, err.Error())
			continue
		}

		resultTable := map[string]interface{}{}
		err = table.Get(ctx, redisClient, mongoClient, projectName, settings.Table.ID, &resultTable)
		if err != nil {
			logger.Errorf("获取工作表定义失败:%s", err.Error())
			continue
		}

		tempTableName := settings.Table.Name + tempTable.String()
		tempID := primitive.NewObjectID().Hex()
		resultTable["id"] = tempID
		resultTable["name"] = tempTableName
		saveReturn := map[string]interface{}{}
		err = apiClient.SaveTable(headerMap, resultTable, &saveReturn)
		if err != nil {
			logger.Errorf("存储临时工作表定义失败:%s", err.Error())
			continue
		}
		var result interface{}
		defer func() {
			err = apiClient.DelTableById(headerMap, tempID, &result)
			if err != nil {
				logger.Errorf("删除临时工作表定义(%s)失败:%s", tempTableName, err.Error())
			}
		}()

		for key, val := range data {
			if val != nil {
				if extRaw, ok := excelColNameTypeExtMap[key]; ok {
					if extRaw.Format != "" {
						if originTime, ok := val.(string); ok {
							eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(originTime), originTime, time.Local)
							if err != nil {
								continue
							}
							switch extRaw.Format {
							case "date":
								data[key] = eleTime.Format("2006-01-02")
							case "datetime":
								data[key] = eleTime.Format("2006-01-02 15:04:05")
							case "time":
								data[key] = eleTime.Format("15:04:05")
							case "custom":
								data[key] = eleTime.Format(extRaw.Layout)
							}
						}
					}
				}
			}
		}

		err = apiClient.SaveExt(headerMap, tempTableName, data, &result)
		if err != nil {
			logger.Errorf("存储临时工作表记录失败:%s", err.Error())
			continue
		}

		//增加关联字段处理
		for key, val := range data {
			if val != nil {
				if extRaw, ok := excelColNameTypeExtMap[key]; ok {
					eleRaw, ok := val.(map[string]interface{})
					if ok {
						if extRaw.RelateField != "" {
							if relateVal, ok := eleRaw[extRaw.RelateField]; ok {
								data[key] = relateVal
							}
						} else if extRaw.RelateTo != "" {
							if relateVal, ok := eleRaw["name"]; ok {
								data[key] = relateVal
							}
						}
					}
				}
			}
		}

		////fmt.Println("settings.Query:",settings.Query)

		queryResult := make([]interface{}, 0)
		err = apiClient.FindExtQuery(headerMap, tempTableName, settings.Query, &queryResult)
		if err != nil {
			logger.Errorf("查询临时工作表记录失败:%s", err.Error())
			continue
		}
		////fmt.Println("queryResult:",queryResult)

		if len(queryResult) != 0 {
			isValid = true
		}
		//=================
		if isValid {

			err = flowx.StartFlow(zbClient, flowInfo.FlowXml, projectName, data)
			if err != nil {
				logger.Errorf("流程推进到下一阶段失败:%s", err.Error())
				continue
			}
			hasExecute = true
		}

		//对只能执行一次的流程进行失效
		if flowInfo.ValidTime == "timeLimit" {
			if flowInfo.Range == "once" && hasExecute {
				logger.Warnln("流程(%s)为只执行一次的流程", flowID)
				//修改流程为失效
				updateMap := bson.M{"invalid": true}
				//_, err := restfulapi.UpdateByID(context.Background(), idb.Database.Collection("event"), eventID, updateMap)
				var r = make(map[string]interface{})
				err := apiClient.UpdateFlowById(headerMap, flowID, updateMap, &r)
				if err != nil {
					logger.Errorf("失效流程(%s)失败:%s", flowID, err.Error())
					continue
				}
			}
		}
	}

	////logger.Debugf(eventDeviceModifyLog, "资产修改流程触发器执行结束")
	return nil
}

func getTableSchemaColsNameMap(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, projectName, id string) (map[string]ExcelColNameTypeExt, error) {
	//查询table中是否存在需要特殊处理的字段
	//queryMap := &bson.M{"filter": bson.M{"name": rawCollection}}
	tableInfo := bson.M{}
	err := table.Get(ctx, redisClient, mongoClient, projectName, id, &tableInfo)
	//_, tableList, err := a.TableModel.Query(ctx, projectName, *queryMap)
	if err != nil {
		return nil, fmt.Errorf(err.Error())
	}

	resultMap := map[string]ExcelColNameTypeExt{}
	schemaTableMapping := map[string]string{
		"Node":       "node",
		"Model":      "model",
		"User":       "user",
		"Department": "dept",
		"Role":       "role",
	}
	schemaTableRelateNameMapping := map[string]string{
		"Node":       "id",
		"Model":      "name",
		"User":       "name",
		"Department": "id",
		"Role":       "name",
	}

	//if len(*tableList) != 0 {
	//	//选定要修改的
	//	tableInfo := (*tableList)[0]
	if schema, ok := tableInfo["schema"].(map[string]interface{}); ok {
		if properties, ok := schema["properties"].(map[string]interface{}); ok {
			for key, propertyVal := range properties {
				if propertyMap, ok := propertyVal.(map[string]interface{}); ok {
					if propertyType, ok := propertyMap["type"].(string); ok {
						if _, ok := propertyMap["title"].(string); ok {
							excelColNameTypeExt := ExcelColNameTypeExt{
								Name:     key,
								DataType: propertyType,
							}
							if need, ok := propertyMap["need"].(bool); ok {
								excelColNameTypeExt.Required = need
							}
							switch propertyType {
							case "number":
								if dbType, ok := propertyMap["dbType"].(string); ok {
									excelColNameTypeExt.DbType = dbType
								}
							case "string":
								if format, ok := propertyMap["format"].(string); ok {
									switch format {
									case "custom":
										excelColNameTypeExt.Format = format
										if layout, ok := propertyMap["layout"].(string); ok {
											excelColNameTypeExt.Layout = layout
										}
									default:
										excelColNameTypeExt.Format = format
									}
								}
							case "object":
								if relateTo, ok := propertyMap["relateTo"].(string); ok {
									excelColNameTypeExt.RelateTo = schemaTableMapping[relateTo]
									excelColNameTypeExt.FieldType = strings.ToLower(relateTo)
									excelColNameTypeExt.RelateName = schemaTableRelateNameMapping[relateTo]
								} else if relateMap, ok := propertyMap["relate"].(map[string]interface{}); ok {
									if relateTo, ok := relateMap["name"].(string); ok {
										excelColNameTypeExt.RelateTo = "ext_" + relateTo
										excelColNameTypeExt.FieldType = relateTo
										if fields, ok := relateMap["fields"].([]interface{}); ok {
											for _, field := range fields {
												if fieldMap, ok := field.(map[string]interface{}); ok {
													if fieldKey, ok := fieldMap["key"].(string); ok {
														excelColNameTypeExt.RelateField = fieldKey
													}
												}
											}
										}
									}
									if relateName, ok := relateMap["relateName"].(string); ok {
										excelColNameTypeExt.RelateName = relateName
									} else if fields, ok := relateMap["fields"].([]interface{}); ok {
										for _, field := range fields {
											if fieldMap, ok := field.(map[string]interface{}); ok {
												if key, ok := fieldMap["key"].(string); ok {
													excelColNameTypeExt.RelateName = key
													break
												}
											}
										}
									}
								} else if fieldType, ok := propertyMap["fieldType"].(string); ok {
									excelColNameTypeExt.SpecialObj = fieldType
								}
							}
							if uniqueRaw, ok := propertyMap["unique"]; ok {
								if unique, ok := uniqueRaw.(bool); ok {
									if unique {
										excelColNameTypeExt.Unique = true
									}
								}
							}
							excelColNameTypeExt.IsForeign = true
							resultMap[key] = excelColNameTypeExt
						}
					}
				}
			}
		}
	}
	//}

	return resultMap, nil
}

type ExcelColNameTypeExt struct {
	//列英文名
	Name string `json:"name" example:"列英文名"`
	//列值类型
	DataType string `json:"type" example:"列值类型"`
	//列关系类型
	FieldType string `json:"fieldType" example:"列关系类型"`
	//是否必填
	Required bool `json:"required" example:"true"`
	//关联字段
	RelateTo string `json:"relateTo" example:"Node"`
	//数据库数字类型
	DbType string `json:"dbType" example:"Double"`
	//时间格式
	Format string `json:"format" example:"date"`
	//日期layout
	Layout string `json:"layout" example:"2006"`
	//关联时的查询字段
	RelateName string `json:"relateName" example:"name"`
	//特殊对象字段
	SpecialObj string `json:"specialObj" example:"特殊对象字段"`
	//字段是否唯一
	Unique bool `json:"unique" example:"true"`
	//是否是外键
	IsForeign bool `json:"isForeign" example:"true"`
	//关联具体字段
	RelateField string `json:"relateField" example:"relateField-1ASC"`
}

func TimeConvertExt(tableFormat ExcelColNameTypeExt, eleRaw string) (int64, error) {
	timeVal := int64(0)
	eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
	if err != nil {
		format := tableFormat.Format
		layout := tableFormat.Layout
		switch format {
		case "date":
			queryTime, err := timex.ConvertStringToTime("2006-01-02", eleRaw, time.Local)
			if err != nil {
				return 0, err
			}
			timeVal = queryTime.Unix()
		case "datetime":
			queryTime, err := timex.ConvertStringToTime("2006-01-02 15:04:05", eleRaw, time.Local)
			if err != nil {
				return 0, err
			}
			timeVal = queryTime.Unix()
		case "time":
			queryTime, err := timex.ConvertStringToTime("15:04:05", eleRaw, time.Local)
			if err != nil {
				return 0, err
			}
			timeVal = queryTime.Unix()
		case "custom":
			queryTime, err := timex.ConvertStringToTime(layout, eleRaw, time.Local)
			if err != nil {
				return 0, err
			}
			timeVal = queryTime.Unix()
		}
	} else {
		timeVal = eleTime.Unix()
	}
	return timeVal, nil
}
