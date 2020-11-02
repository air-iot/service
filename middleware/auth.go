package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/go-redis/redis/v8"
	"net/http"
	"regexp"
	"strings"

	"github.com/dgrijalva/jwt-go"
	"github.com/dgrijalva/jwt-go/request"
	"github.com/labstack/echo/v4"

	iredis "github.com/air-iot/service/db/redis"
	restfulapi "github.com/air-iot/service/restful-api"
	tokenUtil "github.com/air-iot/service/token"
)

// AuthFilter echo权限过滤中间件
func AuthFilter() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(ctx echo.Context) error {
			req := ctx.Request()

			tokenInUrl := req.URL.Query().Get("token")
			if tokenInUrl != "" {
				req.Header.Set("Authorization", tokenInUrl)
			}

			requestType, ok := req.Header["Request-Type"]
			if ok && len(requestType) == 1 && requestType[0] == "service" {
				if err := next(ctx); err != nil {
					return err
				}
			}
			picFileRegexp, _ := regexp.Compile("(?i:html|htm|gif|jpg|jpeg|bmp|png|ico|txt|js|css|json|map|svg|wav|mp3|mp4)$")
			driverMatch, _ := regexp.MatchString("/driver/driver/(.+)/config", req.URL.Path)
			driverWsMatchQuery, _ := regexp.MatchString("/driver/ws?(.+)", req.URL.Path)
			// 校验是否是公开资源地址
			if req.URL.Path == "/core/auth/login" ||
				req.URL.Path == "/core/users/register" ||
				req.URL.Path == "/core/auth/token" ||
				req.URL.Path == "/core/register/admin" ||
				req.URL.Path == "/core/time" ||
				req.URL.Path == "/core/heart" ||
				strings.HasPrefix(req.URL.Path, "/core/catalog") ||
				strings.HasPrefix(req.URL.Path, "/core/ws") ||
				strings.HasPrefix(req.URL.Path, "/driver/ws") ||
				strings.HasPrefix(req.URL.Path, "/core/auth/phone") ||
				req.URL.Path == "/core/node/import/excel/template" ||
				req.URL.Path == "/core/node/update/excel/template" ||
				req.URL.Path == "/core/license/check" ||
				req.URL.Path == "/core/auth/logout" ||
				req.URL.Path == "/core/user/setting" ||
				req.URL.Path == "/core/auth/user" ||
				strings.HasPrefix(req.URL.Path, "/core/auth/captcha") ||
				req.URL.Path == "/core/setting" ||
				picFileRegexp.MatchString(req.URL.Path) ||
				req.URL.Path == "/core/wechat" ||
				req.URL.Path == "/core/file/js" ||
				driverMatch ||
				driverWsMatchQuery {
				// 公开地址放行
				if err := next(ctx); err != nil {
					return err
				}
				return nil
			}

			//从请求中解析出token
			token, err := request.ParseFromRequest(req, request.AuthorizationHeaderExtractor, func(token *jwt.Token) (interface{}, error) {
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("token method error:%v", token.Header["alg"])
				}
				return tokenUtil.GetHMACKey(), nil
			})

			if err != nil {
				return restfulapi.NewHTTPError(http.StatusUnauthorized, "auth", err.Error())
			}

			//token无效则返回
			if !token.Valid {
				return restfulapi.NewHTTPError(http.StatusUnauthorized, "auth", "权限验证,令牌无效")
			}
			// 从token中获取用户id
			userId := ""
			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				if id, ok := claims["id"]; ok {
					if n, ok := id.(string); ok {
						userId = n
					}
				}
			}
			//根据用户id查询用户权限信息（Redis）
			var cmd *redis.StringCmd
			if iredis.ClusterBool {
				cmd = iredis.ClusterClient.Get(context.Background(), userId)
			} else {
				cmd = iredis.Client.Get(context.Background(), userId)
			}
			if cmd.Err() != nil {
				return restfulapi.NewHTTPError(http.StatusBadRequest, "auth", cmd.Err().Error())
			}
			userInfoString := cmd.Val()
			userInfoMap := &map[string]interface{}{}
			err = json.Unmarshal([]byte(userInfoString), userInfoMap)
			if err != nil {
				return restfulapi.NewHTTPError(http.StatusBadRequest, "auth", err.Error())
			}

			//判断是否是超级管理员（是的话直接跳过权限认证）
			if (*userInfoMap)["isSuper"].(bool) == true || ((*userInfoMap)["isShare"].(bool) == true && req.Method == http.MethodGet) {
				if err := next(ctx); err != nil {
					return err
				}
			}

			if strings.Contains(req.URL.Path, "/core/configDb/") {
				return restfulapi.NewHTTPError(http.StatusForbidden, "auth", "无对应权限")
			}

			// 校验用户访问地址+请求方式是否在用户权限范围内
			firstURIListRaw := strings.Split(req.URL.Path, "/")
			firstURIList := firstURIListRaw[1:]
			if len(firstURIList) < 2 {
				return restfulapi.NewHTTPError(http.StatusBadRequest, "auth", "权限验证,URI无效")
			}
			firstURI := firstURIList[1]

			permissionList := (*userInfoMap)["permissions"].([]interface{})
			//firstURI := firstURIList[1]
			requiredPermissionMap := generatePermissionMap(firstURI)
			uriKey := firstURI + "+" + req.Method
			requiredPermission := requiredPermissionMap[uriKey]
			//判断是否是特殊路径（第二个/后不是id的路径）
			if len(firstURIList) > 2 {
				switch firstURIList[1] {
				case "driver":
					requiredPermissionMap = generateSpecialPermissionMap(firstURIList[1])
					uriKey = firstURIList[1] + "+" + req.Method
					requiredPermission = requiredPermissionMap[uriKey]
				case "warning", "report":
					switch firstURIList[2] {
					case "batch", "all", "stats", "data", "query":
						requiredPermissionMap = generateSpecialPermissionMap(firstURIList[1])
						uriKey = firstURIList[1] + "+" + firstURIList[2]
						requiredPermission = requiredPermissionMap[uriKey]
					}
				case "record":
					requiredPermissionMap = generateSpecialPermissionMap(firstURIList[1])
					uriKey = firstURIList[1] + "+" + req.Method
					requiredPermission = requiredPermissionMap[uriKey]
				case "status", "component", "media", "card", "data":
					requiredPermissionMap = generateSpecialPermissionMap(firstURIList[1])
					uriKey = firstURIList[1] + "+" + req.Method
					requiredPermission = requiredPermissionMap[uriKey]
				case "auth":
					switch firstURIList[2] {
					case "license":
						requiredPermissionMap = generateSpecialPermissionMap(firstURIList[1])
						uriKey = firstURIList[1] + "+" + firstURIList[2]
						requiredPermission = requiredPermissionMap[uriKey]
					case "changeToken":
						requiredPermissionMap = generateSpecialPermissionMap(firstURIList[1])
						uriKey = firstURIList[1] + "+" + req.Method
						requiredPermission = requiredPermissionMap[uriKey]
					}
				}
			} else if len(firstURIList) == 2 {
				switch firstURIList[1] {
				case "component", "media", "card", "data":
					requiredPermissionMap = generateSpecialPermissionMap(firstURIList[1])
					uriKey = firstURIList[1] + "+" + req.Method
					requiredPermission = requiredPermissionMap[uriKey]
				}
			}
			for _, v := range permissionList {
				if requiredPermission == v.(string) {
					//数据权限过滤====
					if err := next(ctx); err != nil {
						return err
					}
				}
			}
			return restfulapi.NewHTTPError(http.StatusForbidden, "auth", "无对应权限")
		}
	}
}

func generatePermissionMap(data string) map[string]string {
	//uriValue := strings.Title(data)
	switch data {
	case "dashboardDraft", "svg", "catalog":
		return map[string]string{
			data + "+" + http.MethodGet:    "dashboard" + ".view",
			data + "+" + http.MethodPost:   "dashboard" + ".add",
			data + "+" + http.MethodPatch:  "dashboard" + ".edit",
			data + "+" + http.MethodPut:    "dashboard" + ".edit",
			data + "+" + http.MethodDelete: "dashboard" + ".delete",
		}
	default:
		return map[string]string{
			data + "+" + http.MethodGet:    data + ".view",
			data + "+" + http.MethodPost:   data + ".add",
			data + "+" + http.MethodPatch:  data + ".edit",
			data + "+" + http.MethodPut:    data + ".edit",
			data + "+" + http.MethodDelete: data + ".delete",
		}
	}
}
func generateSpecialPermissionMap(data string) map[string]string {
	switch data {
	case "driver":
		return map[string]string{
			data + "+" + http.MethodGet:  "model.edit",
			data + "+" + http.MethodPost: "model.edit",
		}
	case "warning":
		return map[string]string{
			data + "+" + "batch": data + ".edit",
			data + "+" + "all":   data + ".edit",
			data + "+" + "stats": data + ".view",
		}
	case "report":
		return map[string]string{
			data + "+" + "data":  data + ".view",
			data + "+" + "query": data + ".view",
		}
	case "record":
		return map[string]string{
			data + "+" + http.MethodGet:    "node.view",
			data + "+" + http.MethodPost:   "node.view",
			data + "+" + http.MethodDelete: "node.view",
			data + "+" + http.MethodPut:    "node.view",
			data + "+" + http.MethodPatch:  "node.view",
		}
	case "status":
		return map[string]string{
			data + "+" + http.MethodGet:    "node.view",
			data + "+" + http.MethodPost:   "node.add",
			data + "+" + http.MethodDelete: "node.delete",
			data + "+" + http.MethodPut:    "node.edit",
			data + "+" + http.MethodPatch:  "node.edit",
		}
	case "auth":
		return map[string]string{
			data + "+" + "license":        "license.view",
			data + "+" + http.MethodGet:   "dashboard.edit",
			data + "+" + http.MethodPost:  "dashboard.edit",
			data + "+" + http.MethodPut:   "dashboard.edit",
			data + "+" + http.MethodPatch: "dashboard.edit",
		}
	case "component":
		return map[string]string{
			data + "+" + http.MethodGet:    "dashboard.edit",
			data + "+" + http.MethodPost:   "dashboard.edit",
			data + "+" + http.MethodDelete: "dashboard.edit",
			data + "+" + http.MethodPut:    "dashboard.edit",
			data + "+" + http.MethodPatch:  "dashboard.edit",
		}
	case "media":
		return map[string]string{
			data + "+" + http.MethodGet:    "model.edit",
			data + "+" + http.MethodPost:   "model.edit",
			data + "+" + http.MethodDelete: "model.edit",
			data + "+" + http.MethodPut:    "model.edit",
			data + "+" + http.MethodPatch:  "model.edit",
		}
	case "card":
		return map[string]string{
			data + "+" + http.MethodGet:    "node.edit",
			data + "+" + http.MethodPost:   "node.edit",
			data + "+" + http.MethodDelete: "node.edit",
			data + "+" + http.MethodPut:    "node.edit",
			data + "+" + http.MethodPatch:  "node.edit",
		}
	case "data":
		return map[string]string{
			data + "+" + http.MethodGet:    "data.view",
			data + "+" + http.MethodPost:   "data.view",
			data + "+" + http.MethodDelete: "data.delete",
			data + "+" + http.MethodPut:    "data.edit",
			data + "+" + http.MethodPatch:  "data.edit",
		}
	default:
		return map[string]string{}
	}
}
