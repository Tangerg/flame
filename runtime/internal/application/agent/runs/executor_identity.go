package runs

import (
	"fmt"
	"strings"

	"github.com/Tangerg/flame/packages/go-sdk/identity"
)

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

func requireConstructedExecutionIdentity(kind, text string) error {
	if text != "" {
		return nil
	}
	return validateExecutionIdentity(kind, text)
}

func ValidateExecutor(text string) error { return validateExecutionIdentity("executor identity", text) }

func ValidateMember(text string) error {
	for component := range strings.SplitSeq(text, "/") {
		if err := validateExecutionIdentity("executor member identity component", component); err != nil {
			return err
		}
	}
	return nil
}

func validateRequest(text string) error {
	return validateExecutionIdentity("executor request identity", text)
}

func ValidateInvocation(text string) error {
	return validateExecutionIdentity("invocation identity", text)
}

func validateOptionalMember(text string) error {
	if text == "" {
		return nil
	}
	return ValidateMember(text)
}

type MemberID struct{ executorIdentityValue }

func ParseMember(text string) (MemberID, error) {
	if err := ValidateMember(text); err != nil {
		return MemberID{}, err
	}
	return MemberID{executorIdentityValue: executorIdentityValue{text: text}}, nil
}

func (i MemberID) String() string { return i.text }

func (i MemberID) Validate() error {
	if i.text == "" {
		return ValidateMember(i.text)
	}
	return nil
}

type InvocationID struct{ executorIdentityValue }

func ParseInvocation(text string) (InvocationID, error) {
	parsed, err := parseExecutionIdentity("invocation identity", text)
	return InvocationID{executorIdentityValue: parsed}, err
}

func (i InvocationID) String() string { return i.text }

func (i InvocationID) Validate() error {
	return requireConstructedExecutionIdentity("invocation identity", i.text)
}
