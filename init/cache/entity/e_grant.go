package entity

// Grant 授权
type Grant struct {
	Duration  int    `json:"duration" bson:"duration"`   // 授权时长
	StartTime string `json:"startTime" bson:"startTime"` // 授权开始时间
	UserCount int    `json:"userCount" bson:"userCount"` // 用户数
	Point     int    `json:"point" bson:"point"`         // 用户点数
}
