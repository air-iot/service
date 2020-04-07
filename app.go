package service

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	consulApi "github.com/hashicorp/consul/api"
	"github.com/labstack/echo/v4"
	mw "github.com/labstack/echo/v4/middleware"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"

	"github.com/air-iot/service/consul"
	"github.com/air-iot/service/db/influx"
	"github.com/air-iot/service/db/mongo"
	"github.com/air-iot/service/db/redis"
	"github.com/air-iot/service/db/sql"
	"github.com/air-iot/service/logger"
	"github.com/air-iot/service/mq/mqtt"
	"github.com/air-iot/service/mq/rabbit"
)

var (
	privateBlocks []*net.IPNet
)

func init() {
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	viper.SetConfigType("ini")
	viper.SetConfigName("config")
	viper.AddConfigPath("./etc/")
	if err := viper.ReadInConfig(); err != nil {
		log.Println("read config", err.Error())
	}

	for _, b := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10"} {
		if _, block, err := net.ParseCIDR(b); err == nil {
			privateBlocks = append(privateBlocks, block)
		}
	}
}

type App interface {
	Run() error
	GetServer() *echo.Echo
}

type app struct {
	id         string
	name       string
	host       string
	port       int
	tags       []string
	httpServer *echo.Echo
}

func NewApp() App {
	logger.Init()
	consul.Init()
	influx.Init()
	mongo.Init()
	redis.Init()
	sql.Init()
	mqtt.Init()
	rabbit.Init()
	var (
		serviceID   = viper.GetString("service.id")
		serviceName = viper.GetString("service.name")
		serviceHost = viper.GetString("service.host")
		servicePort = viper.GetInt("service.port")
		serviceTag  = viper.GetString("service.tag")
	)

	if servicePort == 0 {
		servicePort = 9000
	}
	if serviceName == "" {
		logrus.Panic("服务name不能为空")
	}
	if serviceID == "" {
		logrus.Panic("服务id不能为空")
	}
	var err error
	serviceHost, err = extract(serviceHost)
	if err != nil {
		serviceHost = "127.0.0.1"
	}
	e := echo.New()
	e.Use(mw.CORSWithConfig(mw.CORSConfig{
		AllowMethods:  []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodPatch, http.MethodPost, http.MethodDelete},
		ExposeHeaders: []string{"count", "token"},
		AllowOrigins:  []string{"*"},
		AllowHeaders:  []string{"Authorization", echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
	}))
	e.Use(mw.Recover())
	a := &app{
		id:         serviceID,
		name:       serviceName,
		host:       serviceHost,
		port:       servicePort,
		tags:       strings.Split(serviceTag, ","),
		httpServer: e,
	}
	if err := a.register(); err != nil {
		logrus.Panic(err)
	}
	return a
}

func (p *app) Run() error {
	defer p.stop()
	return p.httpServer.Start(net.JoinHostPort(p.host, strconv.Itoa(p.port)))
}

func (p *app) GetServer() *echo.Echo {
	return p.httpServer
}

func (p *app) stop() {
	influx.Close()
	mongo.Close()
	redis.Close()
	sql.Close()
	mqtt.Close()
	rabbit.Close()
	p.deregister()
}

// Register 服务注册
func (p *app) register() error {
	var check = &consulApi.AgentServiceCheck{
		CheckID: p.id,
		HTTP:    fmt.Sprintf("http://%s:%d/check", p.host, p.port),
		//TCP:                            fmt.Sprintf("%s:%d", p.node.Address, p.node.Port),
		Interval:                       fmt.Sprintf("%v", 10*time.Second),
		Timeout:                        fmt.Sprintf("%v", 30*time.Second),
		DeregisterCriticalServiceAfter: fmt.Sprintf("%v", p.getDeregisterTTL(30*time.Second)),
	}

	// register the service
	asr := &consulApi.AgentServiceRegistration{
		ID:      p.id,
		Name:    p.name,
		Tags:    p.tags,
		Port:    p.port,
		Address: p.host,
		Check:   check,
	}

	// Specify consul connect
	asr.Connect = &consulApi.AgentServiceConnect{
		Native: true,
	}

	if err := consul.Client.Agent().ServiceRegister(asr); err != nil {
		return err
	}

	// save our hash and time check of the service
	// pass the healthcheck
	return consul.Client.Agent().CheckDeregister("service:" + p.id)
}

func (*app) getDeregisterTTL(t time.Duration) time.Duration {
	// splay slightly for the watcher?
	splay := time.Second * 5
	deregTTL := t + splay
	// consul has a minimum timeout on deregistration of 1 minute.
	if t < time.Minute {
		deregTTL = time.Minute + splay
	}
	return deregTTL
}

// Deregister 服务断开
func (p *app) deregister() {
	if err := consul.Client.Agent().ServiceDeregister(p.id); err != nil {
		logrus.Errorln("服务断开错误", err.Error())
	}
}

// extract returns a real ip
func extract(addr string) (string, error) {
	// if addr specified then its returned
	if len(addr) > 0 && (addr != "0.0.0.0" && addr != "[::]") {
		return addr, nil
	}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("获取地址失败: %s", err.Error())
	}

	var ipAddr []byte

	for _, rawAddr := range addrs {
		var ip net.IP
		switch addr := rawAddr.(type) {
		case *net.IPAddr:
			ip = addr.IP
		case *net.IPNet:
			ip = addr.IP
		default:
			continue
		}

		if ip.To4() == nil {
			continue
		}

		if !isPrivateIP(ip.String()) {
			continue
		}

		ipAddr = ip
		break
	}

	if ipAddr == nil {
		return "", errors.New("找不到私有IP地址")
	}

	return net.IP(ipAddr).String(), nil
}

func isPrivateIP(ipAddr string) bool {
	ip := net.ParseIP(ipAddr)
	for _, priv := range privateBlocks {
		if priv.Contains(ip) {
			return true
		}
	}
	return false
}
