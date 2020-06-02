package user

import (
	"net"
	"net/url"
	"strconv"

	jsoniter "github.com/json-iterator/go"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/traefik"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type UserClient interface {
	FindQuery(query, result interface{}) error
	FindById(id string, result interface{}) error
	Save(data, result interface{}) error
	DelById(id string, result interface{}) error
	UpdateById(id string, data, result interface{}) error
	ReplaceById(id string, data, result interface{}) error
}

type userClient struct {
	url   url.URL
	token string
}

func NewUserClient() UserClient {
	cli := new(userClient)
	if traefik.Enable {
		u := url.URL{Host: net.JoinHostPort(traefik.Host, strconv.Itoa(traefik.Port)), Path: "core/user"}
		u.Scheme = traefik.Proto
		cli.url = u
		cli.token = api.FindToken()
	} else {
		u := url.URL{Host: "core:9000", Path: "core/user"}
		u.Scheme = traefik.Proto
		cli.url = u
	}
	return cli
}

func (p *userClient) FindQuery(query, result interface{}) error {
	return api.Get(p.url, p.token, query, result)
}

func (p *userClient) FindById(id string, result interface{}) error {
	return api.GetById(p.url, p.token, id, result)
}

func (p *userClient) Save(data, result interface{}) error {
	return api.Post(p.url, p.token, data, result)
}

func (p *userClient) DelById(id string, result interface{}) error {
	return api.Delete(p.url, p.token, id, result)
}

func (p *userClient) UpdateById(id string, data, result interface{}) error {
	return api.Patch(p.url, p.token, id, data, result)
}

func (p *userClient) ReplaceById(id string, data, result interface{}) error {
	return api.Put(p.url, p.token, id, data, result)
}
