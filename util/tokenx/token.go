package tokenx

import (
	"context"
	"fmt"
	"time"

	"github.com/air-iot/service/util/json"
	"github.com/dgrijalva/jwt-go"
	"github.com/go-redis/redis/v8"
)

const key = "iot_jwt_hmac_key"

// CreateToken 创建令牌
func CreateToken(id interface{}) (string, error) {

	token := jwt.New(jwt.SigningMethodHS256)

	claims := make(jwt.MapClaims)
	claims["id"] = id
	claims["iat"] = time.Now().Unix()
	claims["exp"] = time.Now().Add(time.Hour * 365 * 24).Unix()

	token.Claims = claims

	tokenStr, err := token.SignedString([]byte(key))
	if err != nil {
		return "", fmt.Errorf("CreateTokenError:%s", err)
	}

	return tokenStr, nil
}

// CreateOtherAppToken 创建令牌
func CreateOtherAppToken(systemName interface{}) (string, error) {

	token := jwt.New(jwt.SigningMethodHS256)

	claims := make(jwt.MapClaims)
	claims["system"] = systemName
	claims["iat"] = time.Now().Unix()
	claims["exp"] = time.Now().Add(time.Hour * 365 * 24).Unix()

	token.Claims = claims

	tokenStr, err := token.SignedString([]byte(key))
	if err != nil {
		return "", fmt.Errorf("CreateTokenError:%s", err)
	}

	return tokenStr, nil
}

// GetHMACKey 获取key
func GetHMACKey() []byte {
	return []byte(key)
}

// GetUserInfo 获取缓存的用户数据.
func GetUserInfo(ctx context.Context, redisDB *redis.Client, projectName, userID string) (*map[string]interface{}, error) {
	result, err := redisDB.HGet(ctx, fmt.Sprintf("%s/login", projectName), userID).Result()
	if err != nil {
		return nil, err
	}
	userInfo := &map[string]interface{}{}

	err = json.Unmarshal([]byte(result), userInfo)
	if err != nil {
		return nil, err
	}
	return userInfo, nil
}
