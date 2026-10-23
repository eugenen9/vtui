//go:build !nocrashreport

package vtui

import (
	"testing"

	"github.com/unxed/vtinput"
)

// A paste of a megabyte is a million key events, and three log lines were
// formatted for each (unxed/f4#2157); the keys between the marks of a paste are
// not logged one by one, and the log goes on as before after the paste.
func TestFrameManager_KeysOfAPasteAreNotLoggedOneByOne(t *testing.T) {
	fm := gestureManager(t)
	fm.Push(NewDialog(0, 1, 40, 10, "T"))

	last := func() string {
		l := getMemLogs()
		if len(l) == 0 {
			return ""
		}
		return l[len(l)-1]
	}
	key := func(ch rune) *vtinput.InputEvent {
		return &vtinput.InputEvent{Type: vtinput.KeyEventType, KeyDown: true, Char: ch, VirtualKeyCode: vtinput.VK_A}
	}

	fm.dispatchEvent(&vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: true}, false)
	before := last()
	for i := 0; i < 300; i++ {
		fm.dispatchEvent(key('a'), false)
	}
	if after := last(); after != before {
		t.Fatalf("a key of a paste was logged: %q", after)
	}

	fm.dispatchEvent(&vtinput.InputEvent{Type: vtinput.PasteEventType, PasteStart: false}, false)
	fm.dispatchEvent(key('b'), false)
	if after := last(); after == before {
		t.Fatalf("a key after the paste is not logged: %q", after)
	}
}
