package logic

import (
	"testing"
)

func Test_dataLogicFindCacheByUIDAndTagID(t *testing.T) {
	r, err := DataLogic.FindCacheByUIDAndTagID("0709105488", "IA")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", *r)
}

func Test_dataLogicFindCacheByUIDAndTagIDs(t *testing.T) {
	r, err := DataLogic.FindCacheByUIDAndTagIDs("0709105488", []string{"IA", "IB", "IC"})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%+v", r)
}
