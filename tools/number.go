package tools

import (
	"fmt"
	"math/rand"
	"regexp"
	"strconv"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

func IsNumber(data string) bool {
	pattern, _ := regexp.Compile("^-?\\d+(\\.\\d+)?$")
	return pattern.MatchString(data)
}

func GetIntNumberFromMongoDB(data primitive.M, key string) (int, error) {
	number := 0
	numberInDB, ok := data[key].(int32)
	if !ok {
		numberInDBFloat, ok := data[key].(float64)
		if !ok {
			numberInDBInt64, ok := data[key].(int64)
			if !ok {
				numberInDBInt, ok := data[key].(int)
				if !ok {
					return 0, fmt.Errorf("该字段不是数字")
				} else {
					number = int(numberInDBInt)
				}
			} else {
				number = int(numberInDBInt64)
			}
		} else {
			number = int(numberInDBFloat)
		}
	} else {
		number = int(numberInDB)
	}
	return number, nil
}

func GetFloat64NumberFromMongoDB(data primitive.M, key string) (float64, error) {
	number := float64(0)
	numberInDB, ok := data[key].(int32)
	if !ok {
		numberInDBFloat, ok := data[key].(float64)
		if !ok {
			numberInDBInt64, ok := data[key].(int64)
			if !ok {
				return 0, fmt.Errorf("该字段不是数字")
			} else {
				number = float64(numberInDBInt64)
			}
		} else {
			number = float64(numberInDBFloat)
		}
	} else {
		number = float64(numberInDB)
	}
	return number, nil
}

func GetFloatNumber(data interface{}) (float64, error) {
	number := float64(0)
	numberInDBFloat32, ok := data.(float32)
	if !ok {
		numberInDB, ok := data.(int32)
		if !ok {
			numberInDBFloat, ok := data.(float64)
			if !ok {
				numberInDBInt64, ok := data.(int64)
				if !ok {
					numberInDBInt, ok := data.(int)
					if !ok {
						return 0, fmt.Errorf("该字段不是数字")
					} else {
						number = float64(numberInDBInt)
					}
				} else {
					number = float64(numberInDBInt64)
				}
			} else {
				number = float64(numberInDBFloat)
			}
		} else {
			number = float64(numberInDB)
		}
	} else {
		number = float64(numberInDBFloat32)
	}
	return number, nil
}

func GetRandomNumberCode(digit int) string {
	code := ""
	for i := 1; i <= digit; i++ {
		code += strconv.Itoa(rand.Intn(10))
	}
	return code
}
