package toolset

import (
	"cmp"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"slices"
	"strings"

	builtincontract "github.com/Tangerg/flame/packages/go-sdk/builtin"
	pluginsdk "github.com/Tangerg/flame/packages/go-sdk/plugin"
	"github.com/Tangerg/flame/runtime/internal/domain/run/tool"
	toolcontract "github.com/Tangerg/scope/core/tool"
	"github.com/samber/lo"
)

const (
	discoveryDefaultLimit = 5
	discoveryMaxLimit     = 20
	discoveryCatalogBytes = 4096
	omissionNoticeBytes   = 200
)

// discoveryDescription is static because the tool declarations open the
// provider prompt-cache prefix. The Run's deferred catalog changes whenever an
// integration or plugin does, so it travels in a per-request message instead.
const discoveryDescription = "Load deferred built-in or integration tools on demand. A bounded directory in the conversation gives examples; search covers the complete deferred set. " +
	"Search by capability (query=\"...\") or load exact tools (query=\"select:name1,name2\"); matches become directly callable on your next step."

const discoverySelectPrefix = "select:"

type discoveryArgs struct {
	Query string `json:"query" jsonschema:"minLength=1,maxLength=4096" jsonschema_description:"Describe the capability you need, or use select:name1,name2 to load up to 20 exact tool names. Prefix a keyword with + to require it."`
	Limit *int   `json:"limit,omitzero" jsonschema:"minimum=1,maximum=20" jsonschema_description:"Maximum keyword matches to load. Defaults to 5. Exact select: queries ignore this value."`
}

type discoverableTool struct {
	definition toolcontract.Definition
	source     string
	nameTerms  []string
	nameLower  string
	descLower  string
}

type Discovery struct {
	entries []discoverableTool
	byName  map[string]discoverableTool
	inner   toolcontract.Tool
}

var _ toolcontract.Tool = (*Discovery)(nil)

type discoveryResult struct {
	Names   []string `json:"names"`
	Message string   `json:"message"`
}

func NewDiscovery(withheld []toolcontract.Tool) (*Discovery, error) {
	if len(withheld) == 0 {
		return nil, errors.New("discovery: withheld Tools are required")
	}
	t := &Discovery{byName: make(map[string]discoverableTool, len(withheld))}
	for _, executable := range withheld {
		ref, err := Identify(executable)
		if err != nil {
			return nil, fmt.Errorf("discovery: resolve Tool identity: %w", err)
		}
		source := "built-in"
		if server, _, ok := ref.MCP(); ok {
			source = server.String()
		}
		def := executable.Definition()
		e := discoverableTool{
			definition: def,
			source:     source,
			nameTerms:  tokenize(def.Name),
			nameLower:  strings.ToLower(def.Name),
			descLower:  strings.ToLower(def.Description),
		}
		t.entries = append(t.entries, e)
		t.byName[def.Name] = e
	}
	slices.SortFunc(t.entries, func(a, b discoverableTool) int {
		if a.source != b.source {
			return strings.Compare(a.source, b.source)
		}
		return strings.Compare(a.definition.Name, b.definition.Name)
	})
	inner, err := toolcontract.NewFunc(
		toolcontract.FuncConfig{
			Name:        string(builtincontract.SearchTools),
			Description: discoveryDescription,
		},
		t.search,
	)
	if err != nil {
		return nil, fmt.Errorf("discovery: build search_tools: %w", err)
	}
	t.inner = inner
	return t, nil
}

func (d *Discovery) Definition() toolcontract.Definition {
	return d.inner.Definition()
}

func (d *Discovery) Unwrap() toolcontract.Tool { return d.inner }

func (d *Discovery) Catalog() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Deferred tools for this run: %d total. Until loaded they are not in your tool list; load them with search_tools before calling them, either by capability (query=\"...\") or by exact name (query=\"select:name1,name2\").", len(d.entries))
	lastSource := ""
	omitted := 0
	for _, e := range d.entries {
		entry := ", " + e.definition.Name
		if e.source != lastSource {
			entry = fmt.Sprintf("\n[%s] %s", e.source, e.definition.Name)
		}
		if b.Len()+len(entry)+omissionNoticeBytes > discoveryCatalogBytes {
			omitted++
			continue
		}
		b.WriteString(entry)
		lastSource = e.source
	}
	if omitted > 0 {
		fmt.Fprintf(&b, "\n%d additional tools are omitted from this directory. Search by capability to find them; every tool remains searchable and exact-selectable.", omitted)
	}
	return b.String()
}

func (d *Discovery) Call(ctx context.Context, invocation toolcontract.Invocation) (toolcontract.Result, error) {
	result, err := d.inner.Call(ctx, invocation)
	if err != nil {
		return toolcontract.Result{}, err
	}
	matched, err := decodeDiscoveryResult(result)
	if err != nil {
		return toolcontract.Result{}, err
	}
	result.Output.Content = toolcontract.NewTextOutput(matched.Message).Content
	return result, nil
}

// The result carries the names actually selected after any argument rewrite.
// Repeating a search from the original invocation would advertise a different set.
func (d *Discovery) AdvertiseDeferred(_ toolcontract.Invocation, result toolcontract.Result) ([]string, error) {
	matched, err := decodeDiscoveryResult(result)
	return matched.Names, err
}

func decodeDiscoveryResult(result toolcontract.Result) (discoveryResult, error) {
	var matched discoveryResult
	if err := json.Unmarshal(result.Output.Details, &matched, json.RejectUnknownMembers(true)); err != nil {
		return discoveryResult{}, fmt.Errorf("search_tools: decode selected matches: %w", err)
	}
	return matched, nil
}

func (d *Discovery) search(_ context.Context, args discoveryArgs) (discoveryResult, error) {
	query := strings.TrimSpace(args.Query)
	if query == "" {
		return discoveryResult{}, pluginsdk.DefiniteToolFailure(errEmptyQuery)
	}
	limit := lo.FromPtrOr(args.Limit, discoveryDefaultLimit)

	var matches []discoverableTool
	if rest, ok := strings.CutPrefix(query, discoverySelectPrefix); ok {
		matches = d.selectByName(rest)
		if len(matches) > discoveryMaxLimit {
			return discoveryResult{}, pluginsdk.DefiniteToolFailure(fmt.Errorf("search_tools: select at most %d tools per call", discoveryMaxLimit))
		}
	} else {
		matches = d.searchByKeyword(query, limit)
	}
	if len(matches) == 0 {
		return discoveryResult{Message: d.renderNoMatch(query)}, nil
	}

	names := make([]string, len(matches))
	for index, match := range matches {
		names[index] = match.definition.Name
	}
	return discoveryResult{Names: names, Message: d.renderMatches(matches)}, nil
}

func (d *Discovery) selectByName(list string) []discoverableTool {
	var out []discoverableTool
	seen := make(map[string]struct{})
	for name := range strings.SplitSeq(list, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		if e, ok := d.byName[name]; ok {
			seen[name] = struct{}{}
			out = append(out, e)
		}
	}
	return out
}

type rankedTool struct {
	tool  discoverableTool
	score int
}

func (d *Discovery) searchByKeyword(query string, limit int) []discoverableTool {
	terms := strings.Fields(strings.ToLower(query))
	if len(terms) == 0 {
		return nil
	}
	var hits []rankedTool
	for _, e := range d.entries {
		if s, ok := scoreEntry(terms, e); ok {
			hits = append(hits, rankedTool{tool: e, score: s})
		}
	}
	slices.SortStableFunc(hits, func(a, b rankedTool) int {
		if a.score != b.score {
			return cmp.Compare(b.score, a.score)
		}
		return strings.Compare(a.tool.definition.Name, b.tool.definition.Name)
	})
	return roundRobinBySource(hits, limit)
}

func scoreEntry(terms []string, e discoverableTool) (int, bool) {
	total := 0
	for _, term := range terms {
		required := strings.HasPrefix(term, "+")
		term = strings.TrimPrefix(term, "+")
		if term == "" {
			continue
		}
		s := 0
		switch {
		case slices.Contains(e.nameTerms, term):
			s = 3
		case strings.Contains(e.nameLower, term):
			s = 2
		case strings.Contains(e.descLower, term):
			s = 1
		}
		if s == 0 && required {
			return 0, false
		}
		total += s
	}
	return total, total > 0
}

func roundRobinBySource(hits []rankedTool, limit int) []discoverableTool {
	if len(hits) == 0 {
		return nil
	}
	perSource := make(map[string][]discoverableTool)
	var order []string
	for _, h := range hits {
		if _, ok := perSource[h.tool.source]; !ok {
			order = append(order, h.tool.source)
		}
		perSource[h.tool.source] = append(perSource[h.tool.source], h.tool)
	}
	out := make([]discoverableTool, 0, min(limit, len(hits)))
	for len(out) < limit {
		progressed := false
		for _, source := range order {
			queue := perSource[source]
			if len(queue) == 0 {
				continue
			}
			out = append(out, queue[0])
			perSource[source] = queue[1:]
			progressed = true
			if len(out) >= limit {
				break
			}
		}
		if !progressed {
			break
		}
	}
	return out
}

func (d *Discovery) renderMatches(matches []discoverableTool) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Loaded %d tool(s) — now callable directly on your next step:\n", len(matches))
	for _, m := range matches {
		b.WriteString("  - ")
		b.WriteString(m.definition.Name)
		if desc := firstLine(m.definition.Description); desc != "" {
			b.WriteString(": ")
			b.WriteString(desc)
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func (d *Discovery) renderNoMatch(query string) string {
	return fmt.Sprintf("No tools matched %q. %d tool(s) are available — try a broader keyword, or select:name to load one by exact name.", query, len(d.entries))
}

func tokenize(name string) []string {
	var terms []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			terms = append(terms, strings.ToLower(cur.String()))
			cur.Reset()
		}
	}
	runes := []rune(name)
	for i, r := range runes {
		switch {
		case r == '_' || r == '-' || r == ' ' || r == '.':
			flush()
		case i > 0 && isUpper(r) && !isUpper(runes[i-1]):
			flush()
			cur.WriteRune(r)
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return terms
}

func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }

func firstLine(s string) string {
	if line, _, ok := strings.Cut(s, "\n"); ok {
		return strings.TrimSpace(line)
	}
	return strings.TrimSpace(s)
}

func (d *Discovery) ToolRef() tool.Ref {
	ref, _ := tool.BuiltIn(builtincontract.SearchTools)
	return ref
}
