package tools

import (
	"regexp"
	"strings"
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
