package tools

import (
	"fmt"
	"golang.org/x/net/websocket"
)

func WebsocketResponse(ws *websocket.Conn, requestId string, statusCode int, data interface{}) error {
	sendMap := map[string]interface{}{
		//"requestId": requestId,
		"code": statusCode,
		"data": data,
	}
	sendByte, err := json.Marshal(sendMap)
	if err != nil {
		//return fmt.Errorf("ID为(%s)序列化要发送的数据失败:%s",requestId, err.Error())
		return fmt.Errorf("序列化要发送的数据失败:%s", err.Error())

	}
	err = websocket.Message.Send(ws, string(sendByte))
	if err != nil {
		//return fmt.Errorf("ID为(%s)发送Websocket正常响应消息失败:%s", requestId, err.Error())
		return fmt.Errorf("发送Websocket正常响应消息失败:%s", err.Error())
	}
	return nil
}

func WebsocketErrorResponse(ws *websocket.Conn, requestId string, statusCode int, name string, msg interface{}) error {
	sendMap := map[string]interface{}{
		//"requestId": requestId,
		"code":    statusCode,
		"error":   name,
		"message": msg,
	}
	sendByte, err := json.Marshal(sendMap)
	if err != nil {
		//return fmt.Errorf("ID为(%s)序列化要发送的错误消息失败:%s", requestId,err.Error())
		return fmt.Errorf("序列化要发送的错误消息失败:%s", err.Error())
	}
	err = websocket.Message.Send(ws, string(sendByte))
	if err != nil {
		//return fmt.Errorf("ID为(%s)发送Websocket报错消息失败:%s", requestId, err.Error())
		return fmt.Errorf("发送Websocket报错消息失败:%s", err.Error())
	}
	return nil
}
