package tools

import (
	"fmt"
	"reflect"
	"strconv"

	"github.com/go-redis/redis/v8"
	"github.com/air-iot/service/util/json"
)

// InterfaceTypeToRedisMethod 数值类型的redis查询结果转为对应类型
func InterfaceTypeToRedisMethod(t *redis.StringCmd) (interface{}, error) {
	if res, err := t.Float64(); err == nil {
		return res, nil
	}
	if res, err := t.Int64(); err == nil {
		return res, nil
	}
	if res, err := t.Int(); err == nil {
		return res, nil
	}
	if res, err := t.Uint64(); err == nil {
		return res, nil
	}
	return nil, fmt.Errorf("redis查出的数值无法判断为具体数值类型")
}

func InterfaceTypeToString(t interface{}) string {
	if t == nil {
		return ""
	}
	switch reflect.TypeOf(t).String() {
	case "uint", "uintptr", "uint8", "uint16", "uint32", "uint64":
		return strconv.FormatUint(reflect.ValueOf(t).Uint(), 10)
	case "int", "int8", "int16", "int32", "int64":
		return strconv.FormatInt(reflect.ValueOf(t).Int(), 10)
	case "float32", "float64":
		return strconv.FormatFloat(reflect.ValueOf(t).Float(), 'f', -1, 64)
	case "bool":
		return strconv.FormatBool(reflect.ValueOf(t).Bool())
	case "string":
		return reflect.ValueOf(t).String()
	default:
		return ""
	}
}


// MergeDataMap 融合映射Map
func MergeDataMap(key, value string, dataMap *map[string][]string) {
	if key == "" {
		return
	}
	if dataVal, ok := (*dataMap)[key]; ok {
		(*dataMap)[key] = append(dataVal, value)
	} else {
		(*dataMap)[key] = []string{value}
	}
}

func UnmarshalListAndMap(data []byte, eventType string) (*[]map[string]interface{}, error) {
	dataMapList := make([]map[string]interface{}, 0)
	switch eventType {
	case "数据事件":
		err := json.Unmarshal(data, &dataMapList)
		if err != nil {
			return nil, fmt.Errorf("数据事件数据解序列化失败:%s", err.Error())
		}
	default:
		dataMap := map[string]interface{}{}
		err := json.Unmarshal(data, &dataMap)
		if err != nil {
			return nil, fmt.Errorf("数据解序列化失败:%s", err.Error())
		}
		dataMapList = append(dataMapList, dataMap)
	}

	return &dataMapList, nil
}