package model

// Role
type Role struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Users       []string `json:"users"`
	Permission  []string `json:"permission"`
	Description string   `json:"description"`
}

// RoleMongo
type RoleMongo struct {
	ID          string   `json:"id" bson:"_id"`
	Name        string               `json:"name" bson:"name"`
	Users       []string `json:"users" bson:"users"`
	Permission  []string             `json:"permission" bson:"permission"`
	Description string               `json:"description" bson:"description"`
}
