package model

import "go.mongodb.org/mongo-driver/bson/primitive"

// User
type User struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Department []string `json:"department"`
	Roles      []string `json:"roles"`
	OpenID     string   `json:"openid"`
	Phone      string   `json:"phone"`
}

// UserMongo
type UserMongo struct {
	ID         primitive.ObjectID   `json:"id" bson:"_id"`
	Name       string               `json:"name" bson:"name"`
	Department []primitive.ObjectID `json:"department" bson:"department"`
	Roles      []primitive.ObjectID `json:"roles" bson:"roles"`
	OpenID     string               `json:"openid" bson:"openid"`
	Phone      string               `json:"phone" bson:"phone"`
}
