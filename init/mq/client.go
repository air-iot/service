package mq

import (
	"strings"
	"time"

	MQTT "github.com/eclipse/paho.mqtt.golang"
	"github.com/streadway/amqp"

	"github.com/air-iot/service/config"
	"github.com/air-iot/service/logger"
)

// InitMQ 初始化消息队列
func InitMQ() (MQ, func(), error) {
	cfg := config.C.MQ

	switch strings.ToUpper(cfg.MQType) {
	case "RABBIT":
		rabbitCfg := config.C.RabbitMQ
		conn, err := amqp.Dial(rabbitCfg.DNS())
		if err != nil {
			return nil, nil, err
		}
		cleanFunc := func() {
			err := conn.Close()
			if err != nil {
				logger.Errorf("rabbitmq close error: %s", err.Error())
			}
		}
		rabbit := NewRabbit(conn, rabbitCfg.Exchange, rabbitCfg.Queue)
		return rabbit, cleanFunc, nil
	default:
		mqttCfg := config.C.MQTT
		opts := MQTT.NewClientOptions()
		opts.AddBroker(mqttCfg.DNS())
		opts.SetAutoReconnect(true)
		opts.SetCleanSession(true)
		opts.SetUsername(mqttCfg.Username)
		opts.SetPassword(mqttCfg.Password)
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
		mqtt := NewMQTT(client)
		return mqtt, cleanFunc, nil
	}

}
