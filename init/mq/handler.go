package mq

import (
	"context"
)

const (
	DataTopicWithMQTT   = "data/#"
	DataTopicWithRabbit = "data.#"
)

// MQConfig 消息队列参数
type MQConfig struct {
	MQtype   string
	Exchange string
	Queue    string
}

// ConsumeData 采集数据
func ConsumeData(cli MQ, config MQConfig, handler Handler) error {
	ctx := context.Background()
	switch config.MQtype {
	case Rabbit:
		ctx = context.WithValue(ctx, "exchange", config.Exchange)
		ctx = context.WithValue(ctx, "queue", config.Queue)
		return cli.Consume(ctx, DataTopicWithRabbit, handler)
	default:
		return cli.Consume(ctx, DataTopicWithMQTT, handler)
	}
}
