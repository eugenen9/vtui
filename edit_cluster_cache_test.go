package vtui

import (
	"strings"
	"testing"
)

// The clusters of a long line were computed again for every frame, so each
// frame cost time in proportion to the whole text (unxed/vtui#246).
func TestEdit_ShowKeepsTheClustersWhileTheTextIsTheSame(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(20, 1)

	e := NewEdit(0, 0, 20, strings.Repeat("x", 50000))
	for i := 0; i < 50; i++ {
		e.Show(scr)
	}
	if got := e.clusterCache.builds; got != 1 {
		t.Fatalf("the clusters were computed %d times for 50 frames of the same text, want once", got)
	}
	e.SetText(strings.Repeat("y", 50000))
	e.Show(scr)
	if got := e.clusterCache.builds; got != 2 {
		t.Fatalf("after the text changed the clusters were computed %d times in all, want 2", got)
	}
}

// What is painted does not depend on the cache: the same line painted by an
// edit that has painted it before and by a fresh one is the same, with the
// selection and the scrolling.
func TestEdit_ShowWithTheCacheMatchesAFreshEdit(t *testing.T) {
	SetDefaultPalette()
	text := strings.Repeat("abcdefghij", 30)

	paint := func(e *Edit) []CharInfo {
		scr := NewSilentScreenBuf()
		scr.AllocBuf(20, 1)
		e.Show(scr)
		out := make([]CharInfo, 20)
		for x := 0; x < 20; x++ {
			out[x] = scr.GetCell(x, 0)
		}
		return out
	}

	warm := NewEdit(0, 0, 20, text)
	paint(warm)
	paint(warm)
	warm.selStart, warm.selEnd = 295, 299
	fresh := NewEdit(0, 0, 20, text)
	fresh.selStart, fresh.selEnd = 295, 299
	a, b := paint(warm), paint(fresh)
	for x := range a {
		if a[x] != b[x] {
			t.Fatalf("cell %d differs: warm %+v, fresh %+v", x, a[x], b[x])
		}
	}
}
