package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"sync"
	"time"

	"github.com/Masterminds/sprig"
	"github.com/air-iot/service/api/v2"
	"github.com/air-iot/service/model"
	"github.com/air-iot/service/mq/mqtt"
	"github.com/air-iot/service/mq/rabbit"
	"github.com/air-iot/service/mq/srv"
	"github.com/air-iot/service/pubsub"
	"github.com/air-iot/service/tools"
	"github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

type Gateway struct {
	//ctx       *context.Context
	apiClient api.Client
	p         *pubsub.Publisher
	c         *cron.Cron
}

type DataHandlerFunc func(*bytes.Buffer)

func NewGateway(timeout time.Duration, bufferLen int) *Gateway {
	var gw = new(Gateway)
	gw.apiClient = api.NewClient()
	gw.p = pubsub.NewPublisher(timeout, bufferLen)
	gw.c = cron.New(cron.WithSeconds(), cron.WithChain(cron.DelayIfStillRunning(cron.DefaultLogger)))
	gw.c.Start()
	return gw
}

func (p *Gateway) FindGateway(gatewayType string, result interface{}) error {
	err := p.apiClient.FindGatewayQuery(map[string]interface{}{
		"filter": map[string]interface{}{"type": gatewayType},
		"project": map[string]int{
			"name":     1,
			"type":     1,
			"settings": 1,
		}}, result)
	if err != nil {
		return fmt.Errorf("查询网关数据错误,%s", err.Error())
	}
	return nil
}

func (p *Gateway) ReceiveMQ(ctx context.Context, uids []string) error {
	for _, uid := range uids {
		err := srv.DefaultRealtimeUidDataHandler(uid, func(topic string, payload []byte) {
			msg := model.DataMessage{}
			if err := json.Unmarshal(payload, &msg); err != nil {
				logrus.Warnf("接收采集数据格式转换错误:%s", err.Error())
				return
			}
			msg.Uid = uid
			logrus.Debugf("%s 接收到的数据为: %+v", uid, msg)
			p.p.Publish(msg)
		})
		if err != nil {
			return fmt.Errorf("订阅节点 %s 数据错误:%s", uid, err.Error())
		}
		go func(uid string) {
			for {
				select {
				case <-ctx.Done():
					logrus.Infoln("取消订阅,", uid)
					switch viper.GetString("data.action") {
					case "rabbit":
						if err := rabbit.Channel.Cancel(rabbit.RoutingKey+uid, true); err != nil {
							logrus.Errorf("取消 %s 订阅错误:%s", uid, err.Error())
							return
						}
					default:
						if token := mqtt.Client.Unsubscribe(mqtt.Topic + uid); token.Wait() && token.Error() != nil {
							logrus.Errorf("取消 %s 订阅错误:%s", uid, token.Error().Error())
							return
						}
					}
					return
				}
			}
		}(uid)
	}
	return nil
}

func (p *Gateway) Stop() {
	p.c.Stop()
}

func (p *Gateway) GatewayReceiveData(ctx context.Context, uids []string, gatewayName, templateText, messageType string, interval int, handler DataHandlerFunc) {
	nodeDataMsgChan := p.p.SubscribeTopic(func(msg model.DataMessage) bool {
		return tools.IsContain(uids, msg.Uid)
	})

	if messageType == "realData" {
		go func() {
			for {
				select {
				case msg := <-nodeDataMsgChan:
					dataRes := make([]model.RealTimeData, 0)
					currentTime := time.Now().Local()
					if msg.Time > 0 {
						currentTime = time.Unix(0, msg.Time*int64(time.Millisecond))
					}
					for k, v := range msg.Fields {
						dataRes = append(dataRes, model.RealTimeData{
							TagId: k,
							Uid:   msg.Uid,
							Time:  currentTime.UnixNano() / 10e6,
							Value: v,
						})
					}

					if templateText == "" {
						b1, err := json.Marshal(&dataRes)
						if err != nil {
							logrus.Errorf("网关 %s 序列化错误: %s", gatewayName, err.Error())
						} else {
							bufferData := &bytes.Buffer{}
							if _, err := bufferData.Write(b1); err != nil {
								logrus.Errorf("网关 %s buffer write错误: %s", gatewayName, err.Error())
							} else {
								handler(bufferData)
							}
						}

					} else {
						bufferData, err := p.Template(gatewayName, templateText, dataRes)
						if err != nil {
							logrus.Errorf("网关 %s 启动错误: %s", gatewayName, err.Error())
						} else {
							handler(bufferData)
						}
					}
				case <-ctx.Done():
					return
				}
			}
		}()
	} else {
		cache := sync.Map{}
		cacheDataFunc := func(msg model.DataMessage) {
			currentTime := time.Now().Local()
			if msg.Time > 0 {
				currentTime = time.Unix(0, msg.Time*int64(time.Millisecond))
			}
			for k, v := range msg.Fields {
				cache.Store(msg.Uid+"|"+k, model.RealTimeData{
					TagId: k,
					Uid:   msg.Uid,
					Time:  currentTime.UnixNano() / 10e6,
					Value: v,
				})
			}
		}
		entryId, err := p.c.AddFunc(fmt.Sprintf("@every %ds", interval), func() {
			dataRes := make([]model.RealTimeData, 0)
			cache.Range(func(key, value interface{}) bool {
				if val, ok := value.(model.RealTimeData); ok {
					dataRes = append(dataRes, val)
				}
				cache.Delete(key)
				return true
			})
			if templateText == "" {
				b1, err := json.Marshal(&dataRes)
				if err != nil {
					logrus.Errorf("网关 %s 序列化错误: %s", gatewayName, err.Error())
				} else {
					bufferData := &bytes.Buffer{}
					if _, err := bufferData.Write(b1); err != nil {
						logrus.Errorf("网关 %s buffer write错误: %s", gatewayName, err.Error())
					} else {
						handler(bufferData)
					}
				}

			} else {
				bufferData, err := p.Template(gatewayName, templateText, dataRes)
				if err != nil {
					logrus.Errorf("网关 %s 启动错误: %s", gatewayName, err.Error())
				} else {
					handler(bufferData)
				}
			}
		})
		if err != nil {
			logrus.Errorf("网关 %s 定时启动错误: %s", gatewayName, err.Error())
			return
		}

		go func() {
			for {
				select {
				case msg := <-nodeDataMsgChan:
					//handler(msg)
					cacheDataFunc(msg)
				case <-ctx.Done():
					p.c.Remove(entryId)
					return
				}
			}
		}()
	}
}

func (p *Gateway) Template(templateName, templateText string, data []model.RealTimeData) (*bytes.Buffer, error) {
	t := template.New(templateName).Funcs(sprig.FuncMap())
	t = template.Must(t.Parse(templateText))
	d1 := &model.RealTimeDataTemplate{
		Length: len(data),
		Data:   data,
		Time:   time.Now().Unix(),
	}
	b := new(bytes.Buffer)
	if err := t.Execute(b, d1); err != nil {
		return nil, err
	}
	return b, nil
}

func (p *Gateway) TemplateRealData(templateName, templateText string, data model.RealTimeData) (*bytes.Buffer, error) {
	t := template.New(templateName).Funcs(sprig.FuncMap())
	t = template.Must(t.Parse(templateText))
	b := new(bytes.Buffer)
	if err := t.Execute(b, data); err != nil {
		return nil, err
	}
	return b, nil
}
