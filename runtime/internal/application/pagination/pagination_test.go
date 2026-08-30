package pagination

import (
	"errors"
	"strings"
	"testing"

	"github.com/Tangerg/flame/runtime/internal/application/opaquetoken"
)

func TestRoundTripReturnsTheAnchor(t *testing.T) {
	cursor := mustEncode(t, "items", []string{"ses_1"}, []string{"42"})
	key, err := Decode(cursor, "items", []string{"ses_1"})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(key) != 1 || key[0] != "42" {
		t.Fatalf("key = %v, want [42]", key)
	}
}

func TestEmptyCursorIsTheFirstPage(t *testing.T) {
	key, err := Decode("", "items", []string{"ses_1"})
	if err != nil || key != nil {
		t.Fatalf("decode empty = (%v, %v), want (nil, nil)", key, err)
	}
}

func TestNamespaceIsRequired(t *testing.T) {
	if _, err := Decode("", "", nil); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("Decode with empty namespace err = %v, want ErrInvalidCursor", err)
	}
	if _, err := Encode("", nil, []string{"1"}); !errors.Is(err, ErrInvalidCursorMaterial) {
		t.Fatalf("Encode with empty namespace err = %v, want ErrInvalidCursorMaterial", err)
	}
}

// A cursor names the query it was minted for. Accepting one from a different
// namespace or filter set would continue a page against rows it never enumerated,
// which skips and repeats silently — the failure a page cursor exists to prevent.
func TestCursorFromAnotherQueryIsRejected(t *testing.T) {
	cursor := mustEncode(t, "items", []string{"ses_1"}, []string{"42"})

	if _, err := Decode(cursor, "runs", []string{"ses_1"}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("cross-namespace decode err = %v, want ErrInvalidCursor", err)
	}
	if _, err := Decode(cursor, "items", []string{"ses_2"}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("cross-filter decode err = %v, want ErrInvalidCursor", err)
	}
	if _, err := Decode(cursor, "items", []string{"ses_1", "desc"}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("added-filter decode err = %v, want ErrInvalidCursor", err)
	}
}

// Filters remain structured in the token, so shifting a value boundary cannot
// reinterpret one normalized query as another.
func TestFilterBoundariesAreNotInterchangeable(t *testing.T) {
	cursor := mustEncode(t, "items", []string{"a", "bc"}, []string{"1"})
	if _, err := Decode(cursor, "items", []string{"ab", "c"}); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("shifted-boundary decode err = %v, want ErrInvalidCursor", err)
	}
}

func TestDamagedCursorIsRejected(t *testing.T) {
	for name, cursor := range map[string]string{
		"not base64":  "!!!!",
		"not json":    "aGVsbG8",
		"empty key":   mustRawToken(t, token{Version: formatVersion, Namespace: "items", Filters: []string{"ses_1"}}),
		"wrong shape": "eyJ2IjoxfQ",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := Decode(cursor, "items", []string{"ses_1"}); !errors.Is(err, ErrInvalidCursor) {
				t.Fatalf("decode err = %v, want ErrInvalidCursor", err)
			}
		})
	}
}

func TestCursorResourceEnvelopeAppliesBeforeDecodeAndAfterEncode(t *testing.T) {
	oversized := strings.Repeat("a", MaximumCursorCharacters+1)
	if _, err := Decode(oversized, "items", nil); !errors.Is(err, ErrInvalidCursor) || !errors.Is(err, ErrCursorTooLarge) {
		t.Fatalf("Decode oversized err = %v, want ErrInvalidCursor and ErrCursorTooLarge", err)
	}
	if _, err := Encode("items", []string{oversized}, []string{"1"}); !errors.Is(err, ErrCursorTooLarge) {
		t.Fatalf("Encode oversized raw material err = %v, want ErrCursorTooLarge", err)
	}

	// Control characters fit the raw-material preflight but expand under JSON
	// escaping. The exact encoded bound must catch that second growth mode.
	escapingExpansion := strings.Repeat("\x00", MaximumCursorCharacters/2)
	if _, err := Encode("items", []string{escapingExpansion}, []string{"1"}); !errors.Is(err, ErrCursorTooLarge) {
		t.Fatalf("Encode escaping expansion err = %v, want ErrCursorTooLarge", err)
	}
}

func TestEncodeRejectsAnEmptyAnchorWithoutMintingEndOfCollection(t *testing.T) {
	if cursor, err := Encode("items", nil, nil); cursor != "" || !errors.Is(err, ErrInvalidCursorMaterial) {
		t.Fatalf("Encode empty key = (%q, %v), want empty cursor and ErrInvalidCursorMaterial", cursor, err)
	}
}

func TestRequestedLimitClampsToTheReadsCeiling(t *testing.T) {
	const ceiling = 200
	for _, test := range []struct {
		name      string
		requested RequestedLimit
		want      int
	}{
		{name: "named default", requested: DefaultLimit(), want: ceiling},
		{name: "explicit below ceiling", requested: mustLimit(t, 50), want: 50},
		{name: "explicit above ceiling", requested: mustLimit(t, 500), want: ceiling},
		{name: "explicit at ceiling", requested: mustLimit(t, ceiling), want: ceiling},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := test.requested.Resolve(ceiling)
			if err != nil || got != test.want {
				t.Fatalf("Resolve(%d) = (%d, %v), want %d", ceiling, got, err, test.want)
			}
		})
	}
}

func TestRequestedLimitRejectsInvalidConstructionAndState(t *testing.T) {
	for _, value := range []int{0, -1} {
		if _, err := NewLimit(value); !errors.Is(err, ErrInvalidLimit) {
			t.Fatalf("NewLimit(%d) err = %v, want ErrInvalidLimit", value, err)
		}
	}
	if _, err := DefaultLimit().Resolve(0); !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("Resolve with zero ceiling err = %v, want ErrInvalidLimit", err)
	}
	if _, err := (RequestedLimit{mode: requestedLimitExplicit}).Resolve(200); !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("corrupt explicit request err = %v, want ErrInvalidLimit", err)
	}
	if _, err := (RequestedLimit{mode: requestedLimitMode(255)}).Resolve(200); !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("unknown request mode err = %v, want ErrInvalidLimit", err)
	}
}

func mustLimit(t *testing.T, value int) RequestedLimit {
	t.Helper()
	limit, err := NewLimit(value)
	if err != nil {
		t.Fatalf("NewLimit(%d): %v", value, err)
	}
	return limit
}

// A page returns a cursor exactly when the over-fetch proved there is more, so a
// caller can never mistake a capped page for the end of the collection.
func TestPageOfSignalsMoreOnlyWhenTheOverFetchProvesIt(t *testing.T) {
	key := func(row string) []string { return []string{row} }

	full, err := PageOf([]string{"a", "b", "c"}, 2, "items", []string{"ses_1"}, key)
	if err != nil {
		t.Fatalf("PageOf over-fetch: %v", err)
	}
	if len(full.Rows) != 2 || full.NextCursor == "" {
		t.Fatalf("over-fetched page = %+v, want 2 rows and a cursor", full)
	}
	anchor, err := Decode(full.NextCursor, "items", []string{"ses_1"})
	if err != nil || len(anchor) != 1 || anchor[0] != "b" {
		t.Fatalf("anchor = (%v, %v), want the last returned row", anchor, err)
	}

	exact, err := PageOf([]string{"a", "b"}, 2, "items", []string{"ses_1"}, key)
	if err != nil {
		t.Fatalf("PageOf exact: %v", err)
	}
	if len(exact.Rows) != 2 || exact.NextCursor != "" {
		t.Fatalf("exact page = %+v, want 2 rows and no cursor", exact)
	}

	empty, err := PageOf[string](nil, 2, "items", []string{"ses_1"}, key)
	if err != nil {
		t.Fatalf("PageOf empty: %v", err)
	}
	if len(empty.Rows) != 0 || empty.NextCursor != "" {
		t.Fatalf("empty page = %+v, want nothing", empty)
	}
}

func TestPageOfRejectsInvalidConstructionWithoutPanicking(t *testing.T) {
	key := func(row string) []string { return []string{row} }
	if _, err := PageOf([]string{"a", "b"}, 0, "items", nil, key); !errors.Is(err, ErrInvalidLimit) {
		t.Fatalf("PageOf zero limit err = %v, want ErrInvalidLimit", err)
	}
	if _, err := PageOf([]string{"a", "b"}, 1, "", nil, key); !errors.Is(err, ErrInvalidCursorMaterial) {
		t.Fatalf("PageOf empty namespace err = %v, want ErrInvalidCursorMaterial", err)
	}
	if _, err := PageOf([]string{"a", "b"}, 1, "items", nil, nil); !errors.Is(err, ErrInvalidCursorMaterial) {
		t.Fatalf("PageOf nil next key err = %v, want ErrInvalidCursorMaterial", err)
	}
	if _, err := PageOf([]string{"a", "b"}, 1, "items", nil, func(string) []string { return nil }); !errors.Is(err, ErrInvalidCursorMaterial) {
		t.Fatalf("PageOf empty next key err = %v, want ErrInvalidCursorMaterial", err)
	}
}

func mustEncode(t *testing.T, namespace string, filters, key []string) string {
	t.Helper()
	cursor, err := Encode(namespace, filters, key)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	return cursor
}

func mustRawToken(t *testing.T, value token) string {
	t.Helper()
	cursor, err := opaquetoken.Encode(value, MaximumCursorCharacters)
	if err != nil {
		t.Fatalf("encode raw token: %v", err)
	}
	return cursor
}
