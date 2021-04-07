package mq

import (
	"context"
	"strings"

	"github.com/streadway/amqp"

	"github.com/air-iot/service/config"
	"github.com/air-iot/service/errors"
	"github.com/air-iot/service/logger"
)

type rabbit struct {
	//queue    string
	//exchange string
	conn *amqp.Connection
}

const TOPICSEPWITHRABBIT = "."

func NewRabbit(conn *amqp.Connection) MQ {
	m := new(rabbit)
	m.conn = conn
	return m
}

func NewRabbitClient(cfg config.RabbitMQ) (*amqp.Connection, func(), error) {
	conn, err := amqp.Dial(cfg.DNS())
	if err != nil {
		return nil, nil, err
	}
	cleanFunc := func() {
		err := conn.Close()
		if err != nil {
			logger.Errorf("rabbitmq close error: %s", err.Error())
		}
	}
	return conn, cleanFunc, nil
}

func (p *rabbit) NewQueue(channel *amqp.Channel, queueName string) (*amqp.Queue, error) {
	queue, err := channel.QueueDeclare(
		queueName, // name
		true,      // durable
		true,      // delete when unused
		false,     // exclusive
		false,     // no-wait
		nil,       // arguments
	)
	if err != nil {
		return nil, err
	}
	return &queue, nil
}

func (p *rabbit) NewExchange(channel *amqp.Channel, exchange string) error {
	return channel.ExchangeDeclare(
		exchange, // name
		"topic",  // type
		true,     // durable
		false,    // auto-deleted
		false,    // internal
		false,    // no-wait
		nil,      // arguments
	)
}

func (p *rabbit) Publish(ctx context.Context, topicParams []string, payload []byte) error {
	channel, err := p.conn.Channel()
	if err != nil {
		return err
	}
	exchange, ok := ctx.Value("exchange").(string)
	if !ok {
		return errors.New("context exchange not found")
	}
	topic := strings.Join(topicParams, TOPICSEPWITHRABBIT)
	return channel.Publish(
		exchange, // exchange
		topic,    // routing key
		false,    // mandatory
		false,
		amqp.Publishing{
			DeliveryMode: amqp.Transient,
			ContentType:  "text/plain",
			Body:         payload,
		})
}

func (p *rabbit) Consume(ctx context.Context, topicParams []string, splitN int, handler Handler) error {
	channel, err := p.conn.Channel()
	if err != nil {
		return err
	}

	queue, ok := ctx.Value("exchange").(string)
	if !ok {
		return errors.New("context queue not found")
	}

	exchange, ok := ctx.Value("exchange").(string)
	if !ok {
		return errors.New("context exchange not found")
	}
	q, err := p.NewQueue(channel, queue)
	if err != nil {
		return err
	}
	err = p.NewExchange(channel, exchange)
	if err != nil {
		return err
	}
	topic := strings.Join(topicParams, TOPICSEPWITHRABBIT)
	err = channel.QueueBind(
		q.Name,   // queue name
		topic,    // routing key
		exchange, // exchange
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
			handler(d.RoutingKey, strings.SplitN(d.RoutingKey, TOPICSEPWITHRABBIT, splitN), d.Body)
			//if err := d.Ack(false); err != nil {}
		}
	}()
	return nil
}

func (p *rabbit) UnSubscription(ctx context.Context, topicParams []string) error {
	topic := strings.Join(topicParams, TOPICSEPWITHRABBIT)
	channel, err := p.conn.Channel()
	if err != nil {
		return err
	}
	return channel.Cancel(topic, true)
}
