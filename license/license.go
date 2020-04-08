package license

import (
	"fmt"
	"github.com/air-iot/service/traefik"
	"net/http"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"gopkg.in/resty.v1"
)

type LicenseCheck interface {
	Start()
	Stop()
}

type licenseCheck struct {
	finish chan bool
}

func NewLicenseCheck() *licenseCheck {
	l := new(licenseCheck)
	l.finish = make(chan bool, 1)
	return l
}

func (p *licenseCheck) Start() *licenseCheck {
	go func() {
		for {
			select {
			case <-p.finish:
				logrus.Debugln("停止检查授权文件")
				return
			default:
				res, err := resty.R().Get(fmt.Sprintf("%s://%s:%d/core/license/check", traefik.Proto, traefik.Host, traefik.Port))
				if err != nil {
					logrus.Errorln("请检查授权文件接口")
					os.Exit(1)
				}
				if res.StatusCode() != http.StatusOK {
					logrus.Errorln("请检查授权文件")
					os.Exit(1)
				}
				logrus.Debugln("检查授权文件完成")
			}
			time.Sleep(time.Hour * 1)
		}
	}()

	return p
}

func (p *licenseCheck) Stop() {
	p.finish <- true
}
