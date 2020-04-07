package restfulapi

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// HTTPError 自定义返回错误
type HTTPError struct {
	Code    int
	Key     string `json:"error"`
	Message string `json:"message"`
}

// NewHTTPError 创建自定义返回错误
func NewHTTPError(code int, key string, msg string) *HTTPError {
	return &HTTPError{
		Code:    code,
		Key:     key,
		Message: msg,
	}
}

// Error makes it compatible with `error` interface.
func (e *HTTPError) Error() string {
	return e.Key + ": " + e.Message
}

// HTTPErrorHandler customize echo's HTTP error handler.
func HTTPErrorHandler(err error, c echo.Context) {
	var (
		code = http.StatusBadRequest
		key  = "_other"
		msg  interface{}
	)

	if he, ok := err.(*HTTPError); ok {
		code = he.Code
		if he.Key != "" {
			key = he.Key
		}
		msg = he.Message
	} else if he, ok := err.(*echo.HTTPError); ok {
		code = he.Code
		msg = he.Message
	} else {
		msg = http.StatusText(code)
	}

	if !c.Response().Committed {
		if c.Request().Method == echo.HEAD {
			err := c.NoContent(code)
			if err != nil {
				c.Logger().Error(err)
			}
		} else {
			err := c.JSON(code, map[string]interface{}{key: msg})
			if err != nil {
				c.Logger().Error(err)
			}
		}
	}
}
