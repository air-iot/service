package tsdb

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"github.com/air-iot/service/config"
)

func TestWrite(t *testing.T) {
	cfg := config.TSDB{
		DBType: "influx",
		DBName: "test",
		Taos: config.Taos{
			Addr:    "root:taosdata@/tcp(taos:0)/",
			MaxConn: 10,
			Timeout: 10,
		},
		Influx: config.Influx{
			Protocol: "UDP",
			Addr:     "taos:18089",
			Username: "admin",
			Password: "dell123",
		},
	}

	tsdb, clean, err := NewTSDB(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer clean()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	err = tsdb.Write(ctx, cfg.DBName, []Row{
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
		DBName: "tsdb",
		Taos: config.Taos{
			Addr:    "root:taosdata@/tcp(taos:0)/",
			MaxConn: 10,
			Timeout: 10,
		},
		Influx: config.Influx{
			Protocol: "HTTP",
			Addr:     "http://taos:18086",
			Username: "admin",
			Password: "dell123",
		},
	}

	tsdb, clean, err := NewTSDB(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer clean()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	res, err := tsdb.Query(ctx, cfg.DBName, "select * from model1")

	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", res)
}
