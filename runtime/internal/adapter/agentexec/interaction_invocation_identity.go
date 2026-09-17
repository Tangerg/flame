package agentexec

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"

	"github.com/Tangerg/flame/runtime/internal/application/agent/runs"
	runtimeidentity "github.com/Tangerg/flame/runtime/internal/identity"
	agent "github.com/Tangerg/scope/agent"
	"github.com/Tangerg/scope/agent/strategy/interaction"
)

const (
	modelInvocationNamespace = "model"
	toolInvocationNamespace  = "tool"
)

func modelInvocationID(invocation interaction.ModelInvocation) (runtimeidentity.EffectID, error) {
	return modelInvocationIDFrom(invocation.EffectID(), invocation.ModelCallSequence())
}

func modelInvocationIDFrom(effectID agent.EffectID, modelCallSequence uint64) (runtimeidentity.EffectID, error) {
	digest := sha256.New()
	_, _ = digest.Write([]byte(effectID.String()))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(strconv.FormatUint(uint64(modelCallSequence), 10)))
	return parsedInvocationID(modelInvocationNamespace, digest.Sum(nil), modelCallSequence)
}

func toolInvocationID(invocation interaction.ToolInvocation) (runtimeidentity.EffectID, error) {
	caller, present := invocation.Relation().ParentID()
	if !present {
		return runtimeidentity.EffectID{}, agent.ErrInvalidProcessRelation
	}
	call := invocation.ToolCall()
	return logicalToolCallID(caller, invocation.ModelCallSequence(), invocation.ToolCallIndex(), call.ID, call.Name)
}

func logicalToolCallID(
	caller agent.ProcessID,
	modelCallSequence uint64,
	toolCallIndex uint32,
	sourceID string,
	name string,
) (runtimeidentity.EffectID, error) {
	digest := sha256.New()
	_, _ = digest.Write([]byte(caller.String()))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(strconv.FormatUint(uint64(modelCallSequence), 10)))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(sourceID))
	_, _ = digest.Write([]byte{0})
	_, _ = digest.Write([]byte(name))
	return parsedInvocationID(toolInvocationNamespace, digest.Sum(nil), uint64(toolCallIndex))
}

func parsedInvocationID(namespace string, digest []byte, ordinal uint64) (runtimeidentity.EffectID, error) {
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
