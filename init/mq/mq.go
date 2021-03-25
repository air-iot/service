package mq

// MQ is a mq interface
type MQ interface {
	Publish(topic string, payload []byte) error
	Consume(topic string, handler func(topic string, payload []byte)) error
	UnSubscription(sub string) error
}
