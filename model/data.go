package model

type Cache struct {
	Time  int64  `json:"time"`
	Value string `json:"value"`
}

type DataQueryWebsocket struct {
	TagID  string `json:"tagId"`
	Uid    string `json:"uid"`
	AllTag bool   `json:"allTag"`
}

type DataQueryMapWebsocket struct {
	Tags   []string `json:"tags"`
	AllTag bool     `json:"allTag"`
}

type WarnQueryWebsocket struct {
	ModelID      string `json:"modelId"`
	NodeId       string `json:"nodeId"`
	DepartmentId string `json:"departmentId"`
	Level        string `json:"level"`
}

type LogQueryWebsocket struct {
	Uid   string   `json:"uid"`
	Level []string `json:"level"`
}

type ServiceQueryWebsocket struct {
	All  bool     `json:"all"`
	Uids []string `json:"uids"`
}

type ServiceQueryTypeWebsocket struct {
	SubType string      `json:"subType" example:"inOutStation,illegalInfo,serviceRequest"`
	Filter  interface{} `json:"filter"`
}

type InOutStationSub struct {
	LineCode int `json:"lineCode"`
}

type (
	RealTimeData struct {
		TagId string      `json:"tagId"`
		Uid   string      `json:"uid"`
		Time  int64       `json:"time"`
		Value interface{} `json:"value"`
	}

	QueryData struct {
		Results []Results `json:"results"`
	}

	Series struct {
		Name    string          `json:"name"`
		Columns []string        `json:"columns"`
		Values  [][]interface{} `json:"values"`
	}

	Results struct {
		Series []Series `json:"series"`
	}

	RealTimeDataTemplate struct {
		Data   []RealTimeData `json:"data"`
		Length int            `json:"length"`
		Time   int64          `json:"time"`
	}
)
