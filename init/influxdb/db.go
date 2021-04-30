package influxdb

import (
	"errors"
	"time"

	_ "github.com/influxdata/influxdb1-client" // this is important because of the bug in go mod
	"github.com/influxdata/influxdb1-client/v2"
)

//============InfluxDB==================

// FindInfluxBySQL convenience function to query the database
func FindInfluxBySQL(cli client.Client, database, cmd string) (res []client.Result, err error) {
	q := client.Query{
		Command:  cmd,
		Database: database,
	}

	if response, err := cli.Query(q); err == nil {
		if response.Error() != nil {
			return res, response.Error()
		}
		res = response.Results
	} else {
		return res, err
	}
	return res, nil
}

// InfluxResultConvertMap InfluxDB的查询结果[]api.Result转换成map[string]interface{}
func InfluxResultConvertMap(results []client.Result) (map[string]interface{}, error) {
	newResultMap := map[string]interface{}{}
	newResults := make([]map[string]interface{}, 0)
	for _, v := range results {
		if v.Err == "" {
			r := map[string]interface{}{}

			for _, row := range v.Series {
				timeIndex := 0
				hasTime := false
				for i, col := range row.Columns {
					if col == "time" {
						timeIndex = i
						hasTime = true
						break
					}
				}
				if hasTime {
					for _, list := range row.Values {
						for j, ele := range list {
							if j == timeIndex {
								if _, ok := ele.(string); !ok {
									if timeInt, ok := ele.(int64); ok {
										queryTime := time.Unix(0, timeInt*int64(time.Millisecond))
										list[j] = queryTime.Format("2006-01-02T15:04:05.000+08:00")
									}
								}
							}
						}
					}
				}
			}
			r["series"] = v.Series
			newResults = append(newResults, r)
		} else {
			continue
		}
	}

	newResultMap["results"] = newResults

	return newResultMap, nil

}

// InfluxResultConvert InfluxDB的查询结果[]api.Result转换成[][]map[string]interface{}
func InfluxResultConvert(results []client.Result) ([][]map[string]interface{}, error) {
	newResults := make([][]map[string]interface{}, 0)
	for _, v := range results {
		if v.Err == "" {
			rList := make([]map[string]interface{}, 0)
			for _, v1 := range v.Series {
				for _, v2 := range v1.Values {
					r := make(map[string]interface{})
					if v1.Tags != nil && len(v1.Tags) >= 0 {
						r["id"] = v1.Tags["id"]
						//r[TIME] = v2.Tags[TIME]
					}
					for i := 0; i < len(v1.Columns); i++ {
						err := setInfluxResult(r, v1.Columns[i], v2[i])
						if err != nil {
							continue
						}
					}
					rList = append(rList, r)
				}
			}
			newResults = append(newResults, rList)
		} else {
			continue
		}
	}

	return newResults, nil

}

// setInfluxResult 结果实体赋值
func setInfluxResult(result map[string]interface{}, flag interface{}, val interface{}) error {
	switch flag {
	case "time":
		v, ok := val.(string)
		if !ok {
			return errors.New("时间转换错误")
		}
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return err
		}
		result["time"] = t.Unix()
	default:
		v, ok := flag.(string)
		if !ok {
			return errors.New("flag转换错误")
		}
		result[v] = val
	}
	return nil
}
