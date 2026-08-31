package segment

import runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"

func testCommitID(raw string) runtimeidentity.CommitID {
	identity, err := runtimeidentity.ParseCommit(raw)
	if err != nil {
		panic(err)
	}
	return identity
}
