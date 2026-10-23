package vtui

import (
	"testing"
	"time"

	"github.com/unxed/vtinput"
)

// A bracketed paste is drawn once per pasteRenderGap while its keys are queued,
// not once per key (unxed/f4#2157).
func TestFrameManager_ShouldDeferRender(t *testing.T) {
	fm := &frameManager{}
	fm.EventChan = make(chan *vtinput.InputEvent, 4)
	now := time.Now()
	fm.lastRender = now

	if fm.shouldDeferRender(now) {
		t.Error("deferred outside a paste")
	}
	fm.pasting = true
	if fm.shouldDeferRender(now) {
		t.Error("deferred with no event waiting")
	}
	fm.EventChan <- &vtinput.InputEvent{Type: vtinput.KeyEventType}
	if !fm.shouldDeferRender(now.Add(10 * time.Millisecond)) {
		t.Error("not deferred during a paste with events waiting")
	}
	if fm.shouldDeferRender(now.Add(pasteRenderGap + time.Millisecond)) {
		t.Error("deferred for longer than the gap")
	}
	fm.pasting = false
	if fm.shouldDeferRender(now.Add(10 * time.Millisecond)) {
		t.Error("deferred after the paste ended")
	}
}
