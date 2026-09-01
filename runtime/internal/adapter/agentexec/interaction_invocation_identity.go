package agentexec

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/interaction"
	corechat "github.com/Tangerg/scope/core/chat"
)

const (
	modelInvocationNamespace = "model"
	toolInvocationNamespace  = "tool"
)

func modelInvocationID(invocation interaction.ModelInvocation) (runtimeidentity.EffectID, error) {
	return modelInvocationIDFrom(invocation.EffectID(), invocation.ModelCallSequence())
}

func modelInvocationIDFrom(effectID agent.EffectID, modelCallSequence uint32) (runtimeidentity.EffectID, error) {
	digest := sha256.New()
	_, _ = digest.Write([]byte(effectID.String()))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(strconv.FormatUint(uint64(modelCallSequence), 10)))
	return parsedInvocationID(modelInvocationNamespace, digest.Sum(nil), modelCallSequence)
}

func toolInvocationID(invocation interaction.ToolInvocation) (runtimeidentity.EffectID, error) {
	return delegatedToolCallID(
		invocation.Relation(), invocation.ModelCallSequence(), invocation.ToolCallIndex(), invocation.ToolCall(),
	)
}

func delegatedToolCallID(
	relation agent.ProcessRelation,
	modelCallSequence uint32,
	toolCallIndex uint32,
	call corechat.ToolCall,
) (runtimeidentity.EffectID, error) {
	digest := sha256.New()
	_, _ = digest.Write([]byte(relation.ProcessID().String()))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(strconv.FormatUint(uint64(modelCallSequence), 10)))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(call.ID))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(call.Name))
	return parsedInvocationID(toolInvocationNamespace, digest.Sum(nil), toolCallIndex)
}

func parsedInvocationID(namespace string, digest []byte, ordinal uint32) (runtimeidentity.EffectID, error) {
	return runtimeidentity.ParseEffect(
		namespace + ":" + hex.EncodeToString(digest) + ":" + strconv.FormatUint(uint64(ordinal), 10),
	)
}

func basicExecutorMember(relation agent.ProcessRelation) runs.ExecutorMember {
	member := runs.ExecutorMember{MemberID: relation.ProcessID().String()}
	if parentID, child := relation.ParentID(); child {
		member.ParentID = parentID.String()
	}
	return member
}
