package taosx

import (
	"database/sql"
	"time"

	"github.com/air-iot/service/config"
	"github.com/air-iot/service/init/tsdb"
	"github.com/air-iot/service/logger"
)

// Taos taos客户端
type Taos struct {
	addr string
}

// NewTaos 创建Taos存储
func NewTaos(cfg config.Taos) tsdb.TSDB {
	a := new(Taos)
	a.addr = cfg.Addr
	return a
}

func (a *Taos) Write(database, tableName string, ts time.Time, tags map[string]string, fields map[string]interface{}) (err error) {
	db, err := sql.Open("taosSql", a.addr)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			logger.Errorf("关闭连接错误: %s", err.Error())
		}
	}()
	//db.Exec(`insert into %s.n_%s(%s) USING %s.m_%s VALUES(%s)`)
	return nil
}

func (a *Taos) Query(database string, sql string, result interface{}) (err error) {

	return nil
}
