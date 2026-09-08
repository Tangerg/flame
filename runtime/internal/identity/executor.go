package identity

import "fmt"

// MaximumExecutorIdentityBytes matches the executor port's durable URI-safe identity envelope.
const MaximumExecutorIdentityBytes = 256

type value struct {
	text string
}

func parse(kind, text string) (value, error) {
	if len(text) == 0 || len(text) > MaximumExecutorIdentityBytes {
		return value{}, fmt.Errorf("%s must contain 1 to %d URI-safe ASCII bytes", kind, MaximumExecutorIdentityBytes)
	}
	for index := range len(text) {
		character := text[index]
		if character >= 'a' && character <= 'z' ||
			character >= 'A' && character <= 'Z' ||
			character >= '0' && character <= '9' ||
			character == '-' || character == '_' || character == '.' || character == ':' {
			continue
		}
		return value{}, fmt.Errorf("%s must contain 1 to %d URI-safe ASCII bytes", kind, MaximumExecutorIdentityBytes)
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

// ExecutorID identifies one Flame-owned executor instance across Run segments.
type ExecutorID struct{ value }

func ParseExecutor(text string) (ExecutorID, error) {
	parsed, err := parse("executor identity", text)
	return ExecutorID{value: parsed}, err
}

func (i ExecutorID) String() string  { return i.text }
func (i ExecutorID) Validate() error { return requireConstructed("executor identity", i.text) }

// MemberID identifies one executor-owned process in a root/child tree.
type MemberID struct{ value }

func ParseMember(text string) (MemberID, error) {
	parsed, err := parse("executor member identity", text)
	return MemberID{value: parsed}, err
}

func ParseOptionalMember(text string) (MemberID, bool, error) {
	if text == "" {
		return MemberID{}, false, nil
	}
	parsed, err := ParseMember(text)
	return parsed, err == nil, err
}

func (i MemberID) String() string  { return i.text }
func (i MemberID) Validate() error { return requireConstructed("executor member identity", i.text) }

// RequestID identifies one executor-owned external wait request.
type RequestID struct{ value }

func ParseRequest(text string) (RequestID, error) {
	parsed, err := parse("executor request identity", text)
	return RequestID{value: parsed}, err
}

func (i RequestID) String() string  { return i.text }
func (i RequestID) Validate() error { return requireConstructed("executor request identity", i.text) }

// EffectID identifies one executor-owned model or Tool effect.
type EffectID struct{ value }

func ParseEffect(text string) (EffectID, error) {
	parsed, err := parse("executor effect identity", text)
	return EffectID{value: parsed}, err
}

func ParseOptionalEffect(text string) (EffectID, bool, error) {
	if text == "" {
		return EffectID{}, false, nil
	}
	parsed, err := ParseEffect(text)
	return parsed, err == nil, err
}

func (i EffectID) String() string  { return i.text }
func (i EffectID) Validate() error { return requireConstructed("executor effect identity", i.text) }
