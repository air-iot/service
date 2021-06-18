package model

// Table
type Table struct {
	ID     string                 `json:"id"`
	Schema map[string]interface{} `json:"schema"`
}
