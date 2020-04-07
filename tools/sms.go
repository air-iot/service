package tools

import (
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk"
	"github.com/aliyun/alibaba-cloud-sdk-go/sdk/requests"
)

// SendSmsTemplateMessage 发送模板消息
func SendSmsTemplateMessage(phoneNumbers, templateCode, templateParam string) error {

	client, err := sdk.NewClientWithAccessKey("default", "<accessKeyId>", "<accessSecret>")
	if err != nil {
		return err
	}

	request := requests.NewCommonRequest()
	request.Method = "POST"
	request.Scheme = "https" // https | http
	request.Domain = "dysmsapi.aliyuncs.com"
	request.Version = "2017-05-25"
	request.ApiName = "SendSms"
	request.QueryParams["PhoneNumbers"] = phoneNumbers
	request.QueryParams["SignName"] = "阿里云"
	request.QueryParams["TemplateCode"] = templateCode
	request.QueryParams["TemplateParam"] = templateParam

	_, err = client.ProcessCommonRequest(request)
	if err != nil {
		return err
	}
	//fmt.Print(response.GetHttpContentString())

	return nil
}

