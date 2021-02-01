package model

import "go.mongodb.org/mongo-driver/bson/primitive"

// SystemVariable
type SystemVariable struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Uid   string      `json:"uid"`
	Value interface{} `json:"value"`
}

// SystemVariableMongo
type SystemVariableMongo struct {
	ID    string `json:"id" bson:"_id"`
	Name  string             `json:"name" bson:"name"`
	Uid   string             `json:"uid" bson:"uid"`
	Value interface{}        `json:"value" bson:"value"`
}
