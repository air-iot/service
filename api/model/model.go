package model

import (
	"fmt"

	jsoniter "github.com/json-iterator/go"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/traefik"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type ModelClient interface {
	FindQuery(query, result interface{}) error
	Save(data, result interface{}) error
	DelById(id string, result interface{}) error
	UpdateById(id string, data, result interface{}) error
	ReplaceById(id string, data, result interface{}) error
}

type modelClient struct {
	url   string
	token string
}

func NewModelClient() (ModelClient, error) {
	cli := new(modelClient)
	if traefik.Enable {
		cli.url = fmt.Sprintf(`http://%s:%d/core/model`, traefik.Host, traefik.Port)
		token, err := api.FindToken()
		if err != nil {
			return nil, err
		}
		cli.token = token
	} else {
		cli.url = `http://core:9000/core/model`
	}
	return cli, nil
}
func (p *modelClient) FindQuery(query, result interface{}) error {
	return api.Get(p.url, p.token, query, result)
}

func (p *modelClient) Save(data, result interface{}) error {
	return api.Post(p.url, p.token, data, result)
}

func (p *modelClient) DelById(id string, result interface{}) error {
	return api.Delete(p.url, p.token, id, result)
}

func (p *modelClient) UpdateById(id string, data, result interface{}) error {
	return api.Patch(p.url, p.token, id, data, result)
}

func (p *modelClient) ReplaceById(id string, data, result interface{}) error {
	return api.Put(p.url, p.token, id, data, result)
}
