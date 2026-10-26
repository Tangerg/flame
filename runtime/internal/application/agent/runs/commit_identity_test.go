package runs

import runtimeidentity "github.com/Tangerg/flame/packages/go-sdk/identity"

func testCommitID(raw string) runtimeidentity.CommitID {
	identity, err := runtimeidentity.ParseCommit(raw)
	if err != nil {
		panic(err)
	}
	return identity
}
