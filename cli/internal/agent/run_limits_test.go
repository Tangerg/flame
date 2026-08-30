package agent

import (
	"encoding/json"
	"math"
	"testing"
)

func TestRunLimitsUsePresenceInsteadOfNumericSentinels(t *testing.T) {
	if err := (RunLimits{}).Validate(); err == nil {
		t.Fatal("zero RunLimits was accepted as unlimited")
	}
	unlimited := UnlimitedRunLimits()
	if !unlimited.Unlimited() || unlimited.Validate() != nil {
		t.Fatalf("unlimited limits = %+v", unlimited)
	}
	if _, limited := unlimited.MaxTotalTokens(); limited {
		t.Fatal("unlimited policy exposes a token cap")
	}
	if _, limited := unlimited.MaxSteps(); limited {
		t.Fatal("unlimited policy exposes a step cap")
	}
	if _, limited := unlimited.MaxBudgetUSD(); limited {
		t.Fatal("unlimited policy exposes a budget cap")
	}

	steps := 7
	limited, err := NewRunLimits(RunLimitValues{MaxSteps: &steps})
	if err != nil {
		t.Fatal(err)
	}
	if limited.Unlimited() {
		t.Fatal("limited policy reports unlimited")
	}
	if value, present := limited.MaxSteps(); !present || value != steps {
		t.Fatalf("max steps = %d, present=%v", value, present)
	}
	if _, present := limited.MaxBudgetUSD(); present {
		t.Fatal("absent budget cap became present")
	}
}

func TestRunLimitsJSONPreservesStrictPolicyIdentity(t *testing.T) {
	steps := 7
	limited, err := NewRunLimits(RunLimitValues{MaxSteps: &steps})
	if err != nil {
		t.Fatal(err)
	}
	for _, limits := range []RunLimits{UnlimitedRunLimits(), limited} {
		encoded, err := json.Marshal(limits)
		if err != nil {
			t.Fatal(err)
		}
		var decoded RunLimits
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded != limits {
			t.Fatalf("round trip = %+v, want %+v", decoded, limits)
		}
	}
	for _, encoded := range []string{
		`{}`,
		`null`,
		`{"type":"unlimited","maxSteps":1}`,
		`{"type":"limited"}`,
		`{"type":"limited","maxSteps":0}`,
		`{"type":"foreign"}`,
		`{"type":"unlimited","extra":true}`,
		`{"type":"unlimited"} {"type":"unlimited"}`,
	} {
		var decoded RunLimits
		if err := json.Unmarshal([]byte(encoded), &decoded); err == nil {
			t.Fatalf("invalid JSON %s was accepted", encoded)
		}
	}
}

func TestRunLimitsRejectEmptyZeroNegativeAndNonFiniteValues(t *testing.T) {
	zeroInt64, negativeInt64 := int64(0), int64(-1)
	zeroInt, negativeInt := 0, -1
	zeroFloat, negativeFloat := 0.0, -1.0
	for name, values := range map[string]RunLimitValues{
		"empty":             {},
		"zero tokens":       {MaxTotalTokens: &zeroInt64},
		"negative tokens":   {MaxTotalTokens: &negativeInt64},
		"zero steps":        {MaxSteps: &zeroInt},
		"negative steps":    {MaxSteps: &negativeInt},
		"zero budget":       {MaxBudgetUSD: &zeroFloat},
		"negative budget":   {MaxBudgetUSD: &negativeFloat},
		"nan budget":        {MaxBudgetUSD: floatPointer(math.NaN())},
		"infinite budget":   {MaxBudgetUSD: floatPointer(math.Inf(1))},
		"negative infinity": {MaxBudgetUSD: floatPointer(math.Inf(-1))},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewRunLimits(values); err == nil {
				t.Fatalf("NewRunLimits(%+v) succeeded", values)
			}
		})
	}
}

func floatPointer(value float64) *float64 { return &value }
