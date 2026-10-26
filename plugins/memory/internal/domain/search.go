package agentmemory

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"unicode"
)

// Widening each signal lets keyword-only and vector-only matches compete after fusion.
const searchOverfetch = 3

func Rank(query string, queryVector []float32, items []Item, topK int) []Item {
	if topK <= 0 || len(items) == 0 {
		return nil
	}
	keyword := keywordRanked(query, items, topK*searchOverfetch)
	var vector []Item
	if len(queryVector) != 0 {
		vector = topKByCosine(queryVector, items, topK*searchOverfetch)
	}
	return fuseByRank(keyword, vector, topK)
}

func keywordRanked(query string, items []Item, limit int) []Item {
	terms := tokenize(query)
	if len(terms) == 0 {
		return nil
	}
	type scored struct {
		item  Item
		score int
	}
	ranked := make([]scored, 0, len(items))
	for _, item := range items {
		content := strings.ToLower(item.Content())
		hits := 0
		for _, term := range terms {
			if strings.Contains(content, term) {
				hits++
			}
		}
		if hits == 0 {
			continue
		}
		ranked = append(ranked, scored{item: item, score: hits})
	}
	slices.SortStableFunc(ranked, func(a, b scored) int { return cmp.Compare(b.score, a.score) })
	return capItems(ranked, limit, func(s scored) Item { return s.item })
}

func topKByCosine(query []float32, items []Item, limit int) []Item {
	qn := vectorNorm(query)
	if qn == 0 {
		return nil
	}
	type scored struct {
		item  Item
		score float64
	}
	ranked := make([]scored, 0, len(items))
	for _, item := range items {
		score := cosineSim(query, qn, item.embedding)
		if score <= 0 {
			continue
		}
		ranked = append(ranked, scored{item: item, score: score})
	}
	slices.SortStableFunc(ranked, func(a, b scored) int { return cmp.Compare(b.score, a.score) })
	return capItems(ranked, limit, func(s scored) Item { return s.item })
}

func capItems[T any](ranked []T, limit int, pick func(T) Item) []Item {
	out := make([]Item, 0, min(limit, len(ranked)))
	for i := 0; i < len(ranked) && i < limit; i++ {
		out = append(out, pick(ranked[i]))
	}
	return out
}

// Reciprocal ranks avoid comparing keyword counts with cosine scores; agreement contributes through both signals.
func fuseByRank(keyword, vector []Item, topK int) []Item {
	const rrfK = 60.0
	score := make(map[ItemID]float64)
	item := make(map[ItemID]Item)
	for _, list := range [][]Item{keyword, vector} {
		for rank, it := range list {
			score[it.ID()] += 1.0 / (rrfK + float64(rank+1))
			item[it.ID()] = it
		}
	}
	ids := make([]ItemID, 0, len(score))
	for id := range score {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b ItemID) int {
		if d := cmp.Compare(score[b], score[a]); d != 0 {
			return d
		}
		return cmp.Compare(a.String(), b.String())
	})
	out := make([]Item, 0, min(topK, len(ids)))
	for _, id := range ids {
		if len(out) >= topK {
			break
		}
		out = append(out, item[id])
	}
	return out
}

func vectorNorm(v []float32) float64 {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	return math.Sqrt(sum)
}

func cosineSim(query []float32, qn float64, v []float32) float64 {
	if len(v) != len(query) {
		return 0
	}
	var dot, vn float64
	for i := range query {
		dot += float64(query[i]) * float64(v[i])
		vn += float64(v[i]) * float64(v[i])
	}
	vn = math.Sqrt(vn)
	if vn == 0 {
		return 0
	}
	return dot / (qn * vn)
}

func tokenize(text string) []string {
	seen := make(map[string]struct{})
	var terms []string
	for _, field := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	}) {
		if len([]rune(field)) < 2 {
			continue
		}
		if _, dup := seen[field]; dup {
			continue
		}
		seen[field] = struct{}{}
		terms = append(terms, field)
	}
	return terms
}
