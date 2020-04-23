package service

import (
	"errors"
	"fmt"
	"github.com/air-iot/service/mq/srv"
	"github.com/google/uuid"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
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
	"github.com/air-iot/service/logic"
	"github.com/air-iot/service/mq/mqtt"
	"github.com/air-iot/service/mq/rabbit"
	restfulapi "github.com/air-iot/service/restful-api"
	"github.com/air-iot/service/traefik"
)

var (
	privateBlocks []*net.IPNet
)

func init() {
	viper.SetDefault("log.level", "ERROR")

	viper.SetDefault("data.action", "mqtt")

	viper.SetDefault("consul.enable", true)
	viper.SetDefault("consul.host", "consul")
	viper.SetDefault("consul.port", 8500)

	viper.SetDefault("influx.enable", false)
	viper.SetDefault("influx.host", "influx")
	viper.SetDefault("influx.port", 8086)
	viper.SetDefault("influx.udpPort", 8089)
	viper.SetDefault("influx.username", "")
	viper.SetDefault("influx.password", "")
	viper.SetDefault("influx.mode", "http")
	viper.SetDefault("influx.db", "tsdb")

	viper.SetDefault("mongo.enable", false)
	viper.SetDefault("mongo.username", "root")
	viper.SetDefault("mongo.password", "dell123")
	viper.SetDefault("mongo.host", "mongo")
	viper.SetDefault("mongo.port", 27017)
	viper.SetDefault("mongo.db", "iot")
	viper.SetDefault("mongo.adminDb", "admin")
	viper.SetDefault("mongo.poolSize", 10)

	viper.SetDefault("redis.enable", false)
	viper.SetDefault("redis.host", "redis")
	viper.SetDefault("redis.port", 6379)
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("redis.poolSize", 100)

	viper.SetDefault("db.enable", false)
	viper.SetDefault("db.dialect", "")
	viper.SetDefault("db.url", "")
	viper.SetDefault("db.maxIdleConn", 10)
	viper.SetDefault("db.maxOpenConn", 20)

	viper.SetDefault("mqtt.enable", false)
	viper.SetDefault("mqtt.host", "mqtt")
	viper.SetDefault("mqtt.port", 1883)
	viper.SetDefault("mqtt.username", "admin")
	viper.SetDefault("mqtt.password", "public")
	viper.SetDefault("mqtt.topic", "data/")
	viper.SetDefault("mqtt.clientId", uuid.New().String())

	viper.SetDefault("rabbit.enable", false)
	viper.SetDefault("rabbit.host", "rabbit")
	viper.SetDefault("rabbit.port", 5672)
	viper.SetDefault("rabbit.username", "admin")
	viper.SetDefault("rabbit.password", "public")
	viper.SetDefault("rabbit.vhost", "")
	viper.SetDefault("rabbit.routingKey", "data.")

	viper.SetDefault("traefik.host", "traefik")
	viper.SetDefault("traefik.port", 80)

	viper.SetDefault("cache.enable", true)

	viper.SetConfigType("env")
	viper.AutomaticEnv()
	viper.SetConfigType("ini")
	viper.SetConfigName("config")
	viper.AddConfigPath("./etc/")
	if err := viper.ReadInConfig(); err != nil {
		log.Println("读取配置错误,", err.Error())
	}

	for _, b := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "100.64.0.0/10"} {
		if _, block, err := net.ParseCIDR(b); err == nil {
			privateBlocks = append(privateBlocks, block)
		}
	}
}

type Handler interface {
	Start()
	Stop()
}

type App interface {
	Run(...Handler)
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
	traefik.Init()
	influx.Init()
	mongo.Init()
	redis.Init()
	sql.Init()
	mqtt.Init()
	rabbit.Init()
	logic.Init()
	var (
		serviceID   = viper.GetString("service.id")
		serviceName = viper.GetString("service.name")
		serviceHost = viper.GetString("service.host")
		servicePort = viper.GetInt("service.port")
		serviceTag  = viper.GetString("service.tag")
	)

	srv.DataAction = viper.GetString("data.action")
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
	serviceHost, err = Extract(serviceHost)
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
	e.HTTPErrorHandler = restfulapi.HTTPErrorHandler
	e.GET("/check", func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	e.GET(fmt.Sprintf(`/%s/heart`, serviceName), func(c echo.Context) error {
		return c.NoContent(http.StatusOK)
	})
	tags := strings.Split(serviceTag, ",")
	tags = append(tags, "traefik.enable=true")
	tags = append(tags, fmt.Sprintf("traefik.http.routers.%s.rule=PathPrefix(`/%s`)", serviceName, serviceName))
	a := &app{
		id:         serviceID,
		name:       serviceName,
		host:       serviceHost,
		port:       servicePort,
		tags:       tags,
		httpServer: e,
	}
	if viper.GetBool("consul.enable") {
		if err := a.register(); err != nil {
			logrus.Panic(err)
		}
	}
	return a
}

func (p *app) Run(handlers ...Handler) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
	for _, handler := range handlers {
		handler.Start()
	}
	go func() {
		if err := p.httpServer.Start(net.JoinHostPort(p.host, strconv.Itoa(p.port))); err != nil {
			logrus.Errorln(err)
			os.Exit(1)
		}
	}()
	select {
	case sig := <-ch:
		p.stop()
		logrus.Debugln("关闭服务,", sig)
	}
	close(ch)
	for _, handler := range handlers {
		handler.Stop()
	}
	os.Exit(0)
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
	if viper.GetBool("consul.enable") {
		p.deregister()
	}
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
func Extract(addr string) (string, error) {
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
