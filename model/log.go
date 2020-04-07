package model

type DriverLog struct {
	Message interface{} `json:"message"`
	Uid     string      `json:"uid"`
	Time    string      `json:"time"`
	Level   string      `json:"level"`
}
