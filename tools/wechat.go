package tools

import (
	"gopkg.in/resty.v1"
)

// SendWeChatTemplateMessage 发送模板消息
func SendWeChatTemplateMessage(accessToken string, data map[string]interface{}, openID, templateID string) error {

	queryMap := map[string]string{
		"access_token": accessToken,
	}
	////获取所有模板
	//respTemplate, err := resty.NewRequest().
	//	SetQueryParams(queryMap).
	//	Post("https://api.weixin.qq.com/cgi-bin/template/get_all_private_template")
	//if err != nil {
	//	return err
	//}
	//templateObject := map[string]interface{}{}
	//
	//err = json.Unmarshal(respTemplate.Body(), &templateObject)
	//if err != nil {
	//	return err
	//}
	bodyMap := map[string]interface{}{
		"touser":      openID,
		"template_id": templateID,
		"data":        data,
	}
	resp, err := resty.NewRequest().
		SetQueryParams(queryMap).
		SetBody(bodyMap).
		Post("https://api.weixin.qq.com/cgi-bin/message/template/send")
	if err != nil {
		return err
	}

	result := map[string]interface{}{}

	err = json.Unmarshal(resp.Body(), &result)
	if err != nil {
		return err
	}

	return nil
}
