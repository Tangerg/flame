package segment

import "github.com/Tangerg/flame/runtime/internal/application/agent/runs"

func testCommitID(raw string) runs.CommitID {
	identity, err := runs.ParseCommit(raw)
	if err != nil {
		panic(err)
	}
	return identity
}
