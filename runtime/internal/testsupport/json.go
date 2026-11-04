package testsupport

import (
	"testing"

	framevalue "github.com/Tangerg/frame/value"
)

func JSONInput(t testing.TB, data string) framevalue.JSON {
	t.Helper()
	input, err := framevalue.ParseJSON([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return input
}
