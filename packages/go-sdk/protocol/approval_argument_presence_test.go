package protocol

import (
	"bytes"
	"encoding/json/v2"
	"testing"
)

func TestApprovalArgumentsPreserveOmittedAndEmpty(t *testing.T) {
	var encodings [][]byte
	for _, arguments := range []map[string]any{nil, {}, {"target": "chosen"}} {
		value := InterruptResponseValue{
			Type: InterruptResponseApproval, Decision: ApprovalApprove, EditedArgs: arguments,
		}
		encoded, err := json.Marshal(value, json.Deterministic(true))
		if err != nil {
			t.Fatal(err)
		}
		encodings = append(encodings, encoded)
		var decoded InterruptResponseValue
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if (decoded.EditedArgs == nil) != (arguments == nil) || len(decoded.EditedArgs) != len(arguments) {
			t.Fatalf("argument presence changed through %s", encoded)
		}
	}
	if bytes.Equal(encodings[0], encodings[1]) || !bytes.Contains(encodings[1], []byte(`"editedArgs":{}`)) {
		t.Fatalf("omitted and explicit empty arguments share an encoding: %q", encodings)
	}
}
