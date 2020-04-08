package traefik

import "github.com/spf13/viper"

var Host string
var Port int

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
