package handler

import (
	"net"
	"net/url"
	"strconv"

	jsoniter "github.com/json-iterator/go"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/traefik"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type EventHandlerClient interface {
	FindQuery(query, result interface{}) error
	FindById(id string, result interface{}) error
	Save(data, result interface{}) error
	DelById(id string, result interface{}) error
	UpdateById(id string, data, result interface{}) error
	ReplaceById(id string, data, result interface{}) error
}

type eventHandlerClient struct {
	url   url.URL
	token string
}

func NewEventHandlerClient() EventHandlerClient {
	cli := new(eventHandlerClient)
	if traefik.Enable {
		u := url.URL{Host: net.JoinHostPort(traefik.Host, strconv.Itoa(traefik.Port)), Path: "event/eventHandler"}
		u.Scheme = traefik.Proto
		cli.url = u
		cli.token = api.FindToken()
	} else {
		u := url.URL{Host: "event:9000", Path: "event/eventHandler"}
		u.Scheme = traefik.Proto
		cli.url = u
	}
	return cli
}

func (p *eventHandlerClient) FindQuery(query, result interface{}) error {
	return api.Get(p.url, p.token, query, result)
}

func (p *eventHandlerClient) FindById(id string, result interface{}) error {
	return api.GetById(p.url, p.token, id, result)
}

func (p *eventHandlerClient) Save(data, result interface{}) error {
	return api.Post(p.url, p.token, data, result)
}

func (p *eventHandlerClient) DelById(id string, result interface{}) error {
	return api.Delete(p.url, p.token, id, result)
}

func (p *eventHandlerClient) UpdateById(id string, data, result interface{}) error {
	return api.Patch(p.url, p.token, id, data, result)
}

func (p *eventHandlerClient) ReplaceById(id string, data, result interface{}) error {
	return api.Put(p.url, p.token, id, data, result)
}
