package flow

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/gin/ginx"
	"github.com/air-iot/service/util/formatx"
	"github.com/air-iot/service/util/json"
	"github.com/tidwall/gjson"
)

var Reg, _ = regexp.Compile("\\${(.+?)}")

const ExtraSymbol = "$#"

func TrimSymbol(s string) string {
	return strings.TrimRight(strings.TrimLeft(s, "${"), "}")
}

func FindExtra(ctx context.Context, apiClient api.Client, param string, variables []byte) (*gjson.Result, error) {
	paramArr := strings.Split(param, ".")
	var index int
	for i, paramTmp := range paramArr {
		if strings.HasPrefix(paramTmp, ExtraSymbol) {
			index = i
			break
		}
	}
	if index+1 == len(paramArr) {
		return nil, fmt.Errorf("config格式不正确")
	}
	pathID := append(append(make([]string, 0), paramArr[:index+1]...), "id")
	pathTableName := append(append(make([]string, 0), paramArr[:index+1]...), "_tableName")
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
	val := make(map[string]interface{})
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

	dataBytes, err := json.Marshal(val)
	if err != nil {
		return nil, fmt.Errorf("数据库数据转字节错误: %v", err)
	}
	result := gjson.GetBytes(dataBytes, strings.Join(paramArr[index+1:], "."))
	if !result.Exists() {
		return nil, fmt.Errorf("变量不存在")
	}
	return &result, nil
}

// TemplateVariableMappingFlow 流程模板变量映射
func TemplateVariableMappingFlow(ctx context.Context, apiClient api.Client, templateModelString string, mapping string) (string, error) {
	//识别变量,两边带${}
	//匹配出变量数组
	templateMatchString := Reg.FindAllString(templateModelString, -1)
	for _, v := range templateMatchString {
		//去除花括号,${和}
		//replaceBrace, _ := regexp.Compile("(\\${|})")
		//formatVariable := replaceBrace.ReplaceAllString(v, "")
		formatVariable := TrimSymbol(v)
		//映射为具体值
		mappingDataResult := gjson.Get(mapping, formatVariable)
		var mappingData interface{}
		if !mappingDataResult.Exists() {
			//未匹配到变量，需要查询数据库
			jsonResult, err := FindExtra(ctx, apiClient, formatVariable, []byte(mapping))
			if err != nil {
				return "", err
			}
			mappingData = jsonResult.Value()
		} else {
			mappingData = mappingDataResult.Value()
		}
		//变量为替换为具体值
		templateModelString = strings.ReplaceAll(templateModelString, v, formatx.InterfaceTypeToString(mappingData))
	}
	return templateModelString, nil
}
