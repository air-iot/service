package license_test

import (
	"testing"
	"time"

	"github.com/air-iot/service/license"
)

func Test_Start(t *testing.T) {
	l := license.NewLicenseCheck()
	l.Start()

	time.Sleep(time.Second * 10)
	l.Stop()
}
