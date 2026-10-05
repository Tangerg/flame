package execution

import (
	"context"
	"encoding/json/jsontext"
	"errors"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/fingerprint"
	"github.com/Tangerg/flame/runtime/internal/testsupport"

	"github.com/Tangerg/flame/runtime/internal/adapter/toolset"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/core/chat"
	"github.com/Tangerg/scope/core/metadata"
	toolcontract "github.com/Tangerg/scope/core/tool"
)

func TestInteractionToolDeploymentBindsItsFrozenManifest(t *testing.T) {
	binding := func(t *testing.T, description, schema string, fingerprint fingerprint.Digest, deferred bool) (agent.DeploymentRef, agent.DeploymentRef) {
		t.Helper()
		executable := &scopeDefinitionTool{definition: chat.ToolDefinition{
			Name: "echo", Description: description, InputSchema: []byte(schema),
		}}
		identified, err := toolset.WithIdentity(executable, testsupport.A2ATool(t, "echo"), fingerprint)
		if err != nil {
			t.Fatal(err)
		}
		manifest := toolset.Manifest{Visible: []toolcontract.Tool{identified}}
		if deferred {
			manifest.Visible, manifest.Deferred = nil, manifest.Visible
		}
		executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
			return nil, errors.New("deployment assembly must not call the model")
		}), InteractionExecutorConfig{
			ToolResolver:    staticInteractionTools{manifest: manifest},
			ToolInterpreter: testInteractionToolInterpreter{}, ToolAuthorizer: allowInteractionTools{},
		})
		ref, err := executor.StageRoot(t.Context(), interactionTestStart())
		if err != nil {
			t.Fatal(err)
		}
		session, err := executor.session(ref)
		if err != nil {
			t.Fatal(err)
		}
		children := session.deployment.Definition().ChildDeployments()
		if len(children) != 1 {
			t.Fatalf("static Tool deployments = %d, want 1", len(children))
		}
		root, child := session.deployment.DeploymentRef(), children[0].DeploymentRef()
		if err := executor.Release(t.Context(), ref); err != nil {
			t.Fatal(err)
		}
		return root, child
	}

	const schema = `{"type":"object","properties":{"revision":{"type":"integer","const":9007199254740992}}}`
	authority := testsupport.Digest("first authority")
	root, child := binding(t, "Return a fixed response.", schema, authority, false)
	for _, test := range []struct {
		name        string
		description string
		schema      string
		fingerprint fingerprint.Digest
		deferred    bool
		changed     bool
	}{
		{name: "same manifest", description: "Return a fixed response.", schema: schema},
		{name: "changed authority", description: "Return a fixed response.", schema: schema, fingerprint: testsupport.Digest("second authority"), changed: true},
		{name: "changed contract", description: "Return the configured response.", schema: schema, changed: true},
		{name: "deferred authority", description: "Return a fixed response.", schema: schema, deferred: true, changed: true},
		{name: "reordered schema", description: "Return a fixed response.", schema: `{"properties":{"revision":{"const":9007199254740992,"type":"integer"}},"type":"object"}`},
		{name: "changed exact schema number", description: "Return a fixed response.", schema: `{"type":"object","properties":{"revision":{"type":"integer","const":9007199254740993}}}`, changed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			currentFingerprint := authority
			if !test.fingerprint.IsZero() {
				currentFingerprint = test.fingerprint
			}
			candidateRoot, candidateChild := binding(t, test.description, test.schema, currentFingerprint, test.deferred)
			if changed := candidateChild != child; changed != test.changed {
				t.Fatalf("Tool deployment identity changed = %t, want %t", changed, test.changed)
			}
			if changed := candidateRoot.BindingsDigest() != root.BindingsDigest(); changed != test.changed {
				t.Fatalf("static Tool binding changed = %t, want %t", changed, test.changed)
			}
		})
	}
}

func TestInteractionDeploymentBindsCanonicalInstructions(t *testing.T) {
	binding := func(t *testing.T, rawMetadata string) agent.DeploymentRef {
		t.Helper()
		instruction := chat.NewSystemMessage("Follow the workspace policy.")
		instruction.Metadata = metadata.Map{"policy": jsontext.Value(rawMetadata)}
		if err := (contextSources{contextSourceBasePrompt.source("policy")}).attach(&instruction.Metadata, "message"); err != nil {
			t.Fatal(err)
		}
		executor := newObservedTestInteractionExecutor(t, chat.ModelFunc(func(context.Context, *chat.Request) (*chat.Response, error) {
			return nil, errors.New("deployment assembly must not call the model")
		}), InteractionExecutorConfig{})
		start := interactionTestStart()
		start.WorkingContext = append([]chat.Message{instruction}, start.WorkingContext...)
		ref, err := executor.StageRoot(t.Context(), start)
		if err != nil {
			t.Fatal(err)
		}
		session, err := executor.session(ref)
		if err != nil {
			t.Fatal(err)
		}
		deployment := session.deployment.DeploymentRef()
		if err := executor.Release(t.Context(), ref); err != nil {
			t.Fatal(err)
		}
		return deployment
	}

	root := binding(t, `{"id":9007199254740992,"revision":1}`)
	for _, test := range []struct {
		name     string
		metadata string
		changed  bool
	}{
		{name: "reordered metadata", metadata: `{ "revision": 1, "id": 9007199254740992 }`},
		{name: "changed exact metadata number", metadata: `{"id":9007199254740993,"revision":1}`, changed: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			candidate := binding(t, test.metadata)
			if changed := candidate != root; changed != test.changed {
				t.Fatalf("Interaction deployment identity changed = %t, want %t", changed, test.changed)
			}
		})
	}
}
