package service

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"testing"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	mw "github.com/labstack/echo/v4/middleware"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"google.golang.org/grpc"

	"github.com/air-iot/service/db/influx"
	"github.com/air-iot/service/db/mongo"
	"github.com/air-iot/service/db/redis"
	"github.com/air-iot/service/db/sql"
	"github.com/air-iot/service/db/taos/rest"
	"github.com/air-iot/service/db/taos/v2"
	"github.com/air-iot/service/logger"
	"github.com/air-iot/service/logic"
	imw "github.com/air-iot/service/middleware"
	"github.com/air-iot/service/mq/mqtt"
	"github.com/air-iot/service/mq/rabbit"
	"github.com/air-iot/service/mq/srv"
	restfulapi "github.com/air-iot/service/restful-api"
	"github.com/air-iot/service/traefik"
)

var (
	// Version 版本号
	Version       = "2.0"
	privateBlocks []*net.IPNet
	configPath    = flag.String("config", "./etc/", "配置文件所属文件夹路径,默认:./etc/")
	configName    = flag.String("configName", "config", "配置文件名称,默认:config")
	configSuffix  = flag.String("configSuffix", "ini", "配置文件后缀类型,默认:ini")
)

func init() {
	var _ = func() bool {
		testing.Init()
		return true
	}()
	flag.Parse()
	viper.SetDefault("log.level", "ERROR")

	viper.SetDefault("data.action", "mqtt")
	viper.SetDefault("data.save", "influx")
	viper.SetDefault("data.mqApiEnable", false)

	//viper.SetDefault("consul.enable", false)
	//viper.SetDefault("consul.host", "consul")
	//viper.SetDefault("consul.port", 8500)

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
	viper.SetDefault("redis.username", "")
	viper.SetDefault("redis.password", "")
	viper.SetDefault("redis.db", 0)
	viper.SetDefault("redis.poolSize", 100)
	viper.SetDefault("redis.cluster", false)

	viper.SetDefault("db.enable", false)
	viper.SetDefault("db.dialect", "")
	viper.SetDefault("db.url", "")
	viper.SetDefault("db.maxIdleConn", 10)
	viper.SetDefault("db.maxOpenConn", 20)

	viper.SetDefault("taos.enable", false)
	viper.SetDefault("taos.host", "taos")
	viper.SetDefault("taos.port", 0)
	viper.SetDefault("taos.username", "root")
	viper.SetDefault("taos.password", "taosdata")
	viper.SetDefault("taos.db", "tsdb")
	viper.SetDefault("taos.initialCap", 8)
	viper.SetDefault("taos.maxIdleConn", 10)
	viper.SetDefault("taos.maxOpenConn", 20)
	viper.SetDefault("taos.idleTimeout", 15)
	viper.SetDefault("taos.token", "cm9vdDp0YW9zZGF0YQ==")
	viper.SetDefault("taos.httpPort", 6041)
	viper.SetDefault("taos.httpProto", "http")

	viper.SetDefault("mqtt.enable", false)
	viper.SetDefault("mqtt.host", "mqtt")
	viper.SetDefault("mqtt.port", 1883)
	viper.SetDefault("mqtt.apiPort", 15672)
	viper.SetDefault("mqtt.username", "admin")
	viper.SetDefault("mqtt.password", "public")
	viper.SetDefault("mqtt.topic", "data/")
	viper.SetDefault("mqtt.clientId", uuid.New().String())

	viper.SetDefault("rabbit.enable", false)
	viper.SetDefault("rabbit.host", "rabbit")
	viper.SetDefault("rabbit.port", 5672)
	viper.SetDefault("rabbit.apiPort", 15672)
	viper.SetDefault("rabbit.username", "admin")
	viper.SetDefault("rabbit.password", "public")
	viper.SetDefault("rabbit.vhost", "")
	viper.SetDefault("rabbit.routingKey", "data.")
	viper.SetDefault("rabbit.queue", "")

	viper.SetDefault("traefik.host", "traefik")
	viper.SetDefault("traefik.port", 80)
	viper.SetDefault("traefik.enable", false)
	viper.SetDefault("traefik.schema", "http")

	viper.SetDefault("service.enable", true)
	viper.SetDefault("service.pprofEnable", false)
	viper.SetDefault("service.port", 9000)
	viper.SetDefault("service.rpcPort", 9010)
	viper.SetDefault("service.pprofPort", 19000)
	viper.SetDefault("service.rpcEnable", false)

	viper.SetDefault("cache.enable", true)

	viper.SetDefault("auth.middleware", false)

	viper.SetConfigType("env")
	viper.AutomaticEnv()
	viper.SetConfigType(*configSuffix)
	viper.SetConfigName(*configName)
	viper.AddConfigPath(*configPath)
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
	GetRpcServer() *grpc.Server
}

type app struct {
	host               string
	port               int
	pprofPort          int
	rpcPort            int
	rpcEnable          bool
	serviceEnable      bool
	servicePprofEnable bool
	httpServer         *echo.Echo
	grpcServer         *grpc.Server
}

func NewApp() App {
	logger.Init()
	traefik.Init()
	influx.Init()
	if err := taos.DB.Init(); err != nil {
		panic(err)
	}
	taos_rest.Init()
	mongo.Init()
	redis.Init()
	sql.Init()
	mqtt.Init()
	rabbit.Init()
	rabbit.InitApi()
	logic.Init()
	var (
		serviceHost        = viper.GetString("service.host")
		servicePort        = viper.GetInt("service.port")
		servicePprofPort   = viper.GetInt("service.pprofPort")
		rpcPort            = viper.GetInt("service.rpcPort")
		rpcEnable          = viper.GetBool("service.rpcEnable")
		serviceEnable      = viper.GetBool("service.enable")
		servicePprofEnable = viper.GetBool("service.pprofEnable")
	)

	srv.DataAction = viper.GetString("data.action")
	e := echo.New()
	e.Use(mw.CORSWithConfig(mw.CORSConfig{
		AllowMethods:  []string{http.MethodGet, http.MethodHead, http.MethodPut, http.MethodPatch, http.MethodPost, http.MethodDelete},
		ExposeHeaders: []string{"count", "token"},
		AllowOrigins:  []string{"*"},
		AllowHeaders:  []string{"Authorization", echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept},
	}))
	e.Use(mw.Recover())
	if viper.GetBool("auth.middleware") {
		e.Use(imw.AuthFilter())
	}
	e.HTTPErrorHandler = restfulapi.HTTPErrorHandler
	//e.GET("/check", func(c echo.Context) error {
	//	return c.NoContent(http.StatusOK)
	//})
	//e.GET(fmt.Sprintf(`/%s/heart`, serviceName), func(c echo.Context) error {
	//	return c.NoContent(http.StatusOK)
	//})
	var r *grpc.Server
	if rpcEnable {
		r = grpc.NewServer()
	}
	a := &app{
		host:               serviceHost,
		port:               servicePort,
		rpcPort:            rpcPort,
		pprofPort:          servicePprofPort,
		rpcEnable:          rpcEnable,
		serviceEnable:      serviceEnable,
		servicePprofEnable: servicePprofEnable,
		grpcServer:         r,
		httpServer:         e,
	}

	return a
}

func (p *app) Run(handlers ...Handler) {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
	for _, handler := range handlers {
		handler.Start()
	}

	if p.servicePprofEnable {
		go func() {
			//  路径/debug/pprof/
			logrus.Errorln(http.ListenAndServe(fmt.Sprintf(":%d", p.pprofPort), nil))
		}()
	}

	if p.serviceEnable {
		go func() {
			if err := p.httpServer.Start(net.JoinHostPort(p.host, strconv.Itoa(p.port))); err != nil {
				logrus.Errorln(err)
				os.Exit(1)
			}
		}()
	}

	if p.rpcEnable {
		lis, err := net.Listen("tcp", net.JoinHostPort(p.host, strconv.Itoa(p.rpcPort)))
		if err != nil {
			logrus.Errorln(err)
			os.Exit(1)
		}

		go func() {
			if err := p.grpcServer.Serve(lis); err != nil {
				logrus.Errorln(err)
				os.Exit(1)
			}
			println("Rpc Server Port:", p.rpcPort)
		}()
	}

	select {
	case sig := <-ch:
		fmt.Printf("收到关闭信号{%v} \n", sig)
		for _, handler := range handlers {
			handler.Stop()
		}
		p.stop()
		logrus.Debugln("关闭服务,", sig)
	}
	close(ch)
	os.Exit(0)
}

func (p *app) GetServer() *echo.Echo {
	return p.httpServer
}

func (p *app) GetRpcServer() *grpc.Server {
	return p.grpcServer
}

func (p *app) stop() {
	influx.Close()
	mongo.Close()
	redis.Close()
	sql.Close()
	mqtt.Close()
	rabbit.Close()
	if err := taos.DB.Close(); err != nil {
		logrus.Errorln("关闭taos错误,", err)
	}
	if p.grpcServer != nil {
		p.grpcServer.Stop()
	}
	if p.httpServer != nil {
		if err := p.httpServer.Close(); err != nil {
			logrus.Errorln("关闭http错误,", err)
		}
	}
}
