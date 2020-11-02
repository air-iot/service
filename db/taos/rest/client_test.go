package taos_rest

import "testing"

func TestExec(t *testing.T) {
	var result = make(map[string]interface{})
	resp, err := Exec("select * from tsdb.t1", &result)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(result)
	t.Log(resp)
}
