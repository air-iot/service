package taos

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/jmoiron/sqlx"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var DB = new(db)

type db struct {
	sync.RWMutex
	db *sqlx.DB
}

// 初始化taos数据库
func (p *db) Init() error {
	if !viper.GetBool("taos.enable") {
		return errors.New("enable false")
	}
	p.Lock()
	defer p.Unlock()
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
	p.db, err = sqlx.Open("taosSql", fmt.Sprintf(`%s:%s@/tcp(%s:%d)/%s`, username, password, host, port, db))
	if err != nil {
		return err
	}
	p.db.SetMaxIdleConns(maxIdleConn)
	p.db.SetMaxOpenConns(maxOpenConn)
	return nil
}

func (p *db) GetDB() *sqlx.DB {
	return p.db
}

func (p *db) Exec(query string, args ...interface{}) (sql.Result, error) {
	p.RLock()
	defer p.RUnlock()
	if p.db == nil {
		return nil, errors.New("db未初始化")
	}
	return p.db.Exec(query, args...)
}

func (p *db) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	p.RLock()
	defer p.RUnlock()
	if p.db == nil {
		return nil, errors.New("db未初始化")
	}
	return p.db.ExecContext(ctx, query, args...)
}

func (p *db) Query(query string, args ...interface{}) (*sql.Rows, error) {
	p.RLock()
	defer p.RUnlock()
	if p.db == nil {
		return nil, errors.New("db未初始化")
	}
	return p.db.Query(query, args...)
}

func (p *db) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	p.RLock()
	defer p.RUnlock()
	if p.db == nil {
		return nil, errors.New("db未初始化")
	}
	return p.db.QueryContext(ctx, query, args...)
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

func (p *db) Close() error {
	p.Lock()
	defer p.Unlock()
	if p.db != nil {
		if err := p.db.Close(); err != nil {
			logrus.Errorf("taos 关闭失败:%s", err.Error())
			return err
		}
	}
	return nil
}
