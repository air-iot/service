package influxdb2

import (
	client "github.com/influxdata/influxdb-client-go/v2"

	"github.com/air-iot/service/config"
)

// InitInfluxDB 初始化influx存储
func InitInfluxDB() (client.Client, func(), error) {
	cfg := config.C.TSDB.Influx2
	return NewInfluxDB(cfg)
}

// NewInfluxDB 创建influx存储
func NewInfluxDB(cfg config.Influx2) (client.Client, func(), error) {
	var cli client.Client
	opts := client.DefaultOptions()
	if cfg.Timeout != nil {
		opts.SetHTTPRequestTimeout(*cfg.Timeout)
	}
	if cfg.UseGZip != nil {
		opts.SetUseGZip(*cfg.UseGZip)
	}
	if cfg.LogLevel != nil {
		opts.SetLogLevel(*cfg.LogLevel)
	}
	if cfg.MaxRetries != nil {
		opts.SetMaxRetries(*cfg.MaxRetries)
	}
	cli = client.NewClientWithOptions(cfg.Addr, cfg.Token, opts)
	cleanFunc := func() {
		cli.Close()
	}
	return cli, cleanFunc, nil
}
