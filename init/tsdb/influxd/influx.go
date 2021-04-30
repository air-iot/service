package influxd

import (
	"strings"
	"time"

	"github.com/air-iot/service/config"
	"github.com/air-iot/service/logger"
	client "github.com/influxdata/influxdb1-client/v2"
)

// Influx influx存储
type Influx struct {
	Cli client.Client
}

// NewInflux 创建influx存储
func NewInflux(cfg config.Influx) (client.Client, func(), error) {
	var cli client.Client
	var err error
	switch strings.ToUpper(cfg.Protocol) {
	case "UDP":
		cli, err = client.NewUDPClient(client.UDPConfig{
			Addr: cfg.Addr,
		})
	default:
		cli, err = client.NewHTTPClient(client.HTTPConfig{
			Addr:     cfg.Addr,
			Username: cfg.Username,
			Password: cfg.Password,
		})
	}

	if err != nil {
		return nil, nil, err
	}
	cleanFunc := func() {
		err := cli.Close()
		if err != nil {
			logger.Errorf("influxdb close error: %s", err.Error())
		}
	}
	_, _, err = cli.Ping(time.Second * 10)

	if err != nil {
		return nil, cleanFunc, err
	}

	return cli, cleanFunc, nil
}

func (a *Influx) Write(database, tableName string, ts time.Time, tags map[string]string, fields map[string]interface{}) (err error) {

	return nil
}

func (a *Influx) Query(database string, sql string, result interface{}) (err error) {

	return nil
}
