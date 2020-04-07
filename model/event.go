package model

import "go.mongodb.org/mongo-driver/bson/primitive"

// Event
type Event struct {
	ID       string                 `json:"id"`
	Name     string                 `json:"name"`
	Owner    string                 `json:"owner"`
	Settings map[string]interface{} `json:"settings"`
	Type     string                 `json:"type"`
	Handlers []interface{}          `json:"handlers"`
}

// EventMongo
type EventMongo struct {
	ID       primitive.ObjectID `json:"id" bson:"_id"`
	Name     string             `json:"name" bson:"name"`
	Owner    string             `json:"owner" bson:"owner"`
	Settings primitive.M        `json:"settings" bson:"settings"`
	Type     string             `json:"type" bson:"type"`
	Handlers primitive.A        `json:"handlers" bson:"handlers"`
}
