package agent

import "testing"

func TestStreamedTextAppendsInEventOrderAndRejectsEmptyDeltas(t *testing.T) {
	stream := NewStreamedText("first")
	if err := stream.Apply(BlockDelta{BlockID: "answer", Text: " second"}); err != nil {
		t.Fatal(err)
	}
	if got := stream.String(); got != "first second" {
		t.Fatalf("streamed text = %q", got)
	}
	if err := stream.Apply(BlockDelta{BlockID: "answer"}); err == nil {
		t.Fatal("Apply accepted an empty delta")
	}
}
