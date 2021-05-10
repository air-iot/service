package tsdb

import (
	"context"
	client "github.com/influxdata/influxdb1-client/v2"
)

// Influx influx存储
type Influx struct {
	cli client.Client
}

func NewInflux(cli client.Client) TSDB {
	a := new(Influx)
	a.cli = cli
	return a
}

func (a *Influx) Write(ctx context.Context, database string, rows []Row) (err error) {
	ps := make([]*client.Point, 0)
	for _, row := range rows {
		pt, err := client.NewPoint(
			row.TableName,
			row.Tags,
			row.Fields,
			row.Ts,
		)
		if err != nil {
			return err
		}
		ps = append(ps, pt)
	}

	if len(ps) > 0 {
		bp, err := client.NewBatchPoints(client.BatchPointsConfig{
			Database:        database,
			Precision:       "ms",
			RetentionPolicy: "autogen",
		})
		if err != nil {
			return err
		}
		bp.AddPoints(ps)
		return a.cli.Write(bp)
	}

	return nil
}

func (a *Influx) Query(ctx context.Context, database string, query string) (res []client.Result, err error) {
	q := client.Query{
		Command:  query,
		Database: database,
	}

	if response, err := a.cli.Query(q); err == nil {
		if response.Error() != nil {
			return nil, response.Error()
		}
		return response.Results, nil
	} else {
		return nil, err
	}
}

func (a *Influx) QueryFilter(ctx context.Context, database string, query []map[string]interface{}) (res []client.Result, err error) {
	return nil, nil
}

func (a *Influx) queryFilter(ctx context.Context, database, tableName string, val map[string]interface{}) (res []client.Result, err error) {
	//fieldList := make([]string, 0)
	//assetIDList := ""
	//departmentIDList := ""
	//andDeptIDList := ""
	//whereList := ""
	//groupByList := ""
	//fillValue := ""
	//orderByValue := ""
	//limitValue := ""
	//offsetValue := ""
	////nodeIDInQuery := ""
	//fieldStringList := make([]string, 0)
	//fluxIntervalType := ""
	//needFlux := false
	//sqlString := ""
	//if groups, ok := val["group"]; ok {
	//	if list, ok := groups.([]interface{}); ok {
	//		stringList := formatx.InterfaceListToStringList(list)
	//		for _, v := range stringList {
	//			if strings.Contains(v, "time(") {
	//				if strings.Contains(v, "mo") || strings.Contains(v, "y") {
	//					if strings.Contains(v, "mo") {
	//						fluxIntervalType = "mo"
	//					} else {
	//						fluxIntervalType = "y"
	//					}
	//					needFlux = true
	//				}
	//			}
	//		}
	//	}
	//}
	//
	//if fields, ok := val["fields"]; ok {
	//	if list, ok := fields.([]interface{}); ok {
	//		for i, field := range list {
	//			if fieldStr, ok := field.(string); !ok {
	//				return nil, fmt.Errorf("fields字段中第 %d 个的值为空或者类型错误", i+1)
	//			} else {
	//				if strings.Contains(fieldStr, "\"") {
	//					fieldList = append(fieldList, fieldStr)
	//				} else if strings.Contains(fieldStr, "(") {
	//					formatFieldLeft := strings.ReplaceAll(fieldStr, "(", "(\"")
	//					formatFieldRight := strings.ReplaceAll(formatFieldLeft, ")", "\")")
	//					fieldList = append(fieldList, formatFieldRight)
	//				} else {
	//					fieldList = append(fieldList, fmt.Sprintf(`"%s"`, fieldStr))
	//				}
	//			}
	//		}
	//		fieldStringList = formatx.InterfaceListToStringList(list)
	//	} else {
	//		return nil, fmt.Errorf("fields字段类型错误")
	//	}
	//} else {
	//	fieldList = append(fieldList, "*")
	//}
	//
	//if modelId, ok := val["modelId"].(string); ok {
	//	tableName = modelId
	//}
	//
	//if id, ok := val["id"].(string); ok {
	//	assetIDList = assetIDList + " where id =~ /" + id + "/ "
	//}
	//
	//if list, ok := val["department"].([]interface{}); ok {
	//	for _, departmentID := range list {
	//		if len(departmentIDList) != 0 {
	//			departmentIDList = departmentIDList + "|" + departmentID.(string)
	//		} else {
	//			_, okID := val["id"]
	//
	//			if okID {
	//				departmentIDList = departmentIDList + " and department =~ /" + departmentID.(string)
	//			} else {
	//				departmentIDList = departmentIDList + " where department =~ /" + departmentID.(string)
	//			}
	//		}
	//	}
	//	if len(departmentIDList) > 0 {
	//		departmentIDList = departmentIDList + "/ "
	//	}
	//}
	//
	//if list, ok := val["andDeptId"].([]interface{}); ok {
	//	if len(list) == 0 {
	//		return nil, fmt.Errorf("权限范围内没有可以查看的资产")
	//	}
	//	for _, andDeptID := range list {
	//		if len(andDeptIDList) != 0 {
	//			andDeptIDList = andDeptIDList + "|" + andDeptID.(string)
	//		} else {
	//			_, okID := val["id"]
	//			_, okDept := val["department"]
	//
	//			if okID || okDept {
	//				andDeptIDList = andDeptIDList + " and department =~ /" + andDeptID.(string)
	//			} else {
	//				andDeptIDList = andDeptIDList + " where department =~ /" + andDeptID.(string)
	//			}
	//		}
	//	}
	//	if len(andDeptIDList) > 0 {
	//		andDeptIDList = andDeptIDList + "/ "
	//	}
	//}
	//
	//if fill, ok := val["fill"]; ok {
	//	if f, ok := fill.(string); ok {
	//		fillValue = "fill(" + f + ")"
	//	}
	//}
	//if order, ok := val["order"]; ok {
	//	if o, ok := order.(string); ok {
	//		orderByValue = "order by " + o
	//	}
	//}
	//if limit, ok := val["limit"]; ok {
	//	if l, ok := limit.(float64); ok {
	//		limitValue = fmt.Sprintf("limit %.0f", l)
	//	}
	//}
	//if offset, ok := val["offset"]; ok {
	//	if o, ok := offset.(float64); ok {
	//		offsetValue = fmt.Sprintf("offset %.0f", o)
	//	}
	//}
	//
	//if needFlux {
	//	needFlux = false
	//	sqlStringBefore := sqlString
	//	sqlString = ""
	//
	//	whereTimeList := make([]string, 0)
	//
	//	if list, ok := val["where"].([]interface{}); ok {
	//		var err error
	//		var startTime *time.Time
	//		var endTime *time.Time
	//		var nextTime *time.Time
	//		groupTime := 0
	//		formatLayout := "2006-01-02 15:04:05"
	//		for _, whereEle := range list {
	//			if where, ok := whereEle.(string); ok {
	//				//判断是否有开始时间
	//				if groups, ok := val["group"]; ok {
	//					if list, ok := groups.([]interface{}); ok {
	//					grouploopFlux:
	//						for _, group := range list {
	//							if groupString, ok := group.(string); ok {
	//								isTimeGroup := timex.IsTimeGroup(groupString)
	//								if isTimeGroup {
	//									groupStringWithoutTime := strings.ReplaceAll(strings.ReplaceAll(groupString, "time(", ""), ")", "")
	//									groupInterval := ""
	//									if strings.Contains(groupStringWithoutTime, "mo") {
	//										groupInterval = groupStringWithoutTime[len(groupStringWithoutTime)-2:]
	//									} else {
	//										groupInterval = groupStringWithoutTime[len(groupStringWithoutTime)-1:]
	//									}
	//									groupTime, err = strconv.Atoi(groupStringWithoutTime[:len(groupStringWithoutTime)-len(groupInterval)])
	//									if err != nil {
	//										return nil, fmt.Errorf("InfluxDB SQL时间分组参数的数字部分不是整数:%+v", err)
	//									}
	//									intervalTime := 0
	//									switch groupInterval {
	//									case "h":
	//										intervalTime = groupTime * 60 * 60
	//									case "d":
	//										intervalTime = groupTime * 60 * 60 * 24
	//									case "w":
	//										intervalTime = groupTime * 60 * 60 * 24 * 7
	//									case "mo":
	//									case "y":
	//									default:
	//										continue grouploopFlux
	//									}
	//									if strings.Contains(where, "now") {
	//										continue
	//									}
	//									if strings.Contains(where, ">=") {
	//										whereFormat := strings.ReplaceAll(where, "'", "")
	//										splitList := strings.Split(whereFormat, ">=")
	//										if len(splitList) > 1 {
	//											formatTime := strings.TrimSpace(splitList[1])
	//											//formatLayout := "2006-01-02 15:04:05"
	//											if strings.Contains(formatTime, "+") {
	//												formatLayout = "2006-01-02T15:04:05" + formatTime[len(formatTime)-6:]
	//											} else if strings.Contains(formatTime, "Z") && strings.Contains(formatTime, "T") {
	//												if strings.Contains(formatTime, ".") {
	//													splitDotTime := strings.Split(strings.TrimSpace(formatTime), ".")
	//													if len(splitDotTime) >= 2 {
	//														switch len(splitDotTime[1]) {
	//														case 2:
	//															formatLayout = "2006-01-02T15:04:05.0Z"
	//														case 3:
	//															formatLayout = "2006-01-02T15:04:05.00Z"
	//														case 4:
	//															formatLayout = "2006-01-02T15:04:05.000Z"
	//														}
	//													}
	//												} else {
	//													formatLayout = "2006-01-02T15:04:05Z"
	//												}
	//											} else {
	//												formatTime = strings.ReplaceAll(formatTime, "T", " ")
	//												formatTime = strings.ReplaceAll(formatTime, "Z", "")
	//											}
	//
	//											if strings.Contains(formatTime, "Z") {
	//												timeStart, err := time.ParseInLocation(formatLayout, formatTime, time.UTC)
	//												if err != nil {
	//													return nil, fmt.Errorf("UTC时间转time.Time失败:%s", err.Error())
	//												}
	//												startTimeRaw := timex.GetLocalTimeNow(timeStart)
	//												startTime = &startTimeRaw
	//												if intervalTime != 0 {
	//													//offsetTime = int(utils.GetInfluxOffset(startTime.Unix(), int64(intervalTime)))
	//												}
	//											} else {
	//												startTime, err = timex.ConvertStringToTime(formatLayout, formatTime, time.Local)
	//												if err == nil {
	//													if intervalTime != 0 {
	//														//offsetTime = int(utils.GetInfluxOffset(startTime.Unix(), int64(intervalTime)))
	//													}
	//												}
	//											}
	//										}
	//									} else if strings.Contains(where, ">") {
	//										whereFormat := strings.ReplaceAll(where, "'", "")
	//										splitList := strings.Split(whereFormat, ">")
	//										if len(splitList) > 1 {
	//											formatTime := strings.TrimSpace(splitList[1])
	//											//formatLayout := "2006-01-02 15:04:05"
	//											if strings.Contains(formatTime, "+") {
	//												formatLayout = "2006-01-02T15:04:05" + formatTime[len(formatTime)-6:]
	//											} else if strings.Contains(formatTime, "Z") && strings.Contains(formatTime, "T") {
	//												if strings.Contains(formatTime, ".") {
	//													splitDotTime := strings.Split(strings.TrimSpace(formatTime), ".")
	//													if len(splitDotTime) >= 2 {
	//														switch len(splitDotTime[1]) {
	//														case 2:
	//															formatLayout = "2006-01-02T15:04:05.0Z"
	//														case 3:
	//															formatLayout = "2006-01-02T15:04:05.00Z"
	//														case 4:
	//															formatLayout = "2006-01-02T15:04:05.000Z"
	//														}
	//													}
	//												} else {
	//													formatLayout = "2006-01-02T15:04:05Z"
	//												}
	//											} else {
	//												formatTime = strings.ReplaceAll(formatTime, "T", " ")
	//												formatTime = strings.ReplaceAll(formatTime, "Z", "")
	//											}
	//
	//											if strings.Contains(formatTime, "Z") {
	//												timeStart, err := time.ParseInLocation(formatLayout, formatTime, time.UTC)
	//												if err != nil {
	//													return nil, fmt.Errorf("UTC时间转time.Time失败:%s", err.Error())
	//												}
	//												startTimeRaw := timex.GetLocalTimeNow(timeStart)
	//												startTime = &startTimeRaw
	//												if intervalTime != 0 {
	//													//offsetTime = int(utils.GetInfluxOffset(startTime.Unix(), int64(intervalTime)))
	//												}
	//											} else {
	//												startTime, err = timex.ConvertStringToTime(formatLayout, formatTime, time.Local)
	//												if err == nil {
	//													if intervalTime != 0 {
	//														//offsetTime = int(utils.GetInfluxOffset(startTime.Unix(), int64(intervalTime)))
	//													}
	//												}
	//											}
	//										}
	//									}
	//									break
	//								}
	//							}
	//						}
	//					}
	//				}
	//
	//				if strings.Contains(where, "<=") {
	//					if strings.Contains(where, "now") {
	//						endTimeRaw := timex.GetLocalTimeNow(time.Now())
	//						endTime = &endTimeRaw
	//					} else {
	//						whereFormat := strings.ReplaceAll(where, "'", "")
	//						splitList := strings.Split(whereFormat, "<=")
	//						if len(splitList) > 1 {
	//							formatTime := strings.TrimSpace(splitList[1])
	//							//formatLayout := "2006-01-02 15:04:05"
	//							if strings.Contains(formatTime, "+") {
	//								formatLayout = "2006-01-02T15:04:05" + formatTime[len(formatTime)-6:]
	//							} else if strings.Contains(formatTime, "Z") && strings.Contains(formatTime, "T") {
	//								if strings.Contains(formatTime, ".") {
	//									splitDotTime := strings.Split(strings.TrimSpace(formatTime), ".")
	//									if len(splitDotTime) >= 2 {
	//										switch len(splitDotTime[1]) {
	//										case 2:
	//											formatLayout = "2006-01-02T15:04:05.0Z"
	//										case 3:
	//											formatLayout = "2006-01-02T15:04:05.00Z"
	//										case 4:
	//											formatLayout = "2006-01-02T15:04:05.000Z"
	//										}
	//									}
	//								} else {
	//									formatLayout = "2006-01-02T15:04:05Z"
	//								}
	//							} else {
	//								formatTime = strings.ReplaceAll(formatTime, "T", " ")
	//								formatTime = strings.ReplaceAll(formatTime, "Z", "")
	//							}
	//
	//							if strings.Contains(formatTime, "Z") {
	//								timeStart, err := time.ParseInLocation(formatLayout, formatTime, time.UTC)
	//								if err != nil {
	//									return nil, fmt.Errorf("UTC时间转time.Time失败:%s", err.Error())
	//								}
	//								endTimeRaw := timex.GetLocalTimeNow(timeStart)
	//								endTime = &endTimeRaw
	//							} else {
	//								endTime, err = timex.ConvertStringToTime(formatLayout, formatTime, time.Local)
	//								if err == nil {
	//								}
	//							}
	//						}
	//					}
	//				} else if strings.Contains(where, "<") {
	//					if strings.Contains(where, "now") {
	//						endTimeRaw := timex.GetLocalTimeNow(time.Now())
	//						endTime = &endTimeRaw
	//					} else {
	//						whereFormat := strings.ReplaceAll(where, "'", "")
	//						splitList := strings.Split(whereFormat, "<")
	//						if len(splitList) > 1 {
	//							formatTime := strings.TrimSpace(splitList[1])
	//							//formatLayout := "2006-01-02 15:04:05"
	//							if strings.Contains(formatTime, "+") {
	//								formatLayout = "2006-01-02T15:04:05" + formatTime[len(formatTime)-6:]
	//							} else if strings.Contains(formatTime, "Z") && strings.Contains(formatTime, "T") {
	//								if strings.Contains(formatTime, ".") {
	//									splitDotTime := strings.Split(strings.TrimSpace(formatTime), ".")
	//									if len(splitDotTime) >= 2 {
	//										switch len(splitDotTime[1]) {
	//										case 2:
	//											formatLayout = "2006-01-02T15:04:05.0Z"
	//										case 3:
	//											formatLayout = "2006-01-02T15:04:05.00Z"
	//										case 4:
	//											formatLayout = "2006-01-02T15:04:05.000Z"
	//										}
	//									}
	//								} else {
	//									formatLayout = "2006-01-02T15:04:05Z"
	//								}
	//							} else {
	//								formatTime = strings.ReplaceAll(formatTime, "T", " ")
	//								formatTime = strings.ReplaceAll(formatTime, "Z", "")
	//							}
	//
	//							if strings.Contains(formatTime, "Z") {
	//								timeStart, err := time.ParseInLocation(formatLayout, formatTime, time.UTC)
	//								if err != nil {
	//									return nil, fmt.Errorf("UTC时间转time.Time失败:%s", err.Error())
	//								}
	//								endTimeRaw := timex.GetLocalTimeNow(timeStart)
	//								endTime = &endTimeRaw
	//							} else {
	//								endTime, err = timex.ConvertStringToTime(formatLayout, formatTime, time.Local)
	//								if err == nil {
	//								}
	//							}
	//						}
	//					}
	//				}
	//			}
	//		}
	//
	//		nextTime = startTime
	//		for {
	//			if startTime.Unix() >= endTime.Unix() {
	//				break
	//			}
	//			switch fluxIntervalType {
	//			case "mo":
	//				nextTime = timex.GetFutureMonthTime(nextTime, groupTime)
	//			case "y":
	//				nextTime = timex.GetFutureYearTime(nextTime, groupTime)
	//			}
	//			whereList := ""
	//			if nextTime.Unix() > endTime.Unix() || timex.IsSameDay(nextTime, endTime) {
	//				nextTime = endTime
	//			}
	//
	//			_, okID := val["id"]
	//			_, okDept := val["department"]
	//			_, okAndID := val["andDeptId"]
	//
	//			if okID || okAndID || okDept {
	//				whereList = fmt.Sprintf(" and time >= '%s' and time <= '%s' ", startTime.Format(formatLayout), nextTime.Format(formatLayout))
	//			} else {
	//				whereList = fmt.Sprintf(" where time >= '%s' and time <= '%s' ", startTime.Format(formatLayout), nextTime.Format(formatLayout))
	//			}
	//			whereTimeList = append(whereTimeList, whereList)
	//			switch fluxIntervalType {
	//			case "mo":
	//				startTime = nextTime
	//			case "y":
	//				startTime = nextTime
	//			}
	//		}
	//	}
	//
	//	for _, whereList := range whereTimeList {
	//		sqlString = sqlString + fmt.Sprintf("select %s from \"%s\" %s %s %s %s %s %s %s %s %s %s;", fieldList, tableName, assetUIDList, assetIDList, departmentIDList, andDeptIDList, whereList, groupByList, fillValue, orderByValue, limitValue, offsetValue)
	//		a.formatTagAliasMap(fieldStringList, &tagIDAliasMap, tableName)
	//		nodeIDInQueryList = append(nodeIDInQueryList, nodeIDInQuery)
	//		modelIDInQueryList = append(modelIDInQueryList, tableName)
	//	}
	//
	//	//influxProjectName
	//	res, err := influxdb.FindInfluxBySQL(a.InfluxCli, config.C.TSDB.DBName, sqlString)
	//	if err != nil {
	//		return nil, fmt.Errorf("InfluxDB SQL查询错误:%+v", err)
	//	}
	//
	//	seriesReturnRaw, err := a.InfluxResultConvertMapMoAndY(ctx, projectName, res, nodeIDInQueryList, modelIDInQueryList, tagIDAliasMap)
	//	if err != nil {
	//		return nil, fmt.Errorf("数据转换错误:%+v", err)
	//	}
	//	sqlString = sqlStringBefore
	//} else {
	//
	//	offsetTime := 0
	//	isWeekGroup := false
	//
	//	if list, ok := val["where"].([]interface{}); ok {
	//		for _, whereEle := range list {
	//			if where, ok := whereEle.(string); ok {
	//				//判断是否有开始时间
	//				if groups, ok := val["group"]; ok {
	//					if list, ok := groups.([]interface{}); ok {
	//					grouploop:
	//						for _, group := range list {
	//							if groupString, ok := group.(string); ok {
	//								isTimeGroup := timex.IsTimeGroup(groupString)
	//								if isTimeGroup {
	//									groupStringWithoutTime := strings.ReplaceAll(strings.ReplaceAll(groupString, "time(", ""), ")", "")
	//									groupInterval := groupStringWithoutTime[len(groupStringWithoutTime)-1:]
	//									groupTime, err := strconv.Atoi(groupStringWithoutTime[:len(groupStringWithoutTime)-1])
	//									if err != nil {
	//										return nil, fmt.Errorf("InfluxDB SQL时间分组参数的数字部分不是整数:%+v", err)
	//									}
	//									intervalTime := 0
	//									switch groupInterval {
	//									case "h":
	//										intervalTime = groupTime * 60 * 60
	//									case "d":
	//										intervalTime = groupTime * 60 * 60 * 24
	//									case "w":
	//										intervalTime = groupTime * 60 * 60 * 24 * 7
	//										isWeekGroup = true
	//									default:
	//										continue grouploop
	//									}
	//									if strings.Contains(where, "now") {
	//										continue
	//									}
	//									if strings.Contains(where, ">=") {
	//										whereFormat := strings.ReplaceAll(where, "'", "")
	//										splitList := strings.Split(whereFormat, ">=")
	//										if len(splitList) > 1 {
	//											formatTime := strings.TrimSpace(splitList[1])
	//											formatLayout := "2006-01-02 15:04:05"
	//											if strings.Contains(formatTime, "+") {
	//												formatLayout = "2006-01-02T15:04:05" + formatTime[len(formatTime)-6:]
	//											} else if strings.Contains(formatTime, "Z") && strings.Contains(formatTime, "T") {
	//												if strings.Contains(formatTime, ".") {
	//													splitDotTime := strings.Split(strings.TrimSpace(formatTime), ".")
	//													if len(splitDotTime) >= 2 {
	//														switch len(splitDotTime[1]) {
	//														case 2:
	//															formatLayout = "2006-01-02T15:04:05.0Z"
	//														case 3:
	//															formatLayout = "2006-01-02T15:04:05.00Z"
	//														case 4:
	//															formatLayout = "2006-01-02T15:04:05.000Z"
	//														}
	//													}
	//												} else {
	//													formatLayout = "2006-01-02T15:04:05Z"
	//												}
	//											} else {
	//												formatTime = strings.ReplaceAll(formatTime, "T", " ")
	//												formatTime = strings.ReplaceAll(formatTime, "Z", "")
	//											}
	//
	//											if strings.Contains(formatTime, "Z") {
	//												timeStart, err := time.ParseInLocation(formatLayout, formatTime, time.UTC)
	//												if err != nil {
	//													return nil, fmt.Errorf("UTC时间转time.Time失败:%s", err.Error())
	//												}
	//												startTime := timex.GetLocalTimeNow(timeStart)
	//												offsetTime = int(utils.GetInfluxOffset(startTime.Unix(), int64(intervalTime)))
	//											} else {
	//												startTime, err := timex.ConvertStringToTime(formatLayout, formatTime, time.Local)
	//												if err == nil {
	//													offsetTime = int(utils.GetInfluxOffset(startTime.Unix(), int64(intervalTime)))
	//												}
	//											}
	//										}
	//									} else if strings.Contains(where, ">") {
	//										whereFormat := strings.ReplaceAll(where, "'", "")
	//										splitList := strings.Split(whereFormat, ">")
	//										if len(splitList) > 1 {
	//											formatTime := strings.TrimSpace(splitList[1])
	//											formatLayout := "2006-01-02 15:04:05"
	//											if strings.Contains(formatTime, "+") {
	//												formatLayout = "2006-01-02T15:04:05" + formatTime[len(formatTime)-6:]
	//											} else if strings.Contains(formatTime, "Z") && strings.Contains(formatTime, "T") {
	//												if strings.Contains(formatTime, ".") {
	//													splitDotTime := strings.Split(strings.TrimSpace(formatTime), ".")
	//													if len(splitDotTime) >= 2 {
	//														switch len(splitDotTime[1]) {
	//														case 2:
	//															formatLayout = "2006-01-02T15:04:05.0Z"
	//														case 3:
	//															formatLayout = "2006-01-02T15:04:05.00Z"
	//														case 4:
	//															formatLayout = "2006-01-02T15:04:05.000Z"
	//														}
	//													}
	//												} else {
	//													formatLayout = "2006-01-02T15:04:05Z"
	//												}
	//											} else {
	//												formatTime = strings.ReplaceAll(formatTime, "T", " ")
	//												formatTime = strings.ReplaceAll(formatTime, "Z", "")
	//											}
	//
	//											if strings.Contains(formatTime, "Z") {
	//												timeStart, err := time.ParseInLocation(formatLayout, formatTime, time.UTC)
	//												if err != nil {
	//													return nil, fmt.Errorf("UTC时间转time.Time失败:%s", err.Error())
	//												}
	//												startTime := timex.GetLocalTimeNow(timeStart)
	//												offsetTime = int(utils.GetInfluxOffset(startTime.Unix(), int64(intervalTime)))
	//											} else {
	//												startTime, err := timex.ConvertStringToTime(formatLayout, formatTime, time.Local)
	//												if err == nil {
	//													offsetTime = int(utils.GetInfluxOffset(startTime.Unix(), int64(intervalTime)))
	//												}
	//											}
	//										}
	//									}
	//									break
	//								}
	//							}
	//						}
	//					}
	//				}
	//
	//				if len(whereList) != 0 {
	//					whereList = whereList + " and " + where
	//				} else {
	//					_, okID := val["id"]
	//					_, okDept := val["department"]
	//					_, okAndID := val["andDeptId"]
	//
	//					if okID || okAndID || okDept {
	//						whereList = whereList + " and " + where
	//					} else {
	//						whereList = whereList + " where " + where
	//					}
	//				}
	//			}
	//		}
	//	}
	//
	//	if groups, ok := val["group"]; ok {
	//		if list, ok := groups.([]interface{}); ok {
	//			for _, group := range list {
	//				if groupString, ok := group.(string); ok {
	//					groupStringFormat := groupString
	//					if offsetTime != 0 {
	//						groupStringFormat = strings.ReplaceAll(strings.TrimSpace(groupString), ")", "") + "," + strconv.Itoa(offsetTime) + "s)"
	//					}
	//					if len(groupByList) != 0 {
	//						groupByList = groupByList + "," + groupStringFormat
	//					} else {
	//						groupByList = groupByList + " group by " + groupStringFormat
	//					}
	//				}
	//			}
	//		}
	//	}
	//
	//	if isWeekGroup {
	//		sqlString = sqlString + fmt.Sprintf("select %s from \"%s\" %s %s %s %s %s %s %s %s %s;", strings.Join(fieldList, ","), tableName, assetIDList, departmentIDList, andDeptIDList, whereList, groupByList, fillValue, orderByValue, limitValue, offsetValue)
	//	} else {
	//		sqlString = sqlString + fmt.Sprintf("select %s from \"%s\" %s %s %s %s %s %s %s %s %s;", fieldList, tableName, assetIDList, departmentIDList, andDeptIDList, whereList, groupByList, fillValue, orderByValue, limitValue, offsetValue)
	//	}
	//	a.formatTagAliasMap(fieldStringList, &tagIDAliasMap, tableName)
	//	nodeIDInQueryList = append(nodeIDInQueryList, nodeIDInQuery)
	//	modelIDInQueryList = append(modelIDInQueryList, tableName)
	//}
	return nil, err
}
