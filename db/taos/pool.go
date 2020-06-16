package taos

import (
	"errors"

	"github.com/jmoiron/sqlx"
)

var (
	//ErrClosed 连接池已经关闭Error
	ErrClosed = errors.New("pool is closed")
)

// Pool 基本方法
type Pool interface {
	Get() (*sqlx.DB, error)

	Put(*sqlx.DB) error

	Close(*sqlx.DB) error

	Release()

	Len() int
}