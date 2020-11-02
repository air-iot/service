package taos

import (
	"fmt"
	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var DB *sqlx.DB

// 初始化taos数据库
func Init() {
	if !viper.GetBool("taos.enable") {
		return
	}
	var (
		username = viper.GetString("taos.username")
		password = viper.GetString("taos.password")
		host     = viper.GetString("taos.host")
		port     = viper.GetInt("taos.port")
		db       = viper.GetString("taos.db")
		//initialCap  = viper.GetInt("taos.initialCap")
		maxIdleConn = viper.GetInt("taos.maxIdleConn")
		maxOpenConn = viper.GetInt("taos.maxOpenConn")
		//idleTimeout = viper.GetInt("taos.idleTimeout")
	)
	var err error

	DB, err = sqlx.Open("taosSql", fmt.Sprintf(`%s:%s@/tcp(%s:%d)/%s`, username, password, host, port, db))
	if err != nil {
		panic(err)
	}
	DB.SetMaxIdleConns(maxIdleConn)
	DB.SetMaxOpenConns(maxOpenConn)
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
		if err := DB.Close(); err != nil {
			logrus.Errorf("taos 关闭失败:%s", err.Error())
		}
	}
}
