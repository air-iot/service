package formatx

import (
	"regexp"
)

//若有除了数字字母汉字和个别特殊字符以外的其他字符，则匹配失败
func HasSpecialCharacter(data string) bool {
	pattern, _ := regexp.Compile("[^0-9a-zA-Z!@#$%^&*()\\-=_+\\p{Han}]+")
	return pattern.MatchString(data)
}

//func HasSpecialCharacter(data string) bool {
//	pattern, _ := regexp.Compile("[`%~!@#$^&*()=|{}':;',\\[\\].<>/?~！@#￥……&*（）——|{}【】‘；：”“'。，、？]")
//	return pattern.MatchString(data)
//}

func HasChineseCharacterOrSpace(data string) bool {
	pattern, _ := regexp.Compile("[\\s\\p{Han}]")
	return pattern.MatchString(data)
}
