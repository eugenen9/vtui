package uba

import (
	"testing"

	"golang.org/x/text/unicode/bidi"
)

func classify(s string) (types []Class, pairTypes []Bracket, pairValues []rune) {
	for _, r := range s {
		props, _ := bidi.LookupRune(r)
		types = append(types, Class(props.Class()))
		switch {
		case props.IsOpeningBracket():
			pairTypes = append(pairTypes, BracketOpen)
			pairValues = append(pairValues, r)
		case props.IsBracket():
			pairTypes = append(pairTypes, BracketClose)
			pairValues = append(pairValues, []rune(bidi.ReverseString(string(r)))[0])
		default:
			pairTypes = append(pairTypes, BracketNone)
			pairValues = append(pairValues, 0)
		}
	}
	return
}

func levelsOf(t *testing.T, s string, paragraph int) ([]int, int) {
	t.Helper()
	types, pt, pv := classify(s)
	lv, base, err := Levels(types, pt, pv, paragraph)
	if err != nil {
		t.Fatalf("Levels(%q): %v", s, err)
	}
	return lv, base
}

func TestLevels_ParagraphDirection(t *testing.T) {
	// Hebrew first: auto detection picks RTL, a fixed LTR paragraph does not.
	s := "שלום abc"
	if _, base := levelsOf(t, s, ParagraphAuto); base != 1 {
		t.Errorf("auto: paragraph level %d, want 1", base)
	}
	lv, base := levelsOf(t, s, ParagraphLTR)
	if base != 0 {
		t.Errorf("ltr: paragraph level %d, want 0", base)
	}
	want := []int{1, 1, 1, 1, 0, 0, 0, 0}
	for i := range want {
		if lv[i] != want[i] {
			t.Fatalf("ltr levels = %v, want %v", lv, want)
		}
	}
}

func TestLevels_NumberInsideRTLRun(t *testing.T) {
	// A European number inside Hebrew sits at level 2 (rule I2), which is
	// what lets L2 keep its digits in reading order while the words swap.
	lv, _ := levelsOf(t, "אב 12 גד", ParagraphLTR)
	want := []int{1, 1, 1, 2, 2, 1, 1, 1}
	for i := range want {
		if lv[i] != want[i] {
			t.Fatalf("levels = %v, want %v", lv, want)
		}
	}
}

func TestLevels_BracketPairs(t *testing.T) {
	// N0: brackets enclosing Hebrew inside Hebrew are Hebrew too.
	lv, _ := levelsOf(t, "א(ב)ג", ParagraphLTR)
	for i, l := range lv {
		if l != 1 {
			t.Fatalf("position %d level %d, want 1 (%v)", i, l, lv)
		}
	}
}

func TestCoreValidationRejectsMalformedInput(t *testing.T) {
	if err := validateTypes(nil); err == nil {
		t.Fatal("validateTypes accepted empty input")
	}
	if err := validateTypes([]Class{L, B, R}); err == nil {
		t.Fatal("validateTypes accepted B before paragraph end")
	}
	for _, embedding := range []level{2, 3} {
		if err := validateParagraphEmbeddingLevel(embedding); err == nil {
			t.Errorf("embedding level %d was accepted", embedding)
		}
	}
	if err := validateLineBreaks([]int{2, 2}, 2); err == nil {
		t.Fatal("validateLineBreaks accepted duplicate break")
	}
	if err := validateLineBreaks([]int{1}, 2); err == nil {
		t.Fatal("validateLineBreaks accepted incomplete breaks")
	}
	if err := validatePbTypes(nil); err == nil {
		t.Fatal("validatePbTypes accepted empty input")
	}
	if err := validatePbTypes([]bracketType{bpNone, bracketType(99)}); err == nil {
		t.Fatal("validatePbTypes accepted unknown pair type")
	}
	if err := validatePbValues(nil, []bracketType{bpNone}); err == nil {
		t.Fatal("validatePbValues accepted nil values")
	}
	if err := validatePbValues([]rune{'('}, nil); err == nil {
		t.Fatal("validatePbValues accepted mismatched lengths")
	}
	if !Class(L).in(L, R) || Class(L).in(R) {
		t.Fatal("Class.in returned an incorrect membership result")
	}
}

func TestComputeReordering(t *testing.T) {
	tests := []struct {
		name   string
		levels []level
		want   []int
	}{
		{name: "left to right", levels: []level{0, 0, 0}, want: []int{0, 1, 2}},
		{name: "right to left", levels: []level{1, 1, 1}, want: []int{2, 1, 0}},
		{name: "nested runs", levels: []level{0, 1, 2, 2, 1, 0}, want: []int{0, 4, 2, 3, 1, 5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := computeReordering(tt.levels); !equalInts(got, tt.want) {
				t.Fatalf("computeReordering(%v) = %v, want %v", tt.levels, got, tt.want)
			}
		})
	}
}

func TestComputeMultilineReorderingDoesNotCrossLineBreaks(t *testing.T) {
	levels := []level{0, 1, 1, 0, 1, 1, 0}
	want := []int{0, 2, 1, 3, 5, 4, 6}
	if got := computeMultilineReordering(levels, []int{4, 7}); !equalInts(got, want) {
		t.Fatalf("computeMultilineReordering(%v) = %v, want %v", levels, got, want)
	}
}

func equalInts(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
