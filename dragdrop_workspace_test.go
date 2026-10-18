package vtui

import (
	"testing"
	"time"
)

// newWorkspaceDragFixture is a manager with two workspaces, the second one
// active, and their tabs drawn on row 0.
func newWorkspaceDragFixture(t *testing.T) (*frameManager, func() []DragPhase, DropTarget) {
	t.Helper()
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(60, 10)
	fm := &frameManager{}
	fm.Init(scr)
	fm.Push(newMockFrame(0, 1, 59, 9, false))
	fm.AddScreen(newMockFrame(0, 1, 59, 9, false))
	fm.ConfigureWorkspaceTabs(WorkspaceTabsMultiple, WorkspaceCtrlTabDirect)
	fm.drawWorkspaceTabs()
	if len(fm.workspaceTabHits) != 2 || fm.ActiveIdx != 1 {
		t.Fatalf("fixture: %d tabs, active %d", len(fm.workspaceTabHits), fm.ActiveIdx)
	}
	old := WorkspaceDragDwell
	WorkspaceDragDwell = 20 * time.Millisecond
	t.Cleanup(func() { WorkspaceDragDwell = old })

	var phases []DragPhase
	target := DropTargetFunc(func(ev *DragEvent) DropAction {
		phases = append(phases, ev.Phase)
		return DropCopy
	})
	return fm, func() []DragPhase { p := phases; phases = nil; return p }, target
}

func firstTabCell(fm *frameManager) (int, int) {
	return fm.workspaceTabHits[0].x1 + 1, 0
}

func runOneTask(t *testing.T, fm *frameManager, wait time.Duration) bool {
	t.Helper()
	select {
	case task := <-fm.TaskChan:
		task()
		return true
	case <-time.After(wait):
		return false
	}
}

// Resting a followed drag on another workspace's tab brings it up, and the
// application's target lets go of its highlight while the drag is on the
// tabs (f4#2049).
func TestDragRestingOnWorkspaceTabSwitches(t *testing.T) {
	fm, take, target := newWorkspaceDragFixture(t)
	x, y := firstTabCell(fm)

	fm.routeDrag(target, &DragEvent{Phase: DragEnter, X: 5, Y: 5, Tracked: true})
	if got := fm.routeDrag(target, &DragEvent{Phase: DragOver, X: x, Y: y, Tracked: true}); got != DropNone {
		t.Fatalf("a tab takes no drop, got %s", got)
	}
	if !runOneTask(t, fm, 2*time.Second) {
		t.Fatal("the rest on the tab never switched")
	}
	if fm.ActiveIdx != 0 {
		t.Fatalf("active workspace %d, want 0", fm.ActiveIdx)
	}

	fm.routeDrag(target, &DragEvent{Phase: DragOver, X: 5, Y: 5, Tracked: true})
	want := []DragPhase{DragEnter, DragLeave, DragEnter}
	got := take()
	if len(got) != len(want) {
		t.Fatalf("target saw %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("target saw %v, want %v", got, want)
		}
	}
}

// A drag that only passes over a tab does not switch.
func TestDragPassingOverWorkspaceTabStays(t *testing.T) {
	fm, _, target := newWorkspaceDragFixture(t)
	x, y := firstTabCell(fm)

	fm.routeDrag(target, &DragEvent{Phase: DragOver, X: x, Y: y, Tracked: true})
	fm.routeDrag(target, &DragEvent{Phase: DragOver, X: 5, Y: 5, Tracked: true})
	if runOneTask(t, fm, 100*time.Millisecond) && fm.ActiveIdx != 1 {
		t.Fatal("a drag that left the tab before the dwell still switched")
	}
	if fm.ActiveIdx != 1 {
		t.Fatalf("active workspace %d, want 1", fm.ActiveIdx)
	}
}

// A drop replayed at the end never rested anywhere: it reaches the target
// as before, even on row 0.
func TestUntrackedDragIgnoresWorkspaceTabs(t *testing.T) {
	fm, take, target := newWorkspaceDragFixture(t)
	x, y := firstTabCell(fm)

	if got := fm.routeDrag(target, &DragEvent{Phase: DragDrop, X: x, Y: y}); got != DropCopy {
		t.Fatalf("untracked drop answered %s, want the target's copy", got)
	}
	if got := take(); len(got) != 1 || got[0] != DragDrop {
		t.Fatalf("target saw %v, want just the drop", got)
	}
}
