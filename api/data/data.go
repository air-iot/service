package data

import (
	"errors"
	"fmt"
	"gopkg.in/resty.v1"
	"net"
	"net/url"
	"strconv"

	jsoniter "github.com/json-iterator/go"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/traefik"
)

var json = jsoniter.ConfigCompatibleWithStandardLibrary

type DataClient interface {
	GetLatest(query interface{}) (result []RealTimeData, err error)
	PostLatest(data interface{}) (result []RealTimeData, err error)
	GetQuery(query interface{}) (result *QueryData, err error)
	PostQuery(query interface{}) (result *QueryData, err error)
}

type dataClient struct {
	url   url.URL
	token string
}

type (
	RealTimeData struct {
		TagId string      `json:"tagId"`
		Uid   string      `json:"uid"`
		Time  int64       `json:"time"`
		Value interface{} `json:"value"`
	}

	QueryData struct {
		Results []Results `json:"results"`
	}

	Series struct {
		Name    string          `json:"name"`
		Columns []string        `json:"columns"`
		Values  [][]interface{} `json:"values"`
	}

	Results struct {
		Series []Series `json:"series"`
	}
)

func NewDataClient() DataClient {
	cli := new(dataClient)
	if traefik.Enable {
		u := url.URL{Host: net.JoinHostPort(traefik.Host, strconv.Itoa(traefik.Port)), Path: "core/data"}
		u.Scheme = traefik.Proto
		cli.url = u
		cli.token = api.FindToken()
	} else {
		u := url.URL{Host: "core:9000", Path: "core/data"}
		u.Scheme = traefik.Proto
		cli.url = u
	}
	return cli
}

func (p *dataClient) GetLatest(query interface{}) (result []RealTimeData, err error) {

	b, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	v := url.Values{}
	v.Set("query", string(b))
	result = make([]RealTimeData, 0)
	resp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", p.token).
		SetHeader("Request-Type", "service").
		SetResult(&result).
		Get(fmt.Sprintf(`%s/latest?%s`, p.url.String(), v.Encode()))

	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != 200 {
		return nil, errors.New(resp.String())
	}

	return result, nil
}

func (p *dataClient) PostLatest(data interface{}) (result []RealTimeData, err error) {
	result = make([]RealTimeData, 0)
	resp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", p.token).
		SetHeader("Request-Type", "service").
		SetResult(&result).
		SetBody(data).
		Post(fmt.Sprintf(`%s/latest`, p.url.String()))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != 200 {
		return nil, errors.New(resp.String())
	}
	return result, nil
}

func (p *dataClient) GetQuery(query interface{}) (result *QueryData, err error) {
	result = new(QueryData)
	b, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	v := url.Values{}
	v.Set("query", string(b))
	resp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", p.token).
		SetHeader("Request-Type", "service").
		SetResult(result).
		Get(fmt.Sprintf(`%s/query?%s`, p.url.String(), v.Encode()))

	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != 200 {
		return nil, errors.New(resp.String())
	}

	return result, nil
}

func (p *dataClient) PostQuery(query interface{}) (result *QueryData, err error) {
	result = new(QueryData)
	resp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", p.token).
		SetHeader("Request-Type", "service").
		SetResult(result).
		SetBody(query).
		Post(fmt.Sprintf(`%s/query`, p.url.String()))

	if err != nil {
		return nil, err
	}
	if resp.StatusCode() != 200 {
		return nil, errors.New(resp.String())
	}

	return result, nil
}
