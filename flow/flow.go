package flow

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/gin/ginx"
	"github.com/tidwall/gjson"
)

var Reg, _ = regexp.Compile("\\${(.+?)}")

const ExtraSymbol = "$#"

func TrimSymbol(s string) string {
	return strings.TrimRight(strings.TrimLeft(s, "${"), "}")
}

func FindExtra(ctx context.Context, apiClient api.Client, param string, variables []byte) (val map[string]interface{}, err error) {
	paramArr := strings.Split(param, ".")
	var index int

	for i, paramTmp := range paramArr {
		if strings.HasPrefix(paramTmp, ExtraSymbol) {
			index = i
			break
		}
	}
	pathArr := make([]string, 0)

	for i := 0; i <= index; i++ {
		pathArr = append(pathArr, paramArr[i])
	}

	pathID := append(append(make([]string, 0), pathArr...), "id")
	pathTableName := append(append(make([]string, 0), pathArr...), "_tableName")

	id := gjson.GetBytes(variables, strings.Join(pathID, ".")).String()
	if id == "" {
		return nil, fmt.Errorf("ID变量为空")
	}

	tableName := gjson.GetBytes(variables, strings.Join(pathTableName, ".")).String()
	if tableName == "" {
		return nil, fmt.Errorf("表名变量为空")
	}

	projectID := gjson.GetBytes(variables, "#project").String()
	if projectID == "" {
		return nil, fmt.Errorf("项目ID变量为空")
	}
	val = make(map[string]interface{})
	switch tableName {
	case "node":
		if err := apiClient.FindNodeById(map[string]string{ginx.XRequestProject: projectID}, id, &val); err != nil {
			return nil, fmt.Errorf("查询资产错误: %v", err)
		}
	case "model":
		if err := apiClient.FindModelById(map[string]string{ginx.XRequestProject: projectID}, id, &val); err != nil {
			return nil, fmt.Errorf("查询模型错误: %v", err)
		}
	}

	return
}
