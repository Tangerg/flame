package scopewire

import (
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"strings"
	"testing"

	"github.com/Tangerg/frame/portable"
	"github.com/Tangerg/frame/value"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/tool"
)

func TestPortableProjectionsPreserveCompleteScopePayloads(t *testing.T) {
	request := `{"messages":[{"role":"user","parts":[{"kind":"media","media":{"mime":"image/png","source":{"kind":"bytes","bytes":"AAH/"},"id":"image","name":"reference","metadata":{"revision":9007199254740993}}}]},{"role":"assistant","parts":[{"kind":"reasoning","text":"basis","reasoning_state":"AAH/"}]},{"role":"tool","parts":[{"kind":"tool_result","tool_result":{"id":"call","name":"lookup","output":{"details":{"revision":9007199254740993}},"is_error":true}}]}],"options":{"temperature":0.7,"frequency_penalty":0.1,"presence_penalty":-0.2,"top_p":0.9,"max_output_tokens":64}}`
	var native chat.Request
	if err := native.UnmarshalJSON([]byte(request)); err != nil {
		t.Fatal(err)
	}
	checkPortableRoundTrip(t, Request(native))
	for _, message := range native.Messages {
		checkPortableRoundTrip(t, Message(message))
	}
	response := `{"output":{"message":{"role":"assistant","parts":[{"kind":"text","text":"answer","citations":[{"source":{"kind":"uri","value":"https://example.test/source"},"title":"source","quote":"evidence"}]}]},"finish_reason":"stop"},"metadata":{"created_at":"2026-11-10T00:00:00.123Z","usage":{"input_tokens":13,"output_tokens":7},"extra":{"revision":9007199254740993}}}`
	var completed chat.Response
	if err := completed.UnmarshalJSON([]byte(response)); err != nil {
		t.Fatal(err)
	}
	checkPortableRoundTrip(t, Response(completed))
	delta := chat.ResponseDelta{
		Parts:    []chat.PartDelta{chat.NewReasoningDelta("basis", []byte{0, 1, 255})},
		Metadata: completed.Metadata,
	}
	checkPortableRoundTrip(t, ResponseDelta(delta))
	block, ok := native.Messages[0].Parts[0].Content()
	if !ok {
		t.Fatal("media part did not project content")
	}
	output := tool.Output{Details: []byte(`{"revision":9007199254740993}`)}
	output.Content = append(output.Content, block)
	checkPortableRoundTrip(t, ToolOutput(output))
	for _, block := range output.Content {
		checkPortableRoundTrip(t, Block(block))
	}
}

func TestRequestOptionConversionPreservesAuthoredNumbers(t *testing.T) {
	const request = `{"messages":[{"role":"user","parts":[{"kind":"text","text":"question"}]}],"options":{"temperature":NUMBER}}`
	for _, number := range []string{"0.7", "0.70", "7e-1", "0", "0e-1000000000", "2"} {
		var decoded Request
		if err := value.DecodeValue(jsontext.Value(strings.Replace(request, "NUMBER", number, 1)), &decoded, "request"); err != nil {
			t.Errorf("native option %s was refused: %v", number, err)
		}
	}
	for _, number := range []string{"0.700000000000000000001", "1e-400", "1e-1000000000", "1e400"} {
		var decoded Request
		if err := value.DecodeValue(jsontext.Value(strings.Replace(request, "NUMBER", number, 1)), &decoded, "request"); err == nil {
			t.Errorf("native option %s silently changed its numeric value", number)
		}
	}
}

func TestProjectionCannotBypassScopeAdmissionOrPublishPartialDecoding(t *testing.T) {
	original, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("original")))
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"messages":[]}`,
		`{"messages":[{"role":"user","parts":[{"kind":"media","media":{"source":{"kind":"bytes","bytes":"invalid base64"}}}]}]}`,
		`{"messages":[{"role":"user","parts":[{"kind":"text","text":"question"}]}],"options":{"temperature":3}}`,
		`{"messages":[{"role":"user","parts":[{"kind":"text","text":"question","extra":true}]}]}`,
		`{"messages":null}`,
	} {
		decoded := Request(*original.Clone())
		if err := decoded.UnmarshalJSON([]byte(raw)); err == nil {
			t.Errorf("invalid native request was admitted: %s", raw)
		}
		if decoded.Messages[0].Text() != "original" {
			t.Fatal("refused decoding changed the original request")
		}
	}
}

func checkPortableRoundTrip[T any](t *testing.T, original T) {
	t.Helper()
	definition, err := portable.SchemaFor[T]()
	if err != nil {
		t.Fatal(err)
	}
	schema, err := portable.AdmitSchema(definition)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(encoded); err != nil {
		t.Fatal(err)
	}
	var decoded T
	if err := value.DecodeValue(encoded, &decoded, "scope projection"); err != nil {
		t.Fatal(err)
	}
	restored, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	before, err := value.ParseJSON(encoded)
	if err != nil {
		t.Fatal(err)
	}
	after, err := value.ParseJSON(restored)
	if err != nil || before != after {
		t.Fatalf("portable round trip changed Scope payload: %s -> %s: %v", encoded, restored, err)
	}
}
