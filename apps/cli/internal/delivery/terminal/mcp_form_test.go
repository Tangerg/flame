package terminal

import (
	json "encoding/json/v2"
	"maps"
	"slices"
	"testing"

	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	"github.com/Tangerg/oolong/components/headless"
	"github.com/Tangerg/oolong/core/input"
	"github.com/Tangerg/oolong/core/program"
)

func TestMCPFormPreservesExactProcessMaterial(t *testing.T) {
	args := []string{"--label", " value ", "", "first\nsecond"}
	server := protocol.MCPServer{
		ID: protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginUser}, Name: "literal"},
		Connection: protocol.MCPConnection{
			Type: protocol.MCPTransportStdio, Command: "/tools/mcp ", Args: args, Dir: "/work/project ",
		},
		HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded},
	}
	draft := newMCPFormDraft(mcpFormUpdate, server)
	draft.description = "changed"
	update, changed, err := draft.update(server)
	if err != nil || !changed || update.Connection != nil {
		t.Fatalf("metadata edit = %+v, %t, %v", update, changed, err)
	}
	draft.replaceConnection = true
	connection, err := draft.connection()
	if err != nil || connection.Command != server.Connection.Command || connection.Directory != server.Connection.Dir ||
		!slices.Equal(connection.Args, args) || connection.Environment != nil {
		t.Fatalf("connection = %+v, %v", connection, err)
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	draft = newMCPFormDraft(mcpFormCreate, protocol.MCPServer{})
	draft.name, draft.transport, draft.command, draft.directory, draft.arguments = "literal", protocol.MCPTransportStdio, " mcp ", " /project ", string(encoded)
	candidate, err := draft.candidate()
	if err != nil || candidate.Connection.Command != " mcp " || candidate.Connection.Directory != " /project " || !slices.Equal(candidate.Connection.Args, args) {
		t.Fatalf("candidate = %+v, %v", candidate, err)
	}
}

func TestMCPFormPreservesReplacementAuthorization(t *testing.T) {
	change, err := mcpAuthorizationChange(formChangeSet, " Bearer literal ")
	if err != nil || change.Value != " Bearer literal " {
		t.Fatalf("form changed authorization before Runtime admission: %+v, %v", change, err)
	}
}

func TestMCPEnvironmentFormPreservesProcessValues(t *testing.T) {
	change, err := mcpEnvironmentChange(formChangeSet, `{"EMPTY":"","SPACES":" \t "," PADDED ":" value "}`)
	if err != nil {
		t.Fatal(err)
	}
	if change.Kind != protocol.MCPSecretSet || !maps.Equal(change.Value, map[string]string{"EMPTY": "", "SPACES": " \t ", " PADDED ": " value "}) {
		t.Fatalf("environment change = %#v", change)
	}
	for _, input := range []string{"", "null", "{}", `{"EMPTY":2}`, `{"EMPTY":"","EMPTY":"duplicate"}`} {
		if _, err := mcpEnvironmentChange(formChangeSet, input); err == nil {
			t.Errorf("accepted invalid edit %s", input)
		}
	}
}

func TestMCPFormRejectsSubmissionFromAReplacedStepPresentation(t *testing.T) {
	transcript := testTranscriptView(t)
	application := &app{loop: &program.Runtime{}, transcript: transcript}
	application.stack.SetBase(transcript)
	flow := newMCPFormFlow(mcpFormCreate, protocol.MCPServer{})
	flow.draft.name = "docs"
	flow.draft.transport = protocol.MCPTransportStreamableHTTP
	application.showMCPFormStep(flow)
	drawRoot(t, &application.stack, 96, 28)

	oldDialog := application.dialogs.mcpDialog
	application.showMCPFormStep(flow)
	application.stack.Handle(input.Key{Code: input.Enter})
	if flow.step != mcpFormGeneral {
		t.Fatalf("stale form advanced to step %d", flow.step)
	}
	if application.dialogs.mcpDialog == oldDialog || !application.dialogs.mcpDialog.Controller().Open() {
		t.Fatal("stale form replaced or dismissed the current dialog")
	}

	drawRoot(t, &application.stack, 96, 28)
	application.stack.Handle(input.Key{Code: input.Enter})
	if flow.step != mcpFormHTTP {
		t.Fatalf("current form advanced to step %d, want HTTP", flow.step)
	}
	application.closeMCPForm(flow)
}

func TestMCPFormFlowRoutesOnlyThroughRelevantConnection(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		mode              mcpFormMode
		transport         protocol.MCPTransport
		replaceConnection bool
		want              []mcpFormStep
	}{
		{name: "create HTTP", mode: mcpFormCreate, transport: protocol.MCPTransportStreamableHTTP, want: []mcpFormStep{mcpFormGeneral, mcpFormHTTP, mcpFormPolicy}},
		{name: "create stdio", mode: mcpFormCreate, transport: protocol.MCPTransportStdio, want: []mcpFormStep{mcpFormGeneral, mcpFormStdio, mcpFormPolicy}},
		{name: "update keeping connection", mode: mcpFormUpdate, transport: protocol.MCPTransportStreamableHTTP, want: []mcpFormStep{mcpFormGeneral, mcpFormPolicy}},
		{name: "update replacing connection", mode: mcpFormUpdate, transport: protocol.MCPTransportStdio, replaceConnection: true, want: []mcpFormStep{mcpFormGeneral, mcpFormStdio, mcpFormPolicy}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			flow := newMCPFormFlow(test.mode, protocol.MCPServer{})
			flow.draft.transport = test.transport
			flow.draft.replaceConnection = test.replaceConnection
			visited := []mcpFormStep{flow.step}
			for flow.advance() {
				visited = append(visited, flow.step)
			}
			if !slices.Equal(visited, test.want) {
				t.Fatalf("forward steps = %v, want %v", visited, test.want)
			}
			for index := len(test.want) - 2; index >= 0; index-- {
				if !flow.back() || flow.step != test.want[index] {
					t.Fatalf("back step = %v, want %v", flow.step, test.want[index])
				}
			}
			if flow.back() {
				t.Fatal("flow moved before the general step")
			}
		})
	}
}

func TestMCPFormFlowClearsEverySecretProjection(t *testing.T) {
	t.Parallel()
	flow := newMCPFormFlow(mcpFormCreate, protocol.MCPServer{})
	flow.draft.authorization = "Bearer private"
	flow.draft.headers = `{"X-Key":"private"}`
	flow.draft.environment = `{"TOKEN":"private"}`
	for range 3 {
		field := &headless.Text{}
		field.SetText("private")
		flow.secretFields = append(flow.secretFields, field)
	}
	fields := slices.Clone(flow.secretFields)

	flow.clearSecrets()
	if flow.draft.authorization != "" || flow.draft.headers != "" || flow.draft.environment != "" || flow.secretFields != nil {
		t.Fatalf("secret state survived cleanup: %+v", flow)
	}
	for index, field := range fields {
		if value := field.Text(); value != "" {
			t.Fatalf("secret field %d = %q after cleanup", index, value)
		}
	}
}

func TestParseMCPTimeoutDistinguishesUnboundedFromPositiveDeadline(t *testing.T) {
	unbounded, err := parseMCPTimeout("")
	if _, bounded := unbounded.Seconds(); err != nil || bounded {
		t.Fatalf("blank timeout = (%v, %v), want unbounded", unbounded, err)
	}
	bounded, err := parseMCPTimeout("15")
	seconds, ok := bounded.Seconds()
	if err != nil || !ok || seconds != 15 {
		t.Fatalf("positive timeout = (%v, %v), want 15s", bounded, err)
	}
	for _, invalid := range []string{"0", "-1", "1.5", "word"} {
		if _, err := parseMCPTimeout(invalid); err == nil {
			t.Errorf("parseMCPTimeout(%q) accepted", invalid)
		}
	}
}
