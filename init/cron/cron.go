package cron

import (
	"github.com/robfig/cron/v3"

	"github.com/air-iot/service/logger"
)

// InitCron 初始化定时器
func InitCron() (*cron.Cron, func(), error) {
	c := cron.New(cron.WithSeconds(), cron.WithChain(cron.SkipIfStillRunning(cron.DefaultLogger)))
	c.Start()

	cleanFunc := func() {
		ctx := c.Stop()

		if ctx.Err() != nil {
			logger.Errorf("cron stop error: %s", ctx.Err().Error())
		}
	}
	return c, cleanFunc, nil
}
