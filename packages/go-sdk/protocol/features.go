package protocol

import (
	"fmt"
	"slices"
)

const (
	FeatureReasoning     = "reasoning"
	FeatureMultimodal    = "multimodal"
	FeatureCompaction    = "compaction"
	FeaturePlan          = "plan"
	FeatureGoals         = "goals"
	FeatureSkills        = "skills"
	FeatureMCP           = "mcp"
	FeaturePlugins       = "plugins"
	FeatureGit           = "git"
	FeatureCheckpoints   = "checkpoints"
	FeatureFileWatch     = "fileWatch"
	FeatureSessionExport = "sessionExport"
	FeatureRelocate      = "relocate"
	FeatureIsolation     = "isolation"
	FeatureChildRuns     = "childRuns"
)

type Feature struct {
	Key           string
	ClientOptIn   bool
	RequiredByRun bool
}

var features = mustFeatures([]Feature{
	{Key: FeatureReasoning},
	{Key: FeatureMultimodal},
	{Key: FeatureCompaction},
	{Key: FeaturePlan},
	{Key: FeatureGoals},
	{Key: FeatureSkills},
	{Key: FeatureMCP},
	{Key: FeaturePlugins},
	{Key: FeatureGit},
	{Key: FeatureCheckpoints},
	{Key: FeatureFileWatch},
	{Key: FeatureSessionExport},
	{Key: FeatureRelocate},
	{Key: FeatureIsolation},
	{Key: FeatureChildRuns, ClientOptIn: true, RequiredByRun: true},
})

func Features() []Feature { return slices.Clone(features) }

func LookupFeature(key string) (Feature, bool) {
	for _, feature := range features {
		if feature.Key == key {
			return feature, true
		}
	}
	return Feature{}, false
}

func FeatureKeys() []string {
	keys := make([]string, 0, len(features))
	for _, feature := range features {
		keys = append(keys, feature.Key)
	}
	return keys
}

func mustFeatures(features []Feature) []Feature {
	seen := make(map[string]bool, len(features))
	for _, feature := range features {
		switch {
		case feature.Key == "":
			panic("protocol: a feature needs a key")
		case seen[feature.Key]:
			panic(fmt.Sprintf("protocol: feature %q is declared twice", feature.Key))
		case feature.RequiredByRun && !feature.ClientOptIn:
			panic(fmt.Sprintf("protocol: feature %q reshapes the run protocol without opt-in — it belongs in stable core, not in features", feature.Key))
		}
		seen[feature.Key] = true
	}
	return features
}
