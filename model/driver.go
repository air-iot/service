package model

type (
	// Driver 定义驱动结构体
	Driver struct {
		ID       string   `json:"id"`
		Name     string   `json:"name"`
		Services []string `json:"services"`
	}
	// DriverConfigModel ...
	DriverConfigModel struct {
		ID      string    `json:"id"`
		Device  Device    `json:"device"`
		Devices []Devices `json:"devices"`
	}

	// Devices ...
	Devices struct {
		ID     string `json:"id"`
		UID    string `json:"uid"`
		Device Device `json:"device"`
	}

	Command struct {
		Name    string                 `json:"name"`
		NodeID  string                 `json:"nodeId"`
		ModelID string                 `json:"modelId"`
		Type    string                 `json:"type"`
		Params  map[string]interface{} `json:"params"`
	}

	Message struct {
		Message string `json:"message"`
	}

	ResMsg struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}

	ResMsgInterface struct {
		Code    int         `json:"code"`
		Message interface{} `json:"message"`
	}
)
