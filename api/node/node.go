package node

import (
	"fmt"
	"net"
	"net/url"
	"strconv"

	jsoniter "github.com/json-iterator/go"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/traefik"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type NodeClient interface {
	FindQuery(query, result interface{}) error
	FindById(id string, result interface{}) error
	Save(data, result interface{}) error
	DelById(id string, result interface{}) error
	UpdateById(id string, data, result interface{}) error
	ReplaceById(id string, data, result interface{}) error
	FindTagById(id string, result interface{}) error
}

type nodeClient struct {
	url   url.URL
	token string
}

func NewNodeClient() NodeClient {
	cli := new(nodeClient)
	if traefik.Enable {
		u := url.URL{Host: net.JoinHostPort(traefik.Host, strconv.Itoa(traefik.Port)), Path: "core/node"}
		u.Scheme = traefik.Proto
		cli.url = u
		cli.token = api.FindToken()
	} else {
		u := url.URL{Host: "core:9000", Path: "core/node"}
		u.Scheme = traefik.Proto
		cli.url = u
	}
	return cli
}

func (p *nodeClient) FindQuery(query, result interface{}) error {
	return api.Get(p.url, p.token, query, result)
}

func (p *nodeClient) FindById(id string, result interface{}) error {
	return api.GetById(p.url, p.token, id, result)
}

func (p *nodeClient) Save(data, result interface{}) error {
	return api.Post(p.url, p.token, data, result)
}

func (p *nodeClient) DelById(id string, result interface{}) error {
	return api.Delete(p.url, p.token, id, result)
}

func (p *nodeClient) UpdateById(id string, data, result interface{}) error {
	return api.Patch(p.url, p.token, id, data, result)
}

func (p *nodeClient) ReplaceById(id string, data, result interface{}) error {
	return api.Put(p.url, p.token, id, data, result)
}

func (p *nodeClient) FindTagById(id string, result interface{}) error {
	var u url.URL
	if traefik.Enable {
		u = url.URL{Host: net.JoinHostPort(traefik.Host, strconv.Itoa(traefik.Port)), Path: fmt.Sprintf("core/node/tag/%s", id)}
	} else {
		u = url.URL{Host: "core:9000", Path: fmt.Sprintf("core/node/tag/%s", id)}
	}
	u.Scheme = traefik.Proto
	return api.Get(u, p.token, nil, result)
}
