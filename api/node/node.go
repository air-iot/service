package node

import (
	"fmt"

	jsoniter "github.com/json-iterator/go"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/traefik"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type NodeClient interface {
	FindQuery(query, result interface{}) error
	Save(data, result interface{}) error
	DelById(id string, result interface{}) error
	UpdateById(id string, data, result interface{}) error
	ReplaceById(id string, data, result interface{}) error
}

type nodeClient struct {
	url   string
	token string
}

func NewNodeClient() (*nodeClient, error) {
	cli := new(nodeClient)
	if traefik.Enable {
		cli.url = fmt.Sprintf(`http://%s:%d/core/node`, traefik.Host, traefik.Port)
		token, err := api.FindToken()
		if err != nil {
			return nil, err
		}
		cli.token = token
	} else {
		cli.url = `http://core:9000/core/node`
	}
	return cli, nil
}

func (p *nodeClient) FindQuery(query, result interface{}) error {
	return api.Get(p.url, p.token, query, result)
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
