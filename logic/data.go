package logic

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis"
	"github.com/influxdata/influxdb/client/v2"
	"github.com/sirupsen/logrus"

	"github.com/air-iot/service/db/influx"
	ir "github.com/air-iot/service/db/redis"
	"github.com/air-iot/service/model"
)

var DataLogic = new(dataLogic)

type dataLogic struct{}

func (*dataLogic) FindCacheByUIDAndTagID(uid, tagID string) (result *model.Cache, err error) {
	cmd := ir.Client.HGetAll(fmt.Sprintf("%s|%s", uid, tagID))
	if cmd.Err() != nil {
		return nil, cmd.Err()
	}
	r, err := cmd.Result()
	if err != nil {
		return nil, err
	}

	v1, ok1 := r["value"]
	t1, ok2 := r["time"]
	if !ok1 || !ok2 {
		return nil, errors.New("未查询到相关数据值")
	}
	//v2, err := strconv.ParseFloat(v1, 64)
	//if err != nil {
	//	return nil, fmt.Errorf("值解析错误:%s", err.Error())
	//}
	t2, err := strconv.ParseInt(t1, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("时间解析错误:%s", err.Error())
	}
	return &model.Cache{Value: v1, Time: t2}, nil
}

// uid 及 数据点列表查询
func (*dataLogic) FindCacheByUIDAndTagIDs(uid string, tagIDs []string) (result map[string]model.Cache, err error) {
	result1 := make(map[string]*redis.StringStringMapCmd)
	p := ir.Client.Pipeline()
	for _, tagID := range tagIDs {
		cmd := p.HGetAll(fmt.Sprintf("%s|%s", uid, tagID))
		result1[tagID] = cmd
	}
	_, err = p.Exec()
	if err != nil {
		return nil, err
	}
	result = make(map[string]model.Cache, 0)

	for tagID, cmd := range result1 {
		if cmd.Err() != nil {
			logrus.Warnf("数据点:%s 查询错误:%s", tagID, cmd.Err().Error())
			continue
		}
		r, err := cmd.Result()
		if err != nil {
			logrus.Warnf("数据点:%s 查询结果错误:%s", tagID, err.Error())
			continue
		}

		v1, ok1 := r["value"]
		t1, ok2 := r["time"]
		if !ok1 || !ok2 {
			logrus.Warnf("数据点:%s 未查询到相关数据值", tagID)
			continue
		}
		//v2, err := strconv.ParseFloat(v1, 64)
		//if err != nil {
		//	logrus.Warnf("数据点:%s 值解析错误:%s", tagID, err.Error())
		//	continue
		//}
		t2, err := strconv.ParseInt(t1, 10, 64)
		if err != nil {
			logrus.Warnf("数据点:%s 时间解析错误:%s", tagID, err.Error())
			continue
		}
		result[tagID] = model.Cache{Value: v1, Time: t2}
	}

	return result, nil
}

func (*dataLogic) SaveCacheByUIDAndTagID(uid, tagID string, data map[string]interface{}) error {
	cmd := ir.Client.HMSet(fmt.Sprintf("%s|%s", uid, tagID), data)
	if cmd.Err() != nil {
		return cmd.Err()
	}
	return nil
}

func (*dataLogic) SaveCacheByUID(uid string, t time.Time, data map[string]interface{}) error {
	//d := make(map[string]map[string]interface{})
	p := ir.Client.TxPipeline()
	for k, v := range data {
		p.HMSet(fmt.Sprintf("%s|%s", uid, k), map[string]interface{}{
			"time":  t.Unix(),
			"value": v,
		})
	}
	if _, err := p.Exec(); err != nil {
		return err
	}
	return nil
}


func (*dataLogic) SaveDifferenceCacheByUID(uid string, t time.Time, data map[string]interface{},interval string) error {
	//d := make(map[string]map[string]interface{})
	p := ir.Client.TxPipeline()
	for k, v := range data {
		p.HMSet(fmt.Sprintf("%s|%s|%s", uid, k,interval), map[string]interface{}{
			"time":  t.Unix(),
			"value": v,
		})
	}
	if _, err := p.Exec(); err != nil {
		return err
	}
	return nil
}


func (p *dataLogic) SaveInflux(modelID, nodeID, uid string, data map[string]interface{}) error {
	node, err := NodeLogic.FindLocalCache(nodeID)
	var department string
	if err == nil {
		for _, deptID := range node.Department {
			if department == "" {
				department = deptID
			} else {
				department = department + "," + deptID
			}
		}
	}

	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database:        influx.DB,
		Precision:       "s",
		RetentionPolicy: "autogen",
	})
	if err != nil {
		return err
	}
	pt, err := client.NewPoint(
		modelID,
		map[string]string{"id": nodeID, "uid": uid, "department": department},
		data,
		time.Now().Local(),
	)

	if err != nil {
		return err
	}
	bp.AddPoint(pt)
	if err := influx.UDPClient.Write(bp); err != nil {
		return err
	}
	return nil
}

// 批量保存时序数据库
func (p *dataLogic) SaveBatchInflux(ps []*client.Point) error {
	bp, err := client.NewBatchPoints(client.BatchPointsConfig{
		Database:        influx.DB,
		Precision:       "s",
		RetentionPolicy: "autogen",
	})
	if err != nil {
		return err
	}
	bp.AddPoints(ps)
	if influx.Mod == "http" {
		if err := influx.Client.Write(bp); err != nil {
			return err
		}
	} else {
		if err := influx.UDPClient.Write(bp); err != nil {
			return err
		}
	}
	return nil
}
