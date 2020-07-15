package model

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/json-iterator/go"
	"gopkg.in/resty.v1"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/traefik"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type DriverClient interface {
	ChangeCommand(id string, data, result interface{}) error
}

type driverClient struct {
	url   url.URL
	token string
}

func NewDriverClient() DriverClient {
	cli := new(driverClient)
	if traefik.Enable {
		u := url.URL{Host: net.JoinHostPort(traefik.Host, strconv.Itoa(traefik.Port)), Path: "driver/driver"}
		u.Scheme = traefik.Proto
		cli.url = u
		cli.token = api.FindToken()
	} else {
		u := url.URL{Host: "driver:9000", Path: "driver/driver"}
		u.Scheme = traefik.Proto
		cli.url = u
	}
	return cli
}

func (p *driverClient) ChangeCommand(id string, data, result interface{}) error {
	return Command(p.url, p.token, id, data, result)
}

func Command(url url.URL, token, id string, data, result interface{}) error {
	resp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", token).
		SetHeader("Request-Type", "service").
		SetResult(result).
		SetBody(data).
		Post(fmt.Sprintf(`%s/%s/command`, url.String(), id))
	if err != nil {
		return err
	}
	if resp.StatusCode() != 200 {
		return errors.New(resp.String())
	}
	return nil
}
