package traefik

import "github.com/spf13/viper"

var Host string
var Port int
var Proto = "http"

func Init() {
	Host = viper.GetString("traefik.host")
	Port = viper.GetInt("traefik.port")
}
