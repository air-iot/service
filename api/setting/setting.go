package setting

import (
	"net"
	"net/url"
	"strconv"

	jsoniter "github.com/json-iterator/go"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/traefik"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type SettingClient interface {
	FindQuery(query, result interface{}) error
	FindById(id string, result interface{}) error
	Save(data, result interface{}) error
	DelById(id string, result interface{}) error
	UpdateById(id string, data, result interface{}) error
	ReplaceById(id string, data, result interface{}) error
}

type settingClient struct {
	url   url.URL
	token string
}

func NewSettingClient() SettingClient {
	cli := new(settingClient)
	if traefik.Enable {
		u := url.URL{Host: net.JoinHostPort(traefik.Host, strconv.Itoa(traefik.Port)), Path: "core/setting"}
		u.Scheme = traefik.Proto
		cli.url = u
		cli.token = api.FindToken()
	} else {
		u := url.URL{Host: "core:9000", Path: "core/setting"}
		u.Scheme = traefik.Proto
		cli.url = u
	}
	return cli
}

func (p *settingClient) FindQuery(query, result interface{}) error {
	return api.Get(p.url, p.token, query, result)
}

func (p *settingClient) FindById(id string, result interface{}) error {
	return api.GetById(p.url, p.token, id, result)
}

func (p *settingClient) Save(data, result interface{}) error {
	return api.Post(p.url, p.token, data, result)
}

func (p *settingClient) DelById(id string, result interface{}) error {
	return api.Delete(p.url, p.token, id, result)
}

func (p *settingClient) UpdateById(id string, data, result interface{}) error {
	return api.Patch(p.url, p.token, id, data, result)
}

func (p *settingClient) ReplaceById(id string, data, result interface{}) error {
	return api.Put(p.url, p.token, id, data, result)
}
