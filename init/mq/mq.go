package mq

import "context"

// MQ is a mq interface
type MQ interface {
	Publish(ctx context.Context, topicParams []string, payload []byte) error
	Consume(ctx context.Context, topicParams []string, splitN int, handler Handler) error
	UnSubscription(ctx context.Context, sub []string) error
}
