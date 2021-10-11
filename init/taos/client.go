package taos

import (
	"github.com/air-iot/service/config"
	"github.com/air-iot/service/logger"
	"github.com/jmoiron/sqlx"
)

// InitTaosDB 初始化涛思存储
func InitTaosDB() (*sqlx.DB, func(), error) {
	cfg := config.C.TSDB.Taos
	return NewTaosDB(cfg)
}

// NewTaosDB 创建涛思存储
func NewTaosDB(cfg config.Taos) (*sqlx.DB, func(), error) {
	var cli *sqlx.DB
	var err error
	cli, err = sqlx.Open("taosSql", cfg.Addr)
	if err != nil {
		return nil, nil, err
	}
	cli.SetMaxOpenConns(cfg.MaxConn)
	cleanFunc := func() {
		err := cli.Close()
		if err != nil {
			logger.Errorf("taos close error: %s", err.Error())
		}
	}
	return cli, cleanFunc, nil
}
