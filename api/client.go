package api

import (
	"fmt"
	"errors"
	"encoding/json"
	"gopkg.in/resty.v1"

	"github.com/air-iot/service/model"
	"github.com/air-iot/service/traefik"
)

// 根据 app appkey和appsecret 获取token
func FindToken() (string, error) {
	// 生成要访问的url 5c934c87839a5547624eae7c
	if traefik.AppKey == "" || traefik.AppSecret == "" {
		return "", errors.New("app key或者secret为空")
	}
	resp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		Get(fmt.Sprintf(`http://%s:%d/core/auth/token?appkey=%s&appsecret=%s`, traefik.Host, traefik.Port, traefik.AppKey, traefik.AppSecret))
	if err != nil {
		return "", err
	}
	if resp.StatusCode() != 200 {
		return "", errors.New(resp.String())
	}
	auth := new(model.Auth)
	if err := json.Unmarshal(resp.Body(), auth); err != nil {
		return "", err
	}
	return auth.Token, nil
}

func Get(url, token string, query, result interface{}, ) error {
	b, err := json.Marshal(query)
	if err != nil {
		return err
	}
	resp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", token).
		SetResult(result).
		Get(fmt.Sprintf(`%s?query=%s`, url, string(b)))

	if err != nil {
		return err
	}

	if resp.StatusCode() != 200 {
		return errors.New(resp.String())
	}

	return nil
}

func Post(url, token string, data, result interface{}) error {
	//d, err := json.Marshal(data)
	//if err != nil {
	//	return err
	//}
	resp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", token).
		SetResult(result).
		SetBody(data).
		Post(url)
	if err != nil {
		return err
	}
	if resp.StatusCode() != 200 {
		return errors.New(resp.String())
	}
	return json.Unmarshal(resp.Body(), result)
}

func Delete(url, token, id string, result interface{}) error {
	resp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", token).
		SetResult(result).
		Delete(fmt.Sprintf(`%s/%s`, url, id))
	if err != nil {
		return err
	}
	if resp.StatusCode() != 200 {
		return errors.New(resp.String())
	}
	return json.Unmarshal(resp.Body(), result)
}

func Put(url, token, id string, data, result interface{}) error {
	resp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", token).
		SetResult(result).
		SetBody(data).
		Put(fmt.Sprintf(`%s/%s`, url, id))
	if err != nil {
		return err
	}
	if resp.StatusCode() != 200 {
		return errors.New(resp.String())
	}
	return json.Unmarshal(resp.Body(), result)
}

func Patch(url, token, id string, data, result interface{}) error {
	resp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", token).
		SetResult(result).
		SetBody(data).
		Patch(fmt.Sprintf(`%s/%s`, url, id))
	if err != nil {
		return err
	}
	if resp.StatusCode() != 200 {
		return errors.New(resp.String())
	}
	return json.Unmarshal(resp.Body(), result)
}
