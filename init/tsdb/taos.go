package tsdb

import (
	"context"
	"fmt"
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
	return nil, nil
}