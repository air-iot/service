package tsdb

import (
	"github.com/air-iot/service/config"
	"math/rand"
	"testing"
	"time"
)

func TestWrite(t *testing.T) {
	cfg := config.TSDB{
		DBType: "influx",
		Taos: config.Taos{
			Addr:    "root:taosdata@/tcp(taos:0)/",
			MaxConn: 10,
			Timeout: 10,
			DBName:  "test",
		},
		Influx: config.Influx{
			Protocol: "UDP",
			Addr:     "air.htkjbjf.com:18089",
			Username: "admin",
			Password: "dell123",
			DBName:   "tsdb",
		},
	}

	tsdb, clean, err := NewTSDB(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer clean()
	err = tsdb.Write(cfg.Taos.DBName, []Row{
		{
			TableName:    "model1",
			SubTableName: "node1",
			Ts:           time.Now(),
			Tags:         map[string]string{"id": "node11"},
			Fields:       map[string]interface{}{"f1": rand.Intn(10), "d3": "ab"},
		},
	})

	if err != nil {
		t.Fatal(err)
	}
}

func TestQuery(t *testing.T) {
	cfg := config.TSDB{
		DBType: "influx",
		Taos: config.Taos{
			Addr:    "root:taosdata@/tcp(taos:0)/",
			MaxConn: 10,
			Timeout: 10,
			DBName:  "test",
		},
		Influx: config.Influx{
			Protocol: "HTTP",
			Addr:     "http://taos:18086",
			Username: "admin",
			Password: "dell123",
			DBName:   "tsdb",
		},
	}

	tsdb, clean, err := NewTSDB(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer clean()
	res, err := tsdb.Query(cfg.Influx.DBName, "select * from model1")

	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", res)
}
