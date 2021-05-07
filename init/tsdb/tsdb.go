package tsdb

import (
	"time"

	client "github.com/influxdata/influxdb1-client/v2"
)

// TSDB 时序数据库接口
type TSDB interface {
	// Write 写数据
	Write(database string, row []Row) error
	// Query 查询数据
	Query(database string, sql string) (res []client.Result, err error)
}

// Row 每行数据
type Row struct {
	TableName    string
	SubTableName string
	Ts           time.Time
	Tags         map[string]string
	Fields       map[string]interface{}
}
