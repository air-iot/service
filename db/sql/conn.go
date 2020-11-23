package sql

import (
	"database/sql"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var DB *sql.DB

func Init() {
	if !viper.GetBool("db.enable") {
		return
	}
	var (
		driverName     = viper.GetString("db.dialect")
		dataSourceName = viper.GetString("db.url")
		maxIdleConn    = viper.GetInt("db.maxIdleConn")
		maxOpenConn    = viper.GetInt("db.maxOpenConn")
	)
	var err error
	DB, err = sql.Open(driverName, dataSourceName)
	if err != nil {
		logrus.Fatalf("DB 客户端创建错误: %s", err.Error())
	}
	DB.SetMaxIdleConns(maxIdleConn)
	DB.SetMaxOpenConns(maxOpenConn)
}

func Close() {
	if DB != nil {
		if err := DB.Close(); err != nil {
			logrus.Errorln("Conn????", err.Error())
		}
	}
}
