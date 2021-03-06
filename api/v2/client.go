package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/air-iot/service/model"
	"github.com/air-iot/service/traefik"
	"github.com/go-resty/resty/v2"
	"github.com/sirupsen/logrus"
)

type client struct {
	sync.Mutex
	protocol  string
	host      string
	ak        string
	sk        string
	Token     string
	expires   int64
	isTraefik bool
	headers   map[string]string
}

var Cli Client

func Init() {
	Cli = NewClient()
}

func NewClient() Client {
	return &client{
		protocol:  traefik.Proto,
		host:      net.JoinHostPort(traefik.Host, strconv.Itoa(traefik.Port)),
		ak:        traefik.AppKey,
		sk:        traefik.AppSecret,
		isTraefik: traefik.Enable,
		headers: map[string]string{
			"Content-Type": "application/json",
			"Request-Type": "service",
		},
	}
}

// 根据 app appkey和appsecret 获取token
func (p *client) findToken() {
	// 生成要访问的url
	u := url.URL{Scheme: p.protocol, Host: p.host, Path: "core/auth/token"}
	v := url.Values{}
	v.Set("appkey", p.ak)
	v.Set("appsecret", p.sk)
	u.RawQuery = v.Encode()
	auth := new(AuthToken)
	resp, err := resty.New().SetTimeout(time.Second*30).R().
		SetHeader("Content-Type", "application/json").
		SetResult(auth).
		Get(u.String())
	if err != nil {
		logrus.Errorf("token查询错误:%s", err.Error())
		return
	}
	if resp.StatusCode() != 200 {
		logrus.Warnf("token查询错误,状态:%d,信息:%s", resp.StatusCode(), resp.String())
		return
	}
	p.Token = auth.Token
	p.expires = auth.Expires/10e9 + time.Now().Unix()
}

func (p *client) Get(url url.URL, result interface{}) error {
	var resp *resty.Response
	var err error
	if p.isTraefik {
		p.checkToken()
		resp, err = resty.New().SetTimeout(time.Minute * 1).R().
			SetHeaders(p.headers).
			SetResult(result).
			Get(url.String())
	} else {
		resp, err = resty.New().SetTimeout(time.Minute * 1).R().
			SetHeaders(p.headers).
			SetResult(result).
			Get(url.String())
	}

	if err != nil {
		return err
	}

	if resp.StatusCode() >= 200 && resp.StatusCode() <= 204 {
		return nil
	}
	return errors.New(resp.String())

}

func (p *client) Post(url url.URL, data, result interface{}) error {
	var resp *resty.Response
	var err error
	if p.isTraefik {
		p.checkToken()
		resp, err = resty.New().SetTimeout(time.Minute * 1).R().
			SetHeaders(p.headers).
			SetResult(result).
			SetBody(data).
			Post(url.String())

	} else {
		resp, err = resty.New().SetTimeout(time.Minute * 1).R().
			SetHeaders(p.headers).
			SetResult(result).
			SetBody(data).
			Post(url.String())
	}

	if err != nil {
		return err
	}
	if resp.StatusCode() >= 200 && resp.StatusCode() <= 204 {
		return nil
	}
	return errors.New(resp.String())
}

func (p *client) Delete(url url.URL, result interface{}) error {
	var resp *resty.Response
	var err error
	if p.isTraefik {
		p.checkToken()
		resp, err = resty.New().SetTimeout(time.Minute * 1).R().
			SetHeaders(p.headers).
			SetResult(result).
			Delete(url.String())
	} else {
		resp, err = resty.New().SetTimeout(time.Minute * 1).R().
			SetHeaders(p.headers).
			SetResult(result).
			Delete(url.String())
	}

	if err != nil {
		return err
	}
	if resp.StatusCode() >= 200 && resp.StatusCode() <= 204 {
		return nil
	}
	return errors.New(resp.String())
}

func (p *client) Put(url url.URL, data, result interface{}) error {
	var resp *resty.Response
	var err error
	if p.isTraefik {
		p.checkToken()
		resp, err = resty.New().SetTimeout(time.Minute * 1).R().
			SetHeaders(p.headers).
			SetResult(result).
			SetBody(data).
			Put(url.String())
	} else {
		resp, err = resty.New().SetTimeout(time.Minute * 1).R().
			SetHeaders(p.headers).
			SetResult(result).
			SetBody(data).
			Put(url.String())
	}
	if err != nil {
		return err
	}
	if resp.StatusCode() >= 200 && resp.StatusCode() <= 204 {
		return nil
	}
	return errors.New(resp.String())
}

func (p *client) Patch(url url.URL, data, result interface{}) error {
	var resp *resty.Response
	var err error
	if p.isTraefik {
		p.checkToken()
		resp, err = resty.New().SetTimeout(time.Minute * 1).R().
			SetHeaders(p.headers).
			SetResult(result).
			SetBody(data).
			Patch(url.String())
	} else {
		resp, err = resty.New().SetTimeout(time.Minute * 1).R().
			SetHeaders(p.headers).
			SetResult(result).
			SetBody(data).
			Patch(url.String())
	}
	if err != nil {
		return err
	}
	if resp.StatusCode() >= 200 && resp.StatusCode() <= 204 {
		return nil
	}
	return errors.New(resp.String())
}

func (p *client) checkToken() {
	p.Lock()
	defer p.Unlock()
	if p.expires-5 < time.Now().Unix() {
		p.findToken()
		p.headers["Authorization"] = p.Token
	}
	return
}

func (p *client) GetLatest(query interface{}) (result []model.RealTimeData, err error) {
	b, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	v := url.Values{}
	v.Set("query", string(b))
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "core/data/latest"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: "core/data/latest"}
	}
	u.RawQuery = v.Encode()
	result = make([]model.RealTimeData, 0)
	if err := p.Get(u, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (p *client) PostLatest(query interface{}) (result []model.RealTimeData, err error) {
	result = make([]model.RealTimeData, 0)
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "core/data/latest"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: "core/data/latest"}
	}

	if err := p.Post(u, query, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (p *client) GetQuery(query interface{}) (result *model.QueryData, err error) {
	b, err := json.Marshal(query)
	if err != nil {
		return nil, err
	}
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "core/data/query"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: "core/data/query"}
	}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	result = new(model.QueryData)
	if err := p.Get(u, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (p *client) PostQuery(query interface{}) (result *model.QueryData, err error) {
	var u url.URL
	if p.isTraefik {
	} else {
	}
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "core/data/query"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: "core/data/query"}
	}
	result = new(model.QueryData)
	if err := p.Post(u, query, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (p *client) ChangeCommand(id string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("driver/driver/%s/command", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "driver:9000", Path: fmt.Sprintf("driver/driver/%s/command", id)}
	}
	return p.Post(u, data, &result)
}

func (p *client) FindExtQuery(tableName string, query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/ext/%s", tableName)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/ext/%s", tableName)}
	}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	return p.Get(u, result)
}

func (p *client) SaveExt(tableName string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/ext/%s", tableName)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/ext/%s", tableName)}
	}
	return p.Post(u, data, result)
}

func (p *client) SaveManyExt(tableName string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/ext/%s/many", tableName)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/ext/%s/many", tableName)}
	}
	return p.Post(u, data, result)
}

func (p *client) FindExtById(tableName, id string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/ext/%s/%s", tableName, id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/ext/%s/%s", tableName, id)}
	}
	return p.Get(u, result)
}

func (p *client) DelExtById(tableName, id string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/ext/%s/%s", tableName, id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/ext/%s/%s", tableName, id)}
	}
	return p.Delete(u, result)
}

func (p *client) DelExtAll(tableName, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/ext/%s", tableName)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/ext/%s", tableName)}
	}
	return p.Delete(u, result)
}

func (p *client) UpdateExtById(tableName, id string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/ext/%s/%s", tableName, id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/ext/%s/%s", tableName, id)}
	}
	return p.Patch(u, data, result)
}

func (p *client) ReplaceExtById(tableName, id string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/ext/%s/%s", tableName, id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/ext/%s/%s", tableName, id)}
	}
	return p.Put(u, data, result)
}

func (p *client) FindEventQuery(query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "event/event"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "event:9000", Path: "event/event"}
	}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	return p.Get(u, result)
}

func (p *client) FindEventById(id string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("event/event/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "event:9000", Path: fmt.Sprintf("event/event/%s", id)}
	}
	return p.Get(u, result)
}

func (p *client) SaveEvent(data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "event/event"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "event:9000", Path: "event/event"}
	}
	return p.Post(u, data, result)
}

func (p *client) DelEventById(id string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("event/event/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "event:9000", Path: fmt.Sprintf("event/event/%s", id)}
	}
	return p.Delete(u, result)
}

func (p *client) UpdateEventById(id string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("event/event/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "event:9000", Path: fmt.Sprintf("event/event/%s", id)}
	}
	return p.Patch(u, data, result)
}

func (p *client) ReplaceEventById(id string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("event/event/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "event:9000", Path: fmt.Sprintf("event/event/%s", id)}
	}
	return p.Put(u, data, result)
}

func (p *client) FindHandlerQuery(query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "event/eventHandler"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "event:9000", Path: "event/eventHandler"}
	}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	return p.Get(u, result)
}

func (p *client) FindHandlerById(id string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("event/eventHandler/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "event:9000", Path: fmt.Sprintf("event/eventHandler/%s", id)}
	}
	return p.Get(u, result)
}

func (p *client) SaveHandler(data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "event/eventHandler"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "event:9000", Path: "event/eventHandler"}
	}
	return p.Post(u, data, result)
}

func (p *client) DelHandlerById(id string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("event/eventHandler/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "event:9000", Path: fmt.Sprintf("event/eventHandler/%s", id)}
	}
	return p.Delete(u, result)
}

func (p *client) UpdateHandlerById(id string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("event/eventHandler/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "event:9000", Path: fmt.Sprintf("event/eventHandler/%s", id)}
	}
	return p.Patch(u, data, result)
}

func (p *client) ReplaceHandlerById(id string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("event/eventHandler/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "event:9000", Path: fmt.Sprintf("event/eventHandler/%s", id)}
	}
	return p.Put(u, data, result)
}

func (p *client) FindModelQuery(query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "core/model"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: "core/model"}
	}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	return p.Get(u, result)
}

func (p *client) FindModelById(id string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/model/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/model/%s", id)}
	}
	return p.Get(u, result)
}

func (p *client) SaveModel(data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "core/model"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: "core/model"}
	}
	return p.Post(u, data, result)
}

func (p *client) DelModelById(id string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/model/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/model/%s", id)}
	}
	return p.Delete(u, result)
}

func (p *client) UpdateModelById(id string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/model/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/model/%s", id)}
	}
	return p.Patch(u, data, result)
}

func (p *client) ReplaceModelById(id string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/model/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/model/%s", id)}
	}
	return p.Put(u, data, result)
}

func (p *client) FindNodeQuery(query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "core/node"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: "core/node"}
	}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	return p.Get(u, result)
}

func (p *client) FindNodeById(id string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/node/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/node/%s", id)}
	}
	return p.Get(u, result)
}

func (p *client) FindTagsById(id string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/node/tag/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/node/tag/%s", id)}
	}
	return p.Get(u, result)
}

func (p *client) SaveNode(data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "core/node"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: "core/node"}
	}
	return p.Post(u, data, result)
}

func (p *client) DelNodeById(id string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/node/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/node/%s", id)}
	}
	return p.Delete(u, result)
}

func (p *client) UpdateNodeById(id string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/node/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/node/%s", id)}
	}
	return p.Patch(u, data, result)
}

func (p *client) ReplaceNodeById(id string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/node/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/node/%s", id)}
	}
	return p.Put(u, data, result)
}

func (p *client) FindSettingQuery(query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "core/setting"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: "core/setting"}
	}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	return p.Get(u, result)
}

func (p *client) FindSettingById(id string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/setting/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/setting/%s", id)}
	}
	return p.Get(u, result)
}

func (p *client) SaveSetting(data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "core/setting"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: "core/setting"}
	}
	return p.Post(u, data, result)
}

func (p *client) DelSettingById(id string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/setting/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/setting/%s", id)}
	}
	return p.Delete(u, result)
}

func (p *client) UpdateSettingById(id string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/setting/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/setting/%s", id)}
	}
	return p.Patch(u, data, result)
}

func (p *client) ReplaceSettingById(id string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/setting/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/setting/%s", id)}
	}
	return p.Put(u, data, result)
}

func (p *client) FindTableQuery(query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "core/table"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: "core/table"}
	}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	return p.Get(u, result)
}

func (p *client) FindTableById(id string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/table/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/table/%s", id)}
	}
	return p.Get(u, result)
}

func (p *client) SaveTable(data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "core/table"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: "core/table"}
	}
	return p.Post(u, data, result)
}

func (p *client) DelTableById(id string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/table/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/table/%s", id)}
	}
	return p.Delete(u, result)
}

func (p *client) UpdateTableById(id string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/table/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/table/%s", id)}
	}
	return p.Patch(u, data, result)
}

func (p *client) ReplaceTableById(id string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/table/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/table/%s", id)}
	}
	return p.Put(u, data, result)
}

func (p *client) FindUserQuery(query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "core/user"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: "core/user"}
	}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	return p.Get(u, result)
}

func (p *client) FindUserById(id string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/user/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/user/%s", id)}
	}
	return p.Get(u, result)
}

func (p *client) SaveUser(data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "core/user"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: "core/user"}
	}
	return p.Post(u, data, result)
}

func (p *client) DelUserById(id string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/user/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/user/%s", id)}
	}
	return p.Delete(u, result)
}

func (p *client) UpdateUserById(id string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/user/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/user/%s", id)}
	}
	return p.Patch(u, data, result)
}

func (p *client) ReplaceUserById(id string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/user/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/user/%s", id)}
	}
	return p.Put(u, data, result)
}

func (p *client) FindWarnQuery(archive bool, query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "warning/warning"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "warning:9000", Path: "warning/warning"}
	}
	v := url.Values{}
	v.Set("query", string(b))
	v.Set("archive", strconv.FormatBool(archive))
	u.RawQuery = v.Encode()
	return p.Get(u, result)
}

func (p *client) FindWarnById(id string, archive bool, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("warning/warning/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "warning:9000", Path: fmt.Sprintf("warning/warning/%s", id)}
	}
	v := url.Values{}
	v.Set("archive", strconv.FormatBool(archive))
	u.RawQuery = v.Encode()
	return p.Get(u, result)
}

func (p *client) SaveWarn(data, archive bool, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "warning/warning"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "warning:9000", Path: "warning/warning"}
	}
	v := url.Values{}
	v.Set("archive", strconv.FormatBool(archive))
	u.RawQuery = v.Encode()
	return p.Post(u, data, result)
}

func (p *client) DelWarnById(id string, archive bool, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("warning/warning/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "warning:9000", Path: fmt.Sprintf("warning/warning/%s", id)}
	}
	v := url.Values{}
	v.Set("archive", strconv.FormatBool(archive))
	u.RawQuery = v.Encode()
	return p.Delete(u, result)
}

func (p *client) UpdateWarnById(id string, archive bool, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("warning/warning/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "warning:9000", Path: fmt.Sprintf("warning/warning/%s", id)}
	}
	v := url.Values{}
	v.Set("archive", strconv.FormatBool(archive))
	u.RawQuery = v.Encode()
	return p.Patch(u, data, result)
}

func (p *client) ReplaceWarnById(id string, archive bool, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("warning/warning/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "warning:9000", Path: fmt.Sprintf("warning/warning/%s", id)}
	}
	v := url.Values{}
	v.Set("archive", strconv.FormatBool(archive))
	u.RawQuery = v.Encode()
	return p.Put(u, data, result)
}

func (p *client) DriverConfig(driverId, serviceId string) ([]byte, error) {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("driver/driver/%s/%s/config", driverId, serviceId)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "driver:9000", Path: fmt.Sprintf("driver/driver/%s/%s/config", driverId, serviceId)}
	}

	// p.checkToken()
	resp, err := resty.New().SetTimeout(time.Minute*1).R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Request-Type", "service").
		//SetHeader("Authorization", p.Token).
		Get(u.String())

	if err != nil {
		return nil, err
	}

	if resp.StatusCode() == 200 {
		return resp.Body(), nil
	}
	return nil, fmt.Errorf("请求状态:%d,响应:%s", resp.StatusCode(), resp.String())
}

func (p *client) FindGatewayQuery(query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "gateway/gateway"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "gateway:9000", Path: "gateway/gateway"}
	}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	return p.Get(u, result)
}

func (p *client) FindGatewayById(id string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("gateway/gateway/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "gateway:9000", Path: fmt.Sprintf("gateway/gateway/%s", id)}
	}
	return p.Get(u, result)
}

func (p *client) FindGatewayByType(typeName string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("gateway/gateway/type/%s", typeName)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "gateway:9000", Path: fmt.Sprintf("gateway/gateway/type/%s", typeName)}
	}
	return p.Get(u, result)
}

func (p *client) SaveGateway(data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "gateway/gateway"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "gateway:9000", Path: "gateway/gateway"}
	}
	return p.Post(u, data, result)
}

func (p *client) DelGatewayById(id string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("gateway/gateway/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "gateway:9000", Path: fmt.Sprintf("gateway/gateway/%s", id)}
	}
	return p.Delete(u, result)
}

func (p *client) UpdateGatewayById(id string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("gateway/gateway/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "gateway:9000", Path: fmt.Sprintf("gateway/gateway/%s", id)}
	}
	return p.Patch(u, data, result)
}

func (p *client) ReplaceGatewayById(id string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("gateway/gateway/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "gateway:9000", Path: fmt.Sprintf("gateway/gateway/%s", id)}
	}
	return p.Put(u, data, result)
}

func (p *client) CheckDriver(licenseName string) (*model.Signature, error) {

	signature := new(model.Signature)
	var u url.URL
	if p.isTraefik {
		//p.checkToken()
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "core/license/driver"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: "core/license/driver"}
	}
	v := url.Values{}
	v.Set("licenseName", licenseName)

	u.RawQuery = v.Encode()

	err := p.Get(u, signature)
	if err != nil {
		return nil, err
	}
	return signature, nil
	//resp, err := resty.New().SetTimeout(time.Minute*1).R().
	//	SetHeader("Content-Type", "application/json").
	//	SetHeader("Request-Type", "service").
	//	SetHeader("Authorization", p.Token).
	//	SetQueryParam("").
	//	SetResult(signature).
	//	Get(u.String())
	//
	//if err != nil {
	//	return nil, err
	//}
	//
	//if resp.StatusCode() == 200 {
	//	return signature, nil
	//}
	//return nil, fmt.Errorf("请求状态:%d,响应:%s", resp.StatusCode(), resp.String())
}

func (p *client) FindLogQuery(query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "core/log"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: "core/log"}
	}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	return p.Get(u, result)
}

func (p *client) FindLogById(id string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/log/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/log/%s", id)}
	}
	return p.Get(u, result)
}

func (p *client) SaveLog(data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: "core/log"}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: "core/log"}
	}
	return p.Post(u, data, result)
}

func (p *client) DelLogById(id string, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/log/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/log/%s", id)}
	}
	return p.Delete(u, result)
}

func (p *client) UpdateLogById(id string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/log/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/log/%s", id)}
	}
	return p.Patch(u, data, result)
}

func (p *client) ReplaceLogById(id string, data, result interface{}) error {
	var u url.URL
	if p.isTraefik {
		u = url.URL{Scheme: p.protocol, Host: p.host, Path: fmt.Sprintf("core/log/%s", id)}
	} else {
		u = url.URL{Scheme: p.protocol, Host: "core:9000", Path: fmt.Sprintf("core/log/%s", id)}
	}
	return p.Put(u, data, result)
}
