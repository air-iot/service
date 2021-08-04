package entity

import "go.mongodb.org/mongo-driver/bson/primitive"

// Flow
type Flow struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Owner    string                 `json:"owner"`
	Settings map[string]interface{} `json:"settings"`
	Type     string                 `json:"type"`
	Invalid  bool                   `json:"invalid"`
	FlowJson string                 `json:"flowJson"`
	FlowXml  string                 `json:"flowXml"`
}

// FlowMongo
type FlowMongo struct {
	ID       string      `json:"id" bson:"_id"`
	Name     string      `json:"name" bson:"name"`
	Owner    string      `json:"owner" bson:"owner"`
	Settings primitive.M `json:"settings" bson:"settings"`
	Type     string      `json:"type" bson:"type"`
	Invalid  bool        `json:"invalid" bson:"invalid"`
	FlowJson string      `json:"flowJson" bson:"flowJson"`
	FlowXml  string      `json:"flowXml" bson:"flowXml"`
}
