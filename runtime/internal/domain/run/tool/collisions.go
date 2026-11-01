package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"slices"
	"strings"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
)

// NameConflicts identifies sources that require qualified model names. Built-in
// names stay reserved even when that built-in is unavailable in a particular Run.
// Each value names the other identities competing for the same model name.
func NameConflicts(refs []Ref) map[Ref][]Ref {
	byName := make(map[string][]Ref)
	for _, ref := range refs {
		name := ref.ModelName()
		if !slices.Contains(byName[name], ref) {
			byName[name] = append(byName[name], ref)
		}
	}
	conflicts := make(map[Ref][]Ref)
	for name, candidates := range byName {
		if reserved, err := BuiltIn(builtincontract.Name(name)); err == nil && !slices.Contains(candidates, reserved) {
			candidates = append(candidates, reserved)
		}
		slices.SortFunc(candidates, func(a, b Ref) int { return strings.Compare(a.String(), b.String()) })
		for _, ref := range candidates {
			if ref.Kind() == BuiltInKind {
				continue
			}
			for _, other := range candidates {
				if other != ref {
					conflicts[ref] = append(conflicts[ref], other)
				}
			}
		}
	}
	return conflicts
}

const modelNameDigestBytes = 8
const modelNameQualifier = "__"

// An ordinary remote label cannot claim the namespace used for qualified names.
var reservedModelSuffix = regexp.MustCompile(fmt.Sprintf(`%s[0-9a-f]{%d}$`, modelNameQualifier, hex.EncodedLen(modelNameDigestBytes)))

func (r Ref) PublishedName(qualified bool) string {
	name := r.ModelName()
	if !qualified || r.kind != MCPKind {
		return name
	}
	digest := sha256.Sum256([]byte(r.String()))
	suffix := modelNameQualifier + hex.EncodeToString(digest[:modelNameDigestBytes])
	return name[:min(len(name), maximumModelNameBytes-len(suffix))] + suffix
}

func (r Ref) MatchesPublishedName(name string) bool {
	return name == r.ModelName() || r.kind == MCPKind && name == r.PublishedName(true)
}

func (r Ref) NameForCatalog(conflicts map[Ref][]Ref) string {
	return r.PublishedName(len(conflicts[r]) > 0 || reservedModelSuffix.MatchString(r.ModelName()))
}

// An MCP driver can refuse the whole server before catalog-wide naming runs.
// Distinguish lossy labels at this earlier boundary using the same exact source.
func (r Ref) DiscoveryName() string {
	server, remote, ok := r.MCP()
	if !ok {
		return r.ModelName()
	}
	base := r.ModelName()
	return r.PublishedName(base != server.Name().String()+"_"+remote.String() || reservedModelSuffix.MatchString(base))
}
