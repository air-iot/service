package pubsub

import (
	"github.com/air-iot/service/model"
	"sync"
	"testing"
	"time"
)

func TestPubSub(t *testing.T) {
	p := NewPublisher(100*time.Millisecond, 10)
	defer p.Close()

	// all 订阅者订阅所有消息
	all := p.Subscribe()
	// golang 订阅者仅订阅包含 golang 的消息
	golang := p.SubscribeTopic(func(v model.DataMessage) bool {
		//if s, ok := v.(string); ok {
		//	return strings.Contains(s, "golang")
		//}
		return v.NodeID == "nodeid1"
	})

	p.Publish(model.DataMessage{
		ModelID:  "",
		NodeID:   "nodeid1",
		Fields:   nil,
		Uid:      "",
		Time:     0,
		InputMap: nil,
	})
	p.Publish(model.DataMessage{
		ModelID:  "",
		NodeID:   "nodeid2",
		Fields:   nil,
		Uid:      "",
		Time:     0,
		InputMap: nil,
	})

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		for msg := range all {
			println("all", msg.NodeID)
			//assert.True(t, ok)
		}
		wg.Done()
	}()

	go func() {
		for msg := range golang {
			//v, ok := msg.(string)
			println("golang", msg.NodeID)
			//assert.True(t, ok)
			//assert.True(t, strings.Contains(v, "golang"))
		}
		wg.Done()
	}()
	p.Evict(all)
	p.Close()
	wg.Wait()
}
