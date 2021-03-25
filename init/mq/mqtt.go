package mq

import (
	MQTT "github.com/eclipse/paho.mqtt.golang"
)

type mqtt struct {
	client MQTT.Client
}

func NewMQTT(cli MQTT.Client) MQ {
	m := new(mqtt)
	m.client = cli
	return m
}

func (p *mqtt) Publish(topic string, payload []byte) error {
	if token := p.client.Publish(topic, 0, false, string(payload)); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (p *mqtt) Consume(topic string, handler func(topic string, payload []byte)) error {
	if token := p.client.Subscribe(topic, 0, func(client MQTT.Client, message MQTT.Message) {
		handler(message.Topic(), message.Payload())
	}); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

func (p *mqtt) UnSubscription(topic string) error {
	if token := p.client.Unsubscribe(topic); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}
