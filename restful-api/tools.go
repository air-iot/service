package restfulapi

import (
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

// ConvertKeyID 转换查询的ObjectID
func ConvertKeyID(data *bson.M) error {
	for k, v := range *data {
		switch val := v.(type) {
		case primitive.M:
			if k == "_id"{
				for keyIn,valIn := range val{
					(*data)[keyIn] = valIn
				}
				delete(*data,k)
				continue
			}
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
		case string:
			if k == "_id"{
				delete(*data, k)
				(*data)["id"] = val
			}
		//default:
		//	if k == "_id"{
		//		(*data)["id"] = val
		//		delete(*data,k)
		//	}
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
		case primitive.ObjectID,string:
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
		case string:
			if k == "_id"{
				delete(*data, k)
				(*data)["id"] = val
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

// NewResponseMsg 创建求响应消息
func NewResponseMsg(msg interface{}) map[string]interface{} {
	return map[string]interface{}{"name": msg}
}
