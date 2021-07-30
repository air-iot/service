package zeebe

import (
	"github.com/camunda-cloud/zeebe/clients/go/pkg/zbc"

	"github.com/air-iot/service/config"
	"github.com/air-iot/service/logger"
)

// NewZeebe 创建zeebe客户端
func NewZeebe(cfg config.Zeebe) (zbc.Client, func(), error) {

	zbClient, err := zbc.NewClient(&zbc.ClientConfig{
		GatewayAddress:         cfg.GatewayAddress,
		UsePlaintextConnection: cfg.UsePlaintextConnection,
	})

	if err != nil {
		return nil, nil, err
	}
	cleanFunc := func() {
		err := zbClient.Close()
		if err != nil {
			logger.Errorf("zb close error: %s", err.Error())
		}
	}

	return zbClient, cleanFunc, nil
}
