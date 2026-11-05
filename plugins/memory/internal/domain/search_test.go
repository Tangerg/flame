package agentmemory

import (
	"testing"
	"time"
)

func TestLexicalSearchFindsUnspacedAndMixedLanguageFactsWithoutEmbeddings(t *testing.T) {
	var items []Item
	for index, text := range []string{
		"用户主要使用 Go 编程，服务采用 PostgreSQL。",
		"用户喜欢喝茶。",
		"The test command is go test ./...",
	} {
		item, err := NewUserItem(NewItemID([ItemIDEntropyBytes]byte{byte(index + 1)}), ScopeProject, "/repo", text, time.Unix(1, 0))
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, item)
	}
	for _, test := range []struct {
		query string
		want  ItemID
	}{
		{query: "什么编程语言", want: items[0].ID()},
		{query: "Go服务数据库", want: items[0].ID()},
		{query: "喜欢喝茶吗", want: items[1].ID()},
		{query: "TEST command", want: items[2].ID()},
	} {
		matches := Rank(test.query, nil, items, 1)
		if len(matches) != 1 || matches[0].ID() != test.want {
			t.Fatalf("query %q: matches=%v, want %s", test.query, matches, test.want)
		}
	}
	if matches := Rank("火星地质", nil, items, 3); len(matches) != 0 {
		t.Fatalf("unmatched query returned facts: %v", matches)
	}
}

func TestLexicalSearchDownweightsTermsSharedByMostFacts(t *testing.T) {
	var items []Item
	for index, text := range []string{"project build", "project style", "project layout", "deployment"} {
		item, err := NewUserItem(NewItemID([ItemIDEntropyBytes]byte{byte(index + 1)}), ScopeProject, "/repo", text, time.Unix(1, 0))
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, item)
	}
	matches := Rank("project deployment", nil, items, 1)
	if len(matches) != 1 || matches[0].ID() != items[3].ID() {
		t.Fatalf("rare query term lost to a shared term: %v", matches)
	}
}
