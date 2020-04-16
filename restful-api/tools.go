package restfulapi

import (
	"errors"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// DeepCopy 深度复制
func DeepCopy(value interface{}) interface{} {
	if valueMap, ok := value.(map[string]interface{}); ok {
		newMap := make(map[string]interface{})
		for k, v := range valueMap {
			newMap[k] = DeepCopy(v)
		}
		return newMap
	} else if valueSlice, ok := value.([]interface{}); ok {
		newSlice := make([]interface{}, len(valueSlice))
		for k, v := range valueSlice {
			newSlice[k] = DeepCopy(v)
		}
		return newSlice
	} else if valueMap, ok := value.(primitive.M); ok {
		newMap := make(primitive.M)
		for k, v := range valueMap {
			newMap[k] = DeepCopy(v)
		}
		return newMap
	} else if valueSlice, ok := value.(primitive.A); ok {
		newSlice := make(primitive.A, len(valueSlice))
		for k, v := range valueSlice {
			newSlice[k] = DeepCopy(v)
		}
		return newSlice
	}
	return value
}

// DeepCopyPrimitiveM PrimitiveM深度复制
func DeepCopyPrimitiveM(value interface{}) interface{} {
	if valueMap, ok := value.(primitive.M); ok {
		newMap := make(primitive.M)
		for k, v := range valueMap {
			newMap[k] = DeepCopy(v)
		}
		return newMap
	} else if valueSlice, ok := value.(primitive.A); ok {
		newSlice := make(primitive.A, len(valueSlice))
		for k, v := range valueSlice {
			newSlice[k] = DeepCopy(v)
		}
		return newSlice
	}
	return value
}

// ConvertOID 转换查询的ObjectID
func ConvertOID(key string, value interface{}) (interface{}, error) {
	switch val := value.(type) {
	case map[string]interface{}:
		return ConvertMapOID(key, val)
	case primitive.M:
		return ConvertPrimitiveMapOID(key, val)
	case []interface{}:
		return ConvertInterfacesOID(key, val)
	case primitive.A:
		return ConvertPrimitiveAOID(key, val)
	case string:
		return ConvertStrOID(key, val)
	}
	return value, nil
}

// ConvertSeniorOID 转换查询的ObjectID,需匹配match,如果match匹配转换value为ObjectID
func ConvertSeniorOID(match bool, key string, value interface{}) (interface{}, error) {
	switch val := value.(type) {
	case map[string]interface{}:
		return ConvertMapOID(key, val)
	case primitive.M:
		return ConvertPrimitiveMapOID(key, val)
	case []interface{}:
		return ConvertInterfacesSeniorOID(match, key, val)
	case primitive.A:
		return ConvertPrimitiveASeniorOID(match, key, val)
	case string:
		return ConvertStrSeniorOID(match, key, val)
	}
	return value, nil
}

// ConvertStrOID 转换字符串类型
func ConvertStrOID(key string, val string) (interface{}, error) {
	if strings.HasSuffix(key, "Id") || key == "_id" {
		id, err := primitive.ObjectIDFromHex(val)
		if err != nil {
			return nil, err
		}
		return id, nil
	}
	return val, nil
}

// ConvertStrSeniorOID 转换字符串类型,需匹配match,如果match匹配转换value为ObjectID
func ConvertStrSeniorOID(match bool, key string, val string) (interface{}, error) {
	if strings.HasSuffix(key, "Id") || match || key == "_id" {
		id, err := primitive.ObjectIDFromHex(val)
		if err != nil {
			return nil, err
		}
		return id, nil
	}
	return val, nil
}

// ConvertStrsOID 转换字符串数组类型
func ConvertStrsOID(key string, val []string) (interface{}, error) {
	if strings.HasSuffix(key, "Id") || key == "_id" {
		result := make([]interface{}, 0)
		for _, v := range val {
			if id, err := primitive.ObjectIDFromHex(v); err == nil {
				result = append(result, id)
			}
		}
		if len(val) != len(result) {
			return nil, errors.New("数组长度不一致")
		}
		return result, nil
	}
	return val, nil
}

// ConvertStrsSeniorOID 转换字符串数组类型,需匹配match,如果match匹配转换value为ObjectID
func ConvertStrsSeniorOID(match bool, key string, val []string) (interface{}, error) {
	if strings.HasSuffix(key, "Id") || match || key == "_id" {
		result := make([]interface{}, 0)
		for _, v := range val {
			if id, err := primitive.ObjectIDFromHex(v); err == nil {
				result = append(result, id)
			}
		}
		if len(val) != len(result) {
			return nil, errors.New("数组长度不一致")
		}
		return result, nil
	}
	return val, nil
}

// ConvertInterfacesOID 转换interface类型数组
func ConvertInterfacesOID(key string, val []interface{}) ([]interface{}, error) {
	result := make([]interface{}, 0)
	for _, value := range val {
		r, err := ConvertOID(key, value)
		if err == nil {
			result = append(result, r)
		}
	}
	if len(result) != len(val) {
		return nil, errors.New("数组长度不一致")
	}
	return result, nil
}

// ConvertPrimitiveAOID 转换interface类型数组
func ConvertPrimitiveAOID(key string, val primitive.A) (primitive.A, error) {
	result := make(primitive.A, 0)
	for _, value := range val {
		r, err := ConvertOID(key, value)
		if err == nil {
			result = append(result, r)
		}
	}
	if len(result) != len(val) {
		return nil, errors.New("数组长度不一致")
	}
	return result, nil
}

// ConvertInterfacesSeniorOID 转换interface类型数组,需匹配match,如果match匹配转换value为ObjectID
func ConvertInterfacesSeniorOID(match bool, key string, val []interface{}) ([]interface{}, error) {
	result := make([]interface{}, 0)
	for _, value := range val {
		r, err := ConvertSeniorOID(match, key, value)
		if err == nil {
			result = append(result, r)
		}
	}
	if len(result) != len(val) {
		return nil, errors.New("数组长度不一致")
	}
	return result, nil
}

// ConvertPrimitiveASeniorOID 转换interface类型数组,需匹配match,如果match匹配转换value为ObjectID
func ConvertPrimitiveASeniorOID(match bool, key string, val primitive.A) (primitive.A, error) {
	result := make(primitive.A, 0)
	for _, value := range val {
		r, err := ConvertSeniorOID(match, key, value)
		if err == nil {
			result = append(result, r)
		}
	}
	if len(result) != len(val) {
		return nil, errors.New("数组长度不一致")
	}
	return result, nil
}

// ConvertMapOID 转换键值对类型
func ConvertMapOID(key string, value map[string]interface{}) (map[string]interface{}, error) {

	for k, v := range value {
		//if k == "$group"{
		//	continue
		//}
		if strings.HasPrefix(k, "$") && (strings.HasSuffix(key, "Id") || key == "_id") {
			switch val := v.(type) {
			case map[string]interface{}:
				r, err := ConvertMapOID(k, val)
				if err != nil {
					return nil, err
				}
				value[k] = r
			case []interface{}:
				r, err := ConvertInterfacesSeniorOID(true, k, val)
				if err != nil {
					return nil, err
				}
				value[k] = r
			case string:
				id, err := primitive.ObjectIDFromHex(val)
				if err != nil {
					value[k] = val
				} else {
					value[k] = id
				}
			case []string:

			case interface{}:
				r, err := ConvertSeniorOID(true, k, val)
				if err != nil {
					return nil, err
				}
				value[k] = r
			}
		} else {
			switch val := v.(type) {
			case map[string]interface{}:
				r, err := ConvertMapOID(k, val)
				if err != nil {
					return nil, err
				}
				value[k] = r
			case []interface{}:
				r, err := ConvertInterfacesOID(k, val)
				if err != nil {
					return nil, err
				}
				value[k] = r
			case string:
				r, err := ConvertStrOID(k, val)
				if err != nil {
					return nil, err
				}
				if strings.HasSuffix(k, "Id") && k != "requestId" {
					delete(value, k)
					k = k[:len(k)-2]
					k = k + "._id"
				}
				value[k] = r
			case []string:
				r, err := ConvertStrsOID(k, val)
				if err != nil {
					return nil, err
				}
				value[k] = r
			case interface{}:
				r, err := ConvertOID(k, val)
				if err != nil {
					return nil, err
				}
				value[k] = r
			}
		}
	}
	return value, nil
}

// ConvertPrimitiveMapOID 转换为primitive.M
func ConvertPrimitiveMapOID(key string, value primitive.M) (primitive.M, error) {
	for k, v := range value {
		//if k == "$group"{
		//	continue
		//}
		if strings.HasPrefix(k, "$") && (strings.HasSuffix(key, "Id") || key == "_id") {
			switch val := v.(type) {
			case map[string]interface{}:
				r, err := ConvertMapOID(k, val)
				if err != nil {
					return nil, err
				}
				value[k] = r
			case primitive.M:
				r, err := ConvertPrimitiveMapOID(k, val)
				if err != nil {
					return nil, err
				}
				value[k] = r
			case []interface{}:
				r, err := ConvertInterfacesSeniorOID(true, k, val)
				if err != nil {
					return nil, err
				}
				value[k] = r
			case primitive.A:
				r, err := ConvertInterfacesSeniorOID(true, k, val)
				if err != nil {
					return nil, err
				}
				value[k] = r
			case string:
				id, err := primitive.ObjectIDFromHex(val)
				if err != nil {
					value[k] = val
				} else {
					value[k] = id
				}
			case []string:

			case interface{}:
				r, err := ConvertSeniorOID(true, k, val)
				if err != nil {
					return nil, err
				}
				value[k] = r
			}
		} else {
			switch val := v.(type) {
			case map[string]interface{}:
				r, err := ConvertMapOID(k, val)
				if err != nil {
					return nil, err
				}
				value[k] = r
			case primitive.M:
				r, err := ConvertPrimitiveMapOID(k, val)
				if err != nil {
					return nil, err
				}
				if strings.HasSuffix(k, "Id") && k != "requestId" {
					delete(value, k)
					k = k[:len(k)-2]
					k = k + "._id"
					if emptyObject, ok := v.(primitive.M); ok {
						flag := false
						for range emptyObject {
							flag = true
						}
						if !flag {
							continue
						}
					}
				}
				value[k] = r
			case []interface{}:
				r, err := ConvertInterfacesOID(k, val)
				if err != nil {
					return nil, err
				}
				value[k] = r
			case primitive.A:
				r, err := ConvertPrimitiveAOID(k, val)
				if err != nil {
					return nil, err
				}
				value[k] = r
			case string:
				r, err := ConvertStrOID(k, val)
				if err != nil {
					return nil, err
				}
				if strings.HasSuffix(k, "Id") && k != "requestId" {
					delete(value, k)
					k = k[:len(k)-2]
					k = k + "._id"
				}
				value[k] = r
			case []string:
				r, err := ConvertStrsOID(k, val)
				if err != nil {
					return nil, err
				}
				value[k] = r
			case interface{}:
				r, err := ConvertOID(k, val)
				if err != nil {
					return nil, err
				}
				value[k] = r
			}
		}
	}
	return value, nil
}

// ConvertKeyID 转换查询的ObjectID
func ConvertKeyID(data *bson.M) error {
	for k, v := range *data {
		switch val := v.(type) {
		case primitive.M:
			value, err := ConvertPrimitiveMapToID(data, k, val)
			if err != nil {
				return err
			}
			(*data)[k] = value
		case primitive.A:
			value, err := ConvertPrimitiveAToID(data, k, val)
			if err != nil {
				return err
			}
			(*data)[k] = value
		case primitive.ObjectID:
			value, err := ConvertObjectIDToID(data, k, val)
			if err != nil {
				return err
			}
			if value != nil {
				delete(*data, k)
				(*data)["id"] = value
			}
		default:
			if k == "_id"{
				(*data)["id"] = val
				delete(*data,k)
			}
		}
	}

	return nil
}

// ConvertObjectIDToID 转换字符串类型
func ConvertObjectIDToID(data *bson.M, key string, val primitive.ObjectID) (interface{}, error) {
	if key == "_id" {
		return val, nil
	}
	return nil, nil
}

// ConvertPrimitiveAToID 转换interface类型数组
func ConvertPrimitiveAToID(data *bson.M, key string, val primitive.A) (interface{}, error) {
	result := make(primitive.A, 0)
	for _, outValue := range val {
		//r, err := p.recursionFormatToID(&value)
		//if err == nil {
		//	result = append(result, r)
		//}
		switch val := outValue.(type) {
		case primitive.M:
			value, err := ConvertPrimitiveMapToID(data, key, val)
			if err != nil {
				return nil, nil
			}
			result = append(result, value)
			//(*data)[k] = value
		case primitive.A:
			value, err := ConvertPrimitiveAToID(data, key, val)
			if err != nil {
				return nil, nil
			}
			result = append(result, value)
			//(*data)[k] = value
		case primitive.ObjectID:
			//value,err := p.convertObjectIDToID(data, key, val)
			//if err != nil {
			//	return nil, nil
			//}
			//if value != nil{
			//	delete(val,key)
			//	outValue["id"] = value
			//}
			result = append(result, val)
		default:
			result = append(result, val)
		}

	}
	//if len(result) != len(val) {
	//	return nil, errors.New("数组长度不一致")
	//}
	return result, nil
}

// ConvertPrimitiveMapToID 转换为primitive.M
func ConvertPrimitiveMapToID(data *bson.M, key string, value primitive.M) (interface{}, error) {
	for k, v := range value {
		switch val := v.(type) {
		case primitive.M:
			r, err := ConvertPrimitiveMapToID(data, k, val)
			if err != nil {
				return nil, err
			}
			value[k] = r
		case primitive.A:
			r, err := ConvertPrimitiveAToID(data, k, val)
			if err != nil {
				return nil, err
			}
			value[k] = r
		case primitive.ObjectID:
			r, err := ConvertObjectIDToID(data, k, val)
			if err != nil {
				return nil, err
			}
			if r != nil {
				delete(value, k)
				value["id"] = r
			} else {
				value[k] = val
			}
		default:
			value[k] = val
			//case []string:
			//	r, err := p.convertStrs(k, val)
			//	if err != nil {
			//		return nil, err
			//	}
			//	value[k] = r
		}
	}
	return value, nil
}

//// FormatObjectIDList 格式化对象数组为string数组（转换的字段为ObjectID类型或string类型）
//func FormatObjectIDList(doc *bson.M, key string, formatKey string) error {
//	if _, ok := (*doc)[key]; !ok {
//		//(*doc)[key] = make([]string, 0)
//		return nil
//	}
//	interfaceList, ok := (*doc)[key].([]interface{})
//	if !ok {
//		interfaceObject, objOk := (*doc)[key].(interface{})
//		if !objOk {
//			return fmt.Errorf("%s的类型不是interface{}或[]interface{}", key)
//		} else {
//			if emptyObject, ok := interfaceObject.(map[string]interface{}); ok {
//				flag := false
//				for range emptyObject {
//					flag = true
//				}
//				if !flag {
//					return nil
//				}
//			}
//			if _, ok := interfaceObject.([]primitive.ObjectID); ok {
//				(*doc)[key] = interfaceObject
//				return nil
//			}
//			stringID := ""
//			if ele, ok := interfaceObject.(primitive.ObjectID); ok {
//				stringID = ele.Hex()
//			} else if ele, ok := interfaceObject.(string); ok {
//				stringID = ele
//			} else if _, ok := interfaceObject.(map[string]interface{})[formatKey]; !ok {
//				return fmt.Errorf("%s需要格式化的Key:%s，不存在", key, formatKey)
//			} else if ele, ok := interfaceObject.(map[string]interface{})[formatKey].(string); ok {
//				stringID = ele
//			} else if ele, ok := interfaceObject.(map[string]interface{})[formatKey].(primitive.ObjectID); ok {
//				stringID = ele.Hex()
//			} else {
//				return fmt.Errorf("%s需要格式化的Key:%s，数值格式错误", key, formatKey)
//			}
//			oid, err := primitive.ObjectIDFromHex(stringID)
//			if err != nil {
//				return err
//			}
//			(*doc)[key] = oid
//		}
//		//return fmt.Errorf("%s的类型不是[]interface{}", key)
//	} else {
//		stringList := make([]string, 0)
//		for _, v := range interfaceList {
//			if ele, ok := v.(primitive.ObjectID); ok {
//				stringList = append(stringList, ele.Hex())
//			} else if ele, ok := v.(string); ok {
//				stringList = append(stringList, ele)
//			} else if _, ok := v.(map[string]interface{})[formatKey]; !ok {
//				return fmt.Errorf("%s需要格式化的Key:%s，不存在", key, formatKey)
//			} else if ele, ok := v.(map[string]interface{})[formatKey].(string); ok {
//				stringList = append(stringList, ele)
//			} else if ele, ok := v.(map[string]interface{})[formatKey].(primitive.ObjectID); ok {
//				stringList = append(stringList, ele.Hex())
//			} else {
//				return fmt.Errorf("需要格式化的Key:%s，数值格式错误", formatKey)
//			}
//		}
//		oidList, err := util.StringListToObjectIdList(stringList)
//		if err != nil {
//			return err
//		}
//		(*doc)[key] = oidList
//	}
//	return nil
//}
//
//// FormatObjectIDListMap 格式化对象数组为string数组（转换的字段为ObjectID类型或string类型）(doc为map类型)
//func FormatObjectIDListMap(doc *map[string]interface{}, key string, formatKey string) error {
//	if _, ok := (*doc)[key]; !ok {
//		//(*doc)[key] = make([]string, 0)
//		return nil
//	}
//	interfaceList, ok := (*doc)[key].([]interface{})
//	if !ok {
//		interfaceObject, objOk := (*doc)[key].(interface{})
//		if !objOk {
//			return fmt.Errorf("%s的类型不是interface{}或[]interface{}", key)
//		} else {
//			if emptyObject, ok := interfaceObject.(map[string]interface{}); ok {
//				flag := false
//				for range emptyObject {
//					flag = true
//				}
//				if !flag {
//					return nil
//				}
//			}
//			if _, ok := interfaceObject.([]primitive.ObjectID); ok {
//				(*doc)[key] = interfaceObject
//				return nil
//			}
//			stringID := ""
//			if ele, ok := interfaceObject.(primitive.ObjectID); ok {
//				stringID = ele.Hex()
//			} else if ele, ok := interfaceObject.(string); ok {
//				stringID = ele
//			} else if _, ok := interfaceObject.(map[string]interface{})[formatKey]; !ok {
//				return fmt.Errorf("需要格式化的Key:%s，不存在", formatKey)
//			} else if ele, ok := interfaceObject.(map[string]interface{})[formatKey].(string); ok {
//				stringID = ele
//			} else if ele, ok := interfaceObject.(map[string]interface{})[formatKey].(primitive.ObjectID); ok {
//				stringID = ele.Hex()
//			} else {
//				return fmt.Errorf("需要格式化的Key:%s，数值格式错误", formatKey)
//			}
//			oid, err := primitive.ObjectIDFromHex(stringID)
//			if err != nil {
//				return err
//			}
//			(*doc)[key] = oid
//		}
//		//return fmt.Errorf("%s的类型不是[]interface{}", key)
//	} else {
//		stringList := make([]string, 0)
//		for _, v := range interfaceList {
//			if ele, ok := v.(primitive.ObjectID); ok {
//				stringList = append(stringList, ele.Hex())
//			} else if ele, ok := v.(string); ok {
//				stringList = append(stringList, ele)
//			} else if _, ok := v.(map[string]interface{})[formatKey]; !ok {
//				return fmt.Errorf("需要格式化的Key:%s，不存在", formatKey)
//			} else if ele, ok := v.(map[string]interface{})[formatKey].(string); ok {
//				stringList = append(stringList, ele)
//			} else if ele, ok := v.(map[string]interface{})[formatKey].(primitive.ObjectID); ok {
//				stringList = append(stringList, ele.Hex())
//			} else {
//				return fmt.Errorf("需要格式化的Key:%s，数值格式错误", formatKey)
//			}
//		}
//		oidList, err := util.StringListToObjectIdList(stringList)
//		if err != nil {
//			return err
//		}
//		(*doc)[key] = oidList
//	}
//	return nil
//}
//
//// FormatObjectID 格式化对象为string（转换的字段为ObjectID类型或string类型）
//func FormatObjectID(doc *bson.M, key string, formatKey string) error {
//	if _, ok := (*doc)[key]; !ok {
//		//(*doc)[key] = make([]string, 0)
//		return nil
//	}
//	interfaceObject, ok := (*doc)[key].(interface{})
//	if !ok {
//		return fmt.Errorf("%s的类型不是interface{}", key)
//	}
//	if emptyObject, ok := interfaceObject.(map[string]interface{}); ok {
//		flag := false
//		for range emptyObject {
//			flag = true
//		}
//		if !flag {
//			return nil
//		}
//	}
//	if _, ok := interfaceObject.([]primitive.ObjectID); ok {
//		(*doc)[key] = interfaceObject
//		return nil
//	}
//	stringID := ""
//	if ele, ok := interfaceObject.(primitive.ObjectID); ok {
//		stringID = ele.Hex()
//	} else if ele, ok := interfaceObject.(string); ok {
//		stringID = ele
//	} else if _, ok := interfaceObject.(map[string]interface{})[formatKey]; !ok {
//		return fmt.Errorf("需要格式化的Key:%s，不存在", formatKey)
//	} else if ele, ok := interfaceObject.(map[string]interface{})[formatKey].(string); ok {
//		stringID = ele
//	} else if ele, ok := interfaceObject.(map[string]interface{})[formatKey].(primitive.ObjectID); ok {
//		stringID = ele.Hex()
//	} else {
//		return fmt.Errorf("需要格式化的Key:%s，数值格式错误", formatKey)
//	}
//	oid, err := primitive.ObjectIDFromHex(stringID)
//	if err != nil {
//		return err
//	}
//	(*doc)[key] = oid
//	return nil
//}
//
//// GetFormatObjectIDList 格式化对象数组为string数组（转换的字段为ObjectID类型或string类型）(不改变原对象)（处理bson.M）
//func GetFormatObjectIDList(doc *bson.M, key string, formatKey string) ([]string, error) {
//	stringList := make([]string, 0)
//	if _, ok := (*doc)[key]; !ok {
//		//(*doc)[key] = make([]string, 0)
//		return stringList, nil
//	}
//	interfaceList, ok := (*doc)[key].(primitive.A)
//	if !ok {
//		return nil, fmt.Errorf("%s的类型不是primitive.A", key)
//	}
//	for _, v := range interfaceList {
//		if ele, ok := v.(primitive.ObjectID); ok {
//			stringList = append(stringList, ele.Hex())
//		} else if ele, ok := v.(string); ok {
//			stringList = append(stringList, ele)
//		} else if _, ok := v.(bson.M)[formatKey]; !ok {
//			return nil, fmt.Errorf("需要格式化的Key:%s，不存在", formatKey)
//		} else if ele, ok := v.(bson.M)[formatKey].(string); ok {
//			stringList = append(stringList, ele)
//		} else if ele, ok := v.(bson.M)[formatKey].(primitive.ObjectID); ok {
//			stringList = append(stringList, ele.Hex())
//		} else {
//			return nil, fmt.Errorf("需要格式化的Key:%s，数值格式错误", formatKey)
//		}
//	}
//	return stringList, nil
//}
//
//// GetUserInfo 获取缓存的用户数据.
//func GetUserInfo(echoContext echo.Context) (*map[string]interface{}, error) {
//	//从请求中解析出token
//	token, err := request.ParseFromRequest(echoContext.Request(), request.AuthorizationHeaderExtractor, func(token *jwt.Token) (interface{}, error) {
//		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
//			return nil, fmt.Errorf("token method error:%v", token.Header["alg"])
//		}
//		return tokenUtil.GetHMACKey(), nil
//	})
//
//	if err != nil {
//		return nil, err
//	}
//
//	//token无效则返回
//	if !token.Valid {
//		return nil, fmt.Errorf("权限验证,令牌无效")
//	}
//
//	// 从token中获取用户id
//	userId := ""
//	if claims, ok := token.Claims.(jwt.MapClaims); ok {
//		if id, ok := claims["id"]; ok {
//			if n, ok := id.(string); ok {
//				//logrus.Info(global.NewLOG(n, req.Request.Method, req.Request.URL.Path, ip, global.SUCCESS).ToString())
//				userId = n
//			}
//		}
//	}
//
//	//根据用户id查询用户权限信息（Redis）
//	cmd := global.App.RedisClient.Get(userId)
//	if cmd.Err() != nil {
//		return nil, cmd.Err()
//	}
//	userInfoString := cmd.Val()
//	//userInfoString, err := FindByRedisKey(userId, ctx)
//	//if err != nil {
//	//	return nil, err
//	//}
//
//	userInfoMap := &map[string]interface{}{}
//
//	err = json.Unmarshal([]byte(userInfoString), userInfoMap)
//	if err != nil {
//		return nil, err
//	}
//	return userInfoMap, nil
//}

// NewResponseMsg 创建求响应消息
func NewResponseMsg(msg interface{}) map[string]interface{} {
	return map[string]interface{}{"name": msg}
}
