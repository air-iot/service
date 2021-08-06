package formatx

import (
	"context"
	"regexp"
	"strings"

	"github.com/air-iot/service/api"
	"github.com/air-iot/service/flow"
	"github.com/tidwall/gjson"
)

// TemplateVariableMapping 模板变量映射
func TemplateVariableMapping(templateModelString string, mapping map[string]interface{}) string {
	//识别变量,两边带花括号的
	testRegExp, _ := regexp.Compile("{{(.*?)}}")
	//匹配出变量数组
	templateMatchString := testRegExp.FindAllString(templateModelString, -1)
	for _, v := range templateMatchString {
		//去除花括号
		replaceBrace, _ := regexp.Compile("[{}]")
		formatVariable := replaceBrace.ReplaceAllString(v, "")
		//映射为具体值
		mappingData, ok := mapping[formatVariable]
		if !ok {
			//没有则为空字符串
			mappingData = ""
		}
		//变量为替换为具体值
		templateModelString = strings.ReplaceAll(templateModelString, v, InterfaceTypeToString(mappingData))
	}
	return templateModelString
}

// TemplateVariableMappingFlow 流程模板变量映射
func TemplateVariableMappingFlow(ctx context.Context,apiClient api.Client,templateModelString string, mapping string) (string,error) {
	//识别变量,两边带${}
	testRegExp, _ := regexp.Compile("\\${(.*?)}")
	//匹配出变量数组
	templateMatchString := testRegExp.FindAllString(templateModelString, -1)
	for _, v := range templateMatchString {
		//去除花括号,${和}
		replaceBrace, _ := regexp.Compile("(\\${|})")
		formatVariable := replaceBrace.ReplaceAllString(v, "")
		//映射为具体值
		mappingDataResult := gjson.Get(mapping, formatVariable)
		var mappingData interface{}
		if !mappingDataResult.Exists(){
			//未匹配到变量，需要查询数据库
			jsonResult,err := flow.FindExtra(ctx,apiClient,formatVariable,[]byte(mapping))
			if err != nil{
				return "",err
			}
			mappingData = jsonResult.Value()
		}else{
			mappingData = mappingDataResult.Value()
		}
		//变量为替换为具体值
		templateModelString = strings.ReplaceAll(templateModelString, v, InterfaceTypeToString(mappingData))
	}
	return templateModelString,nil
}
