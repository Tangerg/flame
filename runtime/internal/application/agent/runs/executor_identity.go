package runs

import (
	"fmt"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

// MaximumIdentityBytes matches the executor port's durable URI-safe identity envelope.
const MaximumIdentityBytes = 256

type executorIdentityValue struct {
	text string
}

func validateExecutionIdentity(kind, text string) error {
	if err := identity.ValidateURISafeASCII(text, MaximumIdentityBytes); err != nil {
		return fmt.Errorf("%s %w", kind, err)
	}
	return nil
}

func parseExecutionIdentity(kind, text string) (executorIdentityValue, error) {
	if err := validateExecutionIdentity(kind, text); err != nil {
		return executorIdentityValue{}, err
	}
	return executorIdentityValue{text: text}, nil
}

// requireConstructedExecutionIdentity reports whether an identity came from [parseExecutionIdentity]. Its exact
// byte envelope is established there, so the only identity that can reach a
// caller without it is the zero one.
func requireConstructedExecutionIdentity(kind, text string) error {
	if text != "" {
		return nil
	}
	return validateExecutionIdentity(kind, text)
}

// Executor identities travel as fields of the Run facts, checkpoints and
// bindings that carry them, so most callers need the rule rather than an opaque value.
// The Parse constructors below exist for the two that hold the identity itself.
func ValidateExecutor(text string) error { return validateExecutionIdentity("executor identity", text) }

func ValidateMember(text string) error {
	return validateExecutionIdentity("executor member identity", text)
}

func validateRequest(text string) error {
	return validateExecutionIdentity("executor request identity", text)
}

func ValidateInvocation(text string) error {
	return validateExecutionIdentity("invocation identity", text)
}

// validateOptionalMember accepts an absent identity.
// A caller that must tell absent from malformed compares against "" itself; no
// caller has ever needed the distinction returned to it.
func validateOptionalMember(text string) error {
	if text == "" {
		return nil
	}
	return ValidateMember(text)
}

// MemberID identifies one executor-owned process in a root/child tree.
type MemberID struct{ executorIdentityValue }

func ParseMember(text string) (MemberID, error) {
	parsed, err := parseExecutionIdentity("executor member identity", text)
	return MemberID{executorIdentityValue: parsed}, err
}

func (i MemberID) String() string { return i.text }
func (i MemberID) Validate() error {
	return requireConstructedExecutionIdentity("executor member identity", i.text)
}

// InvocationID identifies a logical model or Tool call within a Runtime execution.
// Scope retains its separate Effect identity; the adapter binds that Effect to this call.
type InvocationID struct{ executorIdentityValue }

func ParseInvocation(text string) (InvocationID, error) {
	parsed, err := parseExecutionIdentity("invocation identity", text)
	return InvocationID{executorIdentityValue: parsed}, err
}

func (i InvocationID) String() string { return i.text }
func (i InvocationID) Validate() error {
	return requireConstructedExecutionIdentity("invocation identity", i.text)
}
