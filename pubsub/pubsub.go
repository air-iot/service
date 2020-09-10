package pubsub

import (
	"github.com/air-iot/service/model"
	"sync"
	"time"
)

type (
	Subscriber chan model.DataMessage
	TopicFunc  func(model.DataMessage) bool
)

type Publisher struct {
	// subscribers 是程序的核心，订阅者都会注册在这里，publisher发布消息的时候也会从这里开始
	subscribers map[Subscriber]TopicFunc
	buffer      int           // 订阅者的缓冲区长度
	timeout     time.Duration // publisher 发送消息的超时时间
	// m 用来保护 subscribers
	// 当修改 subscribers 的时候(即新加订阅者或删除订阅者)使用写锁
	// 当向某个订阅者发送消息的时候(即向某个 Subscriber channel 中写入数据)，使用读锁
	m sync.RWMutex
}

func NewPublisher(publishTimeout time.Duration, buffer int) *Publisher {
	return &Publisher{
		buffer:      buffer,
		timeout:     publishTimeout,
		subscribers: make(map[Subscriber]TopicFunc),
	}
}

func (p *Publisher) Subscribe() Subscriber {
	return p.SubscribeTopic(nil)
}

func (p *Publisher) SubscribeTopic(topic TopicFunc) Subscriber {
	ch := make(Subscriber, p.buffer)
	p.m.Lock()
	p.subscribers[ch] = topic
	p.m.Unlock()

	return ch
}

//Evict 删除掉某个订阅者
func (p *Publisher) Evict(sub Subscriber) {
	p.m.Lock()
	defer p.m.Unlock()

	delete(p.subscribers, sub)
	close(sub)
}

func (p *Publisher) Publish(v model.DataMessage) {
	p.m.RLock()
	defer p.m.RUnlock()

	var wg sync.WaitGroup
	// 同时向所有订阅者写消息，订阅者利用 topic 过滤消息
	for sub, topic := range p.subscribers {
		wg.Add(1)
		go p.sendTopic(sub, topic, v, &wg)
	}

	wg.Wait()
}

//Close 关闭 Publisher，删除所有订阅者
func (p *Publisher) Close() {
	p.m.Lock()
	defer p.m.Unlock()

	for sub := range p.subscribers {
		delete(p.subscribers, sub)
		close(sub)
	}
}

func (p *Publisher) sendTopic(sub Subscriber, topic TopicFunc, v model.DataMessage, wg *sync.WaitGroup) {
	defer wg.Done()

	if topic != nil && !topic(v) {
		return
	}

	select {
	case sub <- v:
	case <-time.After(p.timeout):
	}
}
