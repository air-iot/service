package token

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/dgrijalva/jwt-go/request"
	"github.com/go-redis/redis/v8"
	"github.com/labstack/echo/v4"

	iredis "github.com/air-iot/service/db/redis"
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
func GetUserInfo(echoContext echo.Context) (*map[string]interface{}, error) {
	//从请求中解析出token
	token, err := request.ParseFromRequest(echoContext.Request(), request.AuthorizationHeaderExtractor, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("token method error:%v", token.Header["alg"])
		}
		return GetHMACKey(), nil
	})

	if err != nil {
		return nil, err
	}

	//token无效则返回
	if !token.Valid {
		return nil, fmt.Errorf("权限验证,令牌无效")
	}

	// 从token中获取用户id
	userId := ""
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		if id, ok := claims["id"]; ok {
			if n, ok := id.(string); ok {
				//logrus.Info(global.NewLOG(n, req.Request.Method, req.Request.URL.Path, ip, global.SUCCESS).ToString())
				userId = n
			}
		}
	}

	var cmd *redis.StringCmd
	//根据用户id查询用户权限信息（Redis）
	if iredis.ClusterBool {
		cmd = iredis.ClusterClient.Get(context.Background(), userId)
	} else {
		cmd = iredis.Client.Get(context.Background(), userId)
	}
	if cmd.Err() != nil {
		return nil, cmd.Err()
	}
	userInfoString := cmd.Val()
	//userInfoString, err := FindByRedisKey(userId, ctx)
	//if err != nil {
	//	return nil, err
	//}

	userInfoMap := &map[string]interface{}{}

	err = json.Unmarshal([]byte(userInfoString), userInfoMap)
	if err != nil {
		return nil, err
	}
	return userInfoMap, nil
}
