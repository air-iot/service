package tools

import (
	"fmt"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"strings"

	cmodel "github.com/air-iot/service/model"
)


// RemoveRepByLoop 通过循环过滤重复元素
func RemoveRepByLoop(slc []string, removeEle string) []string {
	result := []string{} // 存放结果
	for i := range slc {
		flag := true
		if slc[i] == removeEle {
			flag = false // 存在重复元素，标识为false
		}
		if flag { // 标识为false，不添加进结果
			result = append(result, slc[i])
		}
	}
	return result
}

// AddNonRepByLoop 通过循环添加非重复元素
func AddNonRepByLoop(slc []string, addEle string) []string {
	flag := true
	for i := range slc {
		if slc[i] == addEle {
			flag = false // 存在重复元素，标识为false
		}
	}
	if flag { // 标识为false，不添加进结果
		slc = append(slc, addEle)
	}
	return slc
}

// AddNonRepByLoop 通过循环添加非重复元素
func AddNonRepPrimitiveAByLoop(slc primitive.A, addEle primitive.M) primitive.A {
	flag := true
	for i := range slc {
		if slcM, ok := slc[i].(primitive.M); ok {
			if _, ok := slcM["id"].(string); !ok {
				return slc
			}
		} else {
			return slc
		}
		if _, ok := addEle["id"].(string); !ok {
			return slc
		}
		if slc[i].(primitive.M)["id"].(string) == addEle["id"].(string) {
			flag = false // 存在重复元素，标识为false
		}
	}
	if flag { // 标识为false，不添加进结果
		slc = append(slc, addEle)
	}
	return slc
}

// AddNonRepByLoop 通过循环添加非重复元素
func AddNonRepTagMongoByLoop(slc []cmodel.TagMongo, addEle cmodel.TagMongo) []cmodel.TagMongo {
	flag := true
	for i := range slc {
		slcID := slc[i].ID
		if slcID == "" {
			return slc
		}
		addEleID := addEle.ID
		if addEleID == "" {
			return slc
		}
		if slc[i].ID == addEle.ID {
			flag = false // 存在重复元素，标识为false
		}
	}
	if flag { // 标识为false，不添加进结果
		slc = append(slc, addEle)
	}
	return slc
}

// AddNonRepObjectIDByLoop 通过循环添加非重复元素
func AddNonRepObjectIDByLoop(slc []primitive.ObjectID, addEle primitive.ObjectID) []primitive.ObjectID {
	flag := true
	for i := range slc {
		if slc[i].Hex() == addEle.Hex() {
			flag = false // 存在重复元素，标识为false
		}
	}
	if flag { // 标识为false，不添加进结果
		slc = append(slc, addEle)
	}
	return slc
}

// StringListToObjectIdList 字符串数组转ObjectId数组
func StringListToObjectIdList(idStringList []string) ([]primitive.ObjectID, error) {
	idObjectList := make([]primitive.ObjectID, 0)
	for _, v := range idStringList {
		objectId, err := primitive.ObjectIDFromHex(v)
		if err != nil {
			return nil, err
		}
		idObjectList = append(idObjectList, objectId)
	}
	return idObjectList, nil
}

// ObjectIdListToStringList 字符串数组转ObjectId数组
func ObjectIdListToStringList(idObjectList []primitive.ObjectID) ([]string, error) {
	idStringList := make([]string, 0)
	for _, v := range idObjectList {
		idStringList = append(idStringList, v.Hex())
	}
	return idStringList, nil
}

// StringListToInterfaceList 字符串数组转Interface数组
func StringListToInterfaceList(stringList []string) ([]interface{}) {
	interfaceList := make([]interface{}, 0)
	for _, v := range stringList {
		interfaceList = append(interfaceList, v)
	}
	return interfaceList
}

// InterfaceListToStringList interface数组转字符串数组
func InterfaceListToStringList(params []interface{}) ([]string) {
	var paramSlice []string
	for _, param := range params {
		if _, ok := param.(string); ok {
			paramSlice = append(paramSlice, param.(string))
		} else {
			//默认是走这条分支
			paramSlice = append(paramSlice, param.(primitive.ObjectID).Hex())
		}
	}
	return paramSlice
}

// CombineRecord 合并记录(data需要排过序的)
func CombineRecord(dataMap []map[string]interface{}, combineKey string) *[]map[string]interface{} {
	existMap := map[string]int{}
	for i, v := range dataMap {
		key := v[combineKey].(string)
		if count, ok := existMap[key]; ok {
			existMap[key] = count + 1
			dataMap[i]["maxRank"] = 0
			dataMap[i-count]["maxRank"] = count + 1
		} else {
			existMap[key] = 1
			dataMap[i]["maxRank"] = 1
		}
	}
	return &dataMap
}

// CombineRecordBson 合并记录(data需要排过序的)(bson)
func CombineRecordBson(dataMap []bson.M, combineKey string) *[]bson.M {
	existMap := map[string]int{}
	for i, v := range dataMap {
		key := v[combineKey].(string)
		if count, ok := existMap[key]; ok {
			existMap[key] = count + 1
			dataMap[i]["maxRank"] = 0
			dataMap[i-count]["maxRank"] = count + 1
		} else {
			existMap[key] = 1
			dataMap[i]["maxRank"] = 1
		}
	}
	return &dataMap
}

// FormatObjectIDList 格式化对象数组为string数组（转换的字段为ObjectID类型或string类型）
func FormatObjectIDList(doc *bson.M, key string, formatKey string) error {
	if _, ok := (*doc)[key]; !ok {
		//(*doc)[key] = make([]string, 0)
		return nil
	}
	if (*doc)[key] == nil {
		(*doc)[key] = ""
		return nil
	}
	interfaceList, ok := (*doc)[key].([]interface{})
	if !ok {
		if stringKey, stringOk := (*doc)[key].(string); stringOk {
			if stringKey == "" {
				return nil
			}
		}
		interfaceObject, objOk := (*doc)[key].(interface{})
		if !objOk {
			return fmt.Errorf("%s的类型不是interface{}或[]interface{}", key)
		} else {
			if emptyObject, ok := interfaceObject.(map[string]interface{}); ok {
				flag := false
				for range emptyObject {
					flag = true
				}
				if !flag {
					return nil
				}
			}
			if _, ok := interfaceObject.([]primitive.ObjectID); ok {
				(*doc)[key] = interfaceObject
				return nil
			}
			stringID := ""
			if ele, ok := interfaceObject.(primitive.ObjectID); ok {
				stringID = ele.Hex()
			} else if ele, ok := interfaceObject.(string); ok {
				stringID = ele
			} else if _, ok := interfaceObject.(map[string]interface{})[formatKey]; !ok {
				return fmt.Errorf("%s需要格式化的Key:%s，不存在", key, formatKey)
			} else if ele, ok := interfaceObject.(map[string]interface{})[formatKey].(string); ok {
				stringID = ele
			} else if ele, ok := interfaceObject.(map[string]interface{})[formatKey].(primitive.ObjectID); ok {
				stringID = ele.Hex()
			} else {
				return fmt.Errorf("%s需要格式化的Key:%s，数值格式错误", key, formatKey)
			}
			oid, err := primitive.ObjectIDFromHex(stringID)
			if err != nil {
				return err
			}
			(*doc)[key] = oid
		}
		//return fmt.Errorf("%s的类型不是[]interface{}", key)
	} else {
		if len(interfaceList) == 0 {
			return nil
		}
		stringList := make([]string, 0)
		for _, v := range interfaceList {
			if ele, ok := v.(primitive.ObjectID); ok {
				stringList = append(stringList, ele.Hex())
			} else if ele, ok := v.(string); ok {
				stringList = append(stringList, ele)
			} else if _, ok := v.(map[string]interface{})[formatKey]; !ok {
				return fmt.Errorf("%s需要格式化的Key:%s，不存在", key, formatKey)
			} else if ele, ok := v.(map[string]interface{})[formatKey].(string); ok {
				stringList = append(stringList, ele)
			} else if ele, ok := v.(map[string]interface{})[formatKey].(primitive.ObjectID); ok {
				stringList = append(stringList, ele.Hex())
			} else {
				return fmt.Errorf("需要格式化的Key:%s，数值格式错误", formatKey)
			}
		}
		oidList, err := StringListToObjectIdList(stringList)
		if err != nil {
			return err
		}
		(*doc)[key] = oidList
	}
	return nil
}

// FormatObjectIDPrimitiveList 格式化对象数组为string数组（转换的字段为ObjectID类型或string类型）(Primitive)
func FormatObjectIDPrimitiveList(doc *bson.M, key string, formatKey string) error {
	if _, ok := (*doc)[key]; !ok {
		//(*doc)[key] = make([]string, 0)
		return nil
	}
	if (*doc)[key] == nil {
		(*doc)[key] = ""
		return nil
	}
	interfaceList, ok := (*doc)[key].([]interface{})
	if !ok {
		if stringKey, stringOk := (*doc)[key].(string); stringOk {
			if stringKey == "" {
				return nil
			}
		}
		interfaceObject, objOk := (*doc)[key].(interface{})
		if !objOk {
			return fmt.Errorf("%s的类型不是interface{}或[]interface{}", key)
		} else {
			if emptyObject, ok := interfaceObject.(map[string]interface{}); ok {
				flag := false
				for range emptyObject {
					flag = true
				}
				if !flag {
					return nil
				}
			}
			if _, ok := interfaceObject.([]primitive.ObjectID); ok {
				(*doc)[key] = interfaceObject
				return nil
			}
			if interfaceList, ok := interfaceObject.(primitive.A); ok {
				stringList := make([]string, 0)
				for _, v := range interfaceList {
					if ele, ok := v.(primitive.ObjectID); ok {
						stringList = append(stringList, ele.Hex())
					} else if ele, ok := v.(string); ok {
						stringList = append(stringList, ele)
					} else if _, ok := v.(primitive.M)[formatKey]; !ok {
						return fmt.Errorf("%s需要格式化的Key:%s，不存在", key, formatKey)
					} else if ele, ok := v.(primitive.M)[formatKey].(string); ok {
						stringList = append(stringList, ele)
					} else if ele, ok := v.(primitive.M)[formatKey].(primitive.ObjectID); ok {
						stringList = append(stringList, ele.Hex())
					} else {
						return fmt.Errorf("需要格式化的Key:%s，数值格式错误", formatKey)
					}
				}
				oidList, err := StringListToObjectIdList(stringList)
				if err != nil {
					return err
				}
				(*doc)[key] = oidList
			} else {
				stringID := ""
				if ele, ok := interfaceObject.(primitive.ObjectID); ok {
					stringID = ele.Hex()
				} else if ele, ok := interfaceObject.(string); ok {
					stringID = ele
				} else if ele, ok := interfaceObject.(*cmodel.Node); ok {
					stringID = ele.ID
				} else if ele, ok := interfaceObject.(*cmodel.Model); ok {
					stringID = ele.ID
				} else if _, ok := interfaceObject.(primitive.M)[formatKey]; !ok {
					return fmt.Errorf("%s需要格式化的Key:%s，不存在", key, formatKey)
				} else if ele, ok := interfaceObject.(primitive.M)[formatKey].(string); ok {
					stringID = ele
				} else if ele, ok := interfaceObject.(primitive.M)[formatKey].(primitive.ObjectID); ok {
					stringID = ele.Hex()
				} else {
					return fmt.Errorf("%s需要格式化的Key:%s，数值格式错误", key, formatKey)
				}
				oid, err := primitive.ObjectIDFromHex(stringID)
				if err != nil {
					return err
				}
				(*doc)[key] = oid
			}
			//stringID := ""
			//if ele, ok := interfaceObject.(primitive.ObjectID); ok {
			//	stringID = ele.Hex()
			//} else if ele, ok := interfaceObject.(string); ok {
			//	stringID = ele
			//} else if _, ok := interfaceObject.(primitive.M)[formatKey]; !ok {
			//	return fmt.Errorf("%s需要格式化的Key:%s，不存在", key, formatKey)
			//} else if ele, ok := interfaceObject.(primitive.M)[formatKey].(string); ok {
			//	stringID = ele
			//} else if ele, ok := interfaceObject.(primitive.M)[formatKey].(primitive.ObjectID); ok {
			//	stringID = ele.Hex()
			//} else {
			//	return fmt.Errorf("%s需要格式化的Key:%s，数值格式错误", key, formatKey)
			//}
			//oid, err := primitive.ObjectIDFromHex(stringID)
			//if err != nil {
			//	return err
			//}
			//(*doc)[key] = oid
		}
		//return fmt.Errorf("%s的类型不是[]interface{}", key)
	} else {
		if len(interfaceList) == 0 {
			return nil
		}
		stringList := make([]string, 0)
		for _, v := range interfaceList {
			if ele, ok := v.(primitive.ObjectID); ok {
				stringList = append(stringList, ele.Hex())
			} else if ele, ok := v.(string); ok {
				stringList = append(stringList, ele)
			} else if _, ok := v.(map[string]interface{})[formatKey]; !ok {
				return fmt.Errorf("%s需要格式化的Key:%s，不存在", key, formatKey)
			} else if ele, ok := v.(map[string]interface{})[formatKey].(string); ok {
				stringList = append(stringList, ele)
			} else if ele, ok := v.(map[string]interface{})[formatKey].(primitive.ObjectID); ok {
				stringList = append(stringList, ele.Hex())
			} else {
				return fmt.Errorf("需要格式化的Key:%s，数值格式错误", formatKey)
			}
		}
		oidList, err := StringListToObjectIdList(stringList)
		if err != nil {
			return err
		}
		(*doc)[key] = oidList
	}
	return nil
}

// FormatObjectIDPrimitiveList 格式化对象数组为string数组（转换的字段为ObjectID类型或string类型）(Primitive)
func FormatObjectIDPrimitiveNameList(doc *bson.M, key string, formatKey string) error {
	if _, ok := (*doc)[key]; !ok {
		//(*doc)[key] = make([]string, 0)
		return nil
	}
	if (*doc)[key] == nil {
		(*doc)[key] = ""
		return nil
	}
	interfaceList, ok := (*doc)[key].([]interface{})
	if !ok {
		if stringKey, stringOk := (*doc)[key].(string); stringOk {
			if stringKey == "" {
				return nil
			}
		}
		interfaceObject, objOk := (*doc)[key].(interface{})
		if !objOk {
			return fmt.Errorf("%s的类型不是interface{}或[]interface{}", key)
		} else {
			if emptyObject, ok := interfaceObject.(map[string]interface{}); ok {
				flag := false
				for range emptyObject {
					flag = true
				}
				if !flag {
					return nil
				}
			}
			if _, ok := interfaceObject.([]primitive.ObjectID); ok {
				(*doc)[key] = interfaceObject
				return nil
			}
			if interfaceList, ok := interfaceObject.(primitive.A); ok {
				stringList := make([]string, 0)
				for _, v := range interfaceList {
					if ele, ok := v.(primitive.ObjectID); ok {
						stringList = append(stringList, ele.Hex())
					} else if ele, ok := v.(string); ok {
						stringList = append(stringList, ele)
					} else if _, ok := v.(primitive.M)[formatKey]; !ok {
						return fmt.Errorf("%s需要格式化的Key:%s，不存在", key, formatKey)
					} else if ele, ok := v.(primitive.M)[formatKey].(string); ok {
						stringList = append(stringList, ele)
					} else if ele, ok := v.(primitive.M)[formatKey].(primitive.ObjectID); ok {
						stringList = append(stringList, ele.Hex())
					} else {
						return fmt.Errorf("需要格式化的Key:%s，数值格式错误", formatKey)
					}
				}
				//oidList, err := StringListToObjectIdList(stringList)
				//if err != nil {
				//	return err
				//}
				(*doc)[key] = stringList
			} else {
				stringID := ""
				if ele, ok := interfaceObject.(primitive.ObjectID); ok {
					stringID = ele.Hex()
				} else if ele, ok := interfaceObject.(string); ok {
					stringID = ele
				} else if ele, ok := interfaceObject.(*cmodel.Node); ok {
					stringID = ele.ID
				} else if ele, ok := interfaceObject.(*cmodel.Model); ok {
					stringID = ele.ID
				} else if _, ok := interfaceObject.(primitive.M)[formatKey]; !ok {
					return fmt.Errorf("%s需要格式化的Key:%s，不存在", key, formatKey)
				} else if ele, ok := interfaceObject.(primitive.M)[formatKey].(string); ok {
					stringID = ele
				} else if ele, ok := interfaceObject.(primitive.M)[formatKey].(primitive.ObjectID); ok {
					stringID = ele.Hex()
				} else {
					return fmt.Errorf("%s需要格式化的Key:%s，数值格式错误", key, formatKey)
				}
				//oid, err := primitive.ObjectIDFromHex(stringID)
				//if err != nil {
				//	return err
				//}
				(*doc)[key] = stringID
			}
			//stringID := ""
			//if ele, ok := interfaceObject.(primitive.ObjectID); ok {
			//	stringID = ele.Hex()
			//} else if ele, ok := interfaceObject.(string); ok {
			//	stringID = ele
			//} else if _, ok := interfaceObject.(primitive.M)[formatKey]; !ok {
			//	return fmt.Errorf("%s需要格式化的Key:%s，不存在", key, formatKey)
			//} else if ele, ok := interfaceObject.(primitive.M)[formatKey].(string); ok {
			//	stringID = ele
			//} else if ele, ok := interfaceObject.(primitive.M)[formatKey].(primitive.ObjectID); ok {
			//	stringID = ele.Hex()
			//} else {
			//	return fmt.Errorf("%s需要格式化的Key:%s，数值格式错误", key, formatKey)
			//}
			//oid, err := primitive.ObjectIDFromHex(stringID)
			//if err != nil {
			//	return err
			//}
			//(*doc)[key] = oid
		}
		//return fmt.Errorf("%s的类型不是[]interface{}", key)
	} else {
		if len(interfaceList) == 0 {
			return nil
		}
		stringList := make([]string, 0)
		for _, v := range interfaceList {
			if ele, ok := v.(primitive.ObjectID); ok {
				stringList = append(stringList, ele.Hex())
			} else if ele, ok := v.(string); ok {
				stringList = append(stringList, ele)
			} else if _, ok := v.(map[string]interface{})[formatKey]; !ok {
				return fmt.Errorf("%s需要格式化的Key:%s，不存在", key, formatKey)
			} else if ele, ok := v.(map[string]interface{})[formatKey].(string); ok {
				stringList = append(stringList, ele)
			} else if ele, ok := v.(map[string]interface{})[formatKey].(primitive.ObjectID); ok {
				stringList = append(stringList, ele.Hex())
			} else {
				return fmt.Errorf("需要格式化的Key:%s，数值格式错误", formatKey)
			}
		}
		//oidList, err := StringListToObjectIdList(stringList)
		//if err != nil {
		//	return err
		//}
		(*doc)[key] = stringList
	}
	return nil
}

// FormatObjectIDListMap 格式化对象数组为string数组（转换的字段为ObjectID类型或string类型）(doc为map类型)
func FormatObjectIDListMap(doc *map[string]interface{}, key string, formatKey string) error {
	if _, ok := (*doc)[key]; !ok {
		//(*doc)[key] = make([]string, 0)
		return nil
	}
	if (*doc)[key] == nil {
		(*doc)[key] = ""
		return nil
	}
	interfaceList, ok := (*doc)[key].([]interface{})
	if !ok {
		if stringKey, stringOk := (*doc)[key].(string); stringOk {
			if stringKey == "" {
				return nil
			}
		}
		interfaceObject, objOk := (*doc)[key].(interface{})
		if !objOk {
			return fmt.Errorf("%s的类型不是interface{}或[]interface{}", key)
		} else {
			if emptyObject, ok := interfaceObject.(map[string]interface{}); ok {
				flag := false
				for range emptyObject {
					flag = true
				}
				if !flag {
					return nil
				}
			}
			if _, ok := interfaceObject.([]primitive.ObjectID); ok {
				(*doc)[key] = interfaceObject
				return nil
			}
			stringID := ""
			if interfaceList, ok := interfaceObject.(primitive.A); ok {
				stringList := make([]string, 0)
				for _, v := range interfaceList {
					if ele, ok := v.(primitive.ObjectID); ok {
						stringList = append(stringList, ele.Hex())
					} else if ele, ok := v.(string); ok {
						stringList = append(stringList, ele)
					} else if _, ok := v.(map[string]interface{})[formatKey]; !ok {
						return fmt.Errorf("需要格式化的Key:%s，不存在", formatKey)
					} else if ele, ok := v.(map[string]interface{})[formatKey].(string); ok {
						stringList = append(stringList, ele)
					} else if ele, ok := v.(map[string]interface{})[formatKey].(primitive.ObjectID); ok {
						stringList = append(stringList, ele.Hex())
					} else {
						return fmt.Errorf("需要格式化的Key:%s，数值格式错误", formatKey)
					}
				}
				oidList, err := StringListToObjectIdList(stringList)
				if err != nil {
					return err
				}
				(*doc)[key] = oidList
			} else {
				if ele, ok := interfaceObject.(primitive.ObjectID); ok {
					stringID = ele.Hex()
				} else if ele, ok := interfaceObject.(string); ok {
					stringID = ele
				} else if ele, ok := interfaceObject.(*cmodel.Node); ok {
					stringID = ele.ID
				} else if ele, ok := interfaceObject.(*cmodel.Model); ok {
					stringID = ele.ID
				} else if _, ok := interfaceObject.(map[string]interface{})[formatKey]; !ok {
					return fmt.Errorf("需要格式化的Key:%s，不存在", formatKey)
				} else if ele, ok := interfaceObject.(map[string]interface{})[formatKey].(string); ok {
					stringID = ele
				} else if ele, ok := interfaceObject.(map[string]interface{})[formatKey].(primitive.ObjectID); ok {
					stringID = ele.Hex()
				} else {
					return fmt.Errorf("需要格式化的Key:%s，数值格式错误", formatKey)
				}
				oid, err := primitive.ObjectIDFromHex(stringID)
				if err != nil {
					return err
				}
				(*doc)[key] = oid
			}
		}
		//return fmt.Errorf("%s的类型不是[]interface{}", key)
	} else {
		if len(interfaceList) == 0 {
			return nil
		}
		stringList := make([]string, 0)
		for _, v := range interfaceList {
			if ele, ok := v.(primitive.ObjectID); ok {
				stringList = append(stringList, ele.Hex())
			} else if ele, ok := v.(string); ok {
				stringList = append(stringList, ele)
			} else if _, ok := v.(map[string]interface{})[formatKey]; !ok {
				return fmt.Errorf("需要格式化的Key:%s，不存在", formatKey)
			} else if ele, ok := v.(map[string]interface{})[formatKey].(string); ok {
				stringList = append(stringList, ele)
			} else if ele, ok := v.(map[string]interface{})[formatKey].(primitive.ObjectID); ok {
				stringList = append(stringList, ele.Hex())
			} else {
				return fmt.Errorf("需要格式化的Key:%s，数值格式错误", formatKey)
			}
		}
		oidList, err := StringListToObjectIdList(stringList)
		if err != nil {
			return err
		}
		(*doc)[key] = oidList
	}
	return nil
}

// FormatObjectIDListMap 格式化对象数组为string数组（转换的字段为ObjectID类型或string类型）(doc为map类型)
func FormatObjectIDListMapBak(doc *map[string]interface{}, key string, formatKey string) error {
	if _, ok := (*doc)[key]; !ok {
		//(*doc)[key] = make([]string, 0)
		return nil
	}
	if (*doc)[key] == nil {
		(*doc)[key] = ""
		return nil
	}
	interfaceList, ok := (*doc)[key].([]interface{})
	if !ok {
		if stringKey, stringOk := (*doc)[key].(string); stringOk {
			if stringKey == "" {
				return nil
			}
		}
		interfaceObject, objOk := (*doc)[key].(interface{})
		if !objOk {
			return fmt.Errorf("%s的类型不是interface{}或[]interface{}", key)
		} else {
			if emptyObject, ok := interfaceObject.(map[string]interface{}); ok {
				flag := false
				for range emptyObject {
					flag = true
				}
				if !flag {
					return nil
				}
			}
			if _, ok := interfaceObject.([]primitive.ObjectID); ok {
				(*doc)[key] = interfaceObject
				return nil
			}
			stringID := ""
			if ele, ok := interfaceObject.(primitive.ObjectID); ok {
				stringID = ele.Hex()
			} else if ele, ok := interfaceObject.(string); ok {
				stringID = ele
			} else if _, ok := interfaceObject.(map[string]interface{})[formatKey]; !ok {
				return fmt.Errorf("需要格式化的Key:%s，不存在", formatKey)
			} else if ele, ok := interfaceObject.(map[string]interface{})[formatKey].(string); ok {
				stringID = ele
			} else if ele, ok := interfaceObject.(map[string]interface{})[formatKey].(primitive.ObjectID); ok {
				stringID = ele.Hex()
			} else {
				return fmt.Errorf("需要格式化的Key:%s，数值格式错误", formatKey)
			}
			oid, err := primitive.ObjectIDFromHex(stringID)
			if err != nil {
				return err
			}
			(*doc)[key] = oid
		}
		//return fmt.Errorf("%s的类型不是[]interface{}", key)
	} else {
		if len(interfaceList) == 0 {
			return nil
		}
		stringList := make([]string, 0)
		for _, v := range interfaceList {
			if ele, ok := v.(primitive.ObjectID); ok {
				stringList = append(stringList, ele.Hex())
			} else if ele, ok := v.(string); ok {
				stringList = append(stringList, ele)
			} else if _, ok := v.(map[string]interface{})[formatKey]; !ok {
				return fmt.Errorf("需要格式化的Key:%s，不存在", formatKey)
			} else if ele, ok := v.(map[string]interface{})[formatKey].(string); ok {
				stringList = append(stringList, ele)
			} else if ele, ok := v.(map[string]interface{})[formatKey].(primitive.ObjectID); ok {
				stringList = append(stringList, ele.Hex())
			} else {
				return fmt.Errorf("需要格式化的Key:%s，数值格式错误", formatKey)
			}
		}
		oidList, err := StringListToObjectIdList(stringList)
		if err != nil {
			return err
		}
		(*doc)[key] = oidList
	}
	return nil
}

// FormatObjectID 格式化对象为string（转换的字段为ObjectID类型或string类型）
func FormatObjectID(doc *bson.M, key string, formatKey string) error {
	if _, ok := (*doc)[key]; !ok {
		//(*doc)[key] = make([]string, 0)
		return nil
	}
	if (*doc)[key] == nil {
		(*doc)[key] = ""
		return nil
	}
	if stringKey, stringOk := (*doc)[key].(string); stringOk {
		if stringKey == "" {
			return nil
		}
	}
	interfaceObject, ok := (*doc)[key].(interface{})
	if !ok {
		return fmt.Errorf("%s的类型不是interface{}", key)
	}
	if emptyObject, ok := interfaceObject.(map[string]interface{}); ok {
		flag := false
		for range emptyObject {
			flag = true
		}
		if !flag {
			return nil
		}
	}
	if _, ok := interfaceObject.([]primitive.ObjectID); ok {
		(*doc)[key] = interfaceObject
		return nil
	}
	stringID := ""
	if ele, ok := interfaceObject.(primitive.ObjectID); ok {
		stringID = ele.Hex()
	} else if ele, ok := interfaceObject.(string); ok {
		stringID = ele
	} else if _, ok := interfaceObject.(map[string]interface{})[formatKey]; !ok {
		return fmt.Errorf("需要格式化的Key:%s，不存在", formatKey)
	} else if ele, ok := interfaceObject.(map[string]interface{})[formatKey].(string); ok {
		stringID = ele
	} else if ele, ok := interfaceObject.(map[string]interface{})[formatKey].(primitive.ObjectID); ok {
		stringID = ele.Hex()
	} else {
		return fmt.Errorf("需要格式化的Key:%s，数值格式错误", formatKey)
	}
	oid, err := primitive.ObjectIDFromHex(stringID)
	if err != nil {
		return err
	}
	(*doc)[key] = oid
	return nil
}

// GetFormatObjectIDList 格式化对象数组为string数组（转换的字段为ObjectID类型或string类型）(不改变原对象)（处理bson.M）
func GetFormatObjectIDList(doc *bson.M, key string, formatKey string) ([]string, error) {
	stringList := make([]string, 0)
	if _, ok := (*doc)[key]; !ok {
		//(*doc)[key] = make([]string, 0)
		return stringList, nil
	}
	interfaceList, ok := (*doc)[key].(primitive.A)
	if !ok {
		return nil, fmt.Errorf("%s的类型不是primitive.A", key)
	}
	for _, v := range interfaceList {
		if ele, ok := v.(primitive.ObjectID); ok {
			stringList = append(stringList, ele.Hex())
		} else if ele, ok := v.(string); ok {
			stringList = append(stringList, ele)
		} else if _, ok := v.(bson.M)[formatKey]; !ok {
			return nil, fmt.Errorf("需要格式化的Key:%s，不存在", formatKey)
		} else if ele, ok := v.(bson.M)[formatKey].(string); ok {
			stringList = append(stringList, ele)
		} else if ele, ok := v.(bson.M)[formatKey].(primitive.ObjectID); ok {
			stringList = append(stringList, ele.Hex())
		} else {
			return nil, fmt.Errorf("需要格式化的Key:%s，数值格式错误", formatKey)
		}
	}
	return stringList, nil
}

// GetJsonLogicVarList 递归获取jsonlogic中var字段的值组成数组
func GetJsonLogicVarListPointVar(data primitive.M, varList *[]string) {
	for k, v := range data {
		switch val := v.(type) {
		case primitive.M:
			convertJsonLogicMapPointVar(varList, k, val)
		case primitive.A:
			convertJsonLogicAPointVar(varList, k, val)
		case string:
			if k == "var" || k == "varrate" {
				if strings.HasPrefix(val, "var.") || strings.HasPrefix(val, "varrate.") {
					splitList := strings.Split(val, ".")
					*varList = AddNonRepByLoop(*varList, splitList[1])
				}
			}
		case map[string]interface{}:
			convertJsonLogicMapPointVar(varList, k, val)
		case []interface{}:
			convertJsonLogicAPointVar(varList, k, val)
		default:
		}
	}
}

// ConvertJsonLogicMap 转换为primitive.M
func convertJsonLogicMapPointVar(varList *[]string, key string, value primitive.M) {
	for k, v := range value {
		switch val := v.(type) {
		case primitive.M:
			convertJsonLogicMapPointVar(varList, k, val)
		case primitive.A:
			convertJsonLogicAPointVar(varList, k, val)
		case string:
			if k == "var" || k == "varrate" {
				if strings.HasPrefix(val, "var.") || strings.HasPrefix(val, "varrate.") {
					splitList := strings.Split(val, ".")
					*varList = AddNonRepByLoop(*varList, splitList[1])
				}
			}
		case map[string]interface{}:
			convertJsonLogicMapPointVar(varList, k, val)
		case []interface{}:
			convertJsonLogicAPointVar(varList, k, val)
		default:
		}
	}
}

// ConvertJsonLogicA 转换primitive.A类型数组
func convertJsonLogicAPointVar(varList *[]string, key string, val primitive.A) {
	for _, outValue := range val {
		switch val := outValue.(type) {
		case primitive.M:
			convertJsonLogicMapPointVar(varList, key, val)
		case primitive.A:
			convertJsonLogicAPointVar(varList, key, val)
		case string:
			if key == "var" || key == "varrate" {
				if strings.HasPrefix(val, "var.") || strings.HasPrefix(val, "varrate.") {
					splitList := strings.Split(val, ".")
					*varList = AddNonRepByLoop(*varList, splitList[1])
				}
			}
		case map[string]interface{}:
			convertJsonLogicMapPointVar(varList, key, val)
		case []interface{}:
			convertJsonLogicAPointVar(varList, key, val)
		default:
		}

	}
}

// GetJsonLogicVarList 递归获取jsonlogic中var字段的值组成数组
func GetJsonLogicVarList(data primitive.M, varList *[]string) {
	for k, v := range data {
		switch val := v.(type) {
		case primitive.M:
			convertJsonLogicMap(varList, k, val)
		case primitive.A:
			convertJsonLogicA(varList, k, val)
		case string:
			if k == "var" || k == "varrate" {
				*varList = AddNonRepByLoop(*varList, v.(string))
			}
		case map[string]interface{}:
			convertJsonLogicMap(varList, k, val)
		case []interface{}:
			convertJsonLogicA(varList, k, val)
		default:
		}
	}
}

// ConvertJsonLogicMap 转换为primitive.M
func convertJsonLogicMap(varList *[]string, key string, value primitive.M) {
	for k, v := range value {
		switch val := v.(type) {
		case primitive.M:
			convertJsonLogicMap(varList, k, val)
		case primitive.A:
			convertJsonLogicA(varList, k, val)
		case string:
			if k == "var" || k == "varrate" {
				*varList = AddNonRepByLoop(*varList, v.(string))
			}
		case map[string]interface{}:
			convertJsonLogicMap(varList, k, val)
		case []interface{}:
			convertJsonLogicA(varList, k, val)
		default:
		}
	}
}

// ConvertJsonLogicA 转换primitive.A类型数组
func convertJsonLogicA(varList *[]string, key string, val primitive.A) {
	for _, outValue := range val {
		switch val := outValue.(type) {
		case primitive.M:
			convertJsonLogicMap(varList, key, val)
		case primitive.A:
			convertJsonLogicA(varList, key, val)
		case string:
			if key == "var" || key == "varrate" {
				*varList = AddNonRepByLoop(*varList, outValue.(string))
			}
		case map[string]interface{}:
			convertJsonLogicMap(varList, key, val)
		case []interface{}:
			convertJsonLogicA(varList, key, val)
		default:
		}

	}
}

//// GetJsonLogicVarList 递归获取jsonlogic中var字段的值组成数组
//func GetJsonLogicVarList(data primitive.M, varList *[]string) {
//	for k, v := range data {
//		switch val := v.(type) {
//		case primitive.M:
//			convertJsonLogicMap(varList, k, val)
//		case primitive.A:
//			convertJsonLogicA(varList, k, val)
//		case string:
//			if k == "var" || k == "varrate" {
//				*varList = AddNonRepByLoop(*varList, v.(string))
//			}
//		case map[string]interface{}:
//			convertJsonLogicMap(varList, k, val)
//		case []interface{}:
//			convertJsonLogicA(varList, k, val)
//		default:
//		}
//	}
//}
//
//// ConvertJsonLogicMap 转换为primitive.M
//func convertJsonLogicMap(varList *[]string, key string, value primitive.M) {
//	for k, v := range value {
//		switch val := v.(type) {
//		case primitive.M:
//			convertJsonLogicMap(varList, k, val)
//		case primitive.A:
//			convertJsonLogicA(varList, k, val)
//		case string:
//			if k == "var" || k == "varrate" {
//				*varList = AddNonRepByLoop(*varList, v.(string))
//			}
//		case map[string]interface{}:
//			convertJsonLogicMap(varList, k, val)
//		case []interface{}:
//			convertJsonLogicA(varList, k, val)
//		default:
//		}
//	}
//}
//
//// ConvertJsonLogicA 转换primitive.A类型数组
//func convertJsonLogicA(varList *[]string, key string, val primitive.A) {
//	for _, outValue := range val {
//		switch val := outValue.(type) {
//		case primitive.M:
//			convertJsonLogicMap(varList, key, val)
//		case primitive.A:
//			convertJsonLogicA(varList, key, val)
//		case string:
//			if key == "var" || key == "varrate" {
//				*varList = AddNonRepByLoop(*varList, outValue.(string))
//			}
//		case map[string]interface{}:
//			convertJsonLogicMap(varList, key, val)
//		case []interface{}:
//			convertJsonLogicA(varList, key, val)
//		default:
//		}
//
//	}
//}

// GetJsonLogicVarList 递归获取jsonlogic中var字段的值组成数组
func GetJsonLogicVarListNode(data primitive.M, varList *[]string) {
	for k, v := range data {
		switch val := v.(type) {
		case primitive.M:
			convertJsonLogicMapNode(varList, k, val)
		case primitive.A:
			convertJsonLogicANode(varList, k, val)
		case string:
			if k == "var" || k == "varrate" {
				if strings.HasPrefix(val, "node.") {
					splitList := strings.Split(val, ".")
					*varList = AddNonRepByLoop(*varList, splitList[1])
				}
			}
		case map[string]interface{}:
			convertJsonLogicMapNode(varList, k, val)
		case []interface{}:
			convertJsonLogicANode(varList, k, val)
		default:
		}
	}
}

// ConvertJsonLogicMap 转换为primitive.M
func convertJsonLogicMapNode(varList *[]string, key string, value primitive.M) {
	for k, v := range value {
		switch val := v.(type) {
		case primitive.M:
			convertJsonLogicMapNode(varList, k, val)
		case primitive.A:
			convertJsonLogicANode(varList, k, val)
		case string:
			if k == "var" || k == "varrate" {
				if strings.HasPrefix(val, "node.") {
					splitList := strings.Split(val, ".")
					*varList = AddNonRepByLoop(*varList, splitList[1])
				}
			}
		case map[string]interface{}:
			convertJsonLogicMapNode(varList, k, val)
		case []interface{}:
			convertJsonLogicANode(varList, k, val)
		default:
		}
	}
}

// ConvertJsonLogicA 转换primitive.A类型数组
func convertJsonLogicANode(varList *[]string, key string, val primitive.A) {
	for _, outValue := range val {
		switch val := outValue.(type) {
		case primitive.M:
			convertJsonLogicMapNode(varList, key, val)
		case primitive.A:
			convertJsonLogicANode(varList, key, val)
		case string:
			if key == "var" || key == "varrate" {
				if strings.HasPrefix(val, "node.") {
					splitList := strings.Split(val, ".")
					*varList = AddNonRepByLoop(*varList, splitList[1])
				}
			}
		case map[string]interface{}:
			convertJsonLogicMapNode(varList, key, val)
		case []interface{}:
			convertJsonLogicANode(varList, key, val)
		default:
		}

	}
}

func FormatKeyInfoList(infoList []bson.M, key string) string {
	result := ""
	for _, info := range infoList {
		if result != "" {
			result = result + "；"
		}
		if value, ok := info[key].(string); ok {
			result = result + value
		}
	}
	return result
}


func FormatKeyInfoListMap(infoList []map[string]interface{}, key string) string {
	result := ""
	for _, info := range infoList {
		if result != "" {
			result = result + "；"
		}
		if value, ok := info[key].(string); ok {
			result = result + value
		}
	}
	return result
}

func FormatKeyInfoMapList(infoList []map[string]interface{}, key string) string {
	result := ""
	for _, info := range infoList {
		if result != "" {
			result = result + "；"
		}
		if value, ok := info[key].(string); ok {
			result = result + value
		}
	}
	return result
}

func FormatKeyInfo(info bson.M, key string) string {
	result := ""
	if value, ok := info[key].(string); ok {
		result = result + value
	}
	return result
}

func FormatKeyInfoMap(info map[string]interface{}, key string) string {
	result := ""
	if value, ok := info[key].(string); ok {
		result = result + value
	}
	return result
}

func FormatDataInfoList(infoList []map[string]interface{}) string {
	result := ""
	for _, info := range infoList {
		if result == "" {
			result = "数据点名称："
		} else {
			result = result + "；数据点名称："
		}
		if key, ok := info["name"].(string); ok {
			result = result + key
		}
		result = result + "，数值："
		if value, ok := info["value"]; ok {
			result = result + InterfaceTypeToString(value)
		}
	}
	return result
}

// GetJsonLogicSymbol 递归获取jsonlogic中大于小于字段的值组成数组
func GetJsonLogicSymbol(data *primitive.M, deadArea float64) {
	for k, v := range *data {
		//switch k {
		//case "<", ">", ">=", "<=":
		//	*varList = AddNonRepByLoop(*varList, k)
		//	return
		//}
		switch val := v.(type) {
		case primitive.M:
			convertJsonLogicMapSymbol(k, &val, deadArea)
		case primitive.A:
			convertJsonLogicASymbol(k, &val, deadArea)
		//case string:
		//	if k == "<" || k == ">" || k == ">=" || k == "<=" {
		//		*varList = AddNonRepByLoop(*varList, k)
		//		return
		//	}
		case map[string]interface{}:
			convertJsonLogicMapSymbol(k, (*primitive.M)(&val), deadArea)
		case []interface{}:
			convertJsonLogicASymbol(k, (*primitive.A)(&val), deadArea)
		default:
		}
	}
}

// convertJsonLogicMapSymbol 转换为primitive.M
func convertJsonLogicMapSymbol(key string, value *primitive.M, deadArea float64) {
	for k, v := range *value {
		switch val := v.(type) {
		case primitive.M:
			convertJsonLogicMapSymbol(k, &val, deadArea)
		case primitive.A:
			convertJsonLogicASymbol(k, &val, deadArea)
		//case string:
		//	if k == "<" || k == ">" || k == ">=" || k == "<=" {
		//		*varList = AddNonRepByLoop(*varList, k)
		//		return
		//	}
		case map[string]interface{}:
			convertJsonLogicMapSymbol(k, (*primitive.M)(&val), deadArea)
		case []interface{}:
			convertJsonLogicASymbol(k, (*primitive.A)(&val), deadArea)
		default:
		}
	}
}

// convertJsonLogicASymbol 转换primitive.A类型数组
func convertJsonLogicASymbol(key string, outValList *primitive.A, deadArea float64) {
	for i, outValue := range *outValList {
		switch val := outValue.(type) {
		case primitive.M:
			convertJsonLogicMapSymbol(key, &val, deadArea)
		case primitive.A:
			convertJsonLogicASymbol(key, &val, deadArea)
		default:
			if key == "<" || key == ">" || key == ">=" || key == "<=" {
				symbolVal, err := GetFloatNumber(val)
				if err != nil {
					return
				}
				switch key {
				case "<", "<=":
					(*outValList)[i] = symbolVal - deadArea
				case ">", ">=":
					(*outValList)[i] = symbolVal + deadArea
				}
				//symbol := JsonLogicSymbol{
				//	Symbol: key,
				//	Value:  val,
				//}
				//*varList = append(*varList, symbol)
				return
			}
		case map[string]interface{}:
			convertJsonLogicMapSymbol(key, (*primitive.M)(&val), deadArea)
		case []interface{}:
			convertJsonLogicASymbol(key, (*primitive.A)(&val), deadArea)
		}

	}
}
