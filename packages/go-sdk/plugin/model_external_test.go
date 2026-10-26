package plugin_test

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"

	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	sdk "github.com/Tangerg/go-sdk/mcp"
	"github.com/Tangerg/scope/core/chat"
)

func TestExecutableUsesThePublicCapturedModelContract(t *testing.T) {
	if encoded := os.Getenv("FLAME_TEST_MODEL_CALLBACK"); encoded != "" {
		var ref pluginsdk.CallbackRef
		if err := json.Unmarshal([]byte(encoded), &ref); err != nil {
			t.Fatal(err)
		}
		request, err := chat.NewRequest(chat.NewUserMessage(chat.NewTextPart("question")))
		if err != nil {
			t.Fatal(err)
		}
		response, err := pluginsdk.CallModel(t.Context(), ref, "executable-model-call", request)
		if !errors.Is(err, context.DeadlineExceeded) || response == nil || response.Text() != "known inner answer" || response.Metadata.Usage.InputTokens != 9 {
			t.Fatalf("known inner result/error lost: response=%+v, error=%v", response, err)
		}
		return
	}
	host := pluginsdk.NewCallbacks()
	t.Cleanup(func() {
		if err := host.Close(); err != nil {
			t.Error(err)
		}
	})
	session := new(sdk.ClientSession)
	host.AdmitSession(session)
	callback, err := pluginsdk.NewModelCallback(func(_ context.Context, command pluginsdk.Command[chat.Request]) (*chat.Response, error) {
		if command.ID != "executable-model-call" {
			return nil, errors.New("model identity was replaced")
		}
		message := chat.NewAssistantMessage(chat.NewTextPart("known inner answer"))
		return &chat.Response{Output: &chat.Output{Message: &message, FinishReason: chat.FinishReasonStop}, Metadata: &chat.ResponseMetadata{Usage: &chat.Usage{InputTokens: 9, OutputTokens: 3}}}, fmt.Errorf("outer decoration: %w", context.DeadlineExceeded)
	})
	if err != nil {
		t.Fatal(err)
	}
	ref, release, err := host.Open(t.Context(), session, pluginsdk.CallbackLimits{MaxRequestBytes: 64 * 1024, MaxResponseBytes: 64 * 1024}, callback)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	encoded, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestExecutableUsesThePublicCapturedModelContract$", "-test.count=1")
	command.Env = append(os.Environ(), "FLAME_TEST_MODEL_CALLBACK="+string(encoded))
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("public executable model call failed: %s: %v", output, err)
	}
}
