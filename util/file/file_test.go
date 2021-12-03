package file

import "testing"

func TestExists(t *testing.T) {
	t.Log(Exists("./file.go"))
	t.Log(Exists("../file.go"))
}
