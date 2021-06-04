package entity

import "time"

type Message struct {
	//访问者的host
	ID string `json:"id" bson:"_id"`
	//访问用户
	User string `json:"user" bson:"user"`
	//其他信息
	Message    string    `json:"message" bson:"message" example:"othermsg"`
	CreateTime time.Time `json:"createTime" bson:"createTime"`
}
