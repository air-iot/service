package zeebe

import (
	"context"
	"fmt"

	"github.com/air-iot/service/config"
	"github.com/air-iot/service/logger"
	"github.com/camunda-cloud/zeebe/clients/go/pkg/entities"
	"github.com/camunda-cloud/zeebe/clients/go/pkg/worker"
	"github.com/camunda-cloud/zeebe/clients/go/pkg/zbc"
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

func FailJob(ctx context.Context, client worker.JobClient, job entities.Job, format string, a ...interface{}) error {
	_, err := client.NewFailJobCommand().JobKey(job.GetKey()).Retries(job.Retries - 1).ErrorMessage(fmt.Sprintf(format, a...)).Send(ctx)
	if err != nil {
		return fmt.Errorf("fail job %d: %s", job.GetKey(), err.Error())
	}
	return nil
}
