package mq

import (
	"context"
	"strings"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"

	"github.com/air-iot/service/config"
	"github.com/air-iot/service/logger"
)

type mqtt struct {
	client MQTT.Client
}

const TOPICSEPWITHMQTT = "/"

func NewMQTT(cli MQTT.Client) MQ {
	m := new(mqtt)
	m.client = cli
	return m
}

// NewMQTTClient 创建MQTT消息队列
func NewMQTTClient(cfg config.MQTT) (MQTT.Client, func(), error) {
	opts := MQTT.NewClientOptions()
	opts.AddBroker(cfg.DNS())
	opts.SetAutoReconnect(true)
	opts.SetCleanSession(true)
	opts.SetUsername(cfg.Username)
	opts.SetPassword(cfg.Password)
	opts.SetConnectTimeout(time.Second * 20)
	opts.SetKeepAlive(time.Second * 60)
	opts.SetProtocolVersion(4)
	opts.SetConnectionLostHandler(func(client MQTT.Client, e error) {
		if e != nil {
			logger.Fatalf("MQTT Lost错误: %s", e.Error())
		}
	})
	opts.SetOrderMatters(false)
	// Start the connection
	client := MQTT.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, nil, token.Error()
	}
	cleanFunc := func() {
		client.Disconnect(250)
	}
	return client, cleanFunc, nil

}

func (p *mqtt) Publish(ctx context.Context, topicParams []string, payload []byte) error {
	topic := strings.Join(topicParams, TOPICSEPWITHMQTT)
	if token := p.client.Publish(topic, 0, false, string(payload)); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (p *mqtt) Consume(ctx context.Context, topicParams []string, splitN int, handler Handler) error {
	topic := strings.Join(topicParams, TOPICSEPWITHMQTT)
	if token := p.client.Subscribe(topic, 0, func(client MQTT.Client, message MQTT.Message) {
		handler(message.Topic(), strings.SplitN(message.Topic(), TOPICSEPWITHMQTT, splitN), message.Payload())
	}); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (p *mqtt) UnSubscription(ctx context.Context, topicParams []string) error {
	topic := strings.Join(topicParams, TOPICSEPWITHMQTT)
	if token := p.client.Unsubscribe(topic); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}
