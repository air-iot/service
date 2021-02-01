package model

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
	"time"
)

type WarningMessage struct {
	Id             interface{}              `json:"id"`
	MainType       string                   `json:"mainType"`
	Type           string                   `json:"type"`
	Status         string                   `json:"status"`
	Processed      string                   `json:"processed"`
	Parent         []string     `json:"status"`
	Desc           string                   `json:"desc"`
	Level          string                   `json:"level"`
	Department     []string     `json:"department"`
	Fields         []map[string]interface{} `json:"fields"`
	Model          interface{}              `json:"model"`
	ModelID        string       `json:"modelID"`
	NodeID         string       `json:"nodeID"`
	Node           interface{}              `json:"node"`
	RuleID         string                   `json:"ruleid"`
	Time           int64                    `json:"time"`
	Interval       int64                    `json:"interval"`
	Uid            string                   `json:"uid"`
	Handle         bool                     `json:"handle"`
	Alert          bool                     `json:"alert"`
	Other          map[string]interface{}   `json:"other"`
	Driver         []string     `json:"driver"`
	HandleUserName string                   `json:"handleUserName"`
	WarnTag        map[string]interface{}   `json:"warnTag"`
}

type WarningSaveMessage struct {
	ID         string                   `json:"_id"`
	MainType   string                   `json:"mainType"`
	Type       string                   `json:"type"`
	Status     string                   `json:"status"`
	Processed  string                   `json:"processed"`
	Parent     []string     `json:"status"`
	Desc       string                   `json:"desc"`
	Level      string                   `json:"level"`
	Department []string     `json:"department"`
	Fields     []map[string]interface{} `json:"fields"`
	Model      string       `json:"model"`
	Node       string       `json:"node"`
	RuleID     string                   `json:"ruleid"`
	Time       time.Time                `json:"time"`
	Interval   int64                    `json:"interval"`
	Uid        string                   `json:"uid"`
	Handle     bool                     `json:"handle"`
	Alert      bool                     `json:"alert"`
	Other      map[string]interface{}   `json:"other"`
	Driver     []string     `json:"driver"`
	WarnTag    map[string]interface{}   `json:"warnTag"`
}

type WarningSendMessage struct {
	Id         interface{}            `json:"id"`
	MainType   string                 `json:"mainType"`
	Type       string                 `json:"type"`
	Status     string                 `json:"status"`
	Processed  string                 `json:"processed"`
	Parent     interface{}            `json:"status"`
	Desc       string                 `json:"desc"`
	Level      string                 `json:"level"`
	Department interface{}            `json:"department"`
	Fields     interface{}            `json:"fields"`
	Model      interface{}            `json:"model"`
	Node       interface{}            `json:"node"`
	RuleID     string                 `json:"ruleid"`
	Time       interface{}            `json:"time"`
	Interval   int64                  `json:"interval"`
	Uid        string                 `json:"uid"`
	Handle     bool                   `json:"handle"`
	Alert      bool                   `json:"alert"`
	Other      map[string]interface{} `json:"other"`
	Driver     interface{}            `json:"driver"`
	WarnTag    map[string]interface{} `json:"warnTag"`
}

type WarningUnmarshalMessage struct {
	Id         interface{}              `json:"id"`
	MainType   string                   `json:"mainType"`
	Type       string                   `json:"type"`
	Status     string                   `json:"status"`
	Processed  string                   `json:"processed"`
	Parent     interface{}              `json:"status"`
	Desc       string                   `json:"desc"`
	Level      string                   `json:"level"`
	Department interface{}              `json:"department"`
	Fields     []map[string]interface{} `json:"fields"`
	Model      interface{}              `json:"model"`
	Node       interface{}              `json:"node"`
	ModelID    string                   `json:"modelID"`
	NodeID     string                   `json:"nodeID"`
	RuleID     string                   `json:"ruleid"`
	Time       interface{}              `json:"time"`
	Interval   interface{}              `json:"interval"`
	Uid        string                   `json:"uid"`
	Handle     bool                     `json:"handle"`
	Alert      bool                     `json:"alert"`
	Other      map[string]interface{}   `json:"other"`
	Driver     interface{}              `json:"driver"`
	WarnTag    map[string]interface{}   `json:"warnTag"`
}
