package vtui

import "time"

// A drag of files that rests on a workspace tab brings that workspace up,
// the way a browser's tab strip does, so the files can be dropped there
// without letting go and switching first (f4#2049). It needs a drag the
// backend follows while it moves (DragEvent.Tracked): a drop replayed at the
// end has never rested anywhere.
//
// While the drag is over the tabs the application's target sees a
// DragLeave, so whatever it highlighted goes; back over the content, a
// DragEnter. The tabs themselves take no drop.

// WorkspaceDragDwell is how long a drag has to rest on a workspace tab
// before that workspace comes up.
var WorkspaceDragDwell = 500 * time.Millisecond

// workspaceDragState is the drag's business with the tabs, owned by the UI
// thread.
type workspaceDragState struct {
	onTabs bool       // the last step was over the tab row
	screen *AppScreen // the tab it rests on, nil if none or the active one
	timer  *time.Timer
	gen    uint64 // bumped whenever the rest is over, so a late timer is ignored
}

// routeDrag passes one step of a drag to the application, unless it is over
// the workspace tabs. Runs on the UI thread.
func (fm *frameManager) routeDrag(t DropTarget, ev *DragEvent) DropAction {
	ws := &fm.workspaceDrag
	idx, onTabs := fm.workspaceTabUnderDrag(ev)
	if onTabs {
		if !ws.onTabs {
			ws.onTabs = true
			safeHandleDrag(t, &DragEvent{Phase: DragLeave, X: ev.X, Y: ev.Y, Tracked: ev.Tracked})
		}
		if ev.Phase == DragEnter || ev.Phase == DragOver {
			fm.restOnWorkspaceTab(idx)
		} else {
			fm.endWorkspaceDragRest()
			ws.onTabs = false
		}
		return DropNone
	}

	fm.endWorkspaceDragRest()
	if ws.onTabs {
		ws.onTabs = false
		if ev.Phase == DragOver {
			back := *ev
			back.Phase = DragEnter
			return safeHandleDrag(t, &back)
		}
		if ev.Phase == DragLeave {
			return DropNone // the target already had its leave
		}
	}
	return safeHandleDrag(t, ev)
}

// workspaceTabUnderDrag reports the index of the workspace tab under a
// tracked drag, and whether the drag is on the tab row at all.
func (fm *frameManager) workspaceTabUnderDrag(ev *DragEvent) (int, bool) {
	if !ev.Tracked || ev.Y != 0 || len(fm.workspaceTabHits) == 0 || !fm.workspaceTabsVisible() {
		return -1, false
	}
	for _, hit := range fm.workspaceTabHits {
		if ev.X >= hit.x1 && ev.X <= hit.x2 {
			return hit.index, true
		}
	}
	return -1, ev.X <= fm.workspaceTabHits[len(fm.workspaceTabHits)-1].x2
}

// restOnWorkspaceTab starts, keeps or moves the rest on tab idx. The timer
// remembers the workspace itself rather than its index, which a workspace
// closed meanwhile would shift.
func (fm *frameManager) restOnWorkspaceTab(idx int) {
	ws := &fm.workspaceDrag
	var screen *AppScreen
	if idx >= 0 && idx < len(fm.Screens) && idx != fm.ActiveIdx {
		screen = fm.Screens[idx]
	}
	if screen == ws.screen {
		return
	}
	fm.endWorkspaceDragRest()
	if screen == nil {
		return
	}
	ws.screen = screen
	gen := ws.gen
	ws.timer = time.AfterFunc(WorkspaceDragDwell, func() {
		fm.PostTask(func() {
			if ws.gen != gen || ws.screen != screen {
				return
			}
			for i, s := range fm.Screens {
				if s == screen {
					DebugLog("DND: a drag rested on workspace tab %d, switching to it", i)
					fm.SwitchScreen(i)
					break
				}
			}
			ws.screen = nil
		})
	})
}

// endWorkspaceDragRest forgets the tab the drag rested on.
func (fm *frameManager) endWorkspaceDragRest() {
	ws := &fm.workspaceDrag
	ws.gen++
	ws.screen = nil
	if ws.timer != nil {
		ws.timer.Stop()
		ws.timer = nil
	}
}
