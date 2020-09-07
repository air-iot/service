package user

import (
	"github.com/air-iot/service/traefik"
	"testing"
)

func init() {
	traefik.Host = "iot.tmis.top"
	traefik.Port = 31000
	traefik.Enable = true
	traefik.AppKey = "b9bd592b-2d79-4f5c-d583-aad18ebe00ca"
	traefik.AppSecret = "c5de1068-79fd-b32b-a4f8-291c337111fa"
}

func TestGatewayClient_FindQuery(t *testing.T) {
	cli := NewGatewayClient()
	var r = make([]map[string]interface{}, 0)
	// query := `{"filter":{"device.driver":"test","$lookups":[{"from":"node","localField":"_id","foreignField":"model","as":"devices"},{"from":"node","localField":"devices.parent","foreignField":"_id","as":"devicesParent"},{"from":"model","localField":"devicesParent.model","foreignField":"_id","as":"devicesParentModel"}]},"project":{"device":1,"devices":1,"devicesParent":1,"devicesParentModel":1}}`

	queryMap := make(map[string]interface{})
	// err := json.Unmarshal([]byte(query), &queryMap)
	// if err != nil {
	// 	t.Fatal(err)
	// }
	err := cli.FindQuery(&queryMap, &r)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(r)
}
