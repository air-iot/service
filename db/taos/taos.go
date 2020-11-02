package taos

import (
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var DB Pool

func Init() {
	if !viper.GetBool("taos.enable") || viper.GetString("taos.version") != "v1" {
		return
	}
	var (
		username    = viper.GetString("taos.username")
		password    = viper.GetString("taos.password")
		host        = viper.GetString("taos.host")
		port        = viper.GetInt("taos.port")
		db          = viper.GetString("taos.db")
		initialCap  = viper.GetInt("taos.initialCap")
		maxIdleConn = viper.GetInt("taos.maxIdleConn")
		maxOpenConn = viper.GetInt("taos.maxOpenConn")
		idleTimeout = viper.GetInt("taos.idleTimeout")
	)
	var err error

	factory := func() (*sqlx.DB, error) {
		return sqlx.Open("taosSql", fmt.Sprintf(`%s:%s@/tcp(%s:%d)/%s`, username, password, host, port, db))
	}
	close := func(v *sqlx.DB) error { return v.Close() }
	poolConfig := &Config{
		InitialCap: initialCap,
		MaxIdle:    maxIdleConn,
		MaxCap:     maxOpenConn,
		Factory:    factory,
		Close:      close,
		//连接最大空闲时间，超过该时间的连接 将会关闭，可避免空闲时连接EOF，自动失效的问题
		IdleTimeout: time.Duration(idleTimeout) * time.Second,
	}

	DB, err = NewChannelPool(poolConfig)

	//DB, err = sqlx.Open("taosSql", fmt.Sprintf(`%s:%s@/tcp(%s:%d)/%s`, username, password, host, port, db))
	if err != nil {
		logrus.Panic(err)
	}
	//DB.DB.SetMaxIdleConns(maxIdleConn)
	//DB.DB.SetMaxOpenConns(maxOpenConn)
}

func GetDB() (*sqlx.DB, error) {
	var (
		username = viper.GetString("taos.username")
		password = viper.GetString("taos.password")
		host     = viper.GetString("taos.host")
		port     = viper.GetInt("taos.port")
		db       = viper.GetString("taos.db")
	)
	return sqlx.Open("taosSql", fmt.Sprintf(`%s:%s@/tcp(%s:%d)/%s`, username, password, host, port, db))
}

func Close() {
	if DB != nil {
		DB.Release()
	}
}
