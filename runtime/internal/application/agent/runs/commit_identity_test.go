package runs

func testCommitID(raw string) CommitID {
	identity, err := ParseCommit(raw)
	if err != nil {
		panic(err)
	}
	return identity
}
