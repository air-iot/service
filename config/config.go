package config

import (
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/spf13/viper"

	"github.com/air-iot/service/util/json"
)

var (
	// C 全局配置(需要先执行MustLoad，否则拿不到配置)
	C    = new(Config)
	once sync.Once
)

// MustLoad 加载配置
func MustLoad(fpaths ...string) {
	once.Do(func() {
		viper.SetConfigType("env")
		viper.AutomaticEnv()

		for _, fpath := range fpaths {
			dir, file := path.Split(fpath)
			index := strings.LastIndex(file, ".")
			viper.SetConfigName(file[:index])
			viper.SetConfigType(file[index+1:])
			viper.AddConfigPath(dir)
		}
		if err := viper.ReadInConfig(); err != nil {
			log.Fatalln("Fatal error config file: ", err.Error())
		}
		if err := viper.Unmarshal(C); err != nil {
			log.Fatalln("unable to decode into struct: ", err.Error())
		}
	})
}

// PrintWithJSON 基于JSON格式输出配置
func PrintWithJSON() {
	if C.PrintConfig {
		b, err := json.MarshalIndent(C, "", " ")
		if err != nil {
			os.Stdout.WriteString("[CONFIG] JSON marshal error: " + err.Error())
			return
		}
		os.Stdout.WriteString(string(b) + "\n")
	}
}

// Config 配置参数
type Config struct {
	RunMode     string
	Swagger     bool
	PrintConfig bool
	HTTP        HTTP
	Log         Log
	Root        Root
	JWTAuth     JWTAuth
	Captcha     Captcha
	RateLimiter RateLimiter
	CORS        CORS
	GZIP        GZIP
	Redis       Redis
	Mongo       Mongo
	Influx      Influx
	Gorm        Gorm
	MySQL       MySQL
	Postgres    Postgres
	Sqlite3     Sqlite3
	MQ          MQ
	MQTT        MQTT
	RabbitMQ    RabbitMQ
	ApiGateway  ApiGateway
}

// IsDebugMode 是否是debug模式
func (c *Config) IsDebugMode() bool {
	return c.RunMode == "debug"
}

// Menu 菜单配置参数
type Menu struct {
	Enable bool
	Data   string
}

// Casbin casbin配置参数
type Casbin struct {
	Enable           bool
	Debug            bool
	Model            string
	AutoLoad         bool
	AutoLoadInternal int
}

// LogHook 日志钩子
type LogHook string

// IsGorm 是否是gorm钩子
func (h LogHook) IsGorm() bool {
	return h == "gorm"
}

// IsMongo 是否是mongo钩子
func (h LogHook) IsMongo() bool {
	return h == "mongo"
}

// Log 日志配置参数
type Log struct {
	Level         int
	Format        string
	Output        string
	OutputFile    string
	EnableHook    bool
	HookLevels    []string
	Hook          LogHook
	HookMaxThread int
	HookMaxBuffer int
}

// LogGormHook 日志gorm钩子配置
type LogGormHook struct {
	DBType       string
	MaxLifetime  int
	MaxOpenConns int
	MaxIdleConns int
	Table        string
}

// LogMongoHook 日志mongo钩子配置
type LogMongoHook struct {
	Collection string
}

// Root root用户
type Root struct {
	UserName string
}

// JWTAuth 用户认证
type JWTAuth struct {
	Enable        bool
	SigningMethod string
	SigningKey    string
	Expired       int
	Store         string
	FilePath      string
	RedisDB       int
	RedisPrefix   string
}

// HTTP http配置参数
type HTTP struct {
	Host             string
	Port             int
	CertFile         string
	KeyFile          string
	ShutdownTimeout  int
	MaxContentLength int64
	MaxLoggerLength  int `default:"4096"`
}

// Monitor 监控配置参数
type Monitor struct {
	Enable    bool
	Addr      string
	ConfigDir string
}

// Captcha 图形验证码配置参数
type Captcha struct {
	Store       string
	Length      int
	Width       int
	Height      int
	RedisDB     int
	RedisPrefix string
}

// RateLimiter 请求频率限制配置参数
type RateLimiter struct {
	Enable  bool
	Count   int64
	RedisDB int
}

// CORS 跨域请求配置参数
type CORS struct {
	Enable           bool
	AllowOrigins     []string
	AllowMethods     []string
	AllowHeaders     []string
	AllowCredentials bool
	MaxAge           int
}

// GZIP gzip压缩
type GZIP struct {
	Enable             bool
	ExcludedExtentions []string
	ExcludedPaths      []string
}

// Redis redis配置参数
type Redis struct {
	Addr     string
	Password string
	PoolSize int
}

// Mongo mongodb配置参数
type Mongo struct {
	Addr              string
	PoolSize          uint64
	HeartbeatInterval uint16
	MaxConnIdleTime   uint16
}

// Influx influxdb配置参数
type Influx struct {
	Protocol string
	Addr     string
	Username string
	Password string
	DBName   string
}

// Gorm gorm配置参数
type Gorm struct {
	Debug             bool
	DBType            string
	MaxLifetime       int
	MaxOpenConns      int
	MaxIdleConns      int
	TablePrefix       string
	EnableAutoMigrate bool
}

// MySQL mysql配置参数
type MySQL struct {
	Host       string
	Port       int
	User       string
	Password   string
	DBName     string
	Parameters string
}

// DSN 数据库连接串
func (a MySQL) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s",
		a.User, a.Password, a.Host, a.Port, a.DBName, a.Parameters)
}

// Postgres postgres配置参数
type Postgres struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// DSN 数据库连接串
func (a Postgres) DSN() string {
	return fmt.Sprintf("host=%s port=%d user=%s dbname=%s password=%s sslmode=%s",
		a.Host, a.Port, a.User, a.DBName, a.Password, a.SSLMode)
}

// Sqlite3 sqlite3配置参数
type Sqlite3 struct {
	Path string
}

// DSN 数据库连接串
func (a Sqlite3) DSN() string {
	return a.Path
}

// MQ mq配置参数
type MQ struct {
	MQType      string
	TopicPrefix string
}

// MQTT mqtt配置参数
type MQTT struct {
	Host     string
	Port     int
	Username string
	Password string
}

func (a MQTT) DNS() string {
	return fmt.Sprintf("tcp://%s:%d", a.Host, a.Port)
}

// RabbitMQ rabbitmq配置参数
type RabbitMQ struct {
	Host     string
	Port     int
	Username string
	Password string
	VHost    string
	Exchange string
	Queue    string
}

func (a RabbitMQ) DNS() string {
	return fmt.Sprintf("amqp://%s:%s@%s:%d/%s",
		a.Username,
		a.Password,
		a.Host,
		a.Port,
		a.VHost)
}

type ApiGateway struct {
	Host      string
	Port      int
	AppKey    string
	AppSecret string
}
