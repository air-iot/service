package model

import "go.mongodb.org/mongo-driver/bson/primitive"

// Event
type EventHandler struct {
	ID          string                 `json:"id"`
	HandlerName string                 `json:"handlerName"`
	Event       string                 `json:"event"`
	Settings    map[string]interface{} `json:"settings"`
	Type        string                 `json:"type"`
}

// EventHandlerMongo
type EventHandlerMongo struct {
	ID          string `json:"id" bson:"_id"`
	HandlerName string             `json:"handlerName" bson:"handlerName"`
	Event       string `json:"event" bson:"event"`
	Settings    primitive.M        `json:"settings" bson:"settings"`
	Type        string             `json:"type" bson:"type"`
}
