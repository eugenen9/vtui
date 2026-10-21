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

// A drop on a tab is refused, and the rest it ended switches nothing. A
// leave over the tabs does not reach the target a second time.
func TestDragEndingOnWorkspaceTabs(t *testing.T) {
	fm, take, target := newWorkspaceDragFixture(t)
	x, y := firstTabCell(fm)

	fm.routeDrag(target, &DragEvent{Phase: DragOver, X: x, Y: y, Tracked: true})
	if got := fm.routeDrag(target, &DragEvent{Phase: DragDrop, X: x, Y: y, Tracked: true}); got != DropNone {
		t.Fatalf("a drop on a tab answered %s, want none", got)
	}
	if got := take(); len(got) != 1 || got[0] != DragLeave {
		t.Fatalf("target saw %v, want only the leave on reaching the tabs", got)
	}
	if runOneTask(t, fm, 100*time.Millisecond) || fm.ActiveIdx != 1 {
		t.Fatal("a drag that ended on a tab still switched")
	}

	// Back on the tabs, then the drag leaves the window: the target already
	// had its leave when the drag reached the tabs.
	fm.routeDrag(target, &DragEvent{Phase: DragOver, X: x, Y: y, Tracked: true})
	take()
	fm.routeDrag(target, &DragEvent{Phase: DragLeave, X: 5, Y: 5, Tracked: true})
	if got := take(); len(got) != 0 {
		t.Fatalf("target saw %v after leaving from the tabs, want nothing", got)
	}
}

// The rest follows the tab under the drag: the active workspace's tab and
// the separator between tabs start nothing, staying on a tab keeps its
// rest, and moving to another tab ends it.
func TestDragRestFollowsTheTab(t *testing.T) {
	fm, take, target := newWorkspaceDragFixture(t)
	x, y := firstTabCell(fm)
	separator := fm.workspaceTabHits[0].x2 + 1
	active := fm.workspaceTabHits[1].x1 + 1

	if got := fm.routeDrag(target, &DragEvent{Phase: DragOver, X: separator, Y: 0, Tracked: true}); got != DropNone {
		t.Fatalf("the separator between tabs answered %s, want none", got)
	}
	if fm.workspaceDrag.screen != nil {
		t.Fatal("the separator must not start a rest")
	}
	fm.routeDrag(target, &DragEvent{Phase: DragOver, X: x, Y: y, Tracked: true})
	rest := fm.workspaceDrag.screen
	fm.routeDrag(target, &DragEvent{Phase: DragOver, X: x + 1, Y: y, Tracked: true})
	if rest == nil || fm.workspaceDrag.screen != rest {
		t.Fatal("staying on the tab must keep its rest")
	}
	fm.routeDrag(target, &DragEvent{Phase: DragOver, X: active, Y: 0, Tracked: true})
	if fm.workspaceDrag.screen != nil {
		t.Fatal("the active workspace's tab must end the rest and start none")
	}
	if runOneTask(t, fm, 100*time.Millisecond) || fm.ActiveIdx != 1 {
		t.Fatal("a rest moved off the tab still switched")
	}
	if got := take(); len(got) != 1 || got[0] != DragLeave {
		t.Fatalf("target saw %v, want one leave for the whole time on the tabs", got)
	}
}

// A rest whose timer already fired, but whose switch had not run yet when
// the drag moved on, switches nothing.
func TestStaleWorkspaceTabRestIsIgnored(t *testing.T) {
	fm, _, target := newWorkspaceDragFixture(t)
	x, y := firstTabCell(fm)

	fm.routeDrag(target, &DragEvent{Phase: DragOver, X: x, Y: y, Tracked: true})
	var task func()
	select {
	case task = <-fm.TaskChan:
	case <-time.After(2 * time.Second):
		t.Fatal("the rest never fired")
	}
	fm.routeDrag(target, &DragEvent{Phase: DragOver, X: 5, Y: 5, Tracked: true})
	task()
	if fm.ActiveIdx != 1 {
		t.Fatalf("a rest the drag had left switched to workspace %d", fm.ActiveIdx)
	}
}
