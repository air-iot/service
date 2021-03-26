package logger

import (
	"os"
	"path/filepath"

	"github.com/air-iot/service/config"
	"github.com/air-iot/service/logger"
)

// InitLogger 初始化日志模块
func InitLogger() (func(), error) {
	c := config.C.Log
	return NewLogger(c)
}

// NewLogger 创建日志模块
func NewLogger(c config.Log) (func(), error) {
	logger.SetLevel(c.Level)
	logger.SetFormatter(c.Format)

	// 设定日志输出
	var file *os.File
	if c.Output != "" {
		switch c.Output {
		case "stdout":
			logger.SetOutput(os.Stdout)
		case "stderr":
			logger.SetOutput(os.Stderr)
		case "file":
			if name := c.OutputFile; name != "" {
				_ = os.MkdirAll(filepath.Dir(name), 0777)

				f, err := os.OpenFile(name, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
				if err != nil {
					return nil, err
				}
				logger.SetOutput(f)
				file = f
			}
		}
	}

	return func() {
		if file != nil {
			file.Close()
		}
	}, nil
}
