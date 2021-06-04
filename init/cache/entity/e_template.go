package entity

import "time"

// Template 模版实体
type Template struct {
	ID          string    `json:"id" bson:"_id"`
	Name        string    `json:"name" bson:"name"`
	Industry    string    `json:"industry" bson:"industry"` // 行业
	Description string    `json:"description" bson:"description"`
	Contact     string    `json:"contact" bson:"contact"`
	Icon        string    `json:"icon" bson:"icon"`
	File        string    `json:"file" bson:"file"`
	User        string    `json:"user" bson:"user"`
	CreateTime  time.Time `json:"createTime" bson:"createTime"`
}
