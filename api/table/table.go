package table

import (
	"net"
	"net/url"
	"strconv"

	jsoniter "github.com/json-iterator/go"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/traefik"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type TableClient interface {
	FindQuery(query, result interface{}) error
	Save(data, result interface{}) error
	DelById(id string, result interface{}) error
	UpdateById(id string, data, result interface{}) error
	ReplaceById(id string, data, result interface{}) error
}

type tableClient struct {
	url   url.URL
	token string
}

func NewTableClient() TableClient {
	cli := new(tableClient)
	if traefik.Enable {
		u := url.URL{Host: net.JoinHostPort(traefik.Host, strconv.Itoa(traefik.Port)), Path: "core/table"}
		u.Scheme = traefik.Proto
		cli.url = u
		cli.token = api.FindToken()
	} else {
		u := url.URL{Host: "core:9000", Path: "core/table"}
		u.Scheme = traefik.Proto
		cli.url = u
	}
	return cli
}
func (p *tableClient) FindQuery(query, result interface{}) error {
	return api.Get(p.url, p.token, query, result)
}

func (p *tableClient) Save(data, result interface{}) error {
	return api.Post(p.url, p.token, data, result)
}

func (p *tableClient) DelById(id string, result interface{}) error {
	return api.Delete(p.url, p.token, id, result)
}

func (p *tableClient) UpdateById(id string, data, result interface{}) error {
	return api.Patch(p.url, p.token, id, data, result)
}

func (p *tableClient) ReplaceById(id string, data, result interface{}) error {
	return api.Put(p.url, p.token, id, data, result)
}
