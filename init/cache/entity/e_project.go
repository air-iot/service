package entity

import "time"

// Project 项目实体
type Project struct {
	ID         string    `json:"id" bson:"_id"`
	UserID     string    `json:"user" bson:"user"`
	Name       string    `json:"name" bson:"name"`
	Industry   string    `json:"industry" bson:"industry"` // 行业
	Grant      *Grant    `json:"grant" bson:"grant"`
	Remarks    string    `json:"remarks" bson:"remarks"`
	CreateTime time.Time `json:"createTime" bson:"createTime"`
	Default    bool      `json:"default" bson:"default"` // 状态(true:默认 false:非默认)
	Status     bool      `json:"status" bson:"status"`   // 状态(true:启用 false:停用)
}

// SpmUser 租户对象
type SpmUser struct {
	ID         string    `json:"id" bson:"_id"`
	Name       string    `json:"name" bson:"name"`
	Telephone  string    `json:"telephone" bson:"telephone"`
	Grant      *Grant    `json:"grant" bson:"grant"`
	CreateTime time.Time `json:"createTime" bson:"createTime"`
	Status     bool      `json:"status" bson:"status"` // 状态(true:启用 false:停用)
	Projects   []Project `json:"projects" bson:"projects"`
}

// ProjectUser 项目实体
type ProjectUser struct {
	ID         string    `json:"id" bson:"_id"`
	User       SpmUser   `json:"user" bson:"user"`
	Name       string    `json:"name" bson:"name"`
	Industry   string    `json:"industry" bson:"industry"` // 行业
	Grant      *Grant    `json:"grant" bson:"grant"`
	Remarks    string    `json:"remarks" bson:"remarks"`
	CreateTime time.Time `json:"createTime" bson:"createTime"`
	Default    bool      `json:"default" bson:"default"` // 状态(true:默认 false:非默认)
	Status     bool      `json:"status" bson:"status"`   // 状态(true:启用 false:停用)
}
