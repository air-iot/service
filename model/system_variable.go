package model

import "go.mongodb.org/mongo-driver/bson/primitive"

// SystemVariable
type SystemVariable struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

// SystemVariableMongo
type SystemVariableMongo struct {
	ID    primitive.ObjectID `json:"id" bson:"_id"`
	Name  string             `json:"name" bson:"name"`
	Value interface{}        `json:"value" bson:"value"`
}
