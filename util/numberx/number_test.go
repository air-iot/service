package numberx

import "testing"

func TestRound(t *testing.T) {
	f := 63.442921
	f = Round(f, 2)
	t.Log(f)
}
