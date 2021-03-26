package influxdb

import (
	"strings"
	"time"

	_ "github.com/influxdata/influxdb1-client" // this is important because of the bug in go mod
	client "github.com/influxdata/influxdb1-client/v2"

	"github.com/air-iot/service/config"
	"github.com/air-iot/service/logger"
)

// InitInfluxDB 初始化influx存储
func InitInfluxDB() (client.Client, func(), error) {
	cfg := config.C.Influx
	return NewInfluxDB(cfg)
}

// NewInfluxDB 创建influx存储
func NewInfluxDB(cfg config.Influx) (client.Client, func(), error) {
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
