package model

import "go.mongodb.org/mongo-driver/mongo"

type (
	// GlobalModel 通用
	GlobalModel map[string]interface{}
	// InsertOneResult 插入结果
	InsertOneResult mongo.InsertOneResult
	// DeleteResult 删除结果
	DeleteResult mongo.DeleteResult
	// UpdateResult 更新结果
	UpdateResult mongo.UpdateResult

	DataMessage struct {
		ModelID  string                    `json:"modelId"`
		NodeID   string                    `json:"nodeId"`
		Fields   map[string]interface{}    `json:"fields"`
		Uid      string                    `json:"uid"`
		Time     int64                     `json:"time"`
		InputMap map[string]interface{}    `json:"inputMap"`
		Custom   map[string]interface{}    `json:"custom"`
		Mapping  map[string]MappingSetting `json:"mapping"`
	}

	MappingSetting struct {
		ModelID string `json:"modelId"`
		NodeID  string `json:"nodeId"`
		TagID   string `json:"tagID"`
		Uid     string `json:"uid"`
	}

	DataMessageCustom struct {
		ModelID  string                 `json:"modelId"`
		NodeID   string                 `json:"nodeId"`
		Fields   map[string]interface{} `json:"fields"`
		Uid      string                 `json:"uid"`
		Time     int64                  `json:"time"`
		InputMap map[string]interface{} `json:"inputMap"`
		Custom   map[string]interface{} `json:"custom"`
	}

	DataMessageWebsocket struct {
		ModelID string                 `json:"modelId"`
		NodeID  string                 `json:"nodeId"`
		Fields  map[string]interface{} `json:"fields"`
		Uid     string                 `json:"uid"`
		Time    interface{}            `json:"time"`
	}

	DataMessageWithInteger struct {
		ModelID  string                 `json:"modelId"`
		NodeID   string                 `json:"nodeId"`
		Fields   map[string]int         `json:"fields"`
		Uid      string                 `json:"uid"`
		InputMap map[string]interface{} `json:"inputMap"`
	}

	DataMessageWithInterface struct {
		ModelID string      `json:"modelId"`
		NodeID  string      `json:"nodeId"`
		Fields  interface{} `json:"fields"`
		Uid     string      `json:"uid"`
		MsgType string      `json:"msgType"`
	}

	DataMessageWithWarnInterface struct {
		ModelID string      `json:"modelId"`
		NodeID  string      `json:"nodeId"`
		Fields  interface{} `json:"fields"`
		Uid     string      `json:"uid"`
		MsgType string      `json:"msgType"`
		WarnID  string      `json:"warnId"`
	}
)
