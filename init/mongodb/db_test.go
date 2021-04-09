package mongodb

import (
	"context"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"testing"
)

func TestConvertKeyUnderlineID(t *testing.T) {
	a := map[string]interface{}{
		"_id": 1,
		"t1": map[string]interface{}{
			"_id": 2,
			"t3":  []interface{}{map[string]interface{}{"_id": 3}},
		},
	}

	//a = ConvertKeyUnderlineID(a).(map[string]interface{})
	ConvertKeyUnderlineID(a)

	t.Log(a)
}

func TestQueryOptionToPipeline(t *testing.T) {

	opts := options.Client().
		ApplyURI("mongodb://root:dell123@localhost:27017/admin")

	cli, err := mongo.NewClient(opts)
	if err != nil {
		t.Fatal(err)
	}

	if err := cli.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	withCount := true
	query := QueryOption{
		Filter:    map[string]interface{}{"name": "admin"},
		WithCount: &withCount,
	}

	p1, p2, err := QueryOptionToPipeline(query)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("p1,%+v \n", p1)
	t.Logf("p2,%+v \n", p2)
	//c, err := FindCount(context.Background(), cli.Database("base").Collection("user"), p2)
	//if err != nil {
	//	t.Fatal(err)
	//}
	//
	result := make([]map[string]interface{}, 0)
	//c, err := FindFilter(context.Background(), cli.Database("base").Collection("user"), &result, query)
	//if err != nil {
	//	t.Fatal(err)
	//}
	//t.Log(*c)
	err = FindPipeline(context.Background(), cli.Database("base").Collection("user"), &result, p1)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(result)
}
