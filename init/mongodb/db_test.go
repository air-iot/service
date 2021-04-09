package mongodb

import "testing"

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
