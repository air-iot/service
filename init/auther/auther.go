package auther

import (
	"github.com/dgrijalva/jwt-go"

	"github.com/air-iot/service/config"
	"github.com/air-iot/service/init/redisdb"
	"github.com/air-iot/service/pkg/auth"
	"github.com/air-iot/service/pkg/auth/jwtauth"
	"github.com/air-iot/service/pkg/auth/jwtauth/redisx"
)

// NewAuth 创建auth
func NewAuth(db redisdb.Client, cfg config.JWTAuth) auth.Auther {

	var opts []jwtauth.Option
	opts = append(opts, jwtauth.SetExpired(cfg.Expired))
	opts = append(opts, jwtauth.SetSigningKey([]byte(cfg.SigningKey)))
	opts = append(opts, jwtauth.SetKeyfunc(func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, auth.ErrInvalidToken
		}
		return []byte(cfg.SigningKey), nil
	}))

	var method jwt.SigningMethod
	switch cfg.SigningMethod {
	case "HS256":
		method = jwt.SigningMethodHS256
	case "HS384":
		method = jwt.SigningMethodHS384
	default:
		method = jwt.SigningMethodHS512
	}
	opts = append(opts, jwtauth.SetSigningMethod(method))

	var store jwtauth.Storer = redisx.NewStore(db, cfg.RedisPrefix)

	auth := jwtauth.New(store, opts...)
	return auth
}
