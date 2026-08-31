package runtimefixture

import "math/big"

type identityNamespace string

const (
	sessionIdentity identityNamespace = "ses_mock_"
	runIdentity     identityNamespace = "run_mock_"
	segmentIdentity identityNamespace = "seg_mock_"
	itemIdentity    identityNamespace = "item_mock_"
	eventIdentity   identityNamespace = "evt_mock_"
	ruleIdentity    identityNamespace = "rule_mock_"
)

type mockIdentitySequence struct {
	value big.Int
}

func (s *mockIdentitySequence) next(namespace identityNamespace) string {
	s.value.Add(&s.value, big.NewInt(1))
	return string(namespace) + s.value.String()
}
