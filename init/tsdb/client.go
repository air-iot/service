package tsdb

import (
	"strings"

	"github.com/air-iot/service/config"
	"github.com/air-iot/service/init/influxdb"
)

// DBType 数据库类型
type DBType string

const (
	InfluxType DBType = "INFLUX"
	TaosType   DBType = "TAOS"
)

// NewTSDB 创建时序数据
func NewTSDB(cfg config.TSDB) (TSDB, func(), error) {
	switch DBType(strings.ToUpper(cfg.DBType)) {
	case TaosType:
		a := NewTaos(cfg.Taos)
		cleanFunc := func() {}
		return a, cleanFunc, nil
	default:
		cli, cleanFunc, err := influxdb.NewInfluxDB(cfg.Influx)
		if err != nil {
			return nil, nil, err
		}
		a := NewInflux(cli)

		return a, cleanFunc, nil
	}

}
