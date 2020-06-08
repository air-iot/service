package taos

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/jmoiron/sqlx"
	_ "github.com/taosdata/driver-go/taosSql"
)

var DB *sqlx.DB

func Init() {
	if !viper.GetBool("taos.enable") {
		return
	}
	var (
		username    = viper.GetString("taos.username")
		password    = viper.GetString("taos.password")
		host        = viper.GetString("taos.host")
		port        = viper.GetInt("taos.port")
		db          = viper.GetInt("taos.db")
		maxIdleConn = viper.GetInt("taos.maxIdleConn")
		maxOpenConn = viper.GetInt("taos.maxOpenConn")
	)
	var err error
	DB, err = sqlx.Open("taosSql", fmt.Sprintf(`%s:%s@/tcp(%s:%d)/%s`, username, password, host, port, db))
	if err != nil {
		logrus.Panic(err)
	}
	DB.DB.SetMaxIdleConns(maxIdleConn)
	DB.DB.SetMaxOpenConns(maxOpenConn)
}

func Close() {
	if DB != nil {
		if err := DB.Close(); err != nil {
			logrus.Errorln("Taos 连接关闭", err.Error())
		}
	}
}
