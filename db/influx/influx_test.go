package influx

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	_ "github.com/influxdata/influxdb1-client" // this is important because of the bug in go mod
	"github.com/influxdata/influxdb1-client/v2"
	"github.com/sirupsen/logrus"
)

func TestConnectInflux(t *testing.T) {
	//创建客户端
	host := "iot.tmis.top"
	port := 31746
	username := "admin"
	password := "dell123"
	database := "tsdb"
	cli, err := client.NewHTTPClient(client.HTTPConfig{
		Addr:     fmt.Sprintf("http://%s:%d", host, port),
		Username: username,
		Password: password,
	})
	if err != nil {
		logrus.Fatalf("Influx HTTP客户端创建错误: %s", err.Error())
	}

	//存储数据
	//measurement是表名，值是模型ID
	measurement := "5fbdcee3fdf46de0a75a976e"
	tagsList := make([]map[string]string, 0)
	fieldsList := make([]map[string]interface{}, 0)
	timestampList := make([]time.Time, 0)

	//id是资产ID
	//uid是资产编号
	//department是资产所属部门，多个部门用,分割，如"5f52e75d8a147022095b8c53,5f52e75d8a147022095b8c54"
	tags := map[string]string{
		"id":         "5fbdcee3fdf46de0a75a976e",
		"uid":        "ywjtest07",
		"department": "5f52e75d8a147022095b8c53",
	}
	//数据点
	fields := map[string]interface{}{
		"BZTJ": 440.1,
		"SJD2": 770.3,
	}
	tagsList = append(tagsList, tags)
	fieldsList = append(fieldsList, fields)
	timestampList = append(timestampList, time.Now())
	time.Sleep(time.Second * 1)

	tags = map[string]string{
		"id":         "5fbdcee3fdf46de0a75a976e",
		"uid":        "ywjtest07",
		"department": "5f52e75d8a147022095b8c53",
	}
	fields = map[string]interface{}{
		"BZTJ": 380.2,
		"SJD2": 770.1,
	}
	tagsList = append(tagsList, tags)
	fieldsList = append(fieldsList, fields)
	timestampList = append(timestampList, time.Now())
	err = writePointsRetentionBatch(cli, database, "s", "autogen", measurement, tagsList, fieldsList, timestampList)
	if err != nil {
		logrus.Fatalf("存储Point失败(Retention)%+v", err)
	}
	logrus.Infof("存储Point成功(Retention)")


	//普通查询
	//tz('Asia/Shanghai')用来指定查询时区，必要
	//语法与一般的sql类似，表名，字段等的开头是数字的话要用双引号包起来
	//查询time字段可以用 '2020-12-19T02:43:26.374Z'或 1608345728000000000 (纳秒级)这样的格式作为值
	sqlString := "select  \"data1\" from \"5f4cc0a84c0dcbaa8823dfa9\"  where uid = '00132432'     and time >= '2020-12-19T02:43:26.374Z' and time < '2020-12-19T02:44:26.374Z'      tz('Asia/Shanghai');"
	res, err := queryInfluxData(cli, sqlString, database)
	if err != nil {
		logrus.Fatalf("Influx 查询数据错误: %s", err.Error())
	}

	b, _ := json.Marshal(res)
	fmt.Println("普通查询结果:", string(b))

	//聚合查询
	//as 用来给查询回来的字段起指定的别名
	sqlGroupString := "select  MEAN(\"BZTJ\") as BZTJ from \"5fbdcee3fdf46de0a75a976e\"   where id =~ /5fbdd0dafdf46de0a75a979f/     and time >= 1608345728000000000 and time < 1608345848001418196      tz('Asia/Shanghai')"

	res, err = queryInfluxData(cli, sqlGroupString, database)
	if err != nil {
		logrus.Fatalf("Influx 查询数据错误: %s", err.Error())
	}

	b, _ = json.Marshal(res)
	fmt.Println("聚合查询结果:", string(b))

	//结果
//	=== RUN   TestConnectInflux
//	time="2020-12-19T11:34:46+08:00" level=info msg="存储Point成功(Retention)"
//普通查询结果: [{"statement_id":0,"Series":[{"name":"5f4cc0a84c0dcbaa8823dfa9","columns":["time","data1"],"values":[["2020-12-19T10:43:35.025+08:00",221.6],["2020-12-19T10:43:45.027+08:00",162.4],["2020-12-19T10:43:55.067+08:00",36.8],["2020-12-19T10:44:05.021+08:00",176],["2020-12-19T10:44:15.051+08:00",61.6],["2020-12-19T10:44:25.06+08:00",257.6]]}],"Messages":null}]
//	聚合查询结果: [{"statement_id":0,"Series":[{"name":"5fbdcee3fdf46de0a75a976e","columns":["time","BZTJ"],"values":[["2020-12-19T10:42:08+08:00",27.5]]}],"Messages":null}]
//	--- PASS: TestConnectInflux (1.11s)
//	PASS

}

func queryInfluxData(cli client.Client, sqlString, database string) ([]client.Result, error) {
	q := client.Query{
		Command:  sqlString,
		Database: database,
	}

	res := make([]client.Result, 0)
	if response, err := cli.Query(q); err == nil {
		if response.Error() != nil {
			return nil, fmt.Errorf("查询响应错误: %s", response.Error())
		}
		res = response.Results
	} else {
		return nil, fmt.Errorf("查询错误: %s", err.Error())
	}
	return res, nil
}

func writePointsRetentionBatch(cli client.Client, database, precision, policyName, measurement string, tags []map[string]string, fields []map[string]interface{}, timestamps []time.Time) error {
	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database:        database,
		Precision:       precision,
		RetentionPolicy: policyName,
	})
	if err != nil {
		return fmt.Errorf("新建批量添加操作失败(Retention).%+v", err)
	}

	for i := 0; i < len(tags); i++ {
		pt, err := client.NewPoint(
			measurement,
			tags[i],
			fields[i],
			timestamps[i],
		)
		if err != nil {
			return fmt.Errorf("新建Point失败(Retention).%+v", err)
		}
		bp.AddPoint(pt)
	}

	if err := cli.Write(bp); err != nil {
		return fmt.Errorf("批量存储Point失败(Retention).%+v", err)
	}
	return nil
}
