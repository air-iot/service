package tools_test

import (
	"testing"

	"github.com/air-iot/service/tools"
)

func Test_GetRandomString(t *testing.T) {
	t.Log(tools.GetRandomString(20))
}
