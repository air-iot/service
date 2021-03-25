package tools_test

import (
	"encoding/json"
	"fmt"
	"github.com/air-iot/service/util/tools"
	"testing"
)

func Test_GetCronExpression(t *testing.T) {
	dataList := []map[string]interface{}{
		{
			"id":    "",
			"name":  "测试计划事件(hour)",
			"type":  "计划事件",
			"owner": "system",
			"settings": map[string]interface{}{
				"type": "hour", //hour day week month year once
				"startTime": map[string]interface{}{
					"year":   0,
					"month":  0,
					"week":   0,
					"day":    0,
					"hour":   0,
					"minute": 10,
					"second": 20,
				},
				"endTime": map[string]interface{}{
					"year":   1,
					"month":  1,
					"week":   1,
					"day":    1,
					"hour":   1,
					"minute": 20,
					"second": 20,
				},
			},
		},
		{
			"id":    "",
			"name":  "测试计划事件(day)",
			"type":  "计划事件",
			"owner": "system",
			"settings": map[string]interface{}{
				"type": "day", //hour day week month year once
				"startTime": map[string]interface{}{
					"year":   0,
					"month":  0,
					"week":   0,
					"day":    0,
					"hour":   2,
					"minute": 10,
					"second": 20,
				},
				"endTime": map[string]interface{}{
					"year":   1,
					"month":  1,
					"week":   1,
					"day":    1,
					"hour":   3,
					"minute": 10,
					"second": 20,
				},
			},
		},
		{
			"id":    "",
			"name":  "测试计划事件(week)",
			"type":  "计划事件",
			"owner": "system",
			"settings": map[string]interface{}{
				"type": "week", //hour day week month year once
				"startTime": map[string]interface{}{
					"year":   0,
					"month":  0,
					"week":   1,
					"day":    0,
					"hour":   2,
					"minute": 10,
					"second": 20,
				},
				"endTime": map[string]interface{}{
					"year":   1,
					"month":  1,
					"week":   2,
					"day":    1,
					"hour":   3,
					"minute": 10,
					"second": 20,
				},
			},
		},
		{
			"id":    "",
			"name":  "测试计划事件(month)",
			"type":  "计划事件",
			"owner": "system",
			"settings": map[string]interface{}{
				"type": "month", //hour day week month year once
				"startTime": map[string]interface{}{
					"year":   0,
					"month":  0,
					"week":   1,
					"day":    13,
					"hour":   2,
					"minute": 10,
					"second": 20,
				},
				"endTime": map[string]interface{}{
					"year":   1,
					"month":  1,
					"week":   2,
					"day":    13,
					"hour":   3,
					"minute": 10,
					"second": 20,
				},
			},
		},
		{
			"id":    "",
			"name":  "测试计划事件(year)",
			"type":  "计划事件",
			"owner": "system",
			"settings": map[string]interface{}{
				"type": "year", //hour day week month year once
				"startTime": map[string]interface{}{
					"year":   0,
					"month":  2,
					"week":   1,
					"day":    13,
					"hour":   2,
					"minute": 10,
					"second": 20,
				},
				"endTime": map[string]interface{}{
					"year":   1,
					"month":  2,
					"week":   2,
					"day":    13,
					"hour":   3,
					"minute": 10,
					"second": 20,
				},
			},
		},
		{
			"id":    "",
			"name":  "测试计划事件(once)",
			"type":  "计划事件",
			"owner": "system",
			"settings": map[string]interface{}{
				"type": "once", //hour day week month year once
				"startTime": map[string]interface{}{
					"year":   0,
					"month":  2,
					"week":   1,
					"day":    13,
					"hour":   2,
					"minute": 10,
					"second": 20,
				},
				"endTime": map[string]interface{}{
					"year":   1,
					"month":  2,
					"week":   2,
					"day":    13,
					"hour":   3,
					"minute": 10,
					"second": 20,
				},
			},
		},
	}
	dataMapList := make([]map[string]interface{}, 0)
	dataListByte, err := json.Marshal(dataList)
	if err != nil {
		fmt.Println(err)
		return
	}
	err = json.Unmarshal(dataListByte, &dataMapList)
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, data := range dataMapList {
		if settings, ok := data["settings"].(map[string]interface{}); ok {
			if scheduleType, ok := settings["type"].(string); ok {
				if startTime, ok := settings["startTime"].(map[string]interface{}); ok {
					result := tools.GetCronExpression(scheduleType, startTime)
					fmt.Printf("result %s: %s", scheduleType, result)
					fmt.Println()
				}
			}
		}
	}
}
