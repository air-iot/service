package model

import "go.mongodb.org/mongo-driver/bson/primitive"

// Model
type Model struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Computed  Computed  `json:"computed"`
	Device    Device    `json:"device"`
	Warning   Warning   `json:"warning"`
	Relations Relations `json:"relations"`
}

type ModelMongo struct {
	ID        string `json:"id" bson:"_id"`
	Name      string             `json:"name" bson:"name"`
	Computed  ComputedMongo      `json:"computed" bson:"computed"`
	Device    DeviceMongo        `json:"device" bson:"device"`
	Warning   WarningMongo       `json:"warning" bson:"warning"`
	Relations RelationsMongo     `json:"relations" bson:"relations"`
}

type RelationsMongo struct {
	Child  []string `json:"child" bson:"child"`
	Parent []string `json:"parent" bson:"parent"`
}

type Relations struct {
	Child  []string `json:"child"`
	Parent []string `json:"parent"`
}

type Computed struct {
	Auto      bool       `json:"auto"`
	Tags      []Tag      `json:"tags"`
	ExtraTags []ExtraTag `json:"extraTags"`
}

type DeviceMongo struct {
	Driver   string     `json:"driver" bson:"driver"`
	Tags     []TagMongo `json:"tags" bson:"tags"`
	Commands []struct {
		ID   string `json:"id" bson:"id"`
		Name string `json:"name" bson:"name"`
	} `json:"commands" bson:"commands"`
	Settings Settings `json:"settings" bson:"settings"`
}

type Device struct {
	Driver   string `json:"driver"`
	Tags     []Tag  `json:"tags"`
	Commands []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"commands"`
	Settings Settings `json:"settings"`
}

type Settings struct {
	Interval float64 `json:"interval"`
	Network  Network `json:"network"`
}

type Network struct {
	Timeout  float64 `json:"timeout"`
	Interval float64 `json:"interval"`
}

type Tag struct {
	ID            string                 `json:"id"`
	DataType      string                 `json:"dataType"`
	Type          string                 `json:"type"`
	Value         interface{}            `json:"value"`
	Name          string                 `json:"name"`
	Policy        string                 `json:"policy"`
	GroupName     string                 `json:"groupname"`
	Unit          string                 `json:"unit"`
	Logic         map[string]interface{} `json:"logic"`
	FormulaLogic  string                 `json:"formulaLogic"`
	StatsMethod   string                 `json:"statsMethod"`
	StatsTag      string                 `json:"statsTag"`
	StatsInterval string                 `json:"statsInterval"`
	Rules         map[string]float64     `json:"rules"`
	IntervalType  string                 `json:"intervalType"`
	Interval      interface{}            `json:"interval"`
	StartTime     map[string]interface{} `json:"startTime"`
	StatsVal      interface{}            `json:"statsVal"`
}

type TagRules struct {
	Low   float64 `json:"low"`
	Llow  float64 `json:"llow"`
	High  float64 `json:"high"`
	Hhigh float64 `json:"hhigh"`
}

type RuleInTags struct {
	ID          string             `json:"id"`
	Level       string             `json:"level"`
	Type        string             `json:"type"`
	Description string             `json:"description"`
	Logic       map[string]float64 `json:"logic"`
	Interval    float64            `json:"interval"`
}
