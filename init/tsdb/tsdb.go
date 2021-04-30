package tsdb

import "time"

// TSDB 时序数据库接口
type TSDB interface {
	// Write 写数据
	Write(database, tableName string, ts time.Time, tags map[string]string, fields map[string]interface{}) error
	// Query makes an InfluxDB Query on the database. This will fail if using
	// the UDP client.
	Query(database string, sql string, result interface{}) error
}
