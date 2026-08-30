package runs

import "github.com/Tangerg/flame/runtime/internal/commitidentity"

func testCommitID(raw string) commitidentity.ID {
	identity, err := commitidentity.Parse(raw)
	if err != nil {
		panic(err)
	}
	return identity
}
