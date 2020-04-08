package consul

import (
	"fmt"
	"strings"

	consulApi "github.com/hashicorp/consul/api"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var Client *consulApi.Client

func Init() {
	if !viper.GetBool("consul.enable") {
		return
	}
	var (
		consulHost = viper.GetString("consul.host")
		consulPort = viper.GetInt("consul.port")
	)
	if consulHost == "" {
		consulHost = "consul"
	}
	if consulPort == 0 {
		consulPort = 8500
	}
	cc := consulApi.DefaultConfig()
	cc.Address = fmt.Sprintf("%s:%d", consulHost, consulPort)
	var err error
	Client, err = consulApi.NewClient(cc)
	if err != nil {
		logrus.Panic(err)
	}
}

func TagConvert(tags []string) map[string]string {
	tagMap := make(map[string]string)
	for _, tag := range tags {
		tagArr := strings.Split(tag, "=")
		if len(tagArr) == 2 {
			tagMap[tagArr[0]] = tagArr[1]
		} else {
			tagMap[tag] = ""
		}
	}
	return tagMap
}
