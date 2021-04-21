package cache

// User
type User struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Department     []string `json:"department"`
	Roles          []string `json:"roles"`
	OpenID         string   `json:"openid"`
	Phone          string   `json:"phone"`
	TimeoutTimeUse string   `json:"timeoutTimeUse"`
	AdminAccess    bool     `json:"adminAccess"`
}

// UserMongo
type UserMongo struct {
	ID             string      `json:"id" bson:"_id"`
	Name           string      `json:"name" bson:"name"`
	Department     []string    `json:"department" bson:"department"`
	Roles          []string    `json:"roles" bson:"roles"`
	OpenID         string      `json:"openid" bson:"openid"`
	Phone          string      `json:"phone" bson:"phone"`
	TimeoutTimeUse interface{} `json:"timeoutTimeUse" bson:"timeoutTimeUse"`
	AdminAccess    bool        `json:"adminAccess" bson:"adminAccess"`
}
