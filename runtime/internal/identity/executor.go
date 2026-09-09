package identity

import "fmt"

// MaximumExecutorIdentityBytes matches the executor port's durable URI-safe identity envelope.
const MaximumExecutorIdentityBytes = 256

type value struct {
	text string
}

func validate(kind, text string) error {
	if len(text) == 0 || len(text) > MaximumExecutorIdentityBytes {
		return fmt.Errorf("%s must contain 1 to %d URI-safe ASCII bytes", kind, MaximumExecutorIdentityBytes)
	}
	for index := range len(text) {
		character := text[index]
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '-' || character == '_' || character == '.' || character == ':' {
			continue
		}
		return fmt.Errorf("%s must contain 1 to %d URI-safe ASCII bytes", kind, MaximumExecutorIdentityBytes)
	}
	return nil
}

func parse(kind, text string) (value, error) {
	if err := validate(kind, text); err != nil {
		return value{}, err
	}
	return value{text: text}, nil
}

// requireConstructed reports whether an identity came from [parse]. Its exact
// byte envelope is established there, so the only identity that can reach a
// caller without it is the zero one.
func requireConstructed(kind, text string) error {
	if text == "" {
		return fmt.Errorf("%s must contain 1 to %d URI-safe ASCII bytes", kind, MaximumExecutorIdentityBytes)
	}
	return nil
}

// Executor identities travel as fields of the Run facts, checkpoints and
// bindings that carry them, so most callers need the rule rather than a value.
// The Parse constructors below exist for the two that hold the identity itself.
func ValidateExecutor(text string) error { return validate("executor identity", text) }

func ValidateMember(text string) error { return validate("executor member identity", text) }

func ValidateRequest(text string) error { return validate("executor request identity", text) }

func ValidateEffect(text string) error { return validate("executor effect identity", text) }

// ValidateOptionalMember and ValidateOptionalEffect accept an absent identity.
// A caller that must tell absent from malformed compares against "" itself; no
// caller has ever needed the distinction returned to it.
func ValidateOptionalMember(text string) error {
	if text == "" {
		return nil
	}
	return ValidateMember(text)
}

func ValidateOptionalEffect(text string) error {
	if text == "" {
		return nil
	}
	return ValidateEffect(text)
}

// MemberID identifies one executor-owned process in a root/child tree.
type MemberID struct{ value }

func ParseMember(text string) (MemberID, error) {
	parsed, err := parse("executor member identity", text)
	return MemberID{value: parsed}, err
}

func (i MemberID) String() string  { return i.text }
func (i MemberID) Validate() error { return requireConstructed("executor member identity", i.text) }

// EffectID identifies one executor-owned model or Tool effect.
type EffectID struct{ value }

func ParseEffect(text string) (EffectID, error) {
	parsed, err := parse("executor effect identity", text)
	return EffectID{value: parsed}, err
}

func (i EffectID) String() string  { return i.text }
func (i EffectID) Validate() error { return requireConstructed("executor effect identity", i.text) }
