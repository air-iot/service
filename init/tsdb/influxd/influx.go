package influxd

import (
	"time"

	client "github.com/influxdata/influxdb1-client/v2"

	"github.com/air-iot/service/init/tsdb"
)

// Influx influx存储
type Influx struct {
	cli client.Client
}

func NewInflux(cli client.Client) tsdb.TSDB {
	a := new(Influx)
	a.cli = cli
	return a
}

func (a *Influx) Write(database, tableName string, ts time.Time, tags map[string]string, fields map[string]interface{}) (err error) {

	return nil
}

func (a *Influx) Query(database string, sql string, result interface{}) (err error) {

	return nil
}
