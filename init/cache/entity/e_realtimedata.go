package entity

// MQRealTimeData 消息队列传输数据
type MQRealTimeData struct {
	ID     string                 `json:"id"`
	Time   *int64                 `json:"time"`
	Fields map[string]interface{} `json:"fields"`
}
