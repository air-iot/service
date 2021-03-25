package mq

import (
	"github.com/streadway/amqp"
)

type rabbit struct {
	queue    string
	exchange string
	conn     *amqp.Connection
}

func NewRabbit(conn *amqp.Connection, exchange, queue string) MQ {
	m := new(rabbit)
	m.conn = conn
	m.exchange = exchange
	m.queue = queue
	return m
}

func (p *rabbit) newQueue(channel *amqp.Channel) (*amqp.Queue, error) {
	queue, err := channel.QueueDeclare(
		p.queue, // name
		true,    // durable
		true,    // delete when unused
		false,   // exclusive
		false,   // no-wait
		nil,     // arguments
	)
	if err != nil {
		return nil, err
	}
	return &queue, nil
}

func (p *rabbit) newExchange(channel *amqp.Channel) error {
	return channel.ExchangeDeclare(
		p.exchange, // name
		"topic",    // type
		true,       // durable
		false,      // auto-deleted
		false,      // internal
		false,      // no-wait
		nil,        // arguments
	)
}

func (p *rabbit) Publish(topic string, payload []byte) error {
	channel, err := p.conn.Channel()
	if err != nil {
		return err
	}
	return channel.Publish(
		p.exchange, // exchange
		topic,      // routing key
		false,      // mandatory
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Transient,
			ContentType:  "text/plain",
			Body:         payload,
		})
}

func (p *rabbit) Consume(topic string, handler func(topic string, payload []byte)) error {
	channel, err := p.conn.Channel()
	if err != nil {
		return err
	}
	q, err := p.newQueue(channel)
	if err != nil {
		return err
	}
	err = p.newExchange(channel)
	if err != nil {
		return err
	}
	err = channel.QueueBind(
		q.Name,     // queue name
		topic,      // routing key
		p.exchange, // exchange
		false,
		nil,
	)
	if err != nil {
		return err
	}
	err = channel.Qos(
		1,     // prefetch count
		0,     // prefetch size
		false, // global
	)
	if err != nil {
		return err
	}
	messages, err := channel.Consume(
		q.Name, // queue
		topic,  // consumer
		true,   // auto-ack
		false,  // exclusive
		false,  // no-local
		false,  // no-wait
		nil,    // args
	)
	if err != nil {
		return err
	}
	go func() {
		for d := range messages {
			handler(d.RoutingKey, d.Body)
			//if err := d.Ack(false); err != nil {}
		}
	}()
	return nil
}

func (p *rabbit) UnSubscription(consume string) error {
	channel, err := p.conn.Channel()
	if err != nil {
		return err
	}
	return channel.Cancel(consume, true)
}
