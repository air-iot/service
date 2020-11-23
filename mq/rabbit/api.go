/**
 * @Author: ZhangQiang
 * @Description:
 * @File:  api.go
 * @Version: 1.0.0
 * @Date: 2020/8/12 11:20
 */
package rabbit

import (
	"fmt"
	rabbithole "github.com/michaelklishin/rabbit-hole/v2"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var ApiClient *rabbithole.Client

func InitApi() {
	if !viper.GetBool("data.mqApiEnable") {
		return
	}
	var host string
	var port int
	var username string
	var password string
	if viper.GetString("data.action") == "mqtt" {
		host = viper.GetString("mqtt.host")
		port = viper.GetInt("mqtt.apiPort")
		username = viper.GetString("mqtt.username")
		password = viper.GetString("mqtt.password")
	} else if viper.GetString("data.action") == "rabbit" {
		host = viper.GetString("rabbit.host")
		port = viper.GetInt("rabbit.apiPort")
		username = viper.GetString("rabbit.username")
		password = viper.GetString("rabbit.password")
	}

	var err error
	rmqc, err := rabbithole.NewClient(fmt.Sprintf("http://%s:%d", host, port), username, password)
	if err != nil {
		logrus.Fatalf("Rabbit Api 客户端创建错误: %s", err.Error())
	}
	ApiClient = rmqc
}
