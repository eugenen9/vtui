//go:build !freebsd && !dragonfly && !openbsd && !netbsd && !illumos && !solaris && !plan9 && !android && (amd64 || arm64) && !vtui_nogogpu

package vtui

import (
	"reflect"
	"testing"
	"time"
)

type recordedDrag struct {
	phase   DragPhase
	x, y    int
	tracked bool
	paths   []string
	offers  bool
}

func recordDrags(t *testing.T) *[]recordedDrag {
	t.Helper()
	var got []recordedDrag
	withInlineDropTarget(t, func(ev *DragEvent) DropAction {
		got = append(got, recordedDrag{ev.Phase, ev.X, ev.Y, ev.Tracked, ev.Payload.Paths, ev.Payload.OffersFiles()})
		return DropCopy
	})
	return &got
}

// A drag gogpu follows (macOS, X11, Wayland) reaches the target as it
// moves, marked tracked, and its drop comes alone: the enter was already
// delivered, so it is not replayed (f4#2049).
func TestGogpuTrackedDragReachesTargetAsItMoves(t *testing.T) {
	got := recordDrags(t)
	host := &GogpuHost{cellW: 8, cellH: 16}

	host.deliverDragStep(gogpuDragStep{phase: DragEnter, paths: []string{"/tmp/a.txt"}, x: 8, y: 16})
	host.deliverDragStep(gogpuDragStep{phase: DragOver, x: 40, y: 48})
	host.deliverDragStep(gogpuDragStep{phase: DragDrop, paths: []string{"/tmp/a.txt"}, x: 40, y: 48})

	want := []recordedDrag{
		{DragEnter, 1, 1, true, []string{"/tmp/a.txt"}, true},
		{DragOver, 5, 3, true, []string{"/tmp/a.txt"}, true},
		{DragDrop, 5, 3, true, []string{"/tmp/a.txt"}, true},
	}
	if !reflect.DeepEqual(*got, want) {
		t.Fatalf("gesture = %+v\nwant      %+v", *got, want)
	}
}

// Wayland names the files only on the drop. The target still has to hear
// that files are coming, or it would refuse the drag while it moves.
func TestGogpuTrackedDragWithoutPathsStillOffersFiles(t *testing.T) {
	got := recordDrags(t)
	host := &GogpuHost{cellW: 8, cellH: 16}

	host.deliverDragStep(gogpuDragStep{phase: DragEnter, x: 8, y: 16})
	host.deliverDragStep(gogpuDragStep{phase: DragLeave})

	if len(*got) != 2 || !(*got)[0].offers || (*got)[1].phase != DragLeave || !(*got)[1].tracked {
		t.Fatalf("gesture = %+v, want an enter offering files, then a tracked leave", *got)
	}
}

// A drop gogpu reported nothing ahead of (Windows) is still replayed the old
// way, untracked: the target showed nothing before it.
func TestGogpuUntrackedDropIsReplayed(t *testing.T) {
	got := recordDrags(t)
	host := &GogpuHost{cellW: 8, cellH: 16}

	host.deliverDragStep(gogpuDragStep{phase: DragDrop, paths: []string{"/tmp/a.txt"}, x: 40, y: 48})

	if len(*got) != 2 || (*got)[0].phase != DragEnter || (*got)[1].phase != DragDrop || (*got)[0].tracked || (*got)[1].tracked {
		t.Fatalf("gesture = %+v, want an untracked enter and drop", *got)
	}
}

// A leave ends the gesture: a later drop without a new enter is untracked.
// A move with no enter before it opens the gesture instead.
func TestGogpuDragLeaveEndsGesture(t *testing.T) {
	got := recordDrags(t)
	host := &GogpuHost{cellW: 8, cellH: 16}

	host.deliverDragStep(gogpuDragStep{phase: DragLeave}) // nothing in progress: silent
	host.deliverDragStep(gogpuDragStep{phase: DragOver, x: 8, y: 16})
	host.deliverDragStep(gogpuDragStep{phase: DragLeave})
	host.deliverDragStep(gogpuDragStep{phase: DragDrop, paths: []string{"/tmp/b"}, x: 8, y: 16})

	var phases []DragPhase
	for _, d := range *got {
		phases = append(phases, d.phase)
	}
	if want := []DragPhase{DragEnter, DragLeave, DragEnter, DragDrop}; !reflect.DeepEqual(phases, want) {
		t.Fatalf("phases = %v, want %v", phases, want)
	}
	if (*got)[3].tracked {
		t.Fatal("a drop after the drag left the window must be untracked")
	}
}

// The callbacks gogpu calls only queue the step; the worker the first one
// starts delivers the queue in order. A drop that names nothing ends a
// followed drag as a leave, and a drop after it, with nothing ahead of it,
// is replayed untracked.
func TestGogpuDragCallbacksDeliverThroughTheWorker(t *testing.T) {
	steps := make(chan recordedDrag, 8)
	withInlineDropTarget(t, func(ev *DragEvent) DropAction {
		steps <- recordedDrag{ev.Phase, ev.X, ev.Y, ev.Tracked, ev.Payload.Paths, ev.Payload.OffersFiles()}
		return DropCopy
	})
	host := &GogpuHost{cellW: 8, cellH: 16}

	host.handleDragEnter([]string{"/tmp/a.txt"}, 8, 16)
	host.handleDragMove(40, 48)
	host.handleDragLeave()
	host.handleDragEnter(nil, 8, 16) // Wayland: the paths come with the drop
	host.handleFileDrop([]string{"   "}, 8, 16)
	host.handleFileDrop([]string{"/tmp/b"}, 8, 16)
	// Once every step is in, nothing is left for the worker: closing the
	// channel ends it.
	t.Cleanup(func() { close(host.dragSteps.wake) })

	want := []recordedDrag{
		{DragEnter, 1, 1, true, []string{"/tmp/a.txt"}, true},
		{DragOver, 5, 3, true, []string{"/tmp/a.txt"}, true},
		{DragLeave, 0, 0, true, nil, false},
		{DragEnter, 1, 1, true, nil, true},
		{DragLeave, 0, 0, true, nil, false},
		{DragEnter, 1, 1, false, []string{"/tmp/b"}, true},
		{DragDrop, 1, 1, false, []string{"/tmp/b"}, true},
	}
	var got []recordedDrag
	for range want {
		select {
		case d := <-steps:
			got = append(got, d)
		case <-time.After(5 * time.Second):
			t.Fatalf("the worker delivered %+v and then nothing, want %+v", got, want)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("gesture = %+v\nwant      %+v", got, want)
	}
}

// Moves still waiting for the worker fold into the newest; every other step
// keeps its place.
func TestGogpuDragStepsFoldWaitingMoves(t *testing.T) {
	host := &GogpuHost{}
	host.dragSteps.wake = make(chan struct{}, 1) // no worker: the queue is inspected as is

	host.pushDragStep(gogpuDragStep{phase: DragEnter})
	host.pushDragStep(gogpuDragStep{phase: DragOver, x: 1})
	host.pushDragStep(gogpuDragStep{phase: DragOver, x: 2})
	host.pushDragStep(gogpuDragStep{phase: DragOver, x: 3})
	host.pushDragStep(gogpuDragStep{phase: DragDrop})

	var got []gogpuDragStep
	got = append(got, host.dragSteps.pending...)
	if len(got) != 3 || got[0].phase != DragEnter || got[1].phase != DragOver || got[1].x != 3 || got[2].phase != DragDrop {
		t.Fatalf("queue = %+v, want enter, the newest move, drop", got)
	}
}
