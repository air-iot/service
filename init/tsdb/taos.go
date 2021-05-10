package tsdb

import (
	"context"
	"fmt"
	"github.com/air-iot/service/util/formatx"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/influxdata/influxdb1-client/models"
	client "github.com/influxdata/influxdb1-client/v2"
	"github.com/jmoiron/sqlx"

	"github.com/air-iot/service/config"
	"github.com/air-iot/service/logger"
	"github.com/air-iot/service/util/timex"
)

const (
	TableNotExistMsg        = "Table does not exist" // 表不存在
	InvalidColumnNameMsg    = "invalid column"
	DatabaseNotAvailableMsg = "Database not specified or available"
)

// Taos taos客户端
type Taos struct {
	cfg config.Taos
}

// NewTaos 创建Taos存储
func NewTaos(cfg config.Taos) TSDB {
	a := new(Taos)
	a.cfg = cfg
	return a
}

func (a *Taos) Write(ctx context.Context, database string, rows []Row) (err error) {
	wg := sync.WaitGroup{}
	ch := make(chan struct{}, a.cfg.MaxConn)
	wg.Add(len(rows))
	for i := 0; i < len(rows); i++ {
		ch <- struct{}{}
		go func(database string, sqlTmp Row) {
			defer wg.Done()
			if err := a.insert(ctx, database, sqlTmp); err != nil {
				logger.Errorf(err.Error())
			}
			<-ch
		}(database, rows[i])
	}
	wg.Wait()
	return nil
}

func (a *Taos) insert(ctx context.Context, database string, row Row) (err error) {
	db, err := sqlx.Open("taosSql", a.cfg.Addr)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Errorf("关闭连接错误: %s", err.Error())
		}
	}()

	if len(row.Fields) == 0 {
		return nil
	}
	names := make([]string, 0)
	values := make([]string, 0)
	colTypes := make([]string, 0)
	names = append(names, "time")
	colTypes = append(colTypes, "time TIMESTAMP")
	values = append(values, fmt.Sprintf("'%s'", row.Ts.Format("2006-01-02 15:04:05.000")))
	//row.Fields["ts"] = row.Ts.Format("2006-01-02 15:04:05.000")
	for name, val := range row.Fields {
		names = append(names, name)
		switch value := val.(type) {
		case bool:
			colTypes = append(colTypes, fmt.Sprintf("%s bool", name))
			values = append(values, fmt.Sprintf("%v", value))
		case string:
			colTypes = append(colTypes, fmt.Sprintf("%s binary(100)", name))
			values = append(values, fmt.Sprintf("'%s'", value))
		default:
			colTypes = append(colTypes, fmt.Sprintf("%s double", name))
			values = append(values, fmt.Sprintf("%v", value))
		}
	}
	tagNames := make([]string, 0)
	tagValues := make([]string, 0)
	for name, val := range row.Tags {
		tagNames = append(tagNames, fmt.Sprintf("%s binary(100)", name))
		tagValues = append(tagValues, fmt.Sprintf("'%s'", val))
	}

	createStbSql := fmt.Sprintf(`create table if not exists %s.m_%s (%s) TAGS(%s)`,
		database,
		row.TableName,
		strings.Join(colTypes, ","),
		strings.Join(tagNames, ","),
	)
	insertSql := fmt.Sprintf(`insert into %s.n_%s(%s) USING %s.m_%s TAGS(%s) VALUES(%s)`,
		database,
		row.SubTableName,
		strings.Join(names, ","),
		database,
		row.TableName,
		strings.Join(tagValues, ","),
		strings.Join(values, ","),
	)
	querySql := fmt.Sprintf(`select * from %s.m_%s limit 1`, database, row.TableName)

	//logger.Infof("插入数据sql: %s", insertSql)
	_, err = db.ExecContext(ctx, insertSql)
	if err != nil {
		if strings.Contains(err.Error(), TableNotExistMsg) {
			logger.Debugf(err.Error())
			_, err := db.ExecContext(ctx, createStbSql)
			if err != nil {
				return fmt.Errorf("create table SQL [%s] 错误: %s", createStbSql, err.Error())
			}
			_, err = db.ExecContext(ctx, insertSql)
			if err != nil {
				return fmt.Errorf("创建表后插入 [%s]数据错误: %s", insertSql, err.Error())
			}
		} else if strings.Contains(err.Error(), InvalidColumnNameMsg) {
			logger.Debugf(err.Error())
			colMap := make(map[string]string)
			res, err := db.QueryContext(ctx, querySql)
			if err != nil {
				return fmt.Errorf("查询 SQL [%s] 表结构错误: %s", querySql, err.Error())
			}
			columnTypes, err := res.ColumnTypes()
			if err != nil {
				return fmt.Errorf("获取 SQL [%s] 列信息错误: %s", querySql, err.Error())
			}
			for _, col := range columnTypes {
				colMap[strings.ToUpper(col.Name())] = col.DatabaseTypeName()
			}
			for name, val := range row.Fields {
				if _, ok := colMap[strings.ToUpper(name)]; !ok {
					var nameType string
					switch val.(type) {
					case bool:
						nameType = fmt.Sprintf("%s bool", name)
					case string:
						nameType = fmt.Sprintf("%s binary(100)", name)
					default:
						nameType = fmt.Sprintf("%s double", name)
					}

					stbSql := fmt.Sprintf("alter table %s.m_%s add column %s", database, row.TableName, nameType)
					_, err := db.ExecContext(ctx, stbSql)
					if err != nil {
						return fmt.Errorf("添加列 SQL [%s] 错误: %s", querySql, err.Error())
					}
				}
			}
			_, err = db.ExecContext(ctx, insertSql)
			if err != nil {
				return fmt.Errorf("添加列后插入 [%s] 数据错误: %s", insertSql, err.Error())
			}
		} else if strings.Contains(err.Error(), DatabaseNotAvailableMsg) {
			logger.Debugf(err.Error())
			createDatabase := fmt.Sprintf("create database %s", database)
			_, err := db.ExecContext(ctx, createDatabase)
			if err != nil {
				return fmt.Errorf("create database SQL [%s] 错误: %s", createDatabase, err.Error())
			}
			_, err = db.ExecContext(ctx, createStbSql)
			if err != nil {
				return fmt.Errorf("create table SQL [%s] 错误: %s", createStbSql, err.Error())
			}
			_, err = db.ExecContext(ctx, insertSql)
			if err != nil {
				return fmt.Errorf("创建数据库及表后插入 [%s] 数据错误: %s", insertSql, err.Error())
			}
		} else {
			return fmt.Errorf("插入 [%s] 数据错误: %s", insertSql, err.Error())
		}
	}
	return nil
}

func (a *Taos) Query(ctx context.Context, database string, sql string) (res []client.Result, err error) {
	db, err := sqlx.Open("taosSql", a.cfg.Addr)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Errorf("关闭连接错误: %s", err.Error())
		}
	}()
	_ = database
	colList, resultCombineList, err := a.query(ctx, db, sql)
	if err != nil {
		return nil, err
	}
	return []client.Result{
		{
			Series: []models.Row{
				{
					Columns: colList,
					Values:  resultCombineList,
				},
			},
		},
	}, nil
}

func (a *Taos) query(ctx context.Context, db *sqlx.DB, sqlString string) ([]string, [][]interface{}, error) {
	rows, err := db.QueryxContext(ctx, sqlString) // go text mode
	if err != nil {
		return nil, nil, fmt.Errorf("查询错误: %s", err.Error())
	}
	colList, err := rows.Columns()
	if err != nil {
		return nil, nil, fmt.Errorf("columns获取失败: %s", err.Error())
	}
	resultCombineList := make([][]interface{}, 0)
	for rows.Next() {
		resultList, err := rows.SliceScan()
		if err != nil {
			return nil, nil, fmt.Errorf("结果数组获取失败: %+v", err)
		}
		for i, v := range resultList {
			if ele, ok := v.(string); ok {
				if strings.Contains(ele, ":") {
					queryTime, err := timex.ConvertStringToTime("2006-01-02 15:04:05.000", ele, time.Local)
					if err == nil {
						resultList[i] = queryTime.Format("2006-01-02T15:04:05.000+08:00")
					} else {
						queryTime, err := timex.ConvertStringToTime("2006-01-02 15:04:05.00", ele, time.Local)
						if err == nil {
							splitTime := strings.Split(ele, ".")
							ele := splitTime[0] + ".0" + splitTime[1]
							queryTime, err = timex.ConvertStringToTime("2006-01-02 15:04:05.000", ele, time.Local)
							if err == nil {
								resultList[i] = queryTime.Format("2006-01-02T15:04:05.000+08:00")
							}
						} else {
							queryTime, err := timex.ConvertStringToTime("2006-01-02 15:04:05.0", ele, time.Local)
							if err == nil {
								splitTime := strings.Split(ele, ".")
								ele := splitTime[0] + ".00" + splitTime[1]
								queryTime, err = timex.ConvertStringToTime("2006-01-02 15:04:05.000", ele, time.Local)
								if err == nil {
									resultList[i] = queryTime.Format("2006-01-02T15:04:05.000+08:00")
								}
							} else {
								resultList[i] = strings.ReplaceAll(ele, "\u0000", "")
							}
						}
					}
				} else {
					resultList[i] = strings.ReplaceAll(ele, "\u0000", "")
				}
			}
		}
		resultCombineList = append(resultCombineList, resultList)
	}

	return colList, resultCombineList, nil
}

func (a *Taos) QueryFilter(ctx context.Context, database string, query []map[string]interface{}) (res []client.Result, err error) {
	formatResult := map[string]interface{}{}
	result := make([]map[string]interface{}, 0)
	for _, val := range query {
		//var count int
		sqlString := ""
		hasTime := false
		interval := ""
		withoutOtherGroup := true
		groupWithoutTimeList := make([]string, 0)
		startTime := ""

		fieldsList := ""
		tableName := ""
		assetUIDList := ""
		assetIDList := ""
		departmentIDList := ""
		//andassetIDList := ""
		andDeptIDList := ""
		whereList := ""
		groupByList := ""
		fillValue := ""
		orderByValue := ""
		limitValue := ""
		offsetValue := ""

		hasNoGroup := true
		hasNoPloy := true

		offsetTime := 0

		if list, ok := val["where"].([]interface{}); ok {
			for _, whereEle := range list {
				if where, ok := whereEle.(string); ok {
					//判断是否有开始时间
					if groups, ok := val["group"]; ok {
						if list, ok := groups.([]interface{}); ok {
						grouploop:
							for _, group := range list {
								if groupString, ok := group.(string); ok {
									isTimeGroup := util.IsTimeGroup(groupString)
									if isTimeGroup {
										groupStringWithoutTime := strings.ReplaceAll(strings.ReplaceAll(groupString, "time(", ""), ")", "")
										groupInterval := groupStringWithoutTime[len(groupStringWithoutTime)-1:]
										groupTime, err := strconv.Atoi(groupStringWithoutTime[:len(groupStringWithoutTime)-1])
										if err != nil {
											return nil, fmt.Errorf("TAOS SQL时间分组参数的数字部分不是整数:%+v", err)
										}
										intervalTime := 0
										switch groupInterval {
										case "h":
											intervalTime = groupTime * 60 * 60
										case "d":
											intervalTime = groupTime * 60 * 60 * 24
										case "w":
											intervalTime = groupTime * 60 * 60 * 24 * 7
										default:
											continue grouploop
										}
										if strings.Contains(where, "now") {
											continue
										}
										if strings.Contains(where, ">=") {
											whereFormat := strings.ReplaceAll(where, "'", "")
											splitList := strings.Split(whereFormat, ">=")
											if len(splitList) > 1 {
												formatTime := strings.TrimSpace(splitList[1])
												formatLayout := "2006-01-02 15:04:05"
												if strings.Contains(formatTime, "+") {
													formatLayout = "2006-01-02T15:04:05" + formatTime[len(formatTime)-6:]
												} else if strings.Contains(formatTime, "Z") && strings.Contains(formatTime, "T") {
													if strings.Contains(formatTime, ".") {
														splitDotTime := strings.Split(strings.TrimSpace(formatTime), ".")
														if len(splitDotTime) >= 2 {
															switch len(splitDotTime[1]) {
															case 2:
																formatLayout = "2006-01-02T15:04:05.0Z"
															case 3:
																formatLayout = "2006-01-02T15:04:05.00Z"
															case 4:
																formatLayout = "2006-01-02T15:04:05.000Z"
															}
														}
													} else {
														formatLayout = "2006-01-02T15:04:05Z"
													}
												} else {
													formatTime = strings.ReplaceAll(formatTime, "T", " ")
													formatTime = strings.ReplaceAll(formatTime, "Z", "")
												}

												if strings.Contains(formatTime, "Z") {
													timeStart, err := time.ParseInLocation(formatLayout, formatTime, time.UTC)
													if err != nil {
														return nil, fmt.Errorf("UTC时间转time.Time失败:%s", err.Error())
													}
													startTime := formatx.GetLocalTimeNow(timeStart)
													offsetTime = int(util.GetInfluxOffset(startTime.Unix(), int64(intervalTime)))
												} else {
													startTime, err := formatx.ConvertStringToTime(formatLayout, formatTime, time.Local)
													if err == nil {
														offsetTime = int(util.GetInfluxOffset(startTime.Unix(), int64(intervalTime)))
													}
												}
											}
										} else if strings.Contains(where, ">") {
											whereFormat := strings.ReplaceAll(where, "'", "")
											splitList := strings.Split(whereFormat, ">")
											if len(splitList) > 1 {
												formatTime := strings.TrimSpace(splitList[1])
												formatLayout := "2006-01-02 15:04:05"
												if strings.Contains(formatTime, "+") {
													formatLayout = "2006-01-02T15:04:05" + formatTime[len(formatTime)-6:]
												} else if strings.Contains(formatTime, "Z") && strings.Contains(formatTime, "T") {
													if strings.Contains(formatTime, ".") {
														splitDotTime := strings.Split(strings.TrimSpace(formatTime), ".")
														if len(splitDotTime) >= 2 {
															switch len(splitDotTime[1]) {
															case 2:
																formatLayout = "2006-01-02T15:04:05.0Z"
															case 3:
																formatLayout = "2006-01-02T15:04:05.00Z"
															case 4:
																formatLayout = "2006-01-02T15:04:05.000Z"
															}
														}
													} else {
														formatLayout = "2006-01-02T15:04:05Z"
													}
												} else {
													formatTime = strings.ReplaceAll(formatTime, "T", " ")
													formatTime = strings.ReplaceAll(formatTime, "Z", "")
												}

												if strings.Contains(formatTime, "Z") {
													timeStart, err := time.ParseInLocation(formatLayout, formatTime, time.UTC)
													if err != nil {
														return nil, fmt.Errorf("UTC时间转time.Time失败:%s", err.Error())
													}
													startTime := formatx.GetLocalTimeNow(timeStart)
													offsetTime = int(util.GetInfluxOffset(startTime.Unix(), int64(intervalTime)))
												} else {
													startTime, err := formatx.ConvertStringToTime(formatLayout, formatTime, time.Local)
													if err == nil {
														offsetTime = int(util.GetInfluxOffset(startTime.Unix(), int64(intervalTime)))
													}
												}
											}
										}
										break
									}
								}
							}
						}
					}
				}
			}
		}

		if groups, ok := val["group"]; ok {
			if list, ok := groups.([]interface{}); ok {
				hasNoGroup = false
				stringList := formatx.InterfaceListToStringList(list)
				for _, v := range stringList {
					if strings.Contains(v, "time") {
						hasTime = true
						if offsetTime != 0 {
							interval = strings.ReplaceAll(strings.TrimSpace("interval"+strings.Replace(strings.Replace(v, "time", "", -1), "mo", "n", -1)), ")", "") + "," + strconv.Itoa(offsetTime) + "s)"
						} else {
							interval = "interval" + strings.Replace(strings.Replace(v, "time", "", -1), "mo", "n", -1)
						}
					} else {
						withoutOtherGroup = false
					}
				}
			}
		}

		if fields, ok := val["fields"]; ok {
			if list, ok := fields.([]interface{}); ok {
				for i, field := range list {
					if fieldEle, ok := field.(string); ok {
						if strings.Contains(fieldEle, "(") {
							hasNoPloy = false
							break
						}
					} else {
						return nil, fmt.Errorf("fields字段中第%d个的值为空或者类型错误", i+1)
					}

				}
			}
		}

		if fields, ok := val["fields"]; ok {
			if list, ok := fields.([]interface{}); ok {
				if hasNoGroup && hasNoPloy {
					fieldsList = "ts"
				}
				for i, field := range list {
					if _, ok := field.(string); !ok {
						return nil, fmt.Errorf("fields字段中第%d个的值为空或者类型错误", i+1)
					}
					fieldsString := strings.ReplaceAll(field.(string), "MEAN(", "avg(")
					fieldsString = strings.ReplaceAll(fieldsString, "mean(", "avg(")
					fieldsString = strings.ReplaceAll(fieldsString, "\\", "")
					fieldsString = strings.ReplaceAll(fieldsString, "\"", "")

					if strings.Contains(fieldsString, "(") {
						hasNoPloy = false
					}

					if len(fieldsList) != 0 {
						fieldsList = fieldsList + ", " + fieldsString
					} else {
						fieldsList = fieldsList + " " + fieldsString
					}
				}
			}
		} else {
			fieldsList = "*"
		}

		if modelId, ok := val["modelId"].(string); ok {
			tableName = modelId
		}

		if modelName, ok := val["modelName"].(string); ok {
			if tableName == "" {
				queryNodeMap := &bson.M{"filter": bson.M{"name": modelName}}
				modelInfoList := make([]bson.M, 0)
				_, err := p.FindFilter(*ctx, idb.Database.Collection(model.MODEL), &modelInfoList, *queryNodeMap, func(d *bson.M) {})
				if err != nil {
					return nil, fmt.Errorf("查询模型名称为(%s)的模型的ID失败", modelName)
				}
				if len(modelInfoList) == 0 {
					return nil, fmt.Errorf("模型名称为(%s)的模型不存在", modelName)
				}
				for _, v := range modelInfoList {
					modelID, ok := v["id"].(primitive.ObjectID)
					if !ok {
						return nil, fmt.Errorf("模型名称为(%s)的模型ID字段不存在或类型错误", modelName)
					}
					tableName = modelID.Hex()
				}
			}
		}

		if uid, ok := val["uid"].(string); ok {
			if tableName == "" {
				queryNodeMap := &bson.M{"filter": bson.M{"uid": uid}}
				nodeInfoList := make([]bson.M, 0)
				_, err := p.FindFilter(*ctx, idb.Database.Collection(model.NODE), &nodeInfoList, *queryNodeMap, func(d *bson.M) {})
				if err != nil {
					return nil, fmt.Errorf("查询资产编号(%s)的资产信息失败", uid)
				}
				if len(nodeInfoList) == 0 {
					return nil, fmt.Errorf("资产编号(%s)的资产不存在", uid)
				}
				for _, v := range nodeInfoList {
					modelID, ok := v["model"].(primitive.ObjectID)
					if !ok {
						return nil, fmt.Errorf("资产编号(%s)的资产的模型ID字段不存在或类型错误", uid)
					}
					tableName = modelID.Hex()
				}
			}
			assetUIDList = assetUIDList + " where uid = '" + uid + "'"
		}

		if id, ok := val["id"].(string); ok {
			if tableName == "" {
				nodeInfo := bson.M{}
				err := p.FindByID(*ctx, idb.Database.Collection(model.NODE), &nodeInfo, id, func(d *bson.M) {})
				if err != nil {
					return nil, fmt.Errorf("查询资产ID(%s)的资产信息失败", id)
				}
				modelID, ok := nodeInfo["model"].(primitive.ObjectID)
				if !ok {
					return nil, fmt.Errorf("资产ID(%s)的资产的模型ID字段不存在或类型错误", id)
				}
				tableName = modelID.Hex()
			}
			_, okUID := val["uid"]

			if okUID {
				assetIDList = assetIDList + " and id = '" + id + "' "
			} else {
				assetIDList = assetIDList + " where id  = '" + id + "' "
			}
		}

		if list, ok := val["department"].([]interface{}); ok {
			for _, departmentID := range list {
				departmentIDFormat := ""
				if deptID, ok := departmentID.(string); ok {
					if len(deptID) > 18 {
						departmentIDFormat = deptID[len(deptID)-18:]
					}
				}
				if len(departmentIDList) != 0 {
					departmentIDList = departmentIDList + " or department like '%" + departmentIDFormat + "%' "
				} else {
					_, okID := val["id"]
					_, okUID := val["uid"]

					if okID || okUID {
						departmentIDList = departmentIDList + " and department  like '%" + departmentIDFormat + "%' "
					} else {
						departmentIDList = departmentIDList + " where department  like '%" + departmentIDFormat + "%' "
					}
				}
			}
		}

		if list, ok := val["andDeptId"].([]interface{}); ok {
			if len(list) == 0 {
				return nil, fmt.Errorf("权限范围内没有可以查看的资产")
			}
			for _, andDeptID := range list {
				departmentIDFormat := ""
				if deptID, ok := andDeptID.(string); ok {
					if len(deptID) > 18 {
						departmentIDFormat = deptID[len(deptID)-18:]
					}
				}
				if len(andDeptIDList) != 0 {
					andDeptIDList = andDeptIDList + " or department like '%" + departmentIDFormat + "%' "
				} else {
					_, okID := val["id"]
					_, okUID := val["uid"]
					_, okDept := val["department"]

					if okID || okUID || okDept {
						andDeptIDList = andDeptIDList + " and ( department like '%" + departmentIDFormat + "%' "
					} else {
						andDeptIDList = andDeptIDList + " where ( department like '%" + departmentIDFormat + "%' "
					}
				}
			}
			if len(andDeptIDList) > 0 {
				andDeptIDList = andDeptIDList + ") "
			}
		}

		if list, ok := val["where"].([]interface{}); ok {
			for _, whereEle := range list {
				if where, ok := whereEle.(string); ok {
					if interval != "" {
						if !hasTime {
							if strings.Contains(where, "time") {
								if strings.Contains(where, ">") {
									splitList := strings.Split(where, "'")
									if len(splitList) > 2 {
										startTime = splitList[len(splitList)-2]
										if formatx.IsNumber(startTime) {
											startTime = startTime[:len(startTime)-6]
										}
									}
								}
							}
						}
					}

					if strings.Contains(where, ">=") {
						splitList := strings.Split(where, ">=")
						if len(splitList) > 1 {
							if strings.TrimSpace(splitList[0]) == "time" && formatx.IsNumber(strings.TrimSpace(splitList[1])) {
								where = splitList[0] + ">=" + splitList[1][:len(splitList[1])-6]
							}
						}
					} else if strings.Contains(where, ">") {
						splitList := strings.Split(where, ">")
						if len(splitList) > 1 {
							if strings.TrimSpace(splitList[0]) == "time" && formatx.IsNumber(strings.TrimSpace(splitList[1])) {
								where = splitList[0] + ">" + splitList[1][:len(splitList[1])-6]
							}
						}
					} else if strings.Contains(where, "<=") {
						splitList := strings.Split(where, "<=")
						if len(splitList) > 1 {
							if strings.TrimSpace(splitList[0]) == "time" && formatx.IsNumber(strings.TrimSpace(splitList[1])) {
								where = splitList[0] + "<=" + splitList[1][:len(splitList[1])-6]
							}
						}
					} else if strings.Contains(where, "<") {
						splitList := strings.Split(where, "<")
						if len(splitList) > 1 {
							if strings.TrimSpace(splitList[0]) == "time" && formatx.IsNumber(strings.TrimSpace(splitList[1])) {
								where = splitList[0] + "<" + splitList[1][:len(splitList[1])-6]
							}
						}
					}

					where = strings.ReplaceAll(where, "time", "ts")
					where = strings.ReplaceAll(where, "now()", "now")

					if len(whereList) != 0 {
						whereList = whereList + " and " + where
					} else {
						_, okID := val["id"]
						_, okUID := val["uid"]
						_, okDept := val["department"]
						_, okAndID := val["andDeptId"]

						if okID || okAndID || okUID || okDept {
							whereList = whereList + " and " + where
						} else {
							whereList = whereList + " where " + where
						}
					}
				}
			}
		}

		if groups, ok := val["group"]; ok {
			if list, ok := groups.([]interface{}); ok {
				for _, group := range list {
					if strings.Contains(group.(string), "time") {
						continue
					}
					groupWithoutTimeList = append(groupWithoutTimeList, group.(string))
					if len(groupByList) != 0 {
						groupByList = groupByList + "," + group.(string)
					} else {
						groupByList = groupByList + " group by " + group.(string)
					}
				}
			}
		}

		if hasNoGroup && hasNoPloy {
			if fields, ok := val["fields"]; ok {
				if list, ok := fields.([]interface{}); ok {
					for i, field := range list {
						if _, ok := field.(string); !ok {
							return nil, fmt.Errorf("fields字段中第%d个的值为空或者类型错误", i+1)
						}
						fieldsString := strings.ReplaceAll(field.(string), "\\", "")
						fieldsString = strings.ReplaceAll(fieldsString, "\"", "")
						if strings.Contains(fieldsString, " as ") {
							fieldsString = strings.Split(fieldsString, "as")[0]
						}
						if len(whereList) != 0 {
							whereList = whereList + " and " + fieldsString + "<> 'null' "
						} else {
							_, okID := val["id"]
							_, okUID := val["uid"]
							_, okDept := val["department"]
							_, okAndID := val["andDeptId"]
							_, okAndWhere := val["where"]

							if okID || okAndID || okUID || okDept || okAndWhere {
								whereList = whereList + " and " + fieldsString + "<> 'null' "
							} else {
								whereList = whereList + " where " + fieldsString + "<> 'null' "
							}
						}
					}
				}
			}
		}
		if fill, ok := val["fill"]; ok {
			if f, ok := fill.(string); ok {
				if formatx.IsNumber(f) {
					f = "value," + f
				}
				fillValue = "fill(" + f + ")"
			}
		}
		if order, ok := val["order"]; ok {
			if o, ok := order.(string); ok {
				orderByValue = "order by " + o
			}
		}

		if limit, ok := val["limit"]; ok {
			if l, ok := limit.(float64); ok {
				limitValue = fmt.Sprintf("limit %.0f", l)
			}
		}

		if offset, ok := val["offset"]; ok {
			if o, ok := offset.(float64); ok {
				offsetValue = fmt.Sprintf("offset %.0f", o)
			}
		}

		sqlString = sqlString + fmt.Sprintf("select %s from m_%s %s %s %s %s %s %s %s %s %s %s %s ;", fieldsList, tableName, assetUIDList, assetIDList, departmentIDList, andDeptIDList, whereList, interval, groupByList, fillValue, orderByValue, limitValue, offsetValue)
		var queryFunc = func(sqlString string) ([]string, [][]interface{}, error) {
			//db, err := itaos.DB.Get()
			//if err != nil {
			//	return nil, nil, fmt.Errorf("taos 获取连接失败:%s", err.Error())
			//}
			//defer itaos.DB.Put(db)

			//db, err := itaos.GetDB()
			//if err != nil {
			//	return nil, nil, fmt.Errorf("taos 获取连接失败:%s", err.Error())
			//}
			//defer db.Close()
			rows, err := taos.DB.GetDB().Queryx(sqlString) // go text mode
			if err != nil {
				return nil, nil, fmt.Errorf("taos SQL查询错误:%+v", err)
			}
			//fmt.Printf("%10s%s%8s %5s %9s%s %s %8s%s %7s%s %8s%s %4s%s %5s%s\n", " ", "ts", " ", "id", " ", "name", " ", "len", " ", "flag", " ", "notes", " ", "fv", " ", " ", "dv")
			colList, err := rows.Columns()
			if err != nil {
				return nil, nil, fmt.Errorf("taos columns获取失败:%+v", err)
			}
			for i, v := range colList {
				if v == "ts" {
					colList[i] = "time"
				}
			}
			resultCombineList := make([][]interface{}, 0)
			//fmt.Println("group  colList:", colList)
			for rows.Next() {
				resultList, err := rows.SliceScan()
				if err != nil {
					return nil, nil, fmt.Errorf("taos 结果数组获取失败:%+v", err)
				}
				logger.Debugf(nil, "rows.Next()  resultList:%+v", resultList)
				for i, v := range resultList {
					if ele, ok := v.(string); ok {
						if strings.Contains(ele, ":") {
							queryTime, err := formatx.ConvertStringToTime("2006-01-02 15:04:05.000", ele, time.Local)
							if err == nil {
								resultList[i] = queryTime.Format("2006-01-02T15:04:05.000+08:00")
							} else {
								queryTime, err := formatx.ConvertStringToTime("2006-01-02 15:04:05.00", ele, time.Local)
								if err == nil {
									splitTime := strings.Split(ele, ".")
									ele := splitTime[0] + ".0" + splitTime[1]
									queryTime, err = formatx.ConvertStringToTime("2006-01-02 15:04:05.000", ele, time.Local)
									if err == nil {
										resultList[i] = queryTime.Format("2006-01-02T15:04:05.000+08:00")
									}
								} else {
									queryTime, err := formatx.ConvertStringToTime("2006-01-02 15:04:05.0", ele, time.Local)
									if err == nil {
										splitTime := strings.Split(ele, ".")
										ele := splitTime[0] + ".00" + splitTime[1]
										queryTime, err = formatx.ConvertStringToTime("2006-01-02 15:04:05.000", ele, time.Local)
										if err == nil {
											resultList[i] = queryTime.Format("2006-01-02T15:04:05.000+08:00")
										}
									} else {
										resultList[i] = strings.ReplaceAll(ele, "\u0000", "")
									}
								}
							}
						} else {
							resultList[i] = strings.ReplaceAll(ele, "\u0000", "")
						}
					}
				}
				resultCombineList = append(resultCombineList, resultList)
			}

			return colList, resultCombineList, nil
		}
		colList, resultCombineList, err := queryFunc(sqlString)
		if err != nil {
			return nil, err
		}
		series := make([]map[string]interface{}, 0)
		if withoutOtherGroup {
			seriesMap := p.formatSelectData(colList, resultCombineList, tableName)
			series = append(series, seriesMap)
		} else {
			if !hasTime {
				series = p.formatGroupWithStartTimeData(colList, resultCombineList, tableName, startTime, groupWithoutTimeList)
			} else {
				series = p.formatGroupData(colList, resultCombineList, tableName, groupWithoutTimeList)
			}
		}
		resultMap := p.formatSeriesData(series)
		result = append(result, resultMap)
	}

	formatResult = map[string]interface{}{
		"results": result,
	}
	return nil, nil
}
