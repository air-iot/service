package model

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
	ID         string   `json:"id" bson:"_id"`
	Name       string               `json:"name" bson:"name"`
	Department []string `json:"department" bson:"department"`
	Roles      []string `json:"roles" bson:"roles"`
	OpenID     string               `json:"openid" bson:"openid"`
	Phone      string               `json:"phone" bson:"phone"`
}
