package flow

import (
	"context"
	"fmt"
	"github.com/air-iot/service/init/cache/entity"
	"github.com/air-iot/service/init/cache/table"
	"github.com/air-iot/service/logger"
	"github.com/air-iot/service/util/flowx"
	"github.com/air-iot/service/util/numberx"
	"github.com/camunda-cloud/zeebe/clients/go/pkg/zbc"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/gin/ginx"
	"github.com/air-iot/service/init/mq"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/util/timex"
)

var flowExtModifyLog = map[string]interface{}{"name": "工作表流程触发"}

func TriggerExtModifyFlow(ctx context.Context, redisClient redisdb.Client, mongoClient *mongo.Client, mq mq.MQ, apiClient api.Client, zbClient zbc.Client, projectName string, data map[string]interface{}) error {
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
		"新增或更新记录时": "新增或更新记录时",
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
			"type":               ExtModify,
			"settings.eventType": modifyTypeAfterMapping,
			//"settings.eventRange": "node",
		},
	}
	err := apiClient.FindFlowQuery(headerMap, query, &flowInfoList)
	if err != nil {
		return fmt.Errorf("获取工作表流程失败:%s", err.Error())
	}

	////logger.Debugf(eventDeviceModifyLog, "开始遍历流程列表")
flowloop:
	for _, flowInfo := range flowInfoList {

		//logger.Debugf(eventDeviceModifyLog, "开始分析流程")
		flowID := flowInfo.ID
		settings := flowInfo.Settings

		//判断是否已经失效
		if settings.Invalid {
			logger.Warnln("流程(%s)已经失效", flowID)
			continue
		}

		//判断禁用
		if settings.Disable {
			logger.Warnln("流程(%s)已经被禁用", flowID)
			continue
		}

		rangeDefine := ""
		if settings.ValidTime == "timeLimit" {
			if settings.Range != "once" {
				//判断有效期
				startTime := settings.StartTime
				formatLayout := timex.FormatTimeFormat(startTime)
				formatStartTime, err := timex.ConvertStringToTime(formatLayout, startTime, time.Local)
				if err != nil {
					logger.Errorf("开始时间范围字段值格式错误:%s", err.Error())
					continue
				}
				if timex.GetLocalTimeNow(time.Now()).Unix() < formatStartTime.Unix() {
					logger.Debugf("流程(%s)的定时任务开始时间未到，不执行", flowID)
					continue
				}

				endTime := settings.EndTime
				formatLayout = timex.FormatTimeFormat(endTime)
				formatEndTime, err := timex.ConvertStringToTime(formatLayout, endTime, time.Local)
				if err != nil {
					logger.Errorf("时间范围字段值格式错误:%s", err.Error())
					continue
				}
				if timex.GetLocalTimeNow(time.Now()).Unix() >= formatEndTime.Unix() {
					logger.Debugf("流程(%s)的定时任务结束时间已到，不执行", flowID)
					//修改流程为失效
					updateMap := bson.M{"settings.invalid": true}
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
		//=================
		switch settings.EventType {
		case "新增记录时":
			fmt.Println("新增记录时 projectName ;", projectName, "data:", data)
			counter := 0
		logicLoop:
			for _, logic := range settings.Logic {
				if logic.LogicType == "且" && counter != 0 {
					continue flowloop
				}
				switch logic.DataType {
				case "文本":
					switch logic.Relation {
					case "是":
						for _, compare := range logic.Compare {
							if compare.ID != "" {
								if data[logic.ID] != data[compare.ID] {
									counter++
									continue logicLoop
								}
							} else if data[logic.ID] != compare.Value {
								counter++
								continue logicLoop
							}
						}
					case "不是":
						for _, compare := range logic.Compare {
							if compare.ID != "" {
								if data[logic.ID] == data[compare.ID] {
									counter++
									continue logicLoop
								}
							} else if data[logic.ID] == compare.Value {
								counter++
								continue logicLoop
							}
						}
					case "包含":
						for _, compare := range logic.Compare {
							compareInValue := ""
							if ele, ok := compare.Value.(string); ok {
								compareInValue = ele
							}
							dataVal, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop
							}
							if compare.ID != "" {
								compareVal, ok := data[compare.ID].(string)
								if !ok {
									counter++
									continue logicLoop
								}
								if strings.Contains(dataVal, compareVal) {
									counter++
									continue logicLoop
								}
							} else if strings.Contains(dataVal, compareInValue) {
								counter++
								continue logicLoop
							}
						}
					case "不包含":
						for _, compare := range logic.Compare {
							compareInValue := ""
							if ele, ok := compare.Value.(string); ok {
								compareInValue = ele
							}
							dataVal, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop
							}
							if compare.ID != "" {
								compareVal, ok := data[compare.ID].(string)
								if !ok {
									counter++
									continue logicLoop
								}
								if !strings.Contains(dataVal, compareVal) {
									counter++
									continue logicLoop
								}
							} else if !strings.Contains(dataVal, compareInValue) {
								counter++
								continue logicLoop
							}
						}
					case "开始为":
						for _, compare := range logic.Compare {
							compareInValue := ""
							if ele, ok := compare.Value.(string); ok {
								compareInValue = ele
							}
							dataVal, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop
							}
							if compare.ID != "" {
								compareVal, ok := data[compare.ID].(string)
								if !ok {
									counter++
									continue logicLoop
								}
								if strings.HasPrefix(dataVal, compareVal) {
									counter++
									continue logicLoop
								}
							} else if strings.HasPrefix(dataVal, compareInValue) {
								counter++
								continue logicLoop
							}
						}
					case "结尾为":
						for _, compare := range logic.Compare {
							compareInValue := ""
							if ele, ok := compare.Value.(string); ok {
								compareInValue = ele
							}
							dataVal, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop
							}
							if compare.ID != "" {
								compareVal, ok := data[compare.ID].(string)
								if !ok {
									counter++
									continue logicLoop
								}
								if strings.HasSuffix(dataVal, compareVal) {
									counter++
									continue logicLoop
								}
							} else if strings.HasSuffix(dataVal, compareInValue) {
								counter++
								continue logicLoop
							}
						}
					case "为空":
						if data[logic.ID] != nil {
							counter++
							continue logicLoop
						}
					case "不为空":
						if data[logic.ID] == nil {
							counter++
							continue logicLoop
						}
					}
				case "选择器":
					switch logic.Relation {
					case "是", "等于":
						for _, compare := range logic.Compare {
							if compare.ID != "" {
								if data[logic.ID] != data[compare.ID] {
									counter++
									continue logicLoop
								}
							} else if data[logic.ID] != compare.Value {
								counter++
								continue logicLoop
							}
						}
					case "不是", "不等于":
						for _, compare := range logic.Compare {
							if compare.ID != "" {
								if data[logic.ID] == data[compare.ID] {
									counter++
									continue logicLoop
								}
							} else if data[logic.ID] == compare.Value {
								counter++
								continue logicLoop
							}
						}
					case "为空":
						if data[logic.ID] != nil {
							counter++
							continue logicLoop
						}
					case "不为空":
						if data[logic.ID] == nil {
							counter++
							continue logicLoop
						}
					}
				case "数值":
					switch logic.Relation {
					case "等于":
						for _, compare := range logic.Compare {
							if compare.ID != "" {
								if data[logic.ID] != data[compare.ID] {
									counter++
									continue logicLoop
								}
							} else if data[logic.ID] != compare.Value {
								counter++
								continue logicLoop
							}
						}
					case "不等于":
						for _, compare := range logic.Compare {
							if compare.ID != "" {
								if data[logic.ID] == data[compare.ID] {
									counter++
									continue logicLoop
								}
							} else if data[logic.ID] == compare.Value {
								counter++
								continue logicLoop
							}
						}
					case "大于":
						for _, compare := range logic.Compare {
							logicVal, err := numberx.GetFloatNumber(data[logic.ID])
							if err != nil {
								counter++
								continue logicLoop
							}
							if compare.ID != "" {
								compareVal, err := numberx.GetFloatNumber(data[compare.ID])
								if err != nil {
									counter++
									continue logicLoop
								}
								if logicVal <= compareVal {
									counter++
									continue logicLoop
								}
							} else {
								compareVal, err := numberx.GetFloatNumber(compare.Value)
								if err != nil {
									counter++
									continue logicLoop
								}
								if logicVal <= compareVal {
									counter++
									continue logicLoop
								}
							}
						}
					case "小于":
						for _, compare := range logic.Compare {
							logicVal, err := numberx.GetFloatNumber(data[logic.ID])
							if err != nil {
								counter++
								continue logicLoop
							}
							if compare.ID != "" {
								compareVal, err := numberx.GetFloatNumber(data[compare.ID])
								if err != nil {
									counter++
									continue logicLoop
								}
								if logicVal >= compareVal {
									counter++
									continue logicLoop
								}
							} else {
								compareVal, err := numberx.GetFloatNumber(compare.Value)
								if err != nil {
									counter++
									continue logicLoop
								}
								if logicVal >= compareVal {
									counter++
									continue logicLoop
								}
							}
						}
					case "大于等于":
						for _, compare := range logic.Compare {
							logicVal, err := numberx.GetFloatNumber(data[logic.ID])
							if err != nil {
								counter++
								continue logicLoop
							}
							if compare.ID != "" {
								compareVal, err := numberx.GetFloatNumber(data[compare.ID])
								if err != nil {
									counter++
									continue logicLoop
								}
								if logicVal < compareVal {
									counter++
									continue logicLoop
								}
							} else {
								compareVal, err := numberx.GetFloatNumber(compare.Value)
								if err != nil {
									counter++
									continue logicLoop
								}
								if logicVal < compareVal {
									counter++
									continue logicLoop
								}
							}
						}
					case "小于等于":
						for _, compare := range logic.Compare {
							logicVal, err := numberx.GetFloatNumber(data[logic.ID])
							if err != nil {
								counter++
								continue logicLoop
							}
							if compare.ID != "" {
								compareVal, err := numberx.GetFloatNumber(data[compare.ID])
								if err != nil {
									counter++
									continue logicLoop
								}
								if logicVal > compareVal {
									counter++
									continue logicLoop
								}
							} else {
								compareVal, err := numberx.GetFloatNumber(compare.Value)
								if err != nil {
									counter++
									continue logicLoop
								}
								if logicVal > compareVal {
									counter++
									continue logicLoop
								}
							}
						}
					case "在范围内":
						for _, compare := range logic.Compare {
							logicVal, err := numberx.GetFloatNumber(data[logic.ID])
							if err != nil {
								counter++
								continue logicLoop
							}
							startVal := float64(0)
							endVal := float64(0)
							if compare.StartValue.ID != "" {
								startVal, err = numberx.GetFloatNumber(data[compare.StartValue.ID])
								if err != nil {
									counter++
									continue logicLoop
								}
							} else {
								startVal, err = numberx.GetFloatNumber(compare.StartValue.Value)
								if err != nil {
									counter++
									continue logicLoop
								}
							}
							if compare.EndValue.ID != "" {
								endVal, err = numberx.GetFloatNumber(data[compare.EndValue.ID])
								if err != nil {
									counter++
									continue logicLoop
								}
							} else {
								endVal, err = numberx.GetFloatNumber(compare.EndValue.Value)
								if err != nil {
									counter++
									continue logicLoop
								}
							}
							if logicVal < startVal || logicVal > endVal {
								counter++
								continue logicLoop
							}
						}
					case "不在范围内":
						for _, compare := range logic.Compare {
							logicVal, err := numberx.GetFloatNumber(data[logic.ID])
							if err != nil {
								counter++
								continue logicLoop
							}
							startVal := float64(0)
							endVal := float64(0)
							if compare.StartValue.ID != "" {
								startVal, err = numberx.GetFloatNumber(data[compare.StartValue.ID])
								if err != nil {
									counter++
									continue logicLoop
								}
							} else {
								startVal, err = numberx.GetFloatNumber(compare.StartValue.Value)
								if err != nil {
									counter++
									continue logicLoop
								}
							}
							if compare.EndValue.ID != "" {
								endVal, err = numberx.GetFloatNumber(data[compare.EndValue.ID])
								if err != nil {
									counter++
									continue logicLoop
								}
							} else {
								endVal, err = numberx.GetFloatNumber(compare.EndValue.Value)
								if err != nil {
									counter++
									continue logicLoop
								}
							}
							if logicVal >= startVal && logicVal <= endVal {
								counter++
								continue logicLoop
							}
						}
					case "为空":
						if data[logic.ID] != nil {
							counter++
							continue logicLoop
						}
					case "不为空":
						if data[logic.ID] == nil {
							counter++
							continue logicLoop
						}
					}
				case "时间":
					switch logic.Relation {
					case "等于":
						for _, compare := range logic.Compare {
							compareInValue := int64(0)
							if ele, ok := compare.Value.(string); ok {
								if ele != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
									if err != nil {
										counter++
										continue logicLoop
									}
									compareInValue = eleTime.Unix()
								}
							}
							dataVal := int64(0)
							eleRaw, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop
							} else {
								if eleRaw != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop
									}
									dataVal = eleTime.Unix()
								}
							}
							if compare.TimeType != "" {
								switch compare.TimeType {
								case "今天":
									compareInValue = timex.GetUnixToNewTimeDay(0).Unix()
								case "昨天":
									compareInValue = timex.GetUnixToNewTimeDay(-1).Unix()
								case "明天":
									compareInValue = timex.GetUnixToNewTimeDay(1).Unix()
								case "本周":
									week := int(time.Now().Weekday())
									if week == 0 {
										week = 7
									}
									compareInValue = timex.GetUnixToNewTimeDay(-(week - 1)).Unix()
								case "上周":
									week := int(time.Now().Weekday())
									if week == 0 {
										week = 7
									}
									compareInValue = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Unix()
								case "今年":
									compareInValue = timex.GetUnixToOldYearTime(0, 0).Unix()
								case "去年":
									compareInValue = timex.GetUnixToOldYearTime(1, 0).Unix()
								case "明年":
									compareInValue = timex.GetUnixToOldYearTime(-1, 0).Unix()
								case "指定时间":
									if compare.SpecificTime != "" {
										eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.SpecificTime), compare.SpecificTime, time.Local)
										if err != nil {
											counter++
											continue logicLoop
										}
										compareInValue = eleTime.Unix()
									}
								}
								if dataVal != compareInValue {
									counter++
									continue logicLoop
								}
							} else if compare.ID != "" {
								compareValue := int64(0)
								eleRaw, ok := data[compare.ID].(string)
								if !ok {
									counter++
									continue logicLoop
								} else {
									if eleRaw != "" {
										eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
										if err != nil {
											counter++
											continue logicLoop
										}
										compareValue = eleTime.Unix()
									}
								}
								if dataVal != compareValue {
									counter++
									continue logicLoop
								}
							} else if dataVal != compareInValue {
								counter++
								continue logicLoop
							} else if compareInValue == 0 {
								counter++
								continue logicLoop
							}
						}
					case "不等于":
						for _, compare := range logic.Compare {
							compareInValue := int64(0)
							if ele, ok := compare.Value.(string); ok {
								if ele != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
									if err != nil {
										counter++
										continue logicLoop
									}
									compareInValue = eleTime.Unix()
								}
							}
							dataVal := int64(0)
							eleRaw, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop
							} else {
								if eleRaw != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop
									}
									dataVal = eleTime.Unix()
								}
							}
							if compare.TimeType != "" {
								switch compare.TimeType {
								case "今天":
									compareInValue = timex.GetUnixToNewTimeDay(0).Unix()
								case "昨天":
									compareInValue = timex.GetUnixToNewTimeDay(-1).Unix()
								case "明天":
									compareInValue = timex.GetUnixToNewTimeDay(1).Unix()
								case "本周":
									week := int(time.Now().Weekday())
									if week == 0 {
										week = 7
									}
									compareInValue = timex.GetUnixToNewTimeDay(-(week - 1)).Unix()
								case "上周":
									week := int(time.Now().Weekday())
									if week == 0 {
										week = 7
									}
									compareInValue = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Unix()
								case "今年":
									compareInValue = timex.GetUnixToOldYearTime(0, 0).Unix()
								case "去年":
									compareInValue = timex.GetUnixToOldYearTime(1, 0).Unix()
								case "明年":
									compareInValue = timex.GetUnixToOldYearTime(-1, 0).Unix()
								case "指定时间":
									if compare.SpecificTime != "" {
										eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.SpecificTime), compare.SpecificTime, time.Local)
										if err != nil {
											counter++
											continue logicLoop
										}
										compareInValue = eleTime.Unix()
									}
								}
								if dataVal == compareInValue {
									counter++
									continue logicLoop
								}
							} else if dataVal == compareInValue {
								counter++
								continue logicLoop
							} else if compareInValue == 0 {
								counter++
								continue logicLoop
							}
						}
					case "早于":
						for _, compare := range logic.Compare {
							compareInValue := int64(0)
							if ele, ok := compare.Value.(string); ok {
								if ele != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
									if err != nil {
										counter++
										continue logicLoop
									}
									compareInValue = eleTime.Unix()
								}
							}
							dataVal := int64(0)
							eleRaw, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop
							} else {
								if eleRaw != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop
									}
									dataVal = eleTime.Unix()
								}
							}
							if compare.TimeType != "" {
								switch compare.TimeType {
								case "今天":
									compareInValue = timex.GetUnixToNewTimeDay(0).Unix()
								case "昨天":
									compareInValue = timex.GetUnixToNewTimeDay(-1).Unix()
								case "明天":
									compareInValue = timex.GetUnixToNewTimeDay(1).Unix()
								case "本周":
									week := int(time.Now().Weekday())
									if week == 0 {
										week = 7
									}
									compareInValue = timex.GetUnixToNewTimeDay(-(week - 1)).Unix()
								case "上周":
									week := int(time.Now().Weekday())
									if week == 0 {
										week = 7
									}
									compareInValue = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Unix()
								case "今年":
									compareInValue = timex.GetUnixToOldYearTime(0, 0).Unix()
								case "去年":
									compareInValue = timex.GetUnixToOldYearTime(1, 0).Unix()
								case "明年":
									compareInValue = timex.GetUnixToOldYearTime(-1, 0).Unix()
								case "指定时间":
									if compare.SpecificTime != "" {
										eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.SpecificTime), compare.SpecificTime, time.Local)
										if err != nil {
											counter++
											continue logicLoop
										}
										compareInValue = eleTime.Unix()
									}
								}
								if dataVal >= compareInValue {
									counter++
									continue logicLoop
								}
							} else if dataVal >= compareInValue {
								counter++
								continue logicLoop
							} else if compareInValue == 0 {
								counter++
								continue logicLoop
							}
						}
					case "晚于":
						for _, compare := range logic.Compare {
							compareInValue := int64(0)
							if ele, ok := compare.Value.(string); ok {
								if ele != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
									if err != nil {
										counter++
										continue logicLoop
									}
									compareInValue = eleTime.Unix()
								}
							}
							dataVal := int64(0)
							eleRaw, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop
							} else {
								if eleRaw != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop
									}
									dataVal = eleTime.Unix()
								}
							}
							if compare.TimeType != "" {
								switch compare.TimeType {
								case "今天":
									compareInValue = timex.GetUnixToNewTimeDay(0).Unix()
								case "昨天":
									compareInValue = timex.GetUnixToNewTimeDay(-1).Unix()
								case "明天":
									compareInValue = timex.GetUnixToNewTimeDay(1).Unix()
								case "本周":
									week := int(time.Now().Weekday())
									if week == 0 {
										week = 7
									}
									compareInValue = timex.GetUnixToNewTimeDay(-(week - 1)).Unix()
								case "上周":
									week := int(time.Now().Weekday())
									if week == 0 {
										week = 7
									}
									compareInValue = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Unix()
								case "今年":
									compareInValue = timex.GetUnixToOldYearTime(0, 0).Unix()
								case "去年":
									compareInValue = timex.GetUnixToOldYearTime(1, 0).Unix()
								case "明年":
									compareInValue = timex.GetUnixToOldYearTime(-1, 0).Unix()
								case "指定时间":
									if compare.SpecificTime != "" {
										eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.SpecificTime), compare.SpecificTime, time.Local)
										if err != nil {
											counter++
											continue logicLoop
										}
										compareInValue = eleTime.Unix()
									}
								}
								if dataVal <= compareInValue {
									counter++
									continue logicLoop
								}
							} else if dataVal <= compareInValue {
								counter++
								continue logicLoop
							} else if compareInValue == 0 {
								counter++
								continue logicLoop
							}
						}
					case "在范围内":
						for _, compare := range logic.Compare {
							logicVal := int64(0)
							eleRaw, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop
							} else {
								if eleRaw != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop
									}
									logicVal = eleTime.Unix()
								}
							}
							startVal := int64(0)
							endVal := int64(0)
							if compare.StartTime.ID != "" {
								if eleTimeRaw, ok := data[compare.StartTime.ID].(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop
									}
									startVal = eleTime.Unix()
								}
							} else if compare.StartTime.Value != "" {
								if eleTimeRaw, ok := compare.EndTime.Value.(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop
									}
									startVal = eleTime.Unix()
								}
							}
							if compare.EndTime.ID != "" {
								if eleTimeRaw, ok := data[compare.EndTime.ID].(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop
									}
									startVal = eleTime.Unix()
								}
							} else if compare.EndTime.Value != "" {
								if eleTimeRaw, ok := compare.EndTime.Value.(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop
									}
									endVal = eleTime.Unix()
								}
							}
							if logicVal < startVal || logicVal > endVal {
								counter++
								continue logicLoop
							}
						}
					case "不在范围内":
						for _, compare := range logic.Compare {
							logicVal := int64(0)
							eleRaw, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop
							} else {
								if eleRaw != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop
									}
									logicVal = eleTime.Unix()
								}
							}
							startVal := int64(0)
							endVal := int64(0)
							if compare.StartTime.ID != "" {
								if eleTimeRaw, ok := data[compare.StartTime.ID].(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop
									}
									startVal = eleTime.Unix()
								}
							} else if compare.StartTime.Value != "" {
								if eleTimeRaw, ok := compare.EndTime.Value.(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop
									}
									startVal = eleTime.Unix()
								}
							}
							if compare.EndTime.ID != "" {
								if eleTimeRaw, ok := data[compare.EndTime.ID].(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop
									}
									startVal = eleTime.Unix()
								}
							} else if compare.EndTime.Value != "" {
								if eleTimeRaw, ok := compare.EndTime.Value.(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop
									}
									endVal = eleTime.Unix()
								}
							}
							if logicVal < startVal || logicVal > endVal {
								counter++
								continue logicLoop
							}
						}
					case "为空":
						if data[logic.ID] != nil {
							counter++
							continue logicLoop
						}
					case "不为空":
						if data[logic.ID] == nil {
							counter++
							continue logicLoop
						}
					}
				case "布尔值", "附件", "定位":
					switch logic.Relation {
					case "为空":
						if data[logic.ID] != nil {
							counter++
							continue logicLoop
						}
					case "不为空":
						if data[logic.ID] == nil {
							counter++
							continue logicLoop
						}
					}
				case "关联字段":
					switch logic.Relation {
					case "是":
						for _, compare := range logic.Compare {
							var dataVal interface{}
							if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
								eleRaw, ok := data[logic.ID].(map[string]interface{})
								if !ok {
									counter++
									continue logicLoop
								} else {
									if eleRaw != nil {
										if relateVal, ok := eleRaw[extVal.RelateField]; ok {
											dataVal = relateVal
										} else {
											counter++
											continue logicLoop
										}
									}
								}
							}
							if compare.ID != "" {
								var compareValue interface{}
								if extVal, ok := excelColNameTypeExtMap[compare.ID]; ok {
									eleRaw, ok := data[compare.ID].(map[string]interface{})
									if !ok {
										counter++
										continue logicLoop
									} else {
										if eleRaw != nil {
											if relateVal, ok := eleRaw[extVal.RelateField]; ok {
												compareValue = relateVal
											} else {
												counter++
												continue logicLoop
											}
										}
									}
								}
								if dataVal != compareValue {
									counter++
									continue logicLoop
								}
							} else if dataVal != compare.Value {
								counter++
								continue logicLoop
							}
						}
					case "不是":
						for _, compare := range logic.Compare {
							var dataVal interface{}
							if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
								eleRaw, ok := data[logic.ID].(map[string]interface{})
								if !ok {
									counter++
									continue logicLoop
								} else {
									if eleRaw != nil {
										if relateVal, ok := eleRaw[extVal.RelateField]; ok {
											dataVal = relateVal
										} else {
											counter++
											continue logicLoop
										}
									}
								}
							}
							if compare.ID == "" {
								var compareValue interface{}
								if extVal, ok := excelColNameTypeExtMap[compare.ID]; ok {
									eleRaw, ok := data[compare.ID].(map[string]interface{})
									if !ok {
										counter++
										continue logicLoop
									} else {
										if eleRaw != nil {
											if relateVal, ok := eleRaw[extVal.RelateField]; ok {
												compareValue = relateVal
											} else {
												counter++
												continue logicLoop
											}
										}
									}
								}
								if dataVal != compareValue {
									counter++
									continue logicLoop
								}
							} else if dataVal == compare.Value {
								counter++
								continue logicLoop
							}
						}
					case "包含":
						for _, compare := range logic.Compare {
							compareInValue := ""
							if ele, ok := compare.Value.(string); ok {
								compareInValue = ele
							}
							dataVal := ""
							if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
								eleRaw, ok := data[logic.ID].(map[string]interface{})
								if !ok {
									counter++
									continue logicLoop
								} else {
									if eleRaw != nil {
										if relateVal, ok := eleRaw[extVal.RelateField]; ok {
											if relateValString, ok := relateVal.(string); ok {
												dataVal = relateValString
											} else {
												counter++
												continue logicLoop
											}
										} else {
											counter++
											continue logicLoop
										}
									}
								}
							}
							if compare.ID != "" {
								compareVal := ""
								if extVal, ok := excelColNameTypeExtMap[compare.ID]; ok {
									eleRaw, ok := data[compare.ID].(map[string]interface{})
									if !ok {
										counter++
										continue logicLoop
									} else {
										if eleRaw != nil {
											if relateVal, ok := eleRaw[extVal.RelateField]; ok {
												if relateValString, ok := relateVal.(string); ok {
													compareVal = relateValString
												} else {
													counter++
													continue logicLoop
												}
											} else {
												counter++
												continue logicLoop
											}
										}
									}
								}
								if strings.Contains(dataVal, compareVal) {
									counter++
									continue logicLoop
								}
							} else if strings.Contains(dataVal, compareInValue) {
								counter++
								continue logicLoop
							}
						}
					case "不包含":
						for _, compare := range logic.Compare {
							compareInValue := ""
							if ele, ok := compare.Value.(string); ok {
								compareInValue = ele
							}
							dataVal := ""
							if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
								eleRaw, ok := data[logic.ID].(map[string]interface{})
								if !ok {
									counter++
									continue logicLoop
								} else {
									if eleRaw != nil {
										if relateVal, ok := eleRaw[extVal.RelateField]; ok {
											if relateValString, ok := relateVal.(string); ok {
												dataVal = relateValString
											} else {
												counter++
												continue logicLoop
											}
										} else {
											counter++
											continue logicLoop
										}
									}
								}
							}
							if compare.ID != "" {
								compareVal := ""
								if extVal, ok := excelColNameTypeExtMap[compare.ID]; ok {
									eleRaw, ok := data[compare.ID].(map[string]interface{})
									if !ok {
										counter++
										continue logicLoop
									} else {
										if eleRaw != nil {
											if relateVal, ok := eleRaw[extVal.RelateField]; ok {
												if relateValString, ok := relateVal.(string); ok {
													compareVal = relateValString
												} else {
													counter++
													continue logicLoop
												}
											} else {
												counter++
												continue logicLoop
											}
										}
									}
								}
								if !strings.Contains(dataVal, compareVal) {
									counter++
									continue logicLoop
								}
							} else if !strings.Contains(dataVal, compareInValue) {
								counter++
								continue logicLoop
							}
						}
					case "为空":
						if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
							eleRaw, ok := data[logic.ID].(map[string]interface{})
							if ok {
								if eleRaw != nil {
									if relateVal, ok := eleRaw[extVal.RelateField]; ok {
										if relateVal != nil {
											counter++
											continue logicLoop
										}
									} else {
										counter++
										continue logicLoop
									}
								} else {
									counter++
									continue logicLoop
								}
							} else {
								counter++
								continue logicLoop
							}
						}
					case "不为空":
						if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
							eleRaw, ok := data[logic.ID].(map[string]interface{})
							if ok {
								if eleRaw != nil {
									if relateVal, ok := eleRaw[extVal.RelateField]; ok {
										if relateVal == nil {
											counter++
											continue logicLoop
										}
									} else {
										counter++
										continue logicLoop
									}
								} else {
									counter++
									continue logicLoop
								}
							} else {
								counter++
								continue logicLoop
							}
						}
					}
				}
			}
			if len(settings.Logic)/2+1 == counter {
				continue flowloop
			}
		case "更新记录时":
			fmt.Println("更新记录时 projectName ;", projectName, "data:", data)
			if _, ok := data[settings.UpdateField.ID]; !ok {
				continue
			}
			counter := 0
		logicLoop1:
			for _, logic := range settings.Logic {
				if logic.LogicType == "且" && counter != 0 {
					continue flowloop
				}
				switch logic.DataType {
				case "文本":
					switch logic.Relation {
					case "是":
						for _, compare := range logic.Compare {
							if compare.ID != "" {
								if data[logic.ID] != data[compare.ID] {
									counter++
									continue logicLoop1
								}
							} else if data[logic.ID] != compare.Value {
								counter++
								continue logicLoop1
							}
						}
					case "不是":
						for _, compare := range logic.Compare {
							if compare.ID != "" {
								if data[logic.ID] == data[compare.ID] {
									counter++
									continue logicLoop1
								}
							} else if data[logic.ID] == compare.Value {
								counter++
								continue logicLoop1
							}
						}
					case "包含":
						for _, compare := range logic.Compare {
							compareInValue := ""
							if ele, ok := compare.Value.(string); ok {
								compareInValue = ele
							}
							dataVal, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop1
							}
							if compare.ID != "" {
								compareVal, ok := data[compare.ID].(string)
								if !ok {
									counter++
									continue logicLoop1
								}
								if strings.Contains(dataVal, compareVal) {
									counter++
									continue logicLoop1
								}
							} else if strings.Contains(dataVal, compareInValue) {
								counter++
								continue logicLoop1
							}
						}
					case "不包含":
						for _, compare := range logic.Compare {
							compareInValue := ""
							if ele, ok := compare.Value.(string); ok {
								compareInValue = ele
							}
							dataVal, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop1
							}
							if compare.ID != "" {
								compareVal, ok := data[compare.ID].(string)
								if !ok {
									counter++
									continue logicLoop1
								}
								if !strings.Contains(dataVal, compareVal) {
									counter++
									continue logicLoop1
								}
							} else if !strings.Contains(dataVal, compareInValue) {
								counter++
								continue logicLoop1
							}
						}
					case "开始为":
						for _, compare := range logic.Compare {
							compareInValue := ""
							if ele, ok := compare.Value.(string); ok {
								compareInValue = ele
							}
							dataVal, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop1
							}
							if compare.ID != "" {
								compareVal, ok := data[compare.ID].(string)
								if !ok {
									counter++
									continue logicLoop1
								}
								if strings.HasPrefix(dataVal, compareVal) {
									counter++
									continue logicLoop1
								}
							} else if strings.HasPrefix(dataVal, compareInValue) {
								counter++
								continue logicLoop1
							}
						}
					case "结尾为":
						for _, compare := range logic.Compare {
							compareInValue := ""
							if ele, ok := compare.Value.(string); ok {
								compareInValue = ele
							}
							dataVal, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop1
							}
							if compare.ID != "" {
								compareVal, ok := data[compare.ID].(string)
								if !ok {
									counter++
									continue logicLoop1
								}
								if strings.HasSuffix(dataVal, compareVal) {
									counter++
									continue logicLoop1
								}
							} else if strings.HasSuffix(dataVal, compareInValue) {
								counter++
								continue logicLoop1
							}
						}
					case "为空":
						if data[logic.ID] != nil {
							counter++
							continue logicLoop1
						}
					case "不为空":
						if data[logic.ID] == nil {
							counter++
							continue logicLoop1
						}
					}
				case "选择器":
					switch logic.Relation {
					case "是", "等于":
						for _, compare := range logic.Compare {
							if compare.ID != "" {
								if data[logic.ID] != data[compare.ID] {
									counter++
									continue logicLoop1
								}
							} else if data[logic.ID] != compare.Value {
								counter++
								continue logicLoop1
							}
						}
					case "不是", "不等于":
						for _, compare := range logic.Compare {
							if compare.ID != "" {
								if data[logic.ID] == data[compare.ID] {
									counter++
									continue logicLoop1
								}
							} else if data[logic.ID] == compare.Value {
								counter++
								continue logicLoop1
							}
						}
					case "为空":
						if data[logic.ID] != nil {
							counter++
							continue logicLoop1
						}
					case "不为空":
						if data[logic.ID] == nil {
							counter++
							continue logicLoop1
						}
					}
				case "数值":
					switch logic.Relation {
					case "等于":
						for _, compare := range logic.Compare {
							if compare.ID != "" {
								if data[logic.ID] != data[compare.ID] {
									counter++
									continue logicLoop1
								}
							} else if data[logic.ID] != compare.Value {
								counter++
								continue logicLoop1
							}
						}
					case "不等于":
						for _, compare := range logic.Compare {
							if compare.ID != "" {
								if data[logic.ID] == data[compare.ID] {
									counter++
									continue logicLoop1
								}
							} else if data[logic.ID] == compare.Value {
								counter++
								continue logicLoop1
							}
						}
					case "大于":
						for _, compare := range logic.Compare {
							logicVal, err := numberx.GetFloatNumber(data[logic.ID])
							if err != nil {
								counter++
								continue logicLoop1
							}
							if compare.ID != "" {
								compareVal, err := numberx.GetFloatNumber(data[compare.ID])
								if err != nil {
									counter++
									continue logicLoop1
								}
								if logicVal <= compareVal {
									counter++
									continue logicLoop1
								}
							} else {
								compareVal, err := numberx.GetFloatNumber(compare.Value)
								if err != nil {
									counter++
									continue logicLoop1
								}
								if logicVal <= compareVal {
									counter++
									continue logicLoop1
								}
							}
						}
					case "小于":
						for _, compare := range logic.Compare {
							logicVal, err := numberx.GetFloatNumber(data[logic.ID])
							if err != nil {
								counter++
								continue logicLoop1
							}
							if compare.ID != "" {
								compareVal, err := numberx.GetFloatNumber(data[compare.ID])
								if err != nil {
									counter++
									continue logicLoop1
								}
								if logicVal >= compareVal {
									counter++
									continue logicLoop1
								}
							} else {
								compareVal, err := numberx.GetFloatNumber(compare.Value)
								if err != nil {
									counter++
									continue logicLoop1
								}
								if logicVal >= compareVal {
									counter++
									continue logicLoop1
								}
							}
						}
					case "大于等于":
						for _, compare := range logic.Compare {
							logicVal, err := numberx.GetFloatNumber(data[logic.ID])
							if err != nil {
								counter++
								continue logicLoop1
							}
							if compare.ID != "" {
								compareVal, err := numberx.GetFloatNumber(data[compare.ID])
								if err != nil {
									counter++
									continue logicLoop1
								}
								if logicVal < compareVal {
									counter++
									continue logicLoop1
								}
							} else {
								compareVal, err := numberx.GetFloatNumber(compare.Value)
								if err != nil {
									counter++
									continue logicLoop1
								}
								if logicVal < compareVal {
									counter++
									continue logicLoop1
								}
							}
						}
					case "小于等于":
						for _, compare := range logic.Compare {
							logicVal, err := numberx.GetFloatNumber(data[logic.ID])
							if err != nil {
								counter++
								continue logicLoop1
							}
							if compare.ID != "" {
								compareVal, err := numberx.GetFloatNumber(data[compare.ID])
								if err != nil {
									counter++
									continue logicLoop1
								}
								if logicVal > compareVal {
									counter++
									continue logicLoop1
								}
							} else {
								compareVal, err := numberx.GetFloatNumber(compare.Value)
								if err != nil {
									counter++
									continue logicLoop1
								}
								if logicVal > compareVal {
									counter++
									continue logicLoop1
								}
							}
						}
					case "在范围内":
						for _, compare := range logic.Compare {
							logicVal, err := numberx.GetFloatNumber(data[logic.ID])
							if err != nil {
								counter++
								continue logicLoop1
							}
							startVal := float64(0)
							endVal := float64(0)
							if compare.StartValue.ID != "" {
								startVal, err = numberx.GetFloatNumber(data[compare.StartValue.ID])
								if err != nil {
									counter++
									continue logicLoop1
								}
							} else {
								startVal, err = numberx.GetFloatNumber(compare.StartValue.Value)
								if err != nil {
									counter++
									continue logicLoop1
								}
							}
							if compare.EndValue.ID != "" {
								endVal, err = numberx.GetFloatNumber(data[compare.EndValue.ID])
								if err != nil {
									counter++
									continue logicLoop1
								}
							} else {
								endVal, err = numberx.GetFloatNumber(compare.EndValue.Value)
								if err != nil {
									counter++
									continue logicLoop1
								}
							}
							if logicVal < startVal || logicVal > endVal {
								counter++
								continue logicLoop1
							}
						}
					case "不在范围内":
						for _, compare := range logic.Compare {
							logicVal, err := numberx.GetFloatNumber(data[logic.ID])
							if err != nil {
								counter++
								continue logicLoop1
							}
							startVal := float64(0)
							endVal := float64(0)
							if compare.StartValue.ID != "" {
								startVal, err = numberx.GetFloatNumber(data[compare.StartValue.ID])
								if err != nil {
									counter++
									continue logicLoop1
								}
							} else {
								startVal, err = numberx.GetFloatNumber(compare.StartValue.Value)
								if err != nil {
									counter++
									continue logicLoop1
								}
							}
							if compare.EndValue.ID != "" {
								endVal, err = numberx.GetFloatNumber(data[compare.EndValue.ID])
								if err != nil {
									counter++
									continue logicLoop1
								}
							} else {
								endVal, err = numberx.GetFloatNumber(compare.EndValue.Value)
								if err != nil {
									counter++
									continue logicLoop1
								}
							}
							if logicVal >= startVal && logicVal <= endVal {
								counter++
								continue logicLoop1
							}
						}
					case "为空":
						if data[logic.ID] != nil {
							counter++
							continue logicLoop1
						}
					case "不为空":
						if data[logic.ID] == nil {
							counter++
							continue logicLoop1
						}
					}
				case "时间":
					switch logic.Relation {
					case "等于":
						for _, compare := range logic.Compare {
							compareInValue := int64(0)
							if ele, ok := compare.Value.(string); ok {
								if ele != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
									if err != nil {
										counter++
										continue logicLoop1
									}
									compareInValue = eleTime.Unix()
								}
							}
							dataVal := int64(0)
							eleRaw, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop1
							} else {
								if eleRaw != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop1
									}
									dataVal = eleTime.Unix()
								}
							}
							if compare.ID != "" {
								compareValue := int64(0)
								eleRaw, ok := data[compare.ID].(string)
								if !ok {
									counter++
									continue logicLoop1
								} else {
									if eleRaw != "" {
										eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
										if err != nil {
											counter++
											continue logicLoop1
										}
										compareValue = eleTime.Unix()
									}
								}
								if dataVal != compareValue {
									counter++
									continue logicLoop1
								}
							} else if dataVal != compareInValue {
								counter++
								continue logicLoop1
							} else if compareInValue == 0 {
								counter++
								continue logicLoop1
							}
						}
					case "不等于":
						for _, compare := range logic.Compare {
							compareInValue := int64(0)
							if ele, ok := compare.Value.(string); ok {
								if ele != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
									if err != nil {
										counter++
										continue logicLoop1
									}
									compareInValue = eleTime.Unix()
								}
							}
							dataVal := int64(0)
							eleRaw, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop1
							} else {
								if eleRaw != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop1
									}
									dataVal = eleTime.Unix()
								}
							}
							if compare.ID != "" {
								compareValue := int64(0)
								eleRaw, ok := data[compare.ID].(string)
								if !ok {
									counter++
									continue logicLoop1
								} else {
									if eleRaw != "" {
										eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
										if err != nil {
											counter++
											continue logicLoop1
										}
										compareValue = eleTime.Unix()
									}
								}
								if dataVal == compareValue {
									counter++
									continue logicLoop1
								}
							} else if dataVal == compareInValue {
								counter++
								continue logicLoop1
							} else if compareInValue == 0 {
								counter++
								continue logicLoop1
							}
						}
					case "早于":
						for _, compare := range logic.Compare {
							compareInValue := int64(0)
							if ele, ok := compare.Value.(string); ok {
								if ele != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
									if err != nil {
										counter++
										continue logicLoop1
									}
									compareInValue = eleTime.Unix()
								}
							}
							dataVal := int64(0)
							eleRaw, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop1
							} else {
								if eleRaw != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop1
									}
									dataVal = eleTime.Unix()
								}
							}
							if compare.ID != "" {
								compareValue := int64(0)
								eleRaw, ok := data[compare.ID].(string)
								if !ok {
									counter++
									continue logicLoop1
								} else {
									if eleRaw != "" {
										eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
										if err != nil {
											counter++
											continue logicLoop1
										}
										compareValue = eleTime.Unix()
									}
								}
								if dataVal >= compareValue {
									counter++
									continue logicLoop1
								}
							} else if dataVal >= compareInValue {
								counter++
								continue logicLoop1
							} else if compareInValue == 0 {
								counter++
								continue logicLoop1
							}
						}
					case "晚于":
						for _, compare := range logic.Compare {
							compareInValue := int64(0)
							if ele, ok := compare.Value.(string); ok {
								if ele != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
									if err != nil {
										counter++
										continue logicLoop1
									}
									compareInValue = eleTime.Unix()
								}
							}
							dataVal := int64(0)
							eleRaw, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop1
							} else {
								if eleRaw != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop1
									}
									dataVal = eleTime.Unix()
								}
							}
							if compare.ID != "" {
								compareValue := int64(0)
								eleRaw, ok := data[compare.ID].(string)
								if !ok {
									counter++
									continue logicLoop1
								} else {
									if eleRaw != "" {
										eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
										if err != nil {
											counter++
											continue logicLoop1
										}
										compareValue = eleTime.Unix()
									}
								}
								if dataVal <= compareValue {
									counter++
									continue logicLoop1
								}
							} else if dataVal <= compareInValue {
								counter++
								continue logicLoop1
							} else if compareInValue == 0 {
								counter++
								continue logicLoop1
							}
						}
					case "在范围内":
						for _, compare := range logic.Compare {
							logicVal := int64(0)
							eleRaw, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop1
							} else {
								if eleRaw != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop1
									}
									logicVal = eleTime.Unix()
								}
							}
							startVal := int64(0)
							endVal := int64(0)
							if compare.StartTime.ID != "" {
								if eleTimeRaw, ok := data[compare.StartTime.ID].(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop1
									}
									startVal = eleTime.Unix()
								}
							} else if compare.StartTime.Value != "" {
								if eleTimeRaw, ok := compare.EndTime.Value.(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop1
									}
									startVal = eleTime.Unix()
								}
							}
							if compare.EndTime.ID != "" {
								if eleTimeRaw, ok := data[compare.EndTime.ID].(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop1
									}
									startVal = eleTime.Unix()
								}
							} else if compare.EndTime.Value != "" {
								if eleTimeRaw, ok := compare.EndTime.Value.(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop1
									}
									endVal = eleTime.Unix()
								}
							}
							if logicVal < startVal || logicVal > endVal {
								counter++
								continue logicLoop1
							}
						}
					case "不在范围内":
						for _, compare := range logic.Compare {
							logicVal := int64(0)
							eleRaw, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop1
							} else {
								if eleRaw != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop1
									}
									logicVal = eleTime.Unix()
								}
							}
							startVal := int64(0)
							endVal := int64(0)
							if compare.StartTime.ID != "" {
								if eleTimeRaw, ok := data[compare.StartTime.ID].(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop1
									}
									startVal = eleTime.Unix()
								}
							} else if compare.StartTime.Value != "" {
								if eleTimeRaw, ok := compare.EndTime.Value.(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop1
									}
									startVal = eleTime.Unix()
								}
							}
							if compare.EndTime.ID != "" {
								if eleTimeRaw, ok := data[compare.EndTime.ID].(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop1
									}
									startVal = eleTime.Unix()
								}
							} else if compare.EndTime.Value != "" {
								if eleTimeRaw, ok := compare.EndTime.Value.(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop1
									}
									endVal = eleTime.Unix()
								}
							}
							if logicVal < startVal || logicVal > endVal {
								counter++
								continue logicLoop1
							}
						}
					case "为空":
						if data[logic.ID] != nil {
							counter++
							continue logicLoop1
						}
					case "不为空":
						if data[logic.ID] == nil {
							counter++
							continue logicLoop1
						}
					}
				case "布尔值", "附件", "定位":
					switch logic.Relation {
					case "为空":
						if data[logic.ID] != nil {
							counter++
							continue logicLoop1
						}
					case "不为空":
						if data[logic.ID] == nil {
							counter++
							continue logicLoop1
						}
					}
				case "关联字段":
					switch logic.Relation {
					case "是":
						for _, compare := range logic.Compare {
							var dataVal interface{}
							if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
								eleRaw, ok := data[logic.ID].(map[string]interface{})
								if !ok {
									counter++
									continue logicLoop1
								} else {
									if eleRaw != nil {
										if relateVal, ok := eleRaw[extVal.RelateField]; ok {
											dataVal = relateVal
										} else {
											counter++
											continue logicLoop1
										}
									}
								}
							}
							if compare.ID != "" {
								var compareValue interface{}
								if extVal, ok := excelColNameTypeExtMap[compare.ID]; ok {
									eleRaw, ok := data[compare.ID].(map[string]interface{})
									if !ok {
										counter++
										continue logicLoop1
									} else {
										if eleRaw != nil {
											if relateVal, ok := eleRaw[extVal.RelateField]; ok {
												compareValue = relateVal
											} else {
												counter++
												continue logicLoop1
											}
										}
									}
								}
								if dataVal != compareValue {
									counter++
									continue logicLoop1
								}
							} else if dataVal != compare.Value {
								counter++
								continue logicLoop1
							}
						}
					case "不是":
						for _, compare := range logic.Compare {
							var dataVal interface{}
							if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
								eleRaw, ok := data[logic.ID].(map[string]interface{})
								if !ok {
									counter++
									continue logicLoop1
								} else {
									if eleRaw != nil {
										if relateVal, ok := eleRaw[extVal.RelateField]; ok {
											dataVal = relateVal
										} else {
											counter++
											continue logicLoop1
										}
									}
								}
							}
							if compare.ID == "" {
								var compareValue interface{}
								if extVal, ok := excelColNameTypeExtMap[compare.ID]; ok {
									eleRaw, ok := data[compare.ID].(map[string]interface{})
									if !ok {
										counter++
										continue logicLoop1
									} else {
										if eleRaw != nil {
											if relateVal, ok := eleRaw[extVal.RelateField]; ok {
												compareValue = relateVal
											} else {
												counter++
												continue logicLoop1
											}
										}
									}
								}
								if dataVal != compareValue {
									counter++
									continue logicLoop1
								}
							} else if dataVal == compare.Value {
								counter++
								continue logicLoop1
							}
						}
					case "包含":
						for _, compare := range logic.Compare {
							compareInValue := ""
							if ele, ok := compare.Value.(string); ok {
								compareInValue = ele
							}
							dataVal := ""
							if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
								eleRaw, ok := data[logic.ID].(map[string]interface{})
								if !ok {
									counter++
									continue logicLoop1
								} else {
									if eleRaw != nil {
										if relateVal, ok := eleRaw[extVal.RelateField]; ok {
											if relateValString, ok := relateVal.(string); ok {
												dataVal = relateValString
											} else {
												counter++
												continue logicLoop1
											}
										} else {
											counter++
											continue logicLoop1
										}
									}
								}
							}
							if compare.ID != "" {
								compareVal := ""
								if extVal, ok := excelColNameTypeExtMap[compare.ID]; ok {
									eleRaw, ok := data[compare.ID].(map[string]interface{})
									if !ok {
										counter++
										continue logicLoop1
									} else {
										if eleRaw != nil {
											if relateVal, ok := eleRaw[extVal.RelateField]; ok {
												if relateValString, ok := relateVal.(string); ok {
													compareVal = relateValString
												} else {
													counter++
													continue logicLoop1
												}
											} else {
												counter++
												continue logicLoop1
											}
										}
									}
								}
								if strings.Contains(dataVal, compareVal) {
									counter++
									continue logicLoop1
								}
							} else if strings.Contains(dataVal, compareInValue) {
								counter++
								continue logicLoop1
							}
						}
					case "不包含":
						for _, compare := range logic.Compare {
							compareInValue := ""
							if ele, ok := compare.Value.(string); ok {
								compareInValue = ele
							}
							dataVal := ""
							if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
								eleRaw, ok := data[logic.ID].(map[string]interface{})
								if !ok {
									counter++
									continue logicLoop1
								} else {
									if eleRaw != nil {
										if relateVal, ok := eleRaw[extVal.RelateField]; ok {
											if relateValString, ok := relateVal.(string); ok {
												dataVal = relateValString
											} else {
												counter++
												continue logicLoop1
											}
										} else {
											counter++
											continue logicLoop1
										}
									}
								}
							}
							if compare.ID != "" {
								compareVal := ""
								if extVal, ok := excelColNameTypeExtMap[compare.ID]; ok {
									eleRaw, ok := data[compare.ID].(map[string]interface{})
									if !ok {
										counter++
										continue logicLoop1
									} else {
										if eleRaw != nil {
											if relateVal, ok := eleRaw[extVal.RelateField]; ok {
												if relateValString, ok := relateVal.(string); ok {
													compareVal = relateValString
												} else {
													counter++
													continue logicLoop1
												}
											} else {
												counter++
												continue logicLoop1
											}
										}
									}
								}
								if !strings.Contains(dataVal, compareVal) {
									counter++
									continue logicLoop1
								}
							} else if !strings.Contains(dataVal, compareInValue) {
								counter++
								continue logicLoop1
							}
						}
					case "为空":
						if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
							eleRaw, ok := data[logic.ID].(map[string]interface{})
							if ok {
								if eleRaw != nil {
									if relateVal, ok := eleRaw[extVal.RelateField]; ok {
										if relateVal != nil {
											counter++
											continue logicLoop1
										}
									} else {
										counter++
										continue logicLoop1
									}
								} else {
									counter++
									continue logicLoop1
								}
							} else {
								counter++
								continue logicLoop1
							}
						}
					case "不为空":
						if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
							eleRaw, ok := data[logic.ID].(map[string]interface{})
							if ok {
								if eleRaw != nil {
									if relateVal, ok := eleRaw[extVal.RelateField]; ok {
										if relateVal == nil {
											counter++
											continue logicLoop1
										}
									} else {
										counter++
										continue logicLoop1
									}
								} else {
									counter++
									continue logicLoop1
								}
							} else {
								counter++
								continue logicLoop1
							}
						}
					}
				}
			}
			if len(settings.Logic)/2+1 == counter {
				continue flowloop
			}
		case "删除记录时":
			fmt.Println("删除记录时 projectName ;", projectName, "data:", data)
			counter := 0
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
				switch settings.RangeType {
				case "按字段值":
				logicLoop2:
					for _, logic := range settings.Logic {
						if logic.LogicType == "且" && counter != 0 {
							continue flowloop
						}
						switch logic.DataType {
						case "文本":
							switch logic.Relation {
							case "是":
								for _, compare := range logic.Compare {
									if compare.ID != "" {
										if data[logic.ID] != data[compare.ID] {
											counter++
											continue logicLoop2
										}
									} else if data[logic.ID] != compare.Value {
										counter++
										continue logicLoop2
									}
								}
							case "不是":
								for _, compare := range logic.Compare {
									if compare.ID != "" {
										if data[logic.ID] == data[compare.ID] {
											counter++
											continue logicLoop2
										}
									} else if data[logic.ID] == compare.Value {
										counter++
										continue logicLoop2
									}
								}
							case "包含":
								for _, compare := range logic.Compare {
									compareInValue := ""
									if ele, ok := compare.Value.(string); ok {
										compareInValue = ele
									}
									dataVal, ok := data[logic.ID].(string)
									if !ok {
										counter++
										continue logicLoop2
									}
									if compare.ID != "" {
										compareVal, ok := data[compare.ID].(string)
										if !ok {
											counter++
											continue logicLoop2
										}
										if strings.Contains(dataVal, compareVal) {
											counter++
											continue logicLoop2
										}
									} else if strings.Contains(dataVal, compareInValue) {
										counter++
										continue logicLoop2
									}
								}
							case "不包含":
								for _, compare := range logic.Compare {
									compareInValue := ""
									if ele, ok := compare.Value.(string); ok {
										compareInValue = ele
									}
									dataVal, ok := data[logic.ID].(string)
									if !ok {
										counter++
										continue logicLoop2
									}
									if compare.ID != "" {
										compareVal, ok := data[compare.ID].(string)
										if !ok {
											counter++
											continue logicLoop2
										}
										if !strings.Contains(dataVal, compareVal) {
											counter++
											continue logicLoop2
										}
									} else if !strings.Contains(dataVal, compareInValue) {
										counter++
										continue logicLoop2
									}
								}
							case "开始为":
								for _, compare := range logic.Compare {
									compareInValue := ""
									if ele, ok := compare.Value.(string); ok {
										compareInValue = ele
									}
									dataVal, ok := data[logic.ID].(string)
									if !ok {
										counter++
										continue logicLoop2
									}
									if compare.ID != "" {
										compareVal, ok := data[compare.ID].(string)
										if !ok {
											counter++
											continue logicLoop2
										}
										if strings.HasPrefix(dataVal, compareVal) {
											counter++
											continue logicLoop2
										}
									} else if strings.HasPrefix(dataVal, compareInValue) {
										counter++
										continue logicLoop2
									}
								}
							case "结尾为":
								for _, compare := range logic.Compare {
									compareInValue := ""
									if ele, ok := compare.Value.(string); ok {
										compareInValue = ele
									}
									dataVal, ok := data[logic.ID].(string)
									if !ok {
										counter++
										continue logicLoop2
									}
									if compare.ID != "" {
										compareVal, ok := data[compare.ID].(string)
										if !ok {
											counter++
											continue logicLoop2
										}
										if strings.HasSuffix(dataVal, compareVal) {
											counter++
											continue logicLoop2
										}
									} else if strings.HasSuffix(dataVal, compareInValue) {
										counter++
										continue logicLoop2
									}
								}
							case "为空":
								if data[logic.ID] != nil {
									counter++
									continue logicLoop2
								}
							case "不为空":
								if data[logic.ID] == nil {
									counter++
									continue logicLoop2
								}
							}
						case "选择器":
							switch logic.Relation {
							case "是", "等于":
								for _, compare := range logic.Compare {
									if compare.ID != "" {
										if data[logic.ID] != data[compare.ID] {
											counter++
											continue logicLoop2
										}
									} else if data[logic.ID] != compare.Value {
										counter++
										continue logicLoop2
									}
								}
							case "不是", "不等于":
								for _, compare := range logic.Compare {
									if compare.ID != "" {
										if data[logic.ID] == data[compare.ID] {
											counter++
											continue logicLoop2
										}
									} else if data[logic.ID] == compare.Value {
										counter++
										continue logicLoop2
									}
								}
							case "为空":
								if data[logic.ID] != nil {
									counter++
									continue logicLoop2
								}
							case "不为空":
								if data[logic.ID] == nil {
									counter++
									continue logicLoop2
								}
							}
						case "数值":
							switch logic.Relation {
							case "等于":
								for _, compare := range logic.Compare {
									if compare.ID != "" {
										if data[logic.ID] != data[compare.ID] {
											counter++
											continue logicLoop2
										}
									} else if data[logic.ID] != compare.Value {
										counter++
										continue logicLoop2
									}
								}
							case "不等于":
								for _, compare := range logic.Compare {
									if compare.ID != "" {
										if data[logic.ID] == data[compare.ID] {
											counter++
											continue logicLoop2
										}
									} else if data[logic.ID] == compare.Value {
										counter++
										continue logicLoop2
									}
								}
							case "大于":
								for _, compare := range logic.Compare {
									logicVal, err := numberx.GetFloatNumber(data[logic.ID])
									if err != nil {
										counter++
										continue logicLoop2
									}
									if compare.ID != "" {
										compareVal, err := numberx.GetFloatNumber(data[compare.ID])
										if err != nil {
											counter++
											continue logicLoop2
										}
										if logicVal <= compareVal {
											counter++
											continue logicLoop2
										}
									} else {
										compareVal, err := numberx.GetFloatNumber(compare.Value)
										if err != nil {
											counter++
											continue logicLoop2
										}
										if logicVal <= compareVal {
											counter++
											continue logicLoop2
										}
									}
								}
							case "小于":
								for _, compare := range logic.Compare {
									logicVal, err := numberx.GetFloatNumber(data[logic.ID])
									if err != nil {
										counter++
										continue logicLoop2
									}
									if compare.ID != "" {
										compareVal, err := numberx.GetFloatNumber(data[compare.ID])
										if err != nil {
											counter++
											continue logicLoop2
										}
										if logicVal >= compareVal {
											counter++
											continue logicLoop2
										}
									} else {
										compareVal, err := numberx.GetFloatNumber(compare.Value)
										if err != nil {
											counter++
											continue logicLoop2
										}
										if logicVal >= compareVal {
											counter++
											continue logicLoop2
										}
									}
								}
							case "大于等于":
								for _, compare := range logic.Compare {
									logicVal, err := numberx.GetFloatNumber(data[logic.ID])
									if err != nil {
										counter++
										continue logicLoop2
									}
									if compare.ID != "" {
										compareVal, err := numberx.GetFloatNumber(data[compare.ID])
										if err != nil {
											counter++
											continue logicLoop2
										}
										if logicVal < compareVal {
											counter++
											continue logicLoop2
										}
									} else {
										compareVal, err := numberx.GetFloatNumber(compare.Value)
										if err != nil {
											counter++
											continue logicLoop2
										}
										if logicVal < compareVal {
											counter++
											continue logicLoop2
										}
									}
								}
							case "小于等于":
								for _, compare := range logic.Compare {
									logicVal, err := numberx.GetFloatNumber(data[logic.ID])
									if err != nil {
										counter++
										continue logicLoop2
									}
									if compare.ID != "" {
										compareVal, err := numberx.GetFloatNumber(data[compare.ID])
										if err != nil {
											counter++
											continue logicLoop2
										}
										if logicVal > compareVal {
											counter++
											continue logicLoop2
										}
									} else {
										compareVal, err := numberx.GetFloatNumber(compare.Value)
										if err != nil {
											counter++
											continue logicLoop2
										}
										if logicVal > compareVal {
											counter++
											continue logicLoop2
										}
									}
								}
							case "在范围内":
								for _, compare := range logic.Compare {
									logicVal, err := numberx.GetFloatNumber(data[logic.ID])
									if err != nil {
										counter++
										continue logicLoop2
									}
									startVal := float64(0)
									endVal := float64(0)
									if compare.StartValue.ID != "" {
										startVal, err = numberx.GetFloatNumber(data[compare.StartValue.ID])
										if err != nil {
											counter++
											continue logicLoop2
										}
									} else {
										startVal, err = numberx.GetFloatNumber(compare.StartValue.Value)
										if err != nil {
											counter++
											continue logicLoop2
										}
									}
									if compare.EndValue.ID != "" {
										endVal, err = numberx.GetFloatNumber(data[compare.EndValue.ID])
										if err != nil {
											counter++
											continue logicLoop2
										}
									} else {
										endVal, err = numberx.GetFloatNumber(compare.EndValue.Value)
										if err != nil {
											counter++
											continue logicLoop2
										}
									}
									if logicVal < startVal || logicVal > endVal {
										counter++
										continue logicLoop2
									}
								}
							case "不在范围内":
								for _, compare := range logic.Compare {
									logicVal, err := numberx.GetFloatNumber(data[logic.ID])
									if err != nil {
										counter++
										continue logicLoop2
									}
									startVal := float64(0)
									endVal := float64(0)
									if compare.StartValue.ID != "" {
										startVal, err = numberx.GetFloatNumber(data[compare.StartValue.ID])
										if err != nil {
											counter++
											continue logicLoop2
										}
									} else {
										startVal, err = numberx.GetFloatNumber(compare.StartValue.Value)
										if err != nil {
											counter++
											continue logicLoop2
										}
									}
									if compare.EndValue.ID != "" {
										endVal, err = numberx.GetFloatNumber(data[compare.EndValue.ID])
										if err != nil {
											counter++
											continue logicLoop2
										}
									} else {
										endVal, err = numberx.GetFloatNumber(compare.EndValue.Value)
										if err != nil {
											counter++
											continue logicLoop2
										}
									}
									if logicVal >= startVal && logicVal <= endVal {
										counter++
										continue logicLoop2
									}
								}
							case "为空":
								if data[logic.ID] != nil {
									counter++
									continue logicLoop2
								}
							case "不为空":
								if data[logic.ID] == nil {
									counter++
									continue logicLoop2
								}
							}
						case "时间":
							switch logic.Relation {
							case "等于":
								for _, compare := range logic.Compare {
									compareInValue := int64(0)
									if ele, ok := compare.Value.(string); ok {
										if ele != "" {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
											if err != nil {
												counter++
												continue logicLoop2
											}
											compareInValue = eleTime.Unix()
										}
									}
									dataVal := int64(0)
									eleRaw, ok := data[logic.ID].(string)
									if !ok {
										counter++
										continue logicLoop2
									} else {
										if eleRaw != "" {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop2
											}
											dataVal = eleTime.Unix()
										}
									}
									if compare.TimeType != "" {
										switch compare.TimeType {
										case "今天":
											compareInValue = timex.GetUnixToNewTimeDay(0).Unix()
										case "昨天":
											compareInValue = timex.GetUnixToNewTimeDay(-1).Unix()
										case "明天":
											compareInValue = timex.GetUnixToNewTimeDay(1).Unix()
										case "本周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											compareInValue = timex.GetUnixToNewTimeDay(-(week - 1)).Unix()
										case "上周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											compareInValue = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Unix()
										case "今年":
											compareInValue = timex.GetUnixToOldYearTime(0, 0).Unix()
										case "去年":
											compareInValue = timex.GetUnixToOldYearTime(1, 0).Unix()
										case "明年":
											compareInValue = timex.GetUnixToOldYearTime(-1, 0).Unix()
										case "指定时间":
											if compare.SpecificTime != "" {
												eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.SpecificTime), compare.SpecificTime, time.Local)
												if err != nil {
													counter++
													continue logicLoop2
												}
												compareInValue = eleTime.Unix()
											}
										}
										if dataVal != compareInValue {
											counter++
											continue logicLoop2
										}
									} else if compare.ID != "" {
										compareValue := int64(0)
										eleRaw, ok := data[compare.ID].(string)
										if !ok {
											counter++
											continue logicLoop2
										} else {
											if eleRaw != "" {
												eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
												if err != nil {
													counter++
													continue logicLoop2
												}
												compareValue = eleTime.Unix()
											}
										}
										if dataVal != compareValue {
											counter++
											continue logicLoop2
										}
									} else if dataVal != compareInValue {
										counter++
										continue logicLoop2
									} else if compareInValue == 0 {
										counter++
										continue logicLoop2
									}
								}
							case "不等于":
								for _, compare := range logic.Compare {
									compareInValue := int64(0)
									if ele, ok := compare.Value.(string); ok {
										if ele != "" {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
											if err != nil {
												counter++
												continue logicLoop2
											}
											compareInValue = eleTime.Unix()
										}
									}
									dataVal := int64(0)
									eleRaw, ok := data[logic.ID].(string)
									if !ok {
										counter++
										continue logicLoop2
									} else {
										if eleRaw != "" {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop2
											}
											dataVal = eleTime.Unix()
										}
									}
									if compare.TimeType != "" {
										switch compare.TimeType {
										case "今天":
											compareInValue = timex.GetUnixToNewTimeDay(0).Unix()
										case "昨天":
											compareInValue = timex.GetUnixToNewTimeDay(-1).Unix()
										case "明天":
											compareInValue = timex.GetUnixToNewTimeDay(1).Unix()
										case "本周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											compareInValue = timex.GetUnixToNewTimeDay(-(week - 1)).Unix()
										case "上周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											compareInValue = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Unix()
										case "今年":
											compareInValue = timex.GetUnixToOldYearTime(0, 0).Unix()
										case "去年":
											compareInValue = timex.GetUnixToOldYearTime(1, 0).Unix()
										case "明年":
											compareInValue = timex.GetUnixToOldYearTime(-1, 0).Unix()
										case "指定时间":
											if compare.SpecificTime != "" {
												eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.SpecificTime), compare.SpecificTime, time.Local)
												if err != nil {
													counter++
													continue logicLoop2
												}
												compareInValue = eleTime.Unix()
											}
										}
										if dataVal == compareInValue {
											counter++
											continue logicLoop2
										}
									} else if dataVal == compareInValue {
										counter++
										continue logicLoop2
									} else if compareInValue == 0 {
										counter++
										continue logicLoop2
									}
								}
							case "早于":
								for _, compare := range logic.Compare {
									compareInValue := int64(0)
									if ele, ok := compare.Value.(string); ok {
										if ele != "" {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
											if err != nil {
												counter++
												continue logicLoop2
											}
											compareInValue = eleTime.Unix()
										}
									}
									dataVal := int64(0)
									eleRaw, ok := data[logic.ID].(string)
									if !ok {
										counter++
										continue logicLoop2
									} else {
										if eleRaw != "" {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop2
											}
											dataVal = eleTime.Unix()
										}
									}
									if compare.TimeType != "" {
										switch compare.TimeType {
										case "今天":
											compareInValue = timex.GetUnixToNewTimeDay(0).Unix()
										case "昨天":
											compareInValue = timex.GetUnixToNewTimeDay(-1).Unix()
										case "明天":
											compareInValue = timex.GetUnixToNewTimeDay(1).Unix()
										case "本周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											compareInValue = timex.GetUnixToNewTimeDay(-(week - 1)).Unix()
										case "上周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											compareInValue = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Unix()
										case "今年":
											compareInValue = timex.GetUnixToOldYearTime(0, 0).Unix()
										case "去年":
											compareInValue = timex.GetUnixToOldYearTime(1, 0).Unix()
										case "明年":
											compareInValue = timex.GetUnixToOldYearTime(-1, 0).Unix()
										case "指定时间":
											if compare.SpecificTime != "" {
												eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.SpecificTime), compare.SpecificTime, time.Local)
												if err != nil {
													counter++
													continue logicLoop2
												}
												compareInValue = eleTime.Unix()
											}
										}
										if dataVal >= compareInValue {
											counter++
											continue logicLoop2
										}
									} else if dataVal >= compareInValue {
										counter++
										continue logicLoop2
									} else if compareInValue == 0 {
										counter++
										continue logicLoop2
									}
								}
							case "晚于":
								for _, compare := range logic.Compare {
									compareInValue := int64(0)
									if ele, ok := compare.Value.(string); ok {
										if ele != "" {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
											if err != nil {
												counter++
												continue logicLoop2
											}
											compareInValue = eleTime.Unix()
										}
									}
									dataVal := int64(0)
									eleRaw, ok := data[logic.ID].(string)
									if !ok {
										counter++
										continue logicLoop2
									} else {
										if eleRaw != "" {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop2
											}
											dataVal = eleTime.Unix()
										}
									}
									if compare.TimeType != "" {
										switch compare.TimeType {
										case "今天":
											compareInValue = timex.GetUnixToNewTimeDay(0).Unix()
										case "昨天":
											compareInValue = timex.GetUnixToNewTimeDay(-1).Unix()
										case "明天":
											compareInValue = timex.GetUnixToNewTimeDay(1).Unix()
										case "本周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											compareInValue = timex.GetUnixToNewTimeDay(-(week - 1)).Unix()
										case "上周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											compareInValue = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Unix()
										case "今年":
											compareInValue = timex.GetUnixToOldYearTime(0, 0).Unix()
										case "去年":
											compareInValue = timex.GetUnixToOldYearTime(1, 0).Unix()
										case "明年":
											compareInValue = timex.GetUnixToOldYearTime(-1, 0).Unix()
										case "指定时间":
											if compare.SpecificTime != "" {
												eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.SpecificTime), compare.SpecificTime, time.Local)
												if err != nil {
													counter++
													continue logicLoop2
												}
												compareInValue = eleTime.Unix()
											}
										}
										if dataVal <= compareInValue {
											counter++
											continue logicLoop2
										}
									} else if dataVal <= compareInValue {
										counter++
										continue logicLoop2
									} else if compareInValue == 0 {
										counter++
										continue logicLoop2
									}
								}
							case "在范围内":
								for _, compare := range logic.Compare {
									logicVal := int64(0)
									eleRaw, ok := data[logic.ID].(string)
									if !ok {
										counter++
										continue logicLoop2
									} else {
										if eleRaw != "" {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop2
											}
											logicVal = eleTime.Unix()
										}
									}
									startVal := int64(0)
									endVal := int64(0)
									if compare.StartTime.ID != "" {
										if eleTimeRaw, ok := data[compare.StartTime.ID].(string); ok {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop2
											}
											startVal = eleTime.Unix()
										}
									} else if compare.StartTime.Value != "" {
										if eleTimeRaw, ok := compare.EndTime.Value.(string); ok {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop2
											}
											startVal = eleTime.Unix()
										}
									}
									if compare.EndTime.ID != "" {
										if eleTimeRaw, ok := data[compare.EndTime.ID].(string); ok {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop2
											}
											startVal = eleTime.Unix()
										}
									} else if compare.EndTime.Value != "" {
										if eleTimeRaw, ok := compare.EndTime.Value.(string); ok {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop2
											}
											endVal = eleTime.Unix()
										}
									}
									if logicVal < startVal || logicVal > endVal {
										counter++
										continue logicLoop2
									}
								}
							case "不在范围内":
								for _, compare := range logic.Compare {
									logicVal := int64(0)
									eleRaw, ok := data[logic.ID].(string)
									if !ok {
										counter++
										continue logicLoop2
									} else {
										if eleRaw != "" {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop2
											}
											logicVal = eleTime.Unix()
										}
									}
									startVal := int64(0)
									endVal := int64(0)
									if compare.StartTime.ID != "" {
										if eleTimeRaw, ok := data[compare.StartTime.ID].(string); ok {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop2
											}
											startVal = eleTime.Unix()
										}
									} else if compare.StartTime.Value != "" {
										if eleTimeRaw, ok := compare.EndTime.Value.(string); ok {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop2
											}
											startVal = eleTime.Unix()
										}
									}
									if compare.EndTime.ID != "" {
										if eleTimeRaw, ok := data[compare.EndTime.ID].(string); ok {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop2
											}
											startVal = eleTime.Unix()
										}
									} else if compare.EndTime.Value != "" {
										if eleTimeRaw, ok := compare.EndTime.Value.(string); ok {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop2
											}
											endVal = eleTime.Unix()
										}
									}
									if logicVal < startVal || logicVal > endVal {
										counter++
										continue logicLoop2
									}
								}
							case "为空":
								if data[logic.ID] != nil {
									counter++
									continue logicLoop2
								}
							case "不为空":
								if data[logic.ID] == nil {
									counter++
									continue logicLoop2
								}
							}
						case "布尔值", "附件", "定位":
							switch logic.Relation {
							case "为空":
								if data[logic.ID] != nil {
									counter++
									continue logicLoop2
								}
							case "不为空":
								if data[logic.ID] == nil {
									counter++
									continue logicLoop2
								}
							}
						case "关联字段":
							switch logic.Relation {
							case "是":
								for _, compare := range logic.Compare {
									var dataVal interface{}
									if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
										eleRaw, ok := data[logic.ID].(map[string]interface{})
										if !ok {
											counter++
											continue logicLoop2
										} else {
											if eleRaw != nil {
												if relateVal, ok := eleRaw[extVal.RelateField]; ok {
													dataVal = relateVal
												} else {
													counter++
													continue logicLoop2
												}
											}
										}
									}
									if compare.ID != "" {
										var compareValue interface{}
										if extVal, ok := excelColNameTypeExtMap[compare.ID]; ok {
											eleRaw, ok := data[compare.ID].(map[string]interface{})
											if !ok {
												counter++
												continue logicLoop2
											} else {
												if eleRaw != nil {
													if relateVal, ok := eleRaw[extVal.RelateField]; ok {
														compareValue = relateVal
													} else {
														counter++
														continue logicLoop2
													}
												}
											}
										}
										if dataVal != compareValue {
											counter++
											continue logicLoop2
										}
									} else if dataVal != compare.Value {
										counter++
										continue logicLoop2
									}
								}
							case "不是":
								for _, compare := range logic.Compare {
									var dataVal interface{}
									if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
										eleRaw, ok := data[logic.ID].(map[string]interface{})
										if !ok {
											counter++
											continue logicLoop2
										} else {
											if eleRaw != nil {
												if relateVal, ok := eleRaw[extVal.RelateField]; ok {
													dataVal = relateVal
												} else {
													counter++
													continue logicLoop2
												}
											}
										}
									}
									if compare.ID == "" {
										var compareValue interface{}
										if extVal, ok := excelColNameTypeExtMap[compare.ID]; ok {
											eleRaw, ok := data[compare.ID].(map[string]interface{})
											if !ok {
												counter++
												continue logicLoop2
											} else {
												if eleRaw != nil {
													if relateVal, ok := eleRaw[extVal.RelateField]; ok {
														compareValue = relateVal
													} else {
														counter++
														continue logicLoop2
													}
												}
											}
										}
										if dataVal != compareValue {
											counter++
											continue logicLoop2
										}
									} else if dataVal == compare.Value {
										counter++
										continue logicLoop2
									}
								}
							case "包含":
								for _, compare := range logic.Compare {
									compareInValue := ""
									if ele, ok := compare.Value.(string); ok {
										compareInValue = ele
									}
									dataVal := ""
									if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
										eleRaw, ok := data[logic.ID].(map[string]interface{})
										if !ok {
											counter++
											continue logicLoop2
										} else {
											if eleRaw != nil {
												if relateVal, ok := eleRaw[extVal.RelateField]; ok {
													if relateValString, ok := relateVal.(string); ok {
														dataVal = relateValString
													} else {
														counter++
														continue logicLoop2
													}
												} else {
													counter++
													continue logicLoop2
												}
											}
										}
									}
									if compare.ID != "" {
										compareVal := ""
										if extVal, ok := excelColNameTypeExtMap[compare.ID]; ok {
											eleRaw, ok := data[compare.ID].(map[string]interface{})
											if !ok {
												counter++
												continue logicLoop2
											} else {
												if eleRaw != nil {
													if relateVal, ok := eleRaw[extVal.RelateField]; ok {
														if relateValString, ok := relateVal.(string); ok {
															compareVal = relateValString
														} else {
															counter++
															continue logicLoop2
														}
													} else {
														counter++
														continue logicLoop2
													}
												}
											}
										}
										if strings.Contains(dataVal, compareVal) {
											counter++
											continue logicLoop2
										}
									} else if strings.Contains(dataVal, compareInValue) {
										counter++
										continue logicLoop2
									}
								}
							case "不包含":
								for _, compare := range logic.Compare {
									compareInValue := ""
									if ele, ok := compare.Value.(string); ok {
										compareInValue = ele
									}
									dataVal := ""
									if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
										eleRaw, ok := data[logic.ID].(map[string]interface{})
										if !ok {
											counter++
											continue logicLoop2
										} else {
											if eleRaw != nil {
												if relateVal, ok := eleRaw[extVal.RelateField]; ok {
													if relateValString, ok := relateVal.(string); ok {
														dataVal = relateValString
													} else {
														counter++
														continue logicLoop2
													}
												} else {
													counter++
													continue logicLoop2
												}
											}
										}
									}
									if compare.ID != "" {
										compareVal := ""
										if extVal, ok := excelColNameTypeExtMap[compare.ID]; ok {
											eleRaw, ok := data[compare.ID].(map[string]interface{})
											if !ok {
												counter++
												continue logicLoop2
											} else {
												if eleRaw != nil {
													if relateVal, ok := eleRaw[extVal.RelateField]; ok {
														if relateValString, ok := relateVal.(string); ok {
															compareVal = relateValString
														} else {
															counter++
															continue logicLoop2
														}
													} else {
														counter++
														continue logicLoop2
													}
												}
											}
										}
										if !strings.Contains(dataVal, compareVal) {
											counter++
											continue logicLoop2
										}
									} else if !strings.Contains(dataVal, compareInValue) {
										counter++
										continue logicLoop2
									}
								}
							case "为空":
								if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
									eleRaw, ok := data[logic.ID].(map[string]interface{})
									if ok {
										if eleRaw != nil {
											if relateVal, ok := eleRaw[extVal.RelateField]; ok {
												if relateVal != nil {
													counter++
													continue logicLoop2
												}
											} else {
												counter++
												continue logicLoop2
											}
										} else {
											counter++
											continue logicLoop2
										}
									} else {
										counter++
										continue logicLoop2
									}
								}
							case "不为空":
								if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
									eleRaw, ok := data[logic.ID].(map[string]interface{})
									if ok {
										if eleRaw != nil {
											if relateVal, ok := eleRaw[extVal.RelateField]; ok {
												if relateVal == nil {
													counter++
													continue logicLoop2
												}
											} else {
												counter++
												continue logicLoop2
											}
										} else {
											counter++
											continue logicLoop2
										}
									} else {
										counter++
										continue logicLoop2
									}
								}
							}
						}
					}
					if len(settings.Logic)/2+1 == counter {
						continue flowloop
					}
				case "按创建人员":
				logicLoop3:
					for _, logic := range settings.Logic {
						if logic.LogicType == "且" && counter != 0 {
							continue flowloop
						}
						switch logic.Relation {
						case "是":
							for _, compare := range logic.Compare {
								dataVal := ""
								eleRaw, ok := data["creator"].(map[string]interface{})
								if !ok {
									counter++
									continue logicLoop3
								} else {
									if eleRaw != nil {
										if relateVal, ok := eleRaw["id"]; ok {
											if relateValString, ok := relateVal.(string); ok {
												dataVal = relateValString
											} else {
												counter++
												continue logicLoop3
											}
										} else {
											counter++
											continue logicLoop3
										}
									}
								}
								if compare.ID != "" {
									if dataVal != compare.ID {
										counter++
										continue logicLoop3
									}
								}
							}
						case "不是":
							for _, compare := range logic.Compare {
								dataVal := ""
								eleRaw, ok := data["creator"].(map[string]interface{})
								if !ok {
									counter++
									continue logicLoop3
								} else {
									if eleRaw != nil {
										if relateVal, ok := eleRaw["id"]; ok {
											if relateValString, ok := relateVal.(string); ok {
												dataVal = relateValString
											} else {
												counter++
												continue logicLoop3
											}
										} else {
											counter++
											continue logicLoop3
										}
									}
								}
								if compare.ID != "" {
									if dataVal != compare.ID {
										counter++
										continue logicLoop3
									}
								}
							}
							//case "包含":
							//	for _, compare := range logic.Compare {
							//		compareInValue := ""
							//		if ele, ok := compare.Value.(string); ok {
							//			compareInValue = ele
							//		}
							//		dataVal, ok := data[logic.ID].(string)
							//		if !ok {
							//			counter++
							//			continue logicLoop2
							//		}
							//		if compare.ID != "" {
							//			compareVal, ok := data[compare.ID].(string)
							//			if !ok {
							//				counter++
							//				continue logicLoop2
							//			}
							//			if strings.Contains(dataVal, compareVal) {
							//				counter++
							//				continue logicLoop2
							//			}
							//		} else if strings.Contains(dataVal, compareInValue) {
							//			counter++
							//			continue logicLoop2
							//		}
							//	}
							//case "不包含":
							//	for _, compare := range logic.Compare {
							//		compareInValue := ""
							//		if ele, ok := compare.Value.(string); ok {
							//			compareInValue = ele
							//		}
							//		dataVal, ok := data[logic.ID].(string)
							//		if !ok {
							//			counter++
							//			continue logicLoop2
							//		}
							//		if compare.ID != "" {
							//			compareVal, ok := data[compare.ID].(string)
							//			if !ok {
							//				counter++
							//				continue logicLoop2
							//			}
							//			if !strings.Contains(dataVal, compareVal) {
							//				counter++
							//				continue logicLoop2
							//			}
							//		} else if !strings.Contains(dataVal, compareInValue) {
							//			counter++
							//			continue logicLoop2
							//		}
							//	}
							//case "开始为":
							//	for _, compare := range logic.Compare {
							//		compareInValue := ""
							//		if ele, ok := compare.Value.(string); ok {
							//			compareInValue = ele
							//		}
							//		dataVal, ok := data[logic.ID].(string)
							//		if !ok {
							//			counter++
							//			continue logicLoop2
							//		}
							//		if compare.ID != "" {
							//			compareVal, ok := data[compare.ID].(string)
							//			if !ok {
							//				counter++
							//				continue logicLoop2
							//			}
							//			if strings.HasPrefix(dataVal, compareVal) {
							//				counter++
							//				continue logicLoop2
							//			}
							//		} else if strings.HasPrefix(dataVal, compareInValue) {
							//			counter++
							//			continue logicLoop2
							//		}
							//	}
							//case "结尾为":
							//	for _, compare := range logic.Compare {
							//		compareInValue := ""
							//		if ele, ok := compare.Value.(string); ok {
							//			compareInValue = ele
							//		}
							//		dataVal, ok := data[logic.ID].(string)
							//		if !ok {
							//			counter++
							//			continue logicLoop2
							//		}
							//		if compare.ID != "" {
							//			compareVal, ok := data[compare.ID].(string)
							//			if !ok {
							//				counter++
							//				continue logicLoop2
							//			}
							//			if strings.HasSuffix(dataVal, compareVal) {
							//				counter++
							//				continue logicLoop2
							//			}
							//		} else if strings.HasSuffix(dataVal, compareInValue) {
							//			counter++
							//			continue logicLoop2
							//		}
							//	}
							//case "为空":
							//	if data[logic.ID] != nil {
							//		counter++
							//		continue logicLoop2
							//	}
							//case "不为空":
							//	if data[logic.ID] == nil {
							//		counter++
							//		continue logicLoop2
							//	}
						}
						//case "选择器":
						//	switch logic.Relation {
						//	case "是", "等于":
						//		for _, compare := range logic.Compare {
						//			if compare.ID != "" {
						//				if data[logic.ID] != data[compare.ID] {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else if data[logic.ID] != compare.Value {
						//				counter++
						//				continue logicLoop2
						//			}
						//		}
						//	case "不是", "不等于":
						//		for _, compare := range logic.Compare {
						//			if compare.ID != "" {
						//				if data[logic.ID] == data[compare.ID] {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else if data[logic.ID] == compare.Value {
						//				counter++
						//				continue logicLoop2
						//			}
						//		}
						//	case "为空":
						//		if data[logic.ID] != nil {
						//			counter++
						//			continue logicLoop2
						//		}
						//	case "不为空":
						//		if data[logic.ID] == nil {
						//			counter++
						//			continue logicLoop2
						//		}
						//	}
						//case "数值":
						//	switch logic.Relation {
						//	case "等于":
						//		for _, compare := range logic.Compare {
						//			if compare.ID != "" {
						//				if data[logic.ID] != data[compare.ID] {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else if data[logic.ID] != compare.Value {
						//				counter++
						//				continue logicLoop2
						//			}
						//		}
						//	case "不等于":
						//		for _, compare := range logic.Compare {
						//			if compare.ID != "" {
						//				if data[logic.ID] == data[compare.ID] {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else if data[logic.ID] == compare.Value {
						//				counter++
						//				continue logicLoop2
						//			}
						//		}
						//	case "大于":
						//		for _, compare := range logic.Compare {
						//			logicVal, err := numberx.GetFloatNumber(data[logic.ID])
						//			if err != nil {
						//				counter++
						//				continue logicLoop2
						//			}
						//			if compare.ID != "" {
						//				compareVal, err := numberx.GetFloatNumber(data[compare.ID])
						//				if err != nil {
						//					counter++
						//					continue logicLoop2
						//				}
						//				if logicVal <= compareVal {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else {
						//				compareVal, err := numberx.GetFloatNumber(compare.Value)
						//				if err != nil {
						//					counter++
						//					continue logicLoop2
						//				}
						//				if logicVal <= compareVal {
						//					counter++
						//					continue logicLoop2
						//				}
						//			}
						//		}
						//	case "小于":
						//		for _, compare := range logic.Compare {
						//			logicVal, err := numberx.GetFloatNumber(data[logic.ID])
						//			if err != nil {
						//				counter++
						//				continue logicLoop2
						//			}
						//			if compare.ID != "" {
						//				compareVal, err := numberx.GetFloatNumber(data[compare.ID])
						//				if err != nil {
						//					counter++
						//					continue logicLoop2
						//				}
						//				if logicVal >= compareVal {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else {
						//				compareVal, err := numberx.GetFloatNumber(compare.Value)
						//				if err != nil {
						//					counter++
						//					continue logicLoop2
						//				}
						//				if logicVal >= compareVal {
						//					counter++
						//					continue logicLoop2
						//				}
						//			}
						//		}
						//	case "大于等于":
						//		for _, compare := range logic.Compare {
						//			logicVal, err := numberx.GetFloatNumber(data[logic.ID])
						//			if err != nil {
						//				counter++
						//				continue logicLoop2
						//			}
						//			if compare.ID != "" {
						//				compareVal, err := numberx.GetFloatNumber(data[compare.ID])
						//				if err != nil {
						//					counter++
						//					continue logicLoop2
						//				}
						//				if logicVal < compareVal {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else {
						//				compareVal, err := numberx.GetFloatNumber(compare.Value)
						//				if err != nil {
						//					counter++
						//					continue logicLoop2
						//				}
						//				if logicVal < compareVal {
						//					counter++
						//					continue logicLoop2
						//				}
						//			}
						//		}
						//	case "小于等于":
						//		for _, compare := range logic.Compare {
						//			logicVal, err := numberx.GetFloatNumber(data[logic.ID])
						//			if err != nil {
						//				counter++
						//				continue logicLoop2
						//			}
						//			if compare.ID != "" {
						//				compareVal, err := numberx.GetFloatNumber(data[compare.ID])
						//				if err != nil {
						//					counter++
						//					continue logicLoop2
						//				}
						//				if logicVal > compareVal {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else {
						//				compareVal, err := numberx.GetFloatNumber(compare.Value)
						//				if err != nil {
						//					counter++
						//					continue logicLoop2
						//				}
						//				if logicVal > compareVal {
						//					counter++
						//					continue logicLoop2
						//				}
						//			}
						//		}
						//	case "在范围内":
						//		for _, compare := range logic.Compare {
						//			logicVal, err := numberx.GetFloatNumber(data[logic.ID])
						//			if err != nil {
						//				counter++
						//				continue logicLoop2
						//			}
						//			startVal := float64(0)
						//			endVal := float64(0)
						//			if compare.StartValue.ID != "" {
						//				startVal, err = numberx.GetFloatNumber(data[compare.StartValue.ID])
						//				if err != nil {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else {
						//				startVal, err = numberx.GetFloatNumber(compare.StartValue.Value)
						//				if err != nil {
						//					counter++
						//					continue logicLoop2
						//				}
						//			}
						//			if compare.EndValue.ID != "" {
						//				endVal, err = numberx.GetFloatNumber(data[compare.EndValue.ID])
						//				if err != nil {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else {
						//				endVal, err = numberx.GetFloatNumber(compare.EndValue.Value)
						//				if err != nil {
						//					counter++
						//					continue logicLoop2
						//				}
						//			}
						//			if logicVal < startVal || logicVal > endVal {
						//				counter++
						//				continue logicLoop2
						//			}
						//		}
						//	case "不在范围内":
						//		for _, compare := range logic.Compare {
						//			logicVal, err := numberx.GetFloatNumber(data[logic.ID])
						//			if err != nil {
						//				counter++
						//				continue logicLoop2
						//			}
						//			startVal := float64(0)
						//			endVal := float64(0)
						//			if compare.StartValue.ID != "" {
						//				startVal, err = numberx.GetFloatNumber(data[compare.StartValue.ID])
						//				if err != nil {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else {
						//				startVal, err = numberx.GetFloatNumber(compare.StartValue.Value)
						//				if err != nil {
						//					counter++
						//					continue logicLoop2
						//				}
						//			}
						//			if compare.EndValue.ID != "" {
						//				endVal, err = numberx.GetFloatNumber(data[compare.EndValue.ID])
						//				if err != nil {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else {
						//				endVal, err = numberx.GetFloatNumber(compare.EndValue.Value)
						//				if err != nil {
						//					counter++
						//					continue logicLoop2
						//				}
						//			}
						//			if logicVal >= startVal && logicVal <= endVal {
						//				counter++
						//				continue logicLoop2
						//			}
						//		}
						//	case "为空":
						//		if data[logic.ID] != nil {
						//			counter++
						//			continue logicLoop2
						//		}
						//	case "不为空":
						//		if data[logic.ID] == nil {
						//			counter++
						//			continue logicLoop2
						//		}
						//	}
						//case "时间":
						//	switch logic.Relation {
						//	case "等于":
						//		for _, compare := range logic.Compare {
						//			compareInValue := int64(0)
						//			if ele, ok := compare.Value.(string); ok {
						//				if ele != "" {
						//					eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
						//					if err != nil {
						//						counter++
						//						continue logicLoop2
						//					}
						//					compareInValue = eleTime.Unix()
						//				}
						//			}
						//			dataVal := int64(0)
						//			eleRaw, ok := data[logic.ID].(string)
						//			if !ok {
						//				counter++
						//				continue logicLoop2
						//			} else {
						//				if eleRaw != "" {
						//					eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
						//					if err != nil {
						//						counter++
						//						continue logicLoop2
						//					}
						//					dataVal = eleTime.Unix()
						//				}
						//			}
						//			if compare.TimeType != "" {
						//				switch compare.TimeType {
						//				case "今天":
						//					compareInValue = timex.GetUnixToNewTimeDay(0).Unix()
						//				case "昨天":
						//					compareInValue = timex.GetUnixToNewTimeDay(-1).Unix()
						//				case "明天":
						//					compareInValue = timex.GetUnixToNewTimeDay(1).Unix()
						//				case "本周":
						//					week := int(time.Now().Weekday())
						//					if week == 0 {
						//						week = 7
						//					}
						//					compareInValue = timex.GetUnixToNewTimeDay(-(week-1)).Unix()
						//				case "上周":
						//					week := int(time.Now().Weekday())
						//					if week == 0 {
						//						week = 7
						//					}
						//					compareInValue = timex.GetUnixToNewTimeDay(-(week-1+7)).Unix()
						//				case "今年":
						//					compareInValue = timex.GetUnixToOldYearTime(0, 0).Unix()
						//				case "去年":
						//					compareInValue = timex.GetUnixToOldYearTime(1, 0).Unix()
						//				case "明年":
						//					compareInValue = timex.GetUnixToOldYearTime(-1, 0).Unix()
						//				case "指定时间":
						//					if compare.SpecificTime != "" {
						//						eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.SpecificTime), compare.SpecificTime, time.Local)
						//						if err != nil {
						//							counter++
						//							continue logicLoop2
						//						}
						//						compareInValue = eleTime.Unix()
						//					}
						//				}
						//				if dataVal != compareInValue {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else if compare.ID != "" {
						//				compareValue := int64(0)
						//				eleRaw, ok := data[compare.ID].(string)
						//				if !ok {
						//					counter++
						//					continue logicLoop2
						//				} else {
						//					if eleRaw != "" {
						//						eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
						//						if err != nil {
						//							counter++
						//							continue logicLoop2
						//						}
						//						compareValue = eleTime.Unix()
						//					}
						//				}
						//				if dataVal != compareValue {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else if dataVal != compareInValue {
						//				counter++
						//				continue logicLoop2
						//			} else if compareInValue == 0 {
						//				counter++
						//				continue logicLoop2
						//			}
						//		}
						//	case "不等于":
						//		for _, compare := range logic.Compare {
						//			compareInValue := int64(0)
						//			if ele, ok := compare.Value.(string); ok {
						//				if ele != "" {
						//					eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
						//					if err != nil {
						//						counter++
						//						continue logicLoop2
						//					}
						//					compareInValue = eleTime.Unix()
						//				}
						//			}
						//			dataVal := int64(0)
						//			eleRaw, ok := data[logic.ID].(string)
						//			if !ok {
						//				counter++
						//				continue logicLoop2
						//			} else {
						//				if eleRaw != "" {
						//					eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
						//					if err != nil {
						//						counter++
						//						continue logicLoop2
						//					}
						//					dataVal = eleTime.Unix()
						//				}
						//			}
						//			if compare.TimeType != "" {
						//				switch compare.TimeType {
						//				case "今天":
						//					compareInValue = timex.GetUnixToNewTimeDay(0).Unix()
						//				case "昨天":
						//					compareInValue = timex.GetUnixToNewTimeDay(-1).Unix()
						//				case "明天":
						//					compareInValue = timex.GetUnixToNewTimeDay(1).Unix()
						//				case "本周":
						//					week := int(time.Now().Weekday())
						//					if week == 0 {
						//						week = 7
						//					}
						//					compareInValue = timex.GetUnixToNewTimeDay(-(week-1)).Unix()
						//				case "上周":
						//					week := int(time.Now().Weekday())
						//					if week == 0 {
						//						week = 7
						//					}
						//					compareInValue = timex.GetUnixToNewTimeDay(-(week-1+7)).Unix()
						//				case "今年":
						//					compareInValue = timex.GetUnixToOldYearTime(0, 0).Unix()
						//				case "去年":
						//					compareInValue = timex.GetUnixToOldYearTime(1, 0).Unix()
						//				case "明年":
						//					compareInValue = timex.GetUnixToOldYearTime(-1, 0).Unix()
						//				case "指定时间":
						//					if compare.SpecificTime != "" {
						//						eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.SpecificTime), compare.SpecificTime, time.Local)
						//						if err != nil {
						//							counter++
						//							continue logicLoop2
						//						}
						//						compareInValue = eleTime.Unix()
						//					}
						//				}
						//				if dataVal == compareInValue {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else if dataVal == compareInValue {
						//				counter++
						//				continue logicLoop2
						//			} else if compareInValue == 0 {
						//				counter++
						//				continue logicLoop2
						//			}
						//		}
						//	case "早于":
						//		for _, compare := range logic.Compare {
						//			compareInValue := int64(0)
						//			if ele, ok := compare.Value.(string); ok {
						//				if ele != "" {
						//					eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
						//					if err != nil {
						//						counter++
						//						continue logicLoop2
						//					}
						//					compareInValue = eleTime.Unix()
						//				}
						//			}
						//			dataVal := int64(0)
						//			eleRaw, ok := data[logic.ID].(string)
						//			if !ok {
						//				counter++
						//				continue logicLoop2
						//			} else {
						//				if eleRaw != "" {
						//					eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
						//					if err != nil {
						//						counter++
						//						continue logicLoop2
						//					}
						//					dataVal = eleTime.Unix()
						//				}
						//			}
						//			if compare.TimeType != "" {
						//				switch compare.TimeType {
						//				case "今天":
						//					compareInValue = timex.GetUnixToNewTimeDay(0).Unix()
						//				case "昨天":
						//					compareInValue = timex.GetUnixToNewTimeDay(-1).Unix()
						//				case "明天":
						//					compareInValue = timex.GetUnixToNewTimeDay(1).Unix()
						//				case "本周":
						//					week := int(time.Now().Weekday())
						//					if week == 0 {
						//						week = 7
						//					}
						//					compareInValue = timex.GetUnixToNewTimeDay(-(week-1)).Unix()
						//				case "上周":
						//					week := int(time.Now().Weekday())
						//					if week == 0 {
						//						week = 7
						//					}
						//					compareInValue = timex.GetUnixToNewTimeDay(-(week-1+7)).Unix()
						//				case "今年":
						//					compareInValue = timex.GetUnixToOldYearTime(0, 0).Unix()
						//				case "去年":
						//					compareInValue = timex.GetUnixToOldYearTime(1, 0).Unix()
						//				case "明年":
						//					compareInValue = timex.GetUnixToOldYearTime(-1, 0).Unix()
						//				case "指定时间":
						//					if compare.SpecificTime != "" {
						//						eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.SpecificTime), compare.SpecificTime, time.Local)
						//						if err != nil {
						//							counter++
						//							continue logicLoop2
						//						}
						//						compareInValue = eleTime.Unix()
						//					}
						//				}
						//				if dataVal >= compareInValue {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else if dataVal >= compareInValue {
						//				counter++
						//				continue logicLoop2
						//			} else if compareInValue == 0 {
						//				counter++
						//				continue logicLoop2
						//			}
						//		}
						//	case "晚于":
						//		for _, compare := range logic.Compare {
						//			compareInValue := int64(0)
						//			if ele, ok := compare.Value.(string); ok {
						//				if ele != "" {
						//					eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
						//					if err != nil {
						//						counter++
						//						continue logicLoop2
						//					}
						//					compareInValue = eleTime.Unix()
						//				}
						//			}
						//			dataVal := int64(0)
						//			eleRaw, ok := data[logic.ID].(string)
						//			if !ok {
						//				counter++
						//				continue logicLoop2
						//			} else {
						//				if eleRaw != "" {
						//					eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
						//					if err != nil {
						//						counter++
						//						continue logicLoop2
						//					}
						//					dataVal = eleTime.Unix()
						//				}
						//			}
						//			if compare.TimeType != "" {
						//				switch compare.TimeType {
						//				case "今天":
						//					compareInValue = timex.GetUnixToNewTimeDay(0).Unix()
						//				case "昨天":
						//					compareInValue = timex.GetUnixToNewTimeDay(-1).Unix()
						//				case "明天":
						//					compareInValue = timex.GetUnixToNewTimeDay(1).Unix()
						//				case "本周":
						//					week := int(time.Now().Weekday())
						//					if week == 0 {
						//						week = 7
						//					}
						//					compareInValue = timex.GetUnixToNewTimeDay(-(week-1)).Unix()
						//				case "上周":
						//					week := int(time.Now().Weekday())
						//					if week == 0 {
						//						week = 7
						//					}
						//					compareInValue = timex.GetUnixToNewTimeDay(-(week-1+7)).Unix()
						//				case "今年":
						//					compareInValue = timex.GetUnixToOldYearTime(0, 0).Unix()
						//				case "去年":
						//					compareInValue = timex.GetUnixToOldYearTime(1, 0).Unix()
						//				case "明年":
						//					compareInValue = timex.GetUnixToOldYearTime(-1, 0).Unix()
						//				case "指定时间":
						//					if compare.SpecificTime != "" {
						//						eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.SpecificTime), compare.SpecificTime, time.Local)
						//						if err != nil {
						//							counter++
						//							continue logicLoop2
						//						}
						//						compareInValue = eleTime.Unix()
						//					}
						//				}
						//				if dataVal <= compareInValue {
						//					counter++
						//					continue logicLoop2
						//				}
						//			}else if dataVal <= compareInValue {
						//				counter++
						//				continue logicLoop2
						//			} else if compareInValue == 0 {
						//				counter++
						//				continue logicLoop2
						//			}
						//		}
						//	case "在范围内":
						//		for _, compare := range logic.Compare {
						//			logicVal := int64(0)
						//			eleRaw, ok := data[logic.ID].(string)
						//			if !ok {
						//				counter++
						//				continue logicLoop2
						//			} else {
						//				if eleRaw != "" {
						//					eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
						//					if err != nil {
						//						counter++
						//						continue logicLoop2
						//					}
						//					logicVal = eleTime.Unix()
						//				}
						//			}
						//			startVal := int64(0)
						//			endVal := int64(0)
						//			if compare.StartTime != "" {
						//				eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.StartTime), compare.StartTime, time.Local)
						//				if err != nil {
						//					counter++
						//					continue logicLoop2
						//				}
						//				startVal = eleTime.Unix()
						//			}
						//			if compare.EndTime != "" {
						//				eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.EndTime), compare.EndTime, time.Local)
						//				if err != nil {
						//					counter++
						//					continue logicLoop2
						//				}
						//				endVal = eleTime.Unix()
						//			}
						//			if logicVal < startVal || logicVal > endVal {
						//				counter++
						//				continue logicLoop2
						//			}
						//		}
						//	case "不在范围内":
						//		for _, compare := range logic.Compare {
						//			logicVal := int64(0)
						//			eleRaw, ok := data[logic.ID].(string)
						//			if !ok {
						//				counter++
						//				continue logicLoop2
						//			} else {
						//				if eleRaw != "" {
						//					eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
						//					if err != nil {
						//						counter++
						//						continue logicLoop2
						//					}
						//					logicVal = eleTime.Unix()
						//				}
						//			}
						//			startVal := int64(0)
						//			endVal := int64(0)
						//			if compare.StartTime != "" {
						//				eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.StartTime), compare.StartTime, time.Local)
						//				if err != nil {
						//					counter++
						//					continue logicLoop2
						//				}
						//				startVal = eleTime.Unix()
						//			}
						//			if compare.EndTime != "" {
						//				eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.EndTime), compare.EndTime, time.Local)
						//				if err != nil {
						//					counter++
						//					continue logicLoop2
						//				}
						//				endVal = eleTime.Unix()
						//			}
						//			if logicVal < startVal || logicVal > endVal {
						//				counter++
						//				continue logicLoop2
						//			}
						//		}
						//	case "为空":
						//		if data[logic.ID] != nil {
						//			counter++
						//			continue logicLoop2
						//		}
						//	case "不为空":
						//		if data[logic.ID] == nil {
						//			counter++
						//			continue logicLoop2
						//		}
						//	}
						//case "布尔值", "附件", "定位":
						//	switch logic.Relation {
						//	case "为空":
						//		if data[logic.ID] != nil {
						//			counter++
						//			continue logicLoop2
						//		}
						//	case "不为空":
						//		if data[logic.ID] == nil {
						//			counter++
						//			continue logicLoop2
						//		}
						//	}
						//case "关联字段":
						//	switch logic.Relation {
						//	case "是":
						//		for _, compare := range logic.Compare {
						//			var dataVal interface{}
						//			if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
						//				eleRaw, ok := data[logic.ID].(map[string]interface{})
						//				if !ok {
						//					counter++
						//					continue logicLoop2
						//				} else {
						//					if eleRaw != nil {
						//						if relateVal, ok := eleRaw[extVal.RelateField]; ok {
						//							dataVal = relateVal
						//						} else {
						//							counter++
						//							continue logicLoop2
						//						}
						//					}
						//				}
						//			}
						//			if compare.ID != "" {
						//				var compareValue interface{}
						//				if extVal, ok := excelColNameTypeExtMap[compare.ID]; ok {
						//					eleRaw, ok := data[compare.ID].(map[string]interface{})
						//					if !ok {
						//						counter++
						//						continue logicLoop2
						//					} else {
						//						if eleRaw != nil {
						//							if relateVal, ok := eleRaw[extVal.RelateField]; ok {
						//								compareValue = relateVal
						//							} else {
						//								counter++
						//								continue logicLoop2
						//							}
						//						}
						//					}
						//				}
						//				if dataVal != compareValue {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else if dataVal != compare.Value {
						//				counter++
						//				continue logicLoop2
						//			}
						//		}
						//	case "不是":
						//		for _, compare := range logic.Compare {
						//			var dataVal interface{}
						//			if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
						//				eleRaw, ok := data[logic.ID].(map[string]interface{})
						//				if !ok {
						//					counter++
						//					continue logicLoop2
						//				} else {
						//					if eleRaw != nil {
						//						if relateVal, ok := eleRaw[extVal.RelateField]; ok {
						//							dataVal = relateVal
						//						} else {
						//							counter++
						//							continue logicLoop2
						//						}
						//					}
						//				}
						//			}
						//			if compare.ID == "" {
						//				var compareValue interface{}
						//				if extVal, ok := excelColNameTypeExtMap[compare.ID]; ok {
						//					eleRaw, ok := data[compare.ID].(map[string]interface{})
						//					if !ok {
						//						counter++
						//						continue logicLoop2
						//					} else {
						//						if eleRaw != nil {
						//							if relateVal, ok := eleRaw[extVal.RelateField]; ok {
						//								compareValue = relateVal
						//							} else {
						//								counter++
						//								continue logicLoop2
						//							}
						//						}
						//					}
						//				}
						//				if dataVal != compareValue {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else if dataVal == compare.Value {
						//				counter++
						//				continue logicLoop2
						//			}
						//		}
						//	case "包含":
						//		for _, compare := range logic.Compare {
						//			compareInValue := ""
						//			if ele, ok := compare.Value.(string); ok {
						//				compareInValue = ele
						//			}
						//			dataVal := ""
						//			if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
						//				eleRaw, ok := data[logic.ID].(map[string]interface{})
						//				if !ok {
						//					counter++
						//					continue logicLoop2
						//				} else {
						//					if eleRaw != nil {
						//						if relateVal, ok := eleRaw[extVal.RelateField]; ok {
						//							if relateValString, ok := relateVal.(string); ok {
						//								dataVal = relateValString
						//							} else {
						//								counter++
						//								continue logicLoop2
						//							}
						//						} else {
						//							counter++
						//							continue logicLoop2
						//						}
						//					}
						//				}
						//			}
						//			if compare.ID != "" {
						//				compareVal := ""
						//				if extVal, ok := excelColNameTypeExtMap[compare.ID]; ok {
						//					eleRaw, ok := data[compare.ID].(map[string]interface{})
						//					if !ok {
						//						counter++
						//						continue logicLoop2
						//					} else {
						//						if eleRaw != nil {
						//							if relateVal, ok := eleRaw[extVal.RelateField]; ok {
						//								if relateValString, ok := relateVal.(string); ok {
						//									compareVal = relateValString
						//								} else {
						//									counter++
						//									continue logicLoop2
						//								}
						//							} else {
						//								counter++
						//								continue logicLoop2
						//							}
						//						}
						//					}
						//				}
						//				if strings.Contains(dataVal, compareVal) {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else if strings.Contains(dataVal, compareInValue) {
						//				counter++
						//				continue logicLoop2
						//			}
						//		}
						//	case "不包含":
						//		for _, compare := range logic.Compare {
						//			compareInValue := ""
						//			if ele, ok := compare.Value.(string); ok {
						//				compareInValue = ele
						//			}
						//			dataVal := ""
						//			if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
						//				eleRaw, ok := data[logic.ID].(map[string]interface{})
						//				if !ok {
						//					counter++
						//					continue logicLoop2
						//				} else {
						//					if eleRaw != nil {
						//						if relateVal, ok := eleRaw[extVal.RelateField]; ok {
						//							if relateValString, ok := relateVal.(string); ok {
						//								dataVal = relateValString
						//							} else {
						//								counter++
						//								continue logicLoop2
						//							}
						//						} else {
						//							counter++
						//							continue logicLoop2
						//						}
						//					}
						//				}
						//			}
						//			if compare.ID != "" {
						//				compareVal := ""
						//				if extVal, ok := excelColNameTypeExtMap[compare.ID]; ok {
						//					eleRaw, ok := data[compare.ID].(map[string]interface{})
						//					if !ok {
						//						counter++
						//						continue logicLoop2
						//					} else {
						//						if eleRaw != nil {
						//							if relateVal, ok := eleRaw[extVal.RelateField]; ok {
						//								if relateValString, ok := relateVal.(string); ok {
						//									compareVal = relateValString
						//								} else {
						//									counter++
						//									continue logicLoop2
						//								}
						//							} else {
						//								counter++
						//								continue logicLoop2
						//							}
						//						}
						//					}
						//				}
						//				if !strings.Contains(dataVal, compareVal) {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else if !strings.Contains(dataVal, compareInValue) {
						//				counter++
						//				continue logicLoop2
						//			}
						//		}
						//	case "为空":
						//		if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
						//			eleRaw, ok := data[logic.ID].(map[string]interface{})
						//			if ok {
						//				if eleRaw != nil {
						//					if relateVal, ok := eleRaw[extVal.RelateField]; ok {
						//						if relateVal != nil {
						//							counter++
						//							continue logicLoop2
						//						}
						//					} else {
						//						counter++
						//						continue logicLoop2
						//					}
						//				} else {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else {
						//				counter++
						//				continue logicLoop2
						//			}
						//		}
						//	case "不为空":
						//		if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
						//			eleRaw, ok := data[logic.ID].(map[string]interface{})
						//			if ok {
						//				if eleRaw != nil {
						//					if relateVal, ok := eleRaw[extVal.RelateField]; ok {
						//						if relateVal == nil {
						//							counter++
						//							continue logicLoop2
						//						}
						//					} else {
						//						counter++
						//						continue logicLoop2
						//					}
						//				} else {
						//					counter++
						//					continue logicLoop2
						//				}
						//			} else {
						//				counter++
						//				continue logicLoop2
						//			}
						//		}
						//	}
					}
					if len(settings.Logic)/2+1 == counter {
						continue flowloop
					}
				case "按创建时间":
				logicLoop4:
					for _, logic := range settings.Logic {
						if logic.LogicType == "且" && counter != 0 {
							continue flowloop
						}
						switch logic.DataType {
						case "时间":
							switch logic.Relation {
							case "等于":
								for _, compare := range logic.Compare {
									compareInValue := int64(0)
									if ele, ok := compare.Value.(string); ok {
										if ele != "" {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
											if err != nil {
												counter++
												continue logicLoop4
											}
											compareInValue = eleTime.Unix()
										}
									}
									dataVal := int64(0)
									eleRaw, ok := data["createTime"].(string)
									if !ok {
										counter++
										continue logicLoop4
									} else {
										if eleRaw != "" {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop4
											}
											dataVal = eleTime.Unix()
										}
									}
									if compare.TimeType != "" {
										switch compare.TimeType {
										case "今天":
											compareInValue = timex.GetUnixToNewTimeDay(0).Unix()
										case "昨天":
											compareInValue = timex.GetUnixToNewTimeDay(-1).Unix()
										case "明天":
											compareInValue = timex.GetUnixToNewTimeDay(1).Unix()
										case "本周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											compareInValue = timex.GetUnixToNewTimeDay(-(week - 1)).Unix()
										case "上周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											compareInValue = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Unix()
										case "今年":
											compareInValue = timex.GetUnixToOldYearTime(0, 0).Unix()
										case "去年":
											compareInValue = timex.GetUnixToOldYearTime(1, 0).Unix()
										case "明年":
											compareInValue = timex.GetUnixToOldYearTime(-1, 0).Unix()
										case "指定时间":
											if compare.SpecificTime != "" {
												eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.SpecificTime), compare.SpecificTime, time.Local)
												if err != nil {
													counter++
													continue logicLoop4
												}
												compareInValue = eleTime.Unix()
											}
										}
										if dataVal != compareInValue {
											counter++
											continue logicLoop4
										}
									} else if compare.ID != "" {
										compareValue := int64(0)
										eleRaw, ok := data[compare.ID].(string)
										if !ok {
											counter++
											continue logicLoop4
										} else {
											if eleRaw != "" {
												eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
												if err != nil {
													counter++
													continue logicLoop4
												}
												compareValue = eleTime.Unix()
											}
										}
										if dataVal != compareValue {
											counter++
											continue logicLoop4
										}
									} else if dataVal != compareInValue {
										counter++
										continue logicLoop4
									} else if compareInValue == 0 {
										counter++
										continue logicLoop4
									}
								}
							case "不等于":
								for _, compare := range logic.Compare {
									compareInValue := int64(0)
									if ele, ok := compare.Value.(string); ok {
										if ele != "" {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
											if err != nil {
												counter++
												continue logicLoop4
											}
											compareInValue = eleTime.Unix()
										}
									}
									dataVal := int64(0)
									eleRaw, ok := data["createTime"].(string)
									if !ok {
										counter++
										continue logicLoop4
									} else {
										if eleRaw != "" {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop4
											}
											dataVal = eleTime.Unix()
										}
									}
									if compare.TimeType != "" {
										switch compare.TimeType {
										case "今天":
											compareInValue = timex.GetUnixToNewTimeDay(0).Unix()
										case "昨天":
											compareInValue = timex.GetUnixToNewTimeDay(-1).Unix()
										case "明天":
											compareInValue = timex.GetUnixToNewTimeDay(1).Unix()
										case "本周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											compareInValue = timex.GetUnixToNewTimeDay(-(week - 1)).Unix()
										case "上周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											compareInValue = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Unix()
										case "今年":
											compareInValue = timex.GetUnixToOldYearTime(0, 0).Unix()
										case "去年":
											compareInValue = timex.GetUnixToOldYearTime(1, 0).Unix()
										case "明年":
											compareInValue = timex.GetUnixToOldYearTime(-1, 0).Unix()
										case "指定时间":
											if compare.SpecificTime != "" {
												eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.SpecificTime), compare.SpecificTime, time.Local)
												if err != nil {
													counter++
													continue logicLoop4
												}
												compareInValue = eleTime.Unix()
											}
										}
										if dataVal == compareInValue {
											counter++
											continue logicLoop4
										}
									} else if dataVal == compareInValue {
										counter++
										continue logicLoop4
									} else if compareInValue == 0 {
										counter++
										continue logicLoop4
									}
								}
							case "早于":
								for _, compare := range logic.Compare {
									compareInValue := int64(0)
									if ele, ok := compare.Value.(string); ok {
										if ele != "" {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
											if err != nil {
												counter++
												continue logicLoop4
											}
											compareInValue = eleTime.Unix()
										}
									}
									dataVal := int64(0)
									eleRaw, ok := data["createTime"].(string)
									if !ok {
										counter++
										continue logicLoop4
									} else {
										if eleRaw != "" {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop4
											}
											dataVal = eleTime.Unix()
										}
									}
									if compare.TimeType != "" {
										switch compare.TimeType {
										case "今天":
											compareInValue = timex.GetUnixToNewTimeDay(0).Unix()
										case "昨天":
											compareInValue = timex.GetUnixToNewTimeDay(-1).Unix()
										case "明天":
											compareInValue = timex.GetUnixToNewTimeDay(1).Unix()
										case "本周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											compareInValue = timex.GetUnixToNewTimeDay(-(week - 1)).Unix()
										case "上周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											compareInValue = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Unix()
										case "今年":
											compareInValue = timex.GetUnixToOldYearTime(0, 0).Unix()
										case "去年":
											compareInValue = timex.GetUnixToOldYearTime(1, 0).Unix()
										case "明年":
											compareInValue = timex.GetUnixToOldYearTime(-1, 0).Unix()
										case "指定时间":
											if compare.SpecificTime != "" {
												eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.SpecificTime), compare.SpecificTime, time.Local)
												if err != nil {
													counter++
													continue logicLoop4
												}
												compareInValue = eleTime.Unix()
											}
										}
										if dataVal >= compareInValue {
											counter++
											continue logicLoop4
										}
									} else if dataVal >= compareInValue {
										counter++
										continue logicLoop4
									} else if compareInValue == 0 {
										counter++
										continue logicLoop4
									}
								}
							case "晚于":
								for _, compare := range logic.Compare {
									compareInValue := int64(0)
									if ele, ok := compare.Value.(string); ok {
										if ele != "" {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
											if err != nil {
												counter++
												continue logicLoop4
											}
											compareInValue = eleTime.Unix()
										}
									}
									dataVal := int64(0)
									eleRaw, ok := data["createTime"].(string)
									if !ok {
										counter++
										continue logicLoop4
									} else {
										if eleRaw != "" {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop4
											}
											dataVal = eleTime.Unix()
										}
									}
									if compare.TimeType != "" {
										switch compare.TimeType {
										case "今天":
											compareInValue = timex.GetUnixToNewTimeDay(0).Unix()
										case "昨天":
											compareInValue = timex.GetUnixToNewTimeDay(-1).Unix()
										case "明天":
											compareInValue = timex.GetUnixToNewTimeDay(1).Unix()
										case "本周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											compareInValue = timex.GetUnixToNewTimeDay(-(week - 1)).Unix()
										case "上周":
											week := int(time.Now().Weekday())
											if week == 0 {
												week = 7
											}
											compareInValue = timex.GetUnixToNewTimeDay(-(week - 1 + 7)).Unix()
										case "今年":
											compareInValue = timex.GetUnixToOldYearTime(0, 0).Unix()
										case "去年":
											compareInValue = timex.GetUnixToOldYearTime(1, 0).Unix()
										case "明年":
											compareInValue = timex.GetUnixToOldYearTime(-1, 0).Unix()
										case "指定时间":
											if compare.SpecificTime != "" {
												eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(compare.SpecificTime), compare.SpecificTime, time.Local)
												if err != nil {
													counter++
													continue logicLoop4
												}
												compareInValue = eleTime.Unix()
											}
										}
										if dataVal <= compareInValue {
											counter++
											continue logicLoop4
										}
									} else if dataVal <= compareInValue {
										counter++
										continue logicLoop4
									} else if compareInValue == 0 {
										counter++
										continue logicLoop4
									}
								}
							case "在范围内":
								for _, compare := range logic.Compare {
									logicVal := int64(0)
									eleRaw, ok := data["createTime"].(string)
									if !ok {
										counter++
										continue logicLoop4
									} else {
										if eleRaw != "" {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop4
											}
											logicVal = eleTime.Unix()
										}
									}
									startVal := int64(0)
									endVal := int64(0)
									if compare.StartTime.ID != "" {
										if eleTimeRaw, ok := data[compare.StartTime.ID].(string); ok {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop4
											}
											startVal = eleTime.Unix()
										}
									} else if compare.StartTime.Value != "" {
										if eleTimeRaw, ok := compare.EndTime.Value.(string); ok {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop4
											}
											startVal = eleTime.Unix()
										}
									}
									if compare.EndTime.ID != "" {
										if eleTimeRaw, ok := data[compare.EndTime.ID].(string); ok {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop4
											}
											startVal = eleTime.Unix()
										}
									} else if compare.EndTime.Value != "" {
										if eleTimeRaw, ok := compare.EndTime.Value.(string); ok {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop4
											}
											endVal = eleTime.Unix()
										}
									}
									if logicVal < startVal || logicVal > endVal {
										counter++
										continue logicLoop4
									}
								}
							case "不在范围内":
								for _, compare := range logic.Compare {
									logicVal := int64(0)
									eleRaw, ok := data["createTime"].(string)
									if !ok {
										counter++
										continue logicLoop4
									} else {
										if eleRaw != "" {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop4
											}
											logicVal = eleTime.Unix()
										}
									}
									startVal := int64(0)
									endVal := int64(0)
									if compare.StartTime.ID != "" {
										if eleTimeRaw, ok := data[compare.StartTime.ID].(string); ok {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop4
											}
											startVal = eleTime.Unix()
										}
									} else if compare.StartTime.Value != "" {
										if eleTimeRaw, ok := compare.EndTime.Value.(string); ok {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop4
											}
											startVal = eleTime.Unix()
										}
									}
									if compare.EndTime.ID != "" {
										if eleTimeRaw, ok := data[compare.EndTime.ID].(string); ok {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop4
											}
											startVal = eleTime.Unix()
										}
									} else if compare.EndTime.Value != "" {
										if eleTimeRaw, ok := compare.EndTime.Value.(string); ok {
											eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
											if err != nil {
												counter++
												continue logicLoop4
											}
											endVal = eleTime.Unix()
										}
									}
									if logicVal < startVal || logicVal > endVal {
										counter++
										continue logicLoop4
									}
								}
							case "为空":
								if data["createTime"] != nil {
									counter++
									continue logicLoop4
								}
							case "不为空":
								if data["createTime"] == nil {
									counter++
									continue logicLoop4
								}
							}
						}
					}
					if len(settings.Logic)/2+1 == counter {
						continue flowloop
					}
				}
			}
		case "新增或更新记录时":
			fmt.Println("新增或更新记录时 projectName ;", projectName, "data:", data)
			if settings.UpdateField.ID != "" {
				if _, ok := data[settings.UpdateField.ID]; !ok {
					continue
				}
			}
			counter := 0
		logicLoop5:
			for _, logic := range settings.Logic {
				if logic.LogicType == "且" && counter != 0 {
					continue flowloop
				}
				switch logic.DataType {
				case "文本":
					switch logic.Relation {
					case "是":
						for _, compare := range logic.Compare {
							if compare.ID != "" {
								if data[logic.ID] != data[compare.ID] {
									counter++
									continue logicLoop5
								}
							} else if data[logic.ID] != compare.Value {
								counter++
								continue logicLoop5
							}
						}
					case "不是":
						for _, compare := range logic.Compare {
							if compare.ID != "" {
								if data[logic.ID] == data[compare.ID] {
									counter++
									continue logicLoop5
								}
							} else if data[logic.ID] == compare.Value {
								counter++
								continue logicLoop5
							}
						}
					case "包含":
						for _, compare := range logic.Compare {
							compareInValue := ""
							if ele, ok := compare.Value.(string); ok {
								compareInValue = ele
							}
							dataVal, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop5
							}
							if compare.ID != "" {
								compareVal, ok := data[compare.ID].(string)
								if !ok {
									counter++
									continue logicLoop5
								}
								if strings.Contains(dataVal, compareVal) {
									counter++
									continue logicLoop5
								}
							} else if strings.Contains(dataVal, compareInValue) {
								counter++
								continue logicLoop5
							}
						}
					case "不包含":
						for _, compare := range logic.Compare {
							compareInValue := ""
							if ele, ok := compare.Value.(string); ok {
								compareInValue = ele
							}
							dataVal, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop5
							}
							if compare.ID != "" {
								compareVal, ok := data[compare.ID].(string)
								if !ok {
									counter++
									continue logicLoop5
								}
								if !strings.Contains(dataVal, compareVal) {
									counter++
									continue logicLoop5
								}
							} else if !strings.Contains(dataVal, compareInValue) {
								counter++
								continue logicLoop5
							}
						}
					case "开始为":
						for _, compare := range logic.Compare {
							compareInValue := ""
							if ele, ok := compare.Value.(string); ok {
								compareInValue = ele
							}
							dataVal, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop5
							}
							if compare.ID != "" {
								compareVal, ok := data[compare.ID].(string)
								if !ok {
									counter++
									continue logicLoop5
								}
								if strings.HasPrefix(dataVal, compareVal) {
									counter++
									continue logicLoop5
								}
							} else if strings.HasPrefix(dataVal, compareInValue) {
								counter++
								continue logicLoop5
							}
						}
					case "结尾为":
						for _, compare := range logic.Compare {
							compareInValue := ""
							if ele, ok := compare.Value.(string); ok {
								compareInValue = ele
							}
							dataVal, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop5
							}
							if compare.ID != "" {
								compareVal, ok := data[compare.ID].(string)
								if !ok {
									counter++
									continue logicLoop5
								}
								if strings.HasSuffix(dataVal, compareVal) {
									counter++
									continue logicLoop5
								}
							} else if strings.HasSuffix(dataVal, compareInValue) {
								counter++
								continue logicLoop5
							}
						}
					case "为空":
						if data[logic.ID] != nil {
							counter++
							continue logicLoop5
						}
					case "不为空":
						if data[logic.ID] == nil {
							counter++
							continue logicLoop5
						}
					}
				case "选择器":
					switch logic.Relation {
					case "是", "等于":
						for _, compare := range logic.Compare {
							if compare.ID != "" {
								if data[logic.ID] != data[compare.ID] {
									counter++
									continue logicLoop5
								}
							} else if data[logic.ID] != compare.Value {
								counter++
								continue logicLoop5
							}
						}
					case "不是", "不等于":
						for _, compare := range logic.Compare {
							if compare.ID != "" {
								if data[logic.ID] == data[compare.ID] {
									counter++
									continue logicLoop5
								}
							} else if data[logic.ID] == compare.Value {
								counter++
								continue logicLoop5
							}
						}
					case "为空":
						if data[logic.ID] != nil {
							counter++
							continue logicLoop5
						}
					case "不为空":
						if data[logic.ID] == nil {
							counter++
							continue logicLoop5
						}
					}
				case "数值":
					switch logic.Relation {
					case "等于":
						for _, compare := range logic.Compare {
							if compare.ID != "" {
								if data[logic.ID] != data[compare.ID] {
									counter++
									continue logicLoop5
								}
							} else if data[logic.ID] != compare.Value {
								counter++
								continue logicLoop5
							}
						}
					case "不等于":
						for _, compare := range logic.Compare {
							if compare.ID != "" {
								if data[logic.ID] == data[compare.ID] {
									counter++
									continue logicLoop5
								}
							} else if data[logic.ID] == compare.Value {
								counter++
								continue logicLoop5
							}
						}
					case "大于":
						for _, compare := range logic.Compare {
							logicVal, err := numberx.GetFloatNumber(data[logic.ID])
							if err != nil {
								counter++
								continue logicLoop5
							}
							if compare.ID != "" {
								compareVal, err := numberx.GetFloatNumber(data[compare.ID])
								if err != nil {
									counter++
									continue logicLoop5
								}
								if logicVal <= compareVal {
									counter++
									continue logicLoop5
								}
							} else {
								compareVal, err := numberx.GetFloatNumber(compare.Value)
								if err != nil {
									counter++
									continue logicLoop5
								}
								if logicVal <= compareVal {
									counter++
									continue logicLoop5
								}
							}
						}
					case "小于":
						for _, compare := range logic.Compare {
							logicVal, err := numberx.GetFloatNumber(data[logic.ID])
							if err != nil {
								counter++
								continue logicLoop5
							}
							if compare.ID != "" {
								compareVal, err := numberx.GetFloatNumber(data[compare.ID])
								if err != nil {
									counter++
									continue logicLoop5
								}
								if logicVal >= compareVal {
									counter++
									continue logicLoop5
								}
							} else {
								compareVal, err := numberx.GetFloatNumber(compare.Value)
								if err != nil {
									counter++
									continue logicLoop5
								}
								if logicVal >= compareVal {
									counter++
									continue logicLoop5
								}
							}
						}
					case "大于等于":
						for _, compare := range logic.Compare {
							logicVal, err := numberx.GetFloatNumber(data[logic.ID])
							if err != nil {
								counter++
								continue logicLoop5
							}
							if compare.ID != "" {
								compareVal, err := numberx.GetFloatNumber(data[compare.ID])
								if err != nil {
									counter++
									continue logicLoop5
								}
								if logicVal < compareVal {
									counter++
									continue logicLoop5
								}
							} else {
								compareVal, err := numberx.GetFloatNumber(compare.Value)
								if err != nil {
									counter++
									continue logicLoop5
								}
								if logicVal < compareVal {
									counter++
									continue logicLoop5
								}
							}
						}
					case "小于等于":
						for _, compare := range logic.Compare {
							logicVal, err := numberx.GetFloatNumber(data[logic.ID])
							if err != nil {
								counter++
								continue logicLoop5
							}
							if compare.ID != "" {
								compareVal, err := numberx.GetFloatNumber(data[compare.ID])
								if err != nil {
									counter++
									continue logicLoop5
								}
								if logicVal > compareVal {
									counter++
									continue logicLoop5
								}
							} else {
								compareVal, err := numberx.GetFloatNumber(compare.Value)
								if err != nil {
									counter++
									continue logicLoop5
								}
								if logicVal > compareVal {
									counter++
									continue logicLoop5
								}
							}
						}
					case "在范围内":
						for _, compare := range logic.Compare {
							logicVal, err := numberx.GetFloatNumber(data[logic.ID])
							if err != nil {
								counter++
								continue logicLoop5
							}
							startVal := float64(0)
							endVal := float64(0)
							if compare.StartValue.ID != "" {
								startVal, err = numberx.GetFloatNumber(data[compare.StartValue.ID])
								if err != nil {
									counter++
									continue logicLoop5
								}
							} else {
								startVal, err = numberx.GetFloatNumber(compare.StartValue.Value)
								if err != nil {
									counter++
									continue logicLoop5
								}
							}
							if compare.EndValue.ID != "" {
								endVal, err = numberx.GetFloatNumber(data[compare.EndValue.ID])
								if err != nil {
									counter++
									continue logicLoop5
								}
							} else {
								endVal, err = numberx.GetFloatNumber(compare.EndValue.Value)
								if err != nil {
									counter++
									continue logicLoop5
								}
							}
							if logicVal < startVal || logicVal > endVal {
								counter++
								continue logicLoop5
							}
						}
					case "不在范围内":
						for _, compare := range logic.Compare {
							logicVal, err := numberx.GetFloatNumber(data[logic.ID])
							if err != nil {
								counter++
								continue logicLoop5
							}
							startVal := float64(0)
							endVal := float64(0)
							if compare.StartValue.ID != "" {
								startVal, err = numberx.GetFloatNumber(data[compare.StartValue.ID])
								if err != nil {
									counter++
									continue logicLoop5
								}
							} else {
								startVal, err = numberx.GetFloatNumber(compare.StartValue.Value)
								if err != nil {
									counter++
									continue logicLoop5
								}
							}
							if compare.EndValue.ID != "" {
								endVal, err = numberx.GetFloatNumber(data[compare.EndValue.ID])
								if err != nil {
									counter++
									continue logicLoop5
								}
							} else {
								endVal, err = numberx.GetFloatNumber(compare.EndValue.Value)
								if err != nil {
									counter++
									continue logicLoop5
								}
							}
							if logicVal >= startVal && logicVal <= endVal {
								counter++
								continue logicLoop5
							}
						}
					case "为空":
						if data[logic.ID] != nil {
							counter++
							continue logicLoop5
						}
					case "不为空":
						if data[logic.ID] == nil {
							counter++
							continue logicLoop5
						}
					}
				case "时间":
					switch logic.Relation {
					case "等于":
						for _, compare := range logic.Compare {
							compareInValue := int64(0)
							if ele, ok := compare.Value.(string); ok {
								if ele != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
									if err != nil {
										counter++
										continue logicLoop5
									}
									compareInValue = eleTime.Unix()
								}
							}
							dataVal := int64(0)
							eleRaw, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop5
							} else {
								if eleRaw != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop5
									}
									dataVal = eleTime.Unix()
								}
							}
							if compare.ID != "" {
								compareValue := int64(0)
								eleRaw, ok := data[compare.ID].(string)
								if !ok {
									counter++
									continue logicLoop5
								} else {
									if eleRaw != "" {
										eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
										if err != nil {
											counter++
											continue logicLoop5
										}
										compareValue = eleTime.Unix()
									}
								}
								if dataVal != compareValue {
									counter++
									continue logicLoop5
								}
							} else if dataVal != compareInValue {
								counter++
								continue logicLoop5
							} else if compareInValue == 0 {
								counter++
								continue logicLoop5
							}
						}
					case "不等于":
						for _, compare := range logic.Compare {
							compareInValue := int64(0)
							if ele, ok := compare.Value.(string); ok {
								if ele != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
									if err != nil {
										counter++
										continue logicLoop5
									}
									compareInValue = eleTime.Unix()
								}
							}
							dataVal := int64(0)
							eleRaw, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop5
							} else {
								if eleRaw != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop5
									}
									dataVal = eleTime.Unix()
								}
							}
							if compare.ID != "" {
								compareValue := int64(0)
								eleRaw, ok := data[compare.ID].(string)
								if !ok {
									counter++
									continue logicLoop5
								} else {
									if eleRaw != "" {
										eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
										if err != nil {
											counter++
											continue logicLoop5
										}
										compareValue = eleTime.Unix()
									}
								}
								if dataVal == compareValue {
									counter++
									continue logicLoop5
								}
							} else if dataVal == compareInValue {
								counter++
								continue logicLoop5
							} else if compareInValue == 0 {
								counter++
								continue logicLoop5
							}
						}
					case "早于":
						for _, compare := range logic.Compare {
							compareInValue := int64(0)
							if ele, ok := compare.Value.(string); ok {
								if ele != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
									if err != nil {
										counter++
										continue logicLoop5
									}
									compareInValue = eleTime.Unix()
								}
							}
							dataVal := int64(0)
							eleRaw, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop5
							} else {
								if eleRaw != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop5
									}
									dataVal = eleTime.Unix()
								}
							}
							if compare.ID != "" {
								compareValue := int64(0)
								eleRaw, ok := data[compare.ID].(string)
								if !ok {
									counter++
									continue logicLoop5
								} else {
									if eleRaw != "" {
										eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
										if err != nil {
											counter++
											continue logicLoop5
										}
										compareValue = eleTime.Unix()
									}
								}
								if dataVal >= compareValue {
									counter++
									continue logicLoop5
								}
							} else if dataVal >= compareInValue {
								counter++
								continue logicLoop5
							} else if compareInValue == 0 {
								counter++
								continue logicLoop5
							}
						}
					case "晚于":
						for _, compare := range logic.Compare {
							compareInValue := int64(0)
							if ele, ok := compare.Value.(string); ok {
								if ele != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(ele), ele, time.Local)
									if err != nil {
										counter++
										continue logicLoop5
									}
									compareInValue = eleTime.Unix()
								}
							}
							dataVal := int64(0)
							eleRaw, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop5
							} else {
								if eleRaw != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop5
									}
									dataVal = eleTime.Unix()
								}
							}
							if compare.ID != "" {
								compareValue := int64(0)
								eleRaw, ok := data[compare.ID].(string)
								if !ok {
									counter++
									continue logicLoop5
								} else {
									if eleRaw != "" {
										eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
										if err != nil {
											counter++
											continue logicLoop5
										}
										compareValue = eleTime.Unix()
									}
								}
								if dataVal <= compareValue {
									counter++
									continue logicLoop5
								}
							} else if dataVal <= compareInValue {
								counter++
								continue logicLoop5
							} else if compareInValue == 0 {
								counter++
								continue logicLoop5
							}
						}
					case "在范围内":
						for _, compare := range logic.Compare {
							logicVal := int64(0)
							eleRaw, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop5
							} else {
								if eleRaw != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop5
									}
									logicVal = eleTime.Unix()
								}
							}
							startVal := int64(0)
							endVal := int64(0)
							if compare.StartTime.ID != "" {
								if eleTimeRaw, ok := data[compare.StartTime.ID].(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop5
									}
									startVal = eleTime.Unix()
								}
							} else if compare.StartTime.Value != "" {
								if eleTimeRaw, ok := compare.EndTime.Value.(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop5
									}
									startVal = eleTime.Unix()
								}
							}
							if compare.EndTime.ID != "" {
								if eleTimeRaw, ok := data[compare.EndTime.ID].(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop5
									}
									startVal = eleTime.Unix()
								}
							} else if compare.EndTime.Value != "" {
								if eleTimeRaw, ok := compare.EndTime.Value.(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop5
									}
									endVal = eleTime.Unix()
								}
							}
							if logicVal < startVal || logicVal > endVal {
								counter++
								continue logicLoop5
							}
						}
					case "不在范围内":
						for _, compare := range logic.Compare {
							logicVal := int64(0)
							eleRaw, ok := data[logic.ID].(string)
							if !ok {
								counter++
								continue logicLoop5
							} else {
								if eleRaw != "" {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleRaw), eleRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop5
									}
									logicVal = eleTime.Unix()
								}
							}
							startVal := int64(0)
							endVal := int64(0)
							if compare.StartTime.ID != "" {
								if eleTimeRaw, ok := data[compare.StartTime.ID].(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop5
									}
									startVal = eleTime.Unix()
								}
							} else if compare.StartTime.Value != "" {
								if eleTimeRaw, ok := compare.EndTime.Value.(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop5
									}
									startVal = eleTime.Unix()
								}
							}
							if compare.EndTime.ID != "" {
								if eleTimeRaw, ok := data[compare.EndTime.ID].(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop5
									}
									startVal = eleTime.Unix()
								}
							} else if compare.EndTime.Value != "" {
								if eleTimeRaw, ok := compare.EndTime.Value.(string); ok {
									eleTime, err := timex.ConvertStringToTime(timex.FormatTimeFormat(eleTimeRaw), eleTimeRaw, time.Local)
									if err != nil {
										counter++
										continue logicLoop5
									}
									endVal = eleTime.Unix()
								}
							}
							if logicVal < startVal || logicVal > endVal {
								counter++
								continue logicLoop5
							}
						}
					case "为空":
						if data[logic.ID] != nil {
							counter++
							continue logicLoop5
						}
					case "不为空":
						if data[logic.ID] == nil {
							counter++
							continue logicLoop5
						}
					}
				case "布尔值", "附件", "定位":
					switch logic.Relation {
					case "为空":
						if data[logic.ID] != nil {
							counter++
							continue logicLoop5
						}
					case "不为空":
						if data[logic.ID] == nil {
							counter++
							continue logicLoop5
						}
					}
				case "关联字段":
					switch logic.Relation {
					case "是":
						for _, compare := range logic.Compare {
							var dataVal interface{}
							if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
								eleRaw, ok := data[logic.ID].(map[string]interface{})
								if !ok {
									counter++
									continue logicLoop5
								} else {
									if eleRaw != nil {
										if relateVal, ok := eleRaw[extVal.RelateField]; ok {
											dataVal = relateVal
										} else {
											counter++
											continue logicLoop5
										}
									}
								}
							}
							if compare.ID != "" {
								var compareValue interface{}
								if extVal, ok := excelColNameTypeExtMap[compare.ID]; ok {
									eleRaw, ok := data[compare.ID].(map[string]interface{})
									if !ok {
										counter++
										continue logicLoop5
									} else {
										if eleRaw != nil {
											if relateVal, ok := eleRaw[extVal.RelateField]; ok {
												compareValue = relateVal
											} else {
												counter++
												continue logicLoop5
											}
										}
									}
								}
								if dataVal != compareValue {
									counter++
									continue logicLoop5
								}
							} else if dataVal != compare.Value {
								counter++
								continue logicLoop5
							}
						}
					case "不是":
						for _, compare := range logic.Compare {
							var dataVal interface{}
							if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
								eleRaw, ok := data[logic.ID].(map[string]interface{})
								if !ok {
									counter++
									continue logicLoop5
								} else {
									if eleRaw != nil {
										if relateVal, ok := eleRaw[extVal.RelateField]; ok {
											dataVal = relateVal
										} else {
											counter++
											continue logicLoop5
										}
									}
								}
							}
							if compare.ID == "" {
								var compareValue interface{}
								if extVal, ok := excelColNameTypeExtMap[compare.ID]; ok {
									eleRaw, ok := data[compare.ID].(map[string]interface{})
									if !ok {
										counter++
										continue logicLoop5
									} else {
										if eleRaw != nil {
											if relateVal, ok := eleRaw[extVal.RelateField]; ok {
												compareValue = relateVal
											} else {
												counter++
												continue logicLoop5
											}
										}
									}
								}
								if dataVal != compareValue {
									counter++
									continue logicLoop5
								}
							} else if dataVal == compare.Value {
								counter++
								continue logicLoop5
							}
						}
					case "包含":
						for _, compare := range logic.Compare {
							compareInValue := ""
							if ele, ok := compare.Value.(string); ok {
								compareInValue = ele
							}
							dataVal := ""
							if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
								eleRaw, ok := data[logic.ID].(map[string]interface{})
								if !ok {
									counter++
									continue logicLoop5
								} else {
									if eleRaw != nil {
										if relateVal, ok := eleRaw[extVal.RelateField]; ok {
											if relateValString, ok := relateVal.(string); ok {
												dataVal = relateValString
											} else {
												counter++
												continue logicLoop5
											}
										} else {
											counter++
											continue logicLoop5
										}
									}
								}
							}
							if compare.ID != "" {
								compareVal := ""
								if extVal, ok := excelColNameTypeExtMap[compare.ID]; ok {
									eleRaw, ok := data[compare.ID].(map[string]interface{})
									if !ok {
										counter++
										continue logicLoop5
									} else {
										if eleRaw != nil {
											if relateVal, ok := eleRaw[extVal.RelateField]; ok {
												if relateValString, ok := relateVal.(string); ok {
													compareVal = relateValString
												} else {
													counter++
													continue logicLoop5
												}
											} else {
												counter++
												continue logicLoop5
											}
										}
									}
								}
								if strings.Contains(dataVal, compareVal) {
									counter++
									continue logicLoop5
								}
							} else if strings.Contains(dataVal, compareInValue) {
								counter++
								continue logicLoop5
							}
						}
					case "不包含":
						for _, compare := range logic.Compare {
							compareInValue := ""
							if ele, ok := compare.Value.(string); ok {
								compareInValue = ele
							}
							dataVal := ""
							if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
								eleRaw, ok := data[logic.ID].(map[string]interface{})
								if !ok {
									counter++
									continue logicLoop5
								} else {
									if eleRaw != nil {
										if relateVal, ok := eleRaw[extVal.RelateField]; ok {
											if relateValString, ok := relateVal.(string); ok {
												dataVal = relateValString
											} else {
												counter++
												continue logicLoop5
											}
										} else {
											counter++
											continue logicLoop5
										}
									}
								}
							}
							if compare.ID != "" {
								compareVal := ""
								if extVal, ok := excelColNameTypeExtMap[compare.ID]; ok {
									eleRaw, ok := data[compare.ID].(map[string]interface{})
									if !ok {
										counter++
										continue logicLoop5
									} else {
										if eleRaw != nil {
											if relateVal, ok := eleRaw[extVal.RelateField]; ok {
												if relateValString, ok := relateVal.(string); ok {
													compareVal = relateValString
												} else {
													counter++
													continue logicLoop5
												}
											} else {
												counter++
												continue logicLoop5
											}
										}
									}
								}
								if !strings.Contains(dataVal, compareVal) {
									counter++
									continue logicLoop5
								}
							} else if !strings.Contains(dataVal, compareInValue) {
								counter++
								continue logicLoop5
							}
						}
					case "为空":
						if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
							eleRaw, ok := data[logic.ID].(map[string]interface{})
							if ok {
								if eleRaw != nil {
									if relateVal, ok := eleRaw[extVal.RelateField]; ok {
										if relateVal != nil {
											counter++
											continue logicLoop5
										}
									} else {
										counter++
										continue logicLoop5
									}
								} else {
									counter++
									continue logicLoop5
								}
							} else {
								counter++
								continue logicLoop5
							}
						}
					case "不为空":
						if extVal, ok := excelColNameTypeExtMap[logic.ID]; ok {
							eleRaw, ok := data[logic.ID].(map[string]interface{})
							if ok {
								if eleRaw != nil {
									if relateVal, ok := eleRaw[extVal.RelateField]; ok {
										if relateVal == nil {
											counter++
											continue logicLoop5
										}
									} else {
										counter++
										continue logicLoop5
									}
								} else {
									counter++
									continue logicLoop5
								}
							} else {
								counter++
								continue logicLoop5
							}
						}
					}
				}
			}
			if len(settings.Logic)/2+1 == counter {
				continue flowloop
			}
		}
		//=================
		isValid = true
		if isValid {
			for key, valRaw := range data {
				if valMap, ok := valRaw.(map[string]interface{}); ok {
					if extVal, ok := excelColNameTypeExtMap[key]; ok {
						if id, ok := valMap["id"].(string); ok {
							data["$#"+key] = bson.M{"id": id, "_tableName": extVal.RelateTo}
							delete(data, key)
						}
					}
				}
			}

			if loginTimeRaw, ok := data["time"].(string); ok {
				loginTime, err := timex.ConvertStringToTime("2006-01-02 15:04:05", loginTimeRaw, time.Local)
				if err != nil {
					continue
				}
				data["time"] = loginTime.UnixNano() / 1e6
			}
			fmt.Println("projectName ;", projectName, "data:", data)
			err = flowx.StartFlow(zbClient, flowInfo.FlowXml, projectName, data)
			if err != nil {
				logger.Errorf("流程推进到下一阶段失败:%s", err.Error())
				continue
			}
			hasExecute = true
		}

		//对只能执行一次的流程进行失效
		if settings.ValidTime == "timeLimit" {
			if rangeDefine == "once" && hasExecute {
				logger.Warnln("流程(%s)为只执行一次的流程", flowID)
				//修改流程为失效
				updateMap := bson.M{"settings.invalid": true}
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
	if schema, ok := tableInfo["schema"].(primitive.M); ok {
		if properties, ok := schema["properties"].(primitive.M); ok {
			for key, propertyVal := range properties {
				if propertyMap, ok := propertyVal.(primitive.M); ok {
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
								} else if relateMap, ok := propertyMap["relate"].(primitive.M); ok {
									if relateTo, ok := relateMap["name"].(string); ok {
										excelColNameTypeExt.RelateTo = "ext_" + relateTo
										excelColNameTypeExt.FieldType = relateTo
										if fields, ok := relateMap["fields"].(primitive.A); ok {
											for _, field := range fields {
												if fieldMap, ok := field.(primitive.M); ok {
													if fieldKey, ok := fieldMap["key"].(string); ok {
														excelColNameTypeExt.RelateField = fieldKey
													}
												}
											}
										}
									}
									if relateName, ok := relateMap["relateName"].(string); ok {
										excelColNameTypeExt.RelateName = relateName
									} else if fields, ok := relateMap["fields"].(primitive.A); ok {
										for _, field := range fields {
											if fieldMap, ok := field.(primitive.M); ok {
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
