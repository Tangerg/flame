package testsupport

import (
	"testing"

	"github.com/Tangerg/flame/qimu/jsonvalue"
)

func JSONInput(t testing.TB, data string) jsonvalue.Value {
	t.Helper()
	input, err := jsonvalue.Parse([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return input
}
