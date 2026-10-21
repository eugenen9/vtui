//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !plan9 && !android && (amd64 || arm64) && !vtui_nogogpu

package vtui

import (
	"sync"

	"github.com/unxed/vtinput"
)

// gogpu reports a file drag while it is still moving where the platform
// does - macOS (draggingEntered:/draggingUpdated:), X11 (XdndPosition) and
// Wayland (wl_data_device motion) - through OnDragEnter, OnDragMove and
// OnDragLeave. That is what lets a drop target show where a drop would land
// before it lands (f4#2049). Windows reports only the drop (WM_DROPFILES),
// and a drop gogpu said nothing ahead of is replayed by deliverFileDrop the
// old way, as an untracked gesture.
//
// The callbacks run on the loop that draws the window, and delivery waits
// for the UI thread, which may be about to need that loop; so they only
// queue the step, and one worker delivers the queue in order. Motion arrives
// far faster than a target needs it: a run of moves still waiting is folded
// into the newest.

// gogpuDragStep is one callback from gogpu, in its own pixels.
type gogpuDragStep struct {
	phase DragPhase
	paths []string
	x, y  float64
}

// gogpuDragSteps is the queue between gogpu's callbacks and the worker.
type gogpuDragSteps struct {
	mu      sync.Mutex
	pending []gogpuDragStep
	wake    chan struct{}

	// Worker-only: whether a tracked gesture is in progress, and the
	// payload it announced on the way in.
	inside  bool
	payload DragPayload
}

func (h *GogpuHost) handleDragEnter(paths []string, x, y float64) {
	DebugLog("GOGPU_DND: OnDragEnter at %.1f,%.1f with %d entry/entries", x, y, len(paths))
	h.pushDragStep(gogpuDragStep{phase: DragEnter, paths: paths, x: x, y: y})
}

func (h *GogpuHost) handleDragMove(x, y float64) {
	h.pushDragStep(gogpuDragStep{phase: DragOver, x: x, y: y})
}

func (h *GogpuHost) handleDragLeave() {
	DebugLog("GOGPU_DND: OnDragLeave")
	h.pushDragStep(gogpuDragStep{phase: DragLeave})
}

// pushDragStep queues a step for the worker, starting it on first use.
func (h *GogpuHost) pushDragStep(step gogpuDragStep) {
	q := &h.dragSteps
	q.mu.Lock()
	if q.wake == nil {
		q.wake = make(chan struct{}, 1)
		go h.runDragSteps()
	}
	if n := len(q.pending); step.phase == DragOver && n > 0 && q.pending[n-1].phase == DragOver {
		q.pending[n-1] = step
	} else {
		q.pending = append(q.pending, step)
	}
	q.mu.Unlock()
	select {
	case q.wake <- struct{}{}:
	default:
	}
}

// runDragSteps delivers the queued steps in order, for the life of the host.
func (h *GogpuHost) runDragSteps() {
	q := &h.dragSteps
	for range q.wake {
		q.mu.Lock()
		steps := q.pending
		q.pending = nil
		q.mu.Unlock()
		for _, step := range steps {
			h.deliverDragStep(step)
		}
	}
}

func (h *GogpuHost) deliverDragStep(step gogpuDragStep) {
	q := &h.dragSteps
	switch step.phase {
	case DragEnter:
		q.inside = true
		q.payload = gogpuHoverPayload(step.paths)
		h.deliverTracked(DragEnter, step.x, step.y, q.payload)
	case DragOver:
		if !q.inside {
			// A move with no enter before it: gogpu lost the enter, or
			// the gesture started before the window existed. Treat it as
			// the way in.
			q.inside = true
			q.payload = gogpuHoverPayload(nil)
			h.deliverTracked(DragEnter, step.x, step.y, q.payload)
			return
		}
		h.deliverTracked(DragOver, step.x, step.y, q.payload)
	case DragLeave:
		if q.inside {
			q.inside = false
			DeliverDragEvent(&DragEvent{Phase: DragLeave, Tracked: true})
		}
	case DragDrop:
		if !q.inside {
			h.deliverFileDrop(step.paths, step.x, step.y)
			return
		}
		q.inside = false
		h.deliverTrackedDrop(step.paths, step.x, step.y)
	}
}

// gogpuHoverPayload is what a drag announces on the way in. gogpu only ever
// reports file drags, and Wayland hands the paths over on the drop alone,
// so the type is announced even when the paths are not there yet.
func gogpuHoverPayload(paths []string) DragPayload {
	p := gogpuDragPayload(paths)
	if len(p.Kinds) == 0 {
		p.Kinds = []string{"text/uri-list"}
	}
	return p
}

// dragCell converts a position gogpu reported into a cell, along with the
// modifiers held, which gogpu's drag callbacks do not carry.
func (h *GogpuHost) dragCell(x, y float64) (int, int, vtinput.ControlKeyState) {
	h.mu.Lock()
	cellW, cellH, mods := h.cellW, h.cellH, h.currentMods
	h.mu.Unlock()
	dx, dy := h.dropPixels(x, y)
	return gogpuDropCell(dx, cellW), gogpuDropCell(dy, cellH), mods
}

func (h *GogpuHost) deliverTracked(phase DragPhase, x, y float64, payload DragPayload) DropAction {
	cx, cy, mods := h.dragCell(x, y)
	return DeliverDragEvent(&DragEvent{
		Phase:     phase,
		X:         cx,
		Y:         cy,
		Modifiers: mods,
		Allowed:   gogpuDropAllowed,
		Suggested: DropCopy,
		Payload:   payload,
		Tracked:   true,
	})
}

// deliverTrackedDrop ends a gesture the target followed: just the drop, at
// the place the target has been showing.
func (h *GogpuHost) deliverTrackedDrop(paths []string, x, y float64) {
	payload := gogpuDragPayload(paths)
	if payload.IsEmpty() {
		DebugLog("GOGPU_DND: the drop carried nothing we could decode, ignoring it")
		DeliverDragEvent(&DragEvent{Phase: DragLeave, Tracked: true})
		return
	}
	action := h.deliverTracked(DragDrop, x, y, payload)
	DebugLog("GOGPU_DND: %d file(s) dropped after a tracked drag, handled as %s", len(payload.Paths), action)
	if h.app != nil {
		h.app.RequestRedraw()
	}
}
