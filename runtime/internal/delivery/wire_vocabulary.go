package delivery

import "fmt"

// One table per vocabulary: encoding and decoding both read it so the two
// directions cannot drift.
type wireVocabulary[D, W comparable] []vocabularyTerm[D, W]

type vocabularyTerm[D, W comparable] struct {
	domain D
	wire   W
}

func (v wireVocabulary[D, W]) wire(value D) (W, bool) {
	for _, term := range v {
		if term.domain == value {
			return term.wire, true
		}
	}
	var zero W
	return zero, false
}

// Only for values the domain validated at construction; anything else is a
// programming error.
func (v wireVocabulary[D, W]) mustWire(value D) W {
	wire, ok := v.wire(value)
	if !ok {
		panic(fmt.Sprintf("delivery: %T %v has no wire spelling", value, value))
	}
	return wire
}

func (v wireVocabulary[D, W]) domain(value W) (D, bool) {
	for _, term := range v {
		if term.wire == value {
			return term.domain, true
		}
	}
	var zero D
	return zero, false
}
