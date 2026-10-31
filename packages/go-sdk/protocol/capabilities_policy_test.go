package protocol

import (
	"slices"
	"testing"
)

func TestMissingFeatureRequirementsHonorsServerSupportAndClientOptIn(t *testing.T) {
	t.Parallel()

	advertised := map[string]FeatureCapability{
		FeatureSkills: {
			Enabled: true,
		},
		FeatureChildRuns: {
			Enabled: true, ClientOptIn: true, RequiredByRunProtocol: true,
		},
	}
	wantChildRuns := []CapabilityRequirement{{
		Type: RequirementFeature, Name: FeatureChildRuns,
	}}

	if got := MissingFeatureRequirements(advertised, nil, FeatureSkills); len(got) != 0 {
		t.Fatalf("stable server feature gaps = %v, want none", got)
	}
	if got := MissingFeatureRequirements(advertised, nil, FeatureChildRuns); !slices.Equal(got, wantChildRuns) {
		t.Fatalf("missing client opt-in gaps = %v, want %v", got, wantChildRuns)
	}
	client := &ClientCapabilities{Features: map[string]FeaturePreference{
		FeatureChildRuns: {Enabled: true},
	}}
	if got := MissingFeatureRequirements(advertised, client, FeatureChildRuns); len(got) != 0 {
		t.Fatalf("negotiated childRuns gaps = %v, want none", got)
	}
	if got := MissingFeatureRequirements(nil, client, FeatureChildRuns); !slices.Equal(got, wantChildRuns) {
		t.Fatalf("unsupported server feature gaps = %v, want %v", got, wantChildRuns)
	}
}

func TestMissingFeatureRequirementsIsOrderedAndUnique(t *testing.T) {
	t.Parallel()

	got := MissingFeatureRequirements(nil, nil, FeatureChildRuns, FeatureSkills, FeatureChildRuns)
	want := []CapabilityRequirement{
		{Type: RequirementFeature, Name: FeatureChildRuns},
		{Type: RequirementFeature, Name: FeatureSkills},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("requirements = %v, want ordered unique %v", got, want)
	}
}
