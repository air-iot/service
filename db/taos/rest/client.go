package taos_rest

import (
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/spf13/viper"
	"net"
	"net/url"
	"strconv"
	"time"
)

var host = "taos"
var port = 6041
var token = "cm9vdDp0YW9zZGF0YQ=="
var DB = "tsdb"
var proto = "http"

func Init() {
	host = viper.GetString("taos.host")
	port = viper.GetInt("taos.httpPort")
	token = viper.GetString("taos.token")
	DB = viper.GetString("taos.db")
	proto = viper.GetString("taos.httpProto")
}

func Exec(sql string, timeout time.Duration, result interface{}) (*resty.Response, error) {
	u := url.URL{Scheme: proto, Host: net.JoinHostPort(host, strconv.Itoa(port)), Path: "/rest/sql"}
	return resty.New().
		SetTimeout(timeout).
		R().
		SetHeaders(map[string]string{"Content-Type": "application/json", "Authorization": fmt.Sprintf("Basic %s", token)}).
		SetBody(sql).
		SetResult(result).
		Post(u.String())
}
