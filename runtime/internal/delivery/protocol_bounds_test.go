package delivery_test

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"

	hookdata "github.com/Tangerg/flame/packages/go-sdk/hook"
	"github.com/Tangerg/flame/packages/go-sdk/protocol"
	questioncontract "github.com/Tangerg/flame/packages/go-sdk/question"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/hooks"
	"github.com/Tangerg/flame/runtime/internal/domain/integration/mcpserver"
	"github.com/Tangerg/flame/runtime/internal/domain/run/approval"
)

func wireUserServer(name string) protocol.MCPServerID {
	return protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginUser}, Name: name}
}
func TestQuestionWireConstraints(t *testing.T) {
	t.Parallel()

	valid := protocol.Question{Fields: []protocol.QuestionField{{
		Type:    protocol.QuestionFieldChoice,
		Prompt:  "Choose",
		Header:  "选择一下",
		Options: []protocol.QuestionOption{{Label: "A"}, {Label: "B"}},
	}}}
	if err := protocol.ValidateWireTree(valid); err != nil {
		t.Fatalf("ValidateWireTree rejected a valid question: %v", err)
	}

	oneOption := valid
	oneOption.Fields = slices.Clone(valid.Fields)
	oneOption.Fields[0].Options = []protocol.QuestionOption{{Label: "A"}}
	assertProtocolConstraintField(t, protocol.ValidateWireTree(oneOption), "Question", "fields[0].options")

	tooManyOptions := valid
	tooManyOptions.Fields = slices.Clone(valid.Fields)
	tooManyOptions.Fields[0].Options = []protocol.QuestionOption{{Label: "A"}, {Label: "B"}, {Label: "C"}, {Label: "D"}, {Label: "E"}}
	assertProtocolConstraintField(t, protocol.ValidateWireTree(tooManyOptions), "Question", "fields[0].options")

	blankPrompt := valid
	blankPrompt.Fields = slices.Clone(valid.Fields)
	blankPrompt.Fields[0].Prompt = " \t "
	assertProtocolConstraintField(t, protocol.ValidateWireTree(blankPrompt), "Question", "fields[0].prompt")

	blankOption := valid
	blankOption.Fields = slices.Clone(valid.Fields)
	blankOption.Fields[0].Options = slices.Clone(valid.Fields[0].Options)
	blankOption.Fields[0].Options[0].Label = "\n"
	assertProtocolConstraintField(t, protocol.ValidateWireTree(blankOption), "Question", "fields[0].options[0].label")

	tooManyFields := protocol.Question{Fields: make([]protocol.QuestionField, questioncontract.MaximumFields+1)}
	for index := range tooManyFields.Fields {
		tooManyFields.Fields[index] = protocol.QuestionField{Type: protocol.QuestionFieldText, Prompt: "Answer"}
	}
	assertProtocolConstraintField(t, protocol.ValidateWireTree(tooManyFields), "Question", "fields")

	longHeader := valid
	longHeader.Fields = slices.Clone(valid.Fields)
	longHeader.Fields[0].Header = "一二三四五六七八九十一二三"
	assertProtocolConstraintField(t, protocol.ValidateWireTree(longHeader), "Question", "fields[0].header")
}

func TestApprovalRuleListHasOneRuntimeCapacity(t *testing.T) {
	t.Parallel()
	maximum := protocol.ListApprovalRulesResult{Rules: make([]protocol.ApprovalRule, approval.MaximumVisibleRules)}
	for index := range maximum.Rules {
		maximum.Rules[index] = protocol.ApprovalRule{
			ID:        fmt.Sprintf("rule_%d", index),
			ModelName: "edit",
			Scope:     protocol.ApprovalRuleScopeSession,
			Tool:      protocol.ToolRef{Type: protocol.ToolRefBuiltIn, Name: "edit"},
			Subject:   protocol.ApprovalSubject{Type: protocol.ApprovalSubjectAll},
			Decision:  protocol.ApprovalRuleDecisionAllow,
		}
	}
	if err := protocol.ValidateWireTree(maximum); err != nil {
		t.Fatalf("maximum approval rule list: %v", err)
	}
	overCapacity := maximum
	overCapacity.Rules = append(overCapacity.Rules, maximum.Rules[0])
	assertProtocolConstraintField(t, protocol.ValidateWireTree(overCapacity), "ListApprovalRulesResult", "rules")
}

func TestMCPServerIdentityUsesCanonicalWireGrammar(t *testing.T) {
	maximum := wireUserServer(strings.Repeat("a", mcpserver.MaximumServerNameCharacters))
	installed := protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: "eeb329cd-c7ce-40c9-bd90-6821fef06d30"}, Name: "files"}
	valid := []any{protocol.MCPServerRequest{Server: maximum}, protocol.MCPServerRequest{Server: installed}, protocol.CreateMCPAuthorizationAttemptRequest{Server: maximum}, protocol.MCPListToolsRequest{Server: &installed}, protocol.MCPServerCandidate{Name: maximum.Name, Connection: protocol.MCPConnectionInput{Type: protocol.MCPTransportStdio, Command: "files"}, HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded}}, protocol.UpdateMCPServerRequest{Server: maximum}, protocol.MCPServer{ID: installed, Connection: protocol.MCPConnection{Type: protocol.MCPTransportStdio, Command: "files"}, HandshakeTimeout: protocol.MCPHandshakeTimeout{Type: protocol.MCPHandshakeUnbounded}, Status: protocol.MCPServerState{Type: protocol.MCPServerDisabled}}, protocol.MCPTool{Server: maximum, Name: "read", ModelName: "source_read"}}
	for _, value := range valid {
		if err := protocol.ValidateWireTree(value); err != nil {
			t.Errorf("ValidateWireTree rejected canonical %T identity: %v", value, err)
		}
	}
	if err := protocol.ValidateWireTree(protocol.MCPListToolsRequest{}); err != nil {
		t.Fatalf("optional all-server filter rejected omission: %v", err)
	}

	invalid := wireUserServer(strings.Repeat("a", mcpserver.MaximumServerNameCharacters+1))
	spaced := wireUserServer("with space")
	tests := []struct {
		shape string
		field string
		err   error
	}{
		{"MCPServerRequest", "server.name", protocol.ValidateWireTree(protocol.MCPServerRequest{Server: invalid})},
		{"CreateMCPAuthorizationAttemptRequest", "server.name", protocol.ValidateWireTree(protocol.CreateMCPAuthorizationAttemptRequest{Server: wireUserServer("GitHub")})},
		{"MCPListToolsRequest", "server.name", protocol.ValidateWireTree(protocol.MCPListToolsRequest{Server: &spaced})},
		{"MCPServerCandidate", "name", protocol.ValidateWireTree((protocol.MCPServerCandidate{Name: "server/name"}))},
		{"UpdateMCPServerRequest", "server.name", protocol.ValidateWireTree(protocol.UpdateMCPServerRequest{Server: invalid})},
		{"MCPServer", "id.name", protocol.ValidateWireTree(protocol.MCPServer{ID: wireUserServer("UPPER")})},
		{"MCPTool", "server.name", protocol.ValidateWireTree(protocol.MCPTool{Server: wireUserServer(" server")})},
		{"MCPServerRequest", "server.origin.installationId", protocol.ValidateWireTree(protocol.MCPServerRequest{Server: protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation}, Name: "files"}})},
		{"MCPServerRequest", "server.origin.installationId", protocol.ValidateWireTree(protocol.MCPServerRequest{Server: protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginUser, InstallationID: installed.Origin.InstallationID}, Name: "files"}})},
		{"MCPServerRequest", "server.origin.installationId", protocol.ValidateWireTree(protocol.MCPServerRequest{Server: protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: protocol.MCPOriginInstallation, InstallationID: "installation/x"}, Name: "files"}})},
		{"MCPServerRequest", "server.origin.type", protocol.ValidateWireTree(protocol.MCPServerRequest{Server: protocol.MCPServerID{Origin: protocol.MCPOrigin{Type: "plugin"}, Name: "files"}})},
	}
	for _, test := range tests {
		assertProtocolConstraintField(t, test.err, test.shape, test.field)
	}
}

func TestMCPRemoteToolIdentityUsesCanonicalWireGrammar(t *testing.T) {
	files := wireUserServer("files")
	maximum := strings.Repeat("a", mcpserver.MaximumRemoteToolNameCharacters)
	valid := []any{protocol.MCPToolExposure{Server: files, DisabledTools: []string{maximum}}, protocol.SetMCPToolExposureRequest{Server: files, Name: maximum, Disabled: true}, protocol.MCPTool{Server: files, Name: maximum, ModelName: "files_read"}}
	for _, value := range valid {
		if err := protocol.ValidateWireTree(value); err != nil {
			t.Errorf("ValidateWireTree rejected canonical %T remote tool identity: %v", value, err)
		}
	}

	overlongTool := strings.Repeat("a", mcpserver.MaximumRemoteToolNameCharacters+1)
	tests := []struct {
		shape string
		field string
		err   error
	}{
		{"MCPToolExposure", "disabledTools[0]", protocol.ValidateWireTree((protocol.MCPToolExposure{Server: files, DisabledTools: []string{"with space"}}))},
		{"SetMCPToolExposureRequest", "name", protocol.ValidateWireTree((protocol.SetMCPToolExposureRequest{Server: files, Name: "tool/name"}))},
		{"MCPToolExposure", "disabledTools[0]", protocol.ValidateWireTree((protocol.MCPToolExposure{Server: files, DisabledTools: []string{"工具"}}))},
		{"MCPToolExposure", "disabledTools[0]", protocol.ValidateWireTree((protocol.MCPToolExposure{Server: files, DisabledTools: []string{overlongTool}}))},
		{"MCPTool", "name", protocol.ValidateWireTree((protocol.MCPTool{Server: files, Name: overlongTool}))},
	}
	for _, test := range tests {
		assertProtocolConstraintField(t, test.err, test.shape, test.field)
	}

	tooMany := make([]string, mcpserver.MaxRemoteToolsPerServer+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("tool_%d", i)
	}
	assertProtocolConstraintField(
		t,
		protocol.ValidateWireTree((protocol.MCPToolExposure{Server: files, DisabledTools: tooMany})),
		"MCPToolExposure",
		"disabledTools",
	)
}

func TestRuntimeOutputNumbersPreserveDomainBounds(t *testing.T) {
	t.Parallel()

	negative, tooManyTools := -1, mcpserver.MaxRemoteToolsPerServer+1
	negativeBytes := int64(-1)
	tooLongDuration := protocol.MaximumDurationMilliseconds + 1
	negativeCost, nonFiniteCost := -0.01, math.Inf(1)
	for _, test := range []struct {
		shape string
		field string
		value any
	}{
		{shape: "Item", field: "droppedMessages", value: protocol.Item{DroppedMessages: -1}},
		{shape: "Item", field: "durationMillis", value: protocol.Item{DurationMillis: &tooLongDuration}},
		{shape: "RunMetrics", field: "activeDurationMillis", value: protocol.RunMetrics{ActiveDurationMillis: tooLongDuration}},
		{shape: "WorkspaceSummary", field: "sessionCount", value: protocol.WorkspaceSummary{SessionCount: -1}},
		{shape: "FileContent", field: "totalLines", value: protocol.FileContent{}},
		{shape: "FileContent", field: "startLine", value: protocol.FileContent{TotalLines: 1, StartLine: -1}},
		{shape: "FileContent", field: "endLine", value: protocol.FileContent{TotalLines: 1, EndLine: -1}},
		{shape: "FileEntry", field: "sizeBytes", value: protocol.FileEntry{SizeBytes: &negativeBytes}},
		{shape: "GrepResult", field: "total", value: protocol.GrepResult{Total: -1}},
		{shape: "GrepMatch", field: "lineNumber", value: protocol.GrepMatch{}},
		{shape: "FileDiff", field: "added", value: protocol.FileDiff{Added: &negative}},
		{shape: "WorkspaceFileChange", field: "removed", value: protocol.WorkspaceFileChange{Removed: &negative}},
		{shape: "DiffRow", field: "leftLine", value: protocol.DiffRow{LeftLine: -1}},
		{shape: "UsageCoverage", field: "calls", value: protocol.UsageCoverage{Calls: -1}},
		{shape: "UsageSummary", field: "sessions", value: protocol.UsageSummary{Sessions: -1}},
		{shape: "UsageSummary", field: "runs", value: protocol.UsageSummary{Runs: -1}},
		{shape: "HookInfo", field: "timeoutMillis", value: protocol.HookInfo{TimeoutMillis: -1}},
		{shape: "HookInfo", field: "timeoutMillis", value: protocol.HookInfo{TimeoutMillis: hookdata.MaxTimeoutMillis + 1}},
		{shape: "MCPServerState", field: "toolCount", value: protocol.MCPServerState{ToolCount: &negative}},
		{shape: "MCPServerState", field: "toolCount", value: protocol.MCPServerState{ToolCount: &tooManyTools}},
		{shape: "ModelPricing", field: "inputUsdPerMillionTokens", value: protocol.ModelPricing{InputUSDPerMillionTokens: negativeCost}},
		{shape: "ModelPricing", field: "outputUsdPerMillionTokens", value: protocol.ModelPricing{OutputUSDPerMillionTokens: nonFiniteCost}},
	} {
		t.Run(test.shape+"."+test.field, func(t *testing.T) {
			t.Parallel()
			assertProtocolConstraintField(t, protocol.ValidateWireTree(test.value), test.shape, test.field)
		})
	}
}

func TestRuntimeOutputNumberBoundariesRemainRepresentable(t *testing.T) {
	t.Parallel()

	zero, maximumTools := 0, mcpserver.MaxRemoteToolsPerServer
	zeroBytes := int64(0)
	for _, value := range []any{protocol.WorkspaceSummary{Name: "workspace", Workspace: protocol.WorkspaceInfo{Ref: protocol.WorkspaceRef{Path: "/workspace"}, ProjectRoot: "/workspace", Availability: protocol.WorkspaceAvailable}}, protocol.FileContent{TotalLines: 1, StartLine: 1, EndLine: 1}, protocol.FileEntry{Path: "main.go", Name: "main.go", Type: protocol.FileEntryFile, SizeBytes: &zeroBytes}, protocol.GrepResult{}, protocol.GrepMatch{Path: "main.go", LineNumber: 1}, protocol.FileDiff{Path: "main.go", Status: protocol.FileStatusModified, Added: &zero, Removed: &zero}, protocol.WorkspaceFileChange{Path: "main.go", Status: protocol.FileStatusModified, Added: &zero, Removed: &zero}, protocol.DiffRow{Type: protocol.DiffRowContext, LeftLine: 1, RightLine: 1, Code: new("line")}, protocol.UsageBucket{Key: "deepseek"}, protocol.UsageSummary{}, protocol.HookInfo{Event: protocol.HookEventPreToolUse, Command: "true", TimeoutMillis: hookdata.MaxTimeoutMillis, Scope: protocol.HookScopeGlobal, Source: "/hooks.json"}, protocol.MCPServerState{Type: protocol.MCPServerConnected, ToolCount: &maximumTools}, protocol.ModelPricing{}} {
		if err := protocol.ValidateWireTree(value); err != nil {
			t.Errorf("ValidateWire rejected valid %T numeric boundaries: %v", value, err)
		}
	}
}

func assertProtocolConstraintField(t *testing.T, err error, shape, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("ValidateWire accepted invalid %s.%s", shape, field)
	}
	constraint, ok := errors.AsType[*protocol.ConstraintError](err)
	if !ok {
		t.Fatalf("ValidateWire error = %T %v, want *ConstraintError", err, err)
	}
	for _, violation := range constraint.Fields {
		if violation.Field == field {
			if !strings.Contains(err.Error(), shape+"."+field) {
				t.Fatalf("error = %q, want shape-qualified path %s.%s", err, shape, field)
			}
			return
		}
	}
	t.Fatalf("violations = %+v, want field %q", constraint.Fields, field)
}
