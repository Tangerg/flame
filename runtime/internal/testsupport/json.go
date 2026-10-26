package testsupport

import (
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/jsoninput"
)

func JSONInput(t testing.TB, data string) jsoninput.Input {
	t.Helper()
	input, err := jsoninput.Parse([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return input
}
