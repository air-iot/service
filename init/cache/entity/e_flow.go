package entity

import "go.mongodb.org/mongo-driver/bson/primitive"

// Flow
type Flow struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Owner    string                 `json:"owner"`
	Settings map[string]interface{} `json:"settings"`
	Type     string                 `json:"type"`
	FlowJson string                 `json:"flowJson"`
	FlowXml  string                 `json:"flowXml"`

	Invalid   bool   `json:"invalid"`
	Disable   bool   `json:"disable"`
	Range     string `json:"range"`
	ValidTime string `json:"validTime"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
}

// FlowMongo
type FlowMongo struct {
	ID       string      `json:"id" bson:"_id"`
	Name     string      `json:"name" bson:"name"`
	Owner    string      `json:"owner" bson:"owner"`
	Settings primitive.M `json:"settings" bson:"settings"`
	Type     string      `json:"type" bson:"type"`
	FlowJson string      `json:"flowJson" bson:"flowJson"`
	FlowXml  string      `json:"flowXml" bson:"flowXml"`

	Invalid   bool               `json:"invalid" bson:"invalid"`
	Disable   bool               `json:"disable" bson:"disable"`
	Range     string             `json:"range" bson:"range"`
	ValidTime string             `json:"validTime" bson:"validTime"`
	StartTime primitive.DateTime `json:"startTime" bson:"startTime"`
	EndTime   primitive.DateTime `json:"endTime" bson:"endTime"`
}

// ExtFlow
type ExtFlow struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Owner    string          `json:"owner"`
	Settings ExtFlowSettings `json:"settings"`
	Type     string          `json:"type"`
	FlowJson string          `json:"flowJson"`
	FlowXml  string          `json:"flowXml"`

	Invalid   bool   `json:"invalid"`
	Disable   bool   `json:"disable"`
	Range     string `json:"range"`
	ValidTime string `json:"validTime"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
}

type ExtFlowSettings struct {
	Table Table `json:"table"`
	//Logic        []Logic         `json:"logic"`
	Query        map[string]interface{} `json:"query"`
	EventType    string                 `json:"eventType"`
	SelectTyp    string                 `json:"selectType"`
	SelectRecord []GeneralIDName        `json:"selectRecord"`
	RangeType    string                 `json:"rangeType"`
	//Invalid      bool               `json:"invalid"`
	//Disable      bool               `json:"disable"`
	//Range        string             `json:"range"`
	//ValidTime    string             `json:"validTime"`
	//StartTime    string             `json:"startTime"`
	//EndTime      string             `json:"endTime"`
	Field       []string             `json:"field"`
	UpdateField []GeneralIDNameValue `json:"updateField"`
}

type Logic struct {
	ID        string            `json:"id"`
	Name      string            `json:"name"`
	Relation  string            `json:"relation"`
	Compare   GeneralExtCompare `json:"compare"`
	DataType  string            `json:"dataType"`
	LogicType string            `json:"logicType"`
	PropType  string            `json:"propType"`
}

type Table struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Title string `json:"title"`
}
