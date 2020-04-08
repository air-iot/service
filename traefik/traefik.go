package traefik

import "github.com/spf13/viper"

var Host string
var Port int
var Proto = "http"

func Init() {
	var (
		Host = viper.GetString("traefik.host")
		Port = viper.GetInt("traefik.port")
	)
	if Host == "" {
		Host = "traefik"
	}
	if Port == 0 {
		Port = 80
	}
}
