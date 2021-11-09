package entity

import "go.mongodb.org/mongo-driver/bson/primitive"

// Setting
type Setting struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Warning WarningSetting `json:"warning"`
	// Email 邮件发送配置
	Email Email `json:"email"`
	// Wechat Wechat发送配置
	Wechat               Wechat             `json:"wechat"`
	PropTag              string             `json:"propTag"`
	OnlineCheck          bool               `json:"onlineCheck"`
	Dingtalk             Dingtalk           `json:"dingtalk"`
	Sms                  Sms                `json:"sms"`
	Rectption_login_page RectptionLoginPage `json:"rectption_login_page"`
	ShowCode             bool               `json:"showCode"`
}

type RectptionLoginPage struct {
	ID       string `json:"id"`
	ShowCode bool   `json:"showCode"`
}

type Sms struct {
	AppKey    string `json:"appKey"`
	AppSecret string `json:"appSecret"`
}

type Dingtalk struct {
	AgentID   string `json:"agentid"`
	AppKey    string `json:"appKey"`
	AppSecret string `json:"appSecret"`
}

type DingtalkMongo struct {
	AgentID   string `json:"agentid" bson:"agentid"`
	AppKey    string `json:"appKey" bson:"appKey"`
	AppSecret string `json:"appSecret" bson:"appSecret"`
}

type WarningSetting struct {
	WarnTag     bool          `json:"warnTag"`
	Audio       string        `json:"audio"`
	WarningKind []WarningKind `json:"warningkind"`
}

type WarningKind struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Alert  bool   `json:"alert"`
	Handle bool   `json:"handle"`
}

type Email struct {
	Host string   `json:"host"`
	Port int      `json:"port"`
	From string   `json:"from"`
	To   []string `json:"to"`
	//UserName string   `toml:"userName" description:"SMTP服务器用户名"`
	Password string `json:"password"`
}

type Wechat struct {
	AppID  string `json:"appid"`
	Secret string `json:"secret"`
}

// SettingMongo
type SettingMongo struct {
	ID           string      `json:"id" bson:"_id"`
	Name         string      `json:"name" bson:"name"`
	WarningMongo primitive.M `json:"warning" bson:"warning"`
	// Email 邮件发送配置
	EmailMongo primitive.M `json:"email" bson:"email"`
	// Wechat Wechat发送配置
	WechatMongo   primitive.M `json:"wechat" bson:"wechat"`
	PropTag       string      `json:"propTag" bson:"propTag"`
	OnlineCheck   bool        `json:"onlineCheck" bson:"onlineCheck"`
	DingtalkMongo primitive.M `json:"dingtalk" bson:"dingtalk"`
}

type WarningSettingMongo struct {
	WarnTag     bool               `json:"warnTag" bson:"warnTag"`
	Audio       string             `json:"audio" bson:"audio"`
	WarningKind []WarningKindMongo `json:"warningkind" bson:"warningkind"`
}

type WarningKindMongo struct {
	ID     string `json:"id" bson:"id"`
	Name   string `json:"name" bson:"name"`
	Alert  bool   `json:"alert" bson:"alert"`
	Handle bool   `json:"handle" bson:"handle"`
}

type EmailMongo struct {
	Host string   `json:"host" bson:"host"`
	Port int      `json:"port" bson:"port"`
	From string   `json:"from" bson:"from"`
	To   []string `json:"to" bson:"to"`
	//UserName string   `toml:"userName" description:"SMTP服务器用户名"`
	Password string `json:"password" bson:"password"`
}

type WechatMongo struct {
	AppID  string `json:"appid" bson:"appid"`
	Secret string `json:"secret" bson:"secret"`
}
