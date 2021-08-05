package api

import (
	"context"
	"fmt"
	"github.com/go-redis/redis/v8"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/air-iot/service/errors"
	"github.com/air-iot/service/gin/ginx"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/util/json"
	"github.com/go-resty/resty/v2"
)

type client struct {
	sync.Mutex
	cli     redisdb.Client
	headers map[string]string
	cfg     Config
	tokens  map[string]*AuthToken
}

// Config 接口配置
type Config struct {
	Schema string
	Host   string
}

func NewClient(cli redisdb.Client, cfg Config) Client {
	if cfg.Schema == "" {
		cfg.Schema = "http"
	}
	return &client{
		cfg: cfg,
		cli: cli,
		headers: map[string]string{
			"Content-Type": "application/json",
			"Request-Type": "service",
		},
		tokens: map[string]*AuthToken{},
	}
}

// FindToken 获取token
func (p *client) FindToken(project string) (*AuthToken, error) {
	p.Lock()
	defer p.Unlock()
	if token, ok := p.tokens[project]; ok {
		if token.ExpiresAt-5 >= time.Now().Unix() {
			return token, nil
		}
	}
	tokenStr, err := p.cli.HGet(context.Background(), "authToken", project).Result()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("token查询错误: %s", err.Error())
	} else if err == redis.Nil {
		return nil, err
	}
	auth := new(AuthToken)
	err = json.Unmarshal([]byte(tokenStr), auth)
	if err != nil {
		return nil, fmt.Errorf("token解析错误: %s", err.Error())
	}
	p.tokens[project] = auth

	return auth, nil
}

func (p *client) Get(url url.URL, headers map[string]string, result interface{}) error {
	project, ok := headers[ginx.XRequestProject]
	if !ok {
		project = ginx.XRequestProjectDefault
		headers[ginx.XRequestProject] = ginx.XRequestProjectDefault
	}

	token, err := p.FindToken(project)
	if err != nil {
		return err
	}
	for k, v := range p.headers {
		if _, ok := headers[k]; !ok {
			headers[k] = v
		}
	}
	headers[ginx.XRequestHeaderAuthorization] = fmt.Sprintf("%s %s", token.TokenType, token.AccessToken)
	resp, err := resty.New().SetTimeout(time.Minute * 1).R().
		SetHeaders(headers).
		SetResult(result).
		Get(url.String())
	if err != nil {
		return err
	}
	if resp.StatusCode() >= 200 && resp.StatusCode() <= 204 {
		return nil
	}
	return errors.New(resp.String())

}

func (p *client) Post(url url.URL, headers map[string]string, data, result interface{}) error {
	project, ok := headers[ginx.XRequestProject]
	if !ok {
		project = ginx.XRequestProjectDefault
		headers[ginx.XRequestProject] = ginx.XRequestProjectDefault
	}

	token, err := p.FindToken(project)
	if err != nil {
		return err
	}
	headers[ginx.XRequestHeaderAuthorization] = fmt.Sprintf("%s %s", token.TokenType, token.AccessToken)
	for k, v := range p.headers {
		if _, ok := headers[k]; !ok {
			headers[k] = v
		}
	}
	resp, err := resty.New().SetTimeout(time.Minute * 1).R().
		SetHeaders(headers).
		SetResult(result).
		SetBody(data).
		Post(url.String())

	if err != nil {
		return err
	}
	if resp.StatusCode() >= 200 && resp.StatusCode() <= 204 {
		return nil
	}
	return errors.New(resp.String())
}

func (p *client) Delete(url url.URL, headers map[string]string, result interface{}) error {
	project, ok := headers[ginx.XRequestProject]
	if !ok {
		project = ginx.XRequestProjectDefault
		headers[ginx.XRequestProject] = ginx.XRequestProjectDefault
	}

	token, err := p.FindToken(project)
	if err != nil {
		return err
	}

	headers[ginx.XRequestHeaderAuthorization] = fmt.Sprintf("%s %s", token.TokenType, token.AccessToken)
	for k, v := range p.headers {
		if _, ok := headers[k]; !ok {
			headers[k] = v
		}
	}
	resp, err := resty.New().SetTimeout(time.Minute * 1).R().
		SetHeaders(headers).
		SetResult(result).
		Delete(url.String())
	if err != nil {
		return err
	}
	if resp.StatusCode() >= 200 && resp.StatusCode() <= 204 {
		return nil
	}
	return errors.New(resp.String())
}

func (p *client) Put(url url.URL, headers map[string]string, data, result interface{}) error {
	project, ok := headers[ginx.XRequestProject]
	if !ok {
		project = ginx.XRequestProjectDefault
		headers[ginx.XRequestProject] = ginx.XRequestProjectDefault
	}

	token, err := p.FindToken(project)
	if err != nil {
		return err
	}

	headers[ginx.XRequestHeaderAuthorization] = fmt.Sprintf("%s %s", token.TokenType, token.AccessToken)
	for k, v := range p.headers {
		if _, ok := headers[k]; !ok {
			headers[k] = v
		}
	}
	resp, err := resty.New().SetTimeout(time.Minute * 1).R().
		SetHeaders(headers).
		SetResult(result).
		SetBody(data).
		Put(url.String())
	if err != nil {
		return err
	}
	if resp.StatusCode() >= 200 && resp.StatusCode() <= 204 {
		return nil
	}
	return errors.New(resp.String())
}

func (p *client) Patch(url url.URL, headers map[string]string, data, result interface{}) error {
	project, ok := headers[ginx.XRequestProject]
	if !ok {
		project = ginx.XRequestProjectDefault
		headers[ginx.XRequestProject] = ginx.XRequestProjectDefault
	}

	token, err := p.FindToken(project)
	if err != nil {
		return err
	}
	headers[ginx.XRequestHeaderAuthorization] = fmt.Sprintf("%s %s", token.TokenType, token.AccessToken)
	for k, v := range p.headers {
		if _, ok := headers[k]; !ok {
			headers[k] = v
		}
	}
	resp, err := resty.New().SetTimeout(time.Minute * 1).R().
		SetHeaders(headers).
		SetResult(result).
		SetBody(data).
		Patch(url.String())
	if err != nil {
		return err
	}
	if resp.StatusCode() >= 200 && resp.StatusCode() <= 204 {
		return nil
	}
	return errors.New(resp.String())
}

func (p *client) GetLatest(headers map[string]string, query, result interface{}) (err error) {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	v := url.Values{}
	v.Set("query", string(b))
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "core/data/latest"}
	u.RawQuery = v.Encode()
	if err := p.Get(u, headers, result); err != nil {
		return err
	}
	return nil
}

func (p *client) PostLatest(headers map[string]string, query, result interface{}) (err error) {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "core/data/latest"}
	if err := p.Post(u, headers, query, result); err != nil {
		return err
	}
	return nil
}

func (p *client) GetQuery(headers map[string]string, query, result interface{}) (err error) {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "core/data/query"}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	if err := p.Get(u, headers, result); err != nil {
		return err
	}
	return nil
}

func (p *client) PostQuery(headers map[string]string, query, result interface{}) (err error) {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "core/data/query"}
	if err := p.Post(u, headers, query, result); err != nil {
		return err
	}
	return nil
}

func (p *client) ChangeCommand(headers map[string]string, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "driver:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("driver/driver/%s/command", id)}
	return p.Post(u, headers, data, result)
}

func (p *client) FindExtQuery(headers map[string]string, tableName string, query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/ext/%s", tableName)}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	return p.Get(u, headers, result)
}

func (p *client) SaveExt(headers map[string]string, tableName string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/ext/%s", tableName)}
	return p.Post(u, headers, data, result)
}

func (p *client) SaveManyExt(headers map[string]string, tableName string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/ext/%s/many", tableName)}
	return p.Post(u, headers, data, result)
}

func (p *client) FindExtById(headers map[string]string, tableName, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/ext/%s/%s", tableName, id)}
	return p.Get(u, headers, result)
}

func (p *client) DelExtById(headers map[string]string, tableName, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/ext/%s/%s", tableName, id)}
	return p.Delete(u, headers, result)
}

func (p *client) DelExtAll(headers map[string]string, tableName, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/ext/%s", tableName)}
	return p.Delete(u, headers, result)
}

func (p *client) UpdateExtById(headers map[string]string, tableName, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/ext/%s/%s", tableName, id)}
	return p.Patch(u, headers, data, result)
}

func (p *client) ReplaceExtById(headers map[string]string, tableName, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/ext/%s/%s", tableName, id)}
	return p.Put(u, headers, data, result)
}

func (p *client) FindEventQuery(headers map[string]string, query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	host := p.cfg.Host
	if host == "" {
		host = "event:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "event/event"}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	return p.Get(u, headers, result)
}

func (p *client) FindEventById(headers map[string]string, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "event:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("event/event/%s", id)}
	return p.Get(u, headers, result)
}

func (p *client) SaveEvent(headers map[string]string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "event:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "event/event"}
	return p.Post(u, headers, data, result)
}

func (p *client) DelEventById(headers map[string]string, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "event:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("event/event/%s", id)}
	return p.Delete(u, headers, result)
}

func (p *client) UpdateEventById(headers map[string]string, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "event:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("event/event/%s", id)}
	return p.Patch(u, headers, data, result)
}

func (p *client) ReplaceEventById(headers map[string]string, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "event:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("event/event/%s", id)}
	return p.Put(u, headers, data, result)
}

func (p *client) FindHandlerQuery(headers map[string]string, query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	host := p.cfg.Host
	if host == "" {
		host = "event:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "event/eventHandler"}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	return p.Get(u, headers, result)
}

func (p *client) FindHandlerById(headers map[string]string, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "event:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("event/eventHandler/%s", id)}
	return p.Get(u, headers, result)
}

func (p *client) SaveHandler(headers map[string]string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "event:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "event/eventHandler"}
	return p.Post(u, headers, data, result)
}

func (p *client) DelHandlerById(headers map[string]string, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "event:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("event/eventHandler/%s", id)}
	return p.Delete(u, headers, result)
}

func (p *client) UpdateHandlerById(headers map[string]string, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "event:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("event/eventHandler/%s", id)}
	return p.Patch(u, headers, data, result)
}

func (p *client) ReplaceHandlerById(headers map[string]string, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "event:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("event/eventHandler/%s", id)}
	return p.Put(u, headers, data, result)
}

func (p *client) FindModelQuery(headers map[string]string, query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "core/model"}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	return p.Get(u, headers, result)
}

func (p *client) FindModelById(headers map[string]string, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/model/%s", id)}
	return p.Get(u, headers, result)
}

func (p *client) SaveModel(headers map[string]string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "core/model"}
	return p.Post(u, headers, data, result)
}

func (p *client) DelModelById(headers map[string]string, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/model/%s", id)}
	return p.Delete(u, headers, result)
}

func (p *client) UpdateModelById(headers map[string]string, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/model/%s", id)}
	return p.Patch(u, headers, data, result)
}

func (p *client) ReplaceModelById(headers map[string]string, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/model/%s", id)}
	return p.Put(u, headers, data, result)
}

func (p *client) FindNodeQuery(headers map[string]string, query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "core/node"}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	return p.Get(u, headers, result)
}

func (p *client) FindNodeById(headers map[string]string, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/node/_id/%s", id)}
	return p.Get(u, headers, result)
}

func (p *client) FindTagsById(headers map[string]string, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/node/tag/%s", id)}
	return p.Get(u, headers, result)
}

func (p *client) SaveNode(headers map[string]string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "core/node"}
	return p.Post(u, headers, data, result)
}

func (p *client) DelNodeById(headers map[string]string, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/node/%s", id)}
	return p.Delete(u, headers, result)
}

func (p *client) UpdateNodeById(headers map[string]string, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/node/%s", id)}
	return p.Patch(u, headers, data, result)
}

func (p *client) ReplaceNodeById(headers map[string]string, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/node/%s", id)}
	return p.Put(u, headers, data, result)
}

func (p *client) FindSettingQuery(headers map[string]string, query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "core/setting"}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	return p.Get(u, headers, result)
}

func (p *client) FindSettingById(headers map[string]string, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/setting/%s", id)}
	return p.Get(u, headers, result)
}

func (p *client) SaveSetting(headers map[string]string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "core/setting"}
	return p.Post(u, headers, data, result)
}

func (p *client) DelSettingById(headers map[string]string, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/setting/%s", id)}
	return p.Delete(u, headers, result)
}

func (p *client) UpdateSettingById(headers map[string]string, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/setting/%s", id)}
	return p.Patch(u, headers, data, result)
}

func (p *client) ReplaceSettingById(headers map[string]string, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/setting/%s", id)}
	return p.Put(u, headers, data, result)
}

func (p *client) FindTableQuery(headers map[string]string, query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "core/table"}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	return p.Get(u, headers, result)
}

func (p *client) FindTableById(headers map[string]string, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/table/%s", id)}
	return p.Get(u, headers, result)
}

func (p *client) SaveTable(headers map[string]string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "core/table"}
	return p.Post(u, headers, data, result)
}

func (p *client) DelTableById(headers map[string]string, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/table/%s", id)}
	return p.Delete(u, headers, result)
}

func (p *client) UpdateTableById(headers map[string]string, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/table/%s", id)}
	return p.Patch(u, headers, data, result)
}

func (p *client) ReplaceTableById(headers map[string]string, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/table/%s", id)}
	return p.Put(u, headers, data, result)
}

func (p *client) FindUserQuery(headers map[string]string, query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "core/user"}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	return p.Get(u, headers, result)
}

func (p *client) FindUserById(headers map[string]string, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/user/%s", id)}
	return p.Get(u, headers, result)
}

func (p *client) SaveUser(headers map[string]string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "core/user"}
	return p.Post(u, headers, data, result)
}

func (p *client) DelUserById(headers map[string]string, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/user/%s", id)}
	return p.Delete(u, headers, result)
}

func (p *client) UpdateUserById(headers map[string]string, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/user/%s", id)}
	return p.Patch(u, headers, data, result)
}

func (p *client) ReplaceUserById(headers map[string]string, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/user/%s", id)}
	return p.Put(u, headers, data, result)
}

func (p *client) FindWarnQuery(headers map[string]string, archive bool, query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	host := p.cfg.Host
	if host == "" {
		host = "warning:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "warning/warning"}
	v := url.Values{}
	v.Set("query", string(b))
	v.Set("archive", strconv.FormatBool(archive))
	u.RawQuery = v.Encode()
	return p.Get(u, headers, result)
}

func (p *client) FindWarnById(headers map[string]string, id string, archive bool, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "warning:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("warning/warning/%s", id)}
	v := url.Values{}
	v.Set("archive", strconv.FormatBool(archive))
	u.RawQuery = v.Encode()
	return p.Get(u, headers, result)
}

func (p *client) SaveWarn(headers map[string]string, data, archive bool, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "warning:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "warning/warning"}
	v := url.Values{}
	v.Set("archive", strconv.FormatBool(archive))
	u.RawQuery = v.Encode()
	return p.Post(u, headers, data, result)
}

func (p *client) DelWarnById(headers map[string]string, id string, archive bool, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "warning:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("warning/warning/%s", id)}
	v := url.Values{}
	v.Set("archive", strconv.FormatBool(archive))
	u.RawQuery = v.Encode()
	return p.Delete(u, headers, result)
}

func (p *client) UpdateWarnById(headers map[string]string, id string, archive bool, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "warning:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("warning/warning/%s", id)}
	v := url.Values{}
	v.Set("archive", strconv.FormatBool(archive))
	u.RawQuery = v.Encode()
	return p.Patch(u, headers, data, result)
}

func (p *client) ReplaceWarnById(headers map[string]string, id string, archive bool, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "warning:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("warning/warning/%s", id)}
	v := url.Values{}
	v.Set("archive", strconv.FormatBool(archive))
	u.RawQuery = v.Encode()
	return p.Put(u, headers, data, result)
}

func (p *client) DriverConfig(headers map[string]string, driverId, serviceId string) ([]byte, error) {
	project, ok := headers[ginx.XRequestProject]
	if !ok {
		project = ginx.XRequestProjectDefault
		headers[ginx.XRequestProject] = ginx.XRequestProjectDefault
	}

	token, err := p.FindToken(project)
	if err != nil {
		return nil, err
	}
	for k, v := range p.headers {
		headers[k] = v
	}
	headers[ginx.XRequestHeaderAuthorization] = fmt.Sprintf("%s %s", token.TokenType, token.AccessToken)
	host := p.cfg.Host
	if host == "" {
		host = "driver:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("driver/driver/%s/%s/config", driverId, serviceId)}
	resp, err := resty.New().SetTimeout(time.Minute * 1).R().
		SetHeaders(p.headers).
		Get(u.String())

	if err != nil {
		return nil, err
	}

	if resp.StatusCode() == 200 {
		return resp.Body(), nil
	}
	return nil, fmt.Errorf("请求状态:%d,响应:%s", resp.StatusCode(), resp.String())
}

func (p *client) FindGatewayQuery(headers map[string]string, query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	host := p.cfg.Host
	if host == "" {
		host = "gateway:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "gateway/gateway"}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	return p.Get(u, headers, result)
}

func (p *client) FindGatewayById(headers map[string]string, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "gateway:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("gateway/gateway/%s", id)}
	return p.Get(u, headers, result)
}

func (p *client) FindGatewayByType(headers map[string]string, typeName string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "gateway:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("gateway/gateway/type/%s", typeName)}
	return p.Get(u, headers, result)
}

func (p *client) SaveGateway(headers map[string]string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "gateway:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "gateway/gateway"}
	return p.Post(u, headers, data, result)
}

func (p *client) DelGatewayById(headers map[string]string, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "gateway:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("gateway/gateway/%s", id)}
	return p.Delete(u, headers, result)
}

func (p *client) UpdateGatewayById(headers map[string]string, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "gateway:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("gateway/gateway/%s", id)}
	return p.Patch(u, headers, data, result)
}

func (p *client) ReplaceGatewayById(headers map[string]string, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "gateway:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("gateway/gateway/%s", id)}
	return p.Put(u, headers, data, result)
}

func (p *client) FindLogQuery(headers map[string]string, query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "core/log"}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	return p.Get(u, headers, result)
}

func (p *client) FindLogById(headers map[string]string, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/log/%s", id)}
	return p.Get(u, headers, result)
}

func (p *client) SaveLog(headers map[string]string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "core/log"}
	return p.Post(u, headers, data, result)
}

func (p *client) DelLogById(headers map[string]string, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/log/%s", id)}
	return p.Delete(u, headers, result)
}

func (p *client) UpdateLogById(headers map[string]string, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/log/%s", id)}
	return p.Patch(u, headers, data, result)
}

func (p *client) ReplaceLogById(headers map[string]string, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("core/log/%s", id)}
	return p.Put(u, headers, data, result)
}

func (p *client) FindProjectQuery(headers map[string]string, timeout time.Duration, query, result interface{}) (int, error) {
	b, err := json.Marshal(query)
	if err != nil {
		return http.StatusBadRequest, err
	}
	host := p.cfg.Host
	if host == "" {
		host = "spm:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "spm/project"}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	token, err := p.FindToken("baseToken")
	if err != nil && err != redis.Nil {
		return http.StatusBadRequest, err
	} else if err == redis.Nil {
		return http.StatusOK, nil
	}
	headers[ginx.XRequestHeaderAuthorization] = fmt.Sprintf("%s %s", token.TokenType, token.AccessToken)
	resp, err := resty.New().SetTimeout(timeout).R().
		SetHeaders(headers).
		SetResult(result).
		Get(u.String())
	if err != nil {
		return http.StatusInternalServerError, err
	}
	if resp.StatusCode() == 200 {
		return resp.StatusCode(), nil
	}
	return resp.StatusCode(), errors.New(resp.String())
}

func (p *client) CheckDriver(headers map[string]string, licenseName string, signature interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "core:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "core/license/driver"}
	v := url.Values{}
	v.Set("licenseName", licenseName)
	u.RawQuery = v.Encode()
	return p.Get(u, headers, signature)

}

func (p *client) FindFlowQuery(headers map[string]string, query, result interface{}) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	host := p.cfg.Host
	if host == "" {
		host = "flow:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "flow/flow"}
	v := url.Values{}
	v.Set("query", string(b))
	u.RawQuery = v.Encode()
	return p.Get(u, headers, result)
}

func (p *client) FindFlowById(headers map[string]string, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "flow:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("flow/flow/%s", id)}
	return p.Get(u, headers, result)
}

func (p *client) SaveFlow(headers map[string]string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "flow:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: "flow/flow"}
	return p.Post(u, headers, data, result)
}

func (p *client) DelFlowById(headers map[string]string, id string, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "flow:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("flow/flow/%s", id)}
	return p.Delete(u, headers, result)
}

func (p *client) UpdateFlowById(headers map[string]string, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "flow:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("flow/flow/%s", id)}
	return p.Patch(u, headers, data, result)
}

func (p *client) ReplaceFlowById(headers map[string]string, id string, data, result interface{}) error {
	host := p.cfg.Host
	if host == "" {
		host = "flow:9000"
	}
	u := url.URL{Scheme: p.cfg.Schema, Host: host, Path: fmt.Sprintf("flow/flow/%s", id)}
	return p.Put(u, headers, data, result)
}
