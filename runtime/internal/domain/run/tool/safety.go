package tool

type RiskLevel string

const (
	RiskLow    RiskLevel = "low"
	RiskMedium RiskLevel = "medium"
	RiskHigh   RiskLevel = "high"
)

func (s SafetyClass) Valid() bool {
	switch s {
	case SafetyClassSafe, SafetyClassWrite, SafetyClassExec, SafetyClassNetwork:
		return true
	default:
		return false
	}
}

// An unknown class is high risk so an uninitialized or future value never weakens
// a prompt.
func (s SafetyClass) Risk() RiskLevel {
	switch s {
	case SafetyClassSafe:
		return RiskLow
	case SafetyClassWrite:
		return RiskMedium
	default:
		return RiskHigh
	}
}

func (r RiskLevel) Valid() bool {
	switch r {
	case RiskLow, RiskMedium, RiskHigh:
		return true
	default:
		return false
	}
}
