package vtui

import "testing"

// titleButtonWindow is one window or dialog type under test: how to draw it,
// where its top-right corner is and which title-bar buttons it shows.
type titleButtonWindow struct {
	name                string
	show                func(scr *ScreenBuf)
	x2, y1              int
	offset              int // BorderedFrame.getControlOffset of the window's frame
	showClose, showZoom bool
}

func titleButtonWindows(t *testing.T) []titleButtonWindow {
	t.Helper()
	SetDefaultPalette()
	var out []titleButtonWindow

	frame := NewBorderedFrame(5, 5, 45, 10, DoubleBox, " Frame ")
	frame.ShowClose = true
	out = append(out, titleButtonWindow{"BorderedFrame", frame.DisplayObject, 45, 5, frame.getControlOffset(), true, false})

	win := NewWindow(5, 5, 45, 12, " Window ")
	out = append(out, titleButtonWindow{"Window (close and zoom)", win.Show, 45, 5, win.frame.getControlOffset(), true, true})

	dlg := NewDialog(5, 5, 45, 12, " Dialog ")
	dlg.ShowClose = true
	out = append(out, titleButtonWindow{"Dialog with close", dlg.Show, 45, 5, dlg.frame.getControlOffset(), true, false})

	dlgZoom := NewDialog(5, 5, 45, 12, " Dialog ")
	dlgZoom.ShowClose = true
	dlgZoom.ShowZoom = true
	out = append(out, titleButtonWindow{"Dialog with close and zoom", dlgZoom.Show, 45, 5, dlgZoom.frame.getControlOffset(), true, true})

	zoomOnly := NewWindow(5, 5, 45, 12, " Zoom only ")
	zoomOnly.ShowClose = false
	out = append(out, titleButtonWindow{"Window (zoom only)", zoomOnly.Show, 45, 5, zoomOnly.frame.getControlOffset(), false, true})

	table, _ := tableDialogFixture(t)
	_, ty1, tx2, _ := table.GetPosition()
	out = append(out, titleButtonWindow{"Table dialog", table.Show, tx2, ty1, table.frame.getControlOffset(), true, false})

	engine := NewHelpEngine(&mockHelpVFS{})
	engine.AddTopic(&HelpTopic{Name: "Root", Lines: []string{"Root help"}})
	hv := NewHelpView(engine, "Root")
	_, hy1, hx2, _ := hv.GetPosition()
	out = append(out, titleButtonWindow{"HelpView", hv.Show, hx2, hy1, hv.frame.getControlOffset(), true, false})
	return out
}

// titleCells returns the three cells of the title-bar button starting at x.
func titleCells(scr *ScreenBuf, x, y int) [3]CharInfo {
	var cells [3]CharInfo
	for i := range cells {
		cells[i] = scr.buf[y*scr.width+x+i]
	}
	return cells
}

func checkTitleButton(t *testing.T, what string, cells [3]CharInfo, want [3]string) {
	t.Helper()
	for i, c := range cells {
		if got := CellString(c.Char); got != want[i] {
			t.Errorf("%s cell %d: got %q, want %q", what, i, got, want[i])
		}
		if c.Attributes != cells[0].Attributes {
			t.Errorf("%s cell %d: attribute %x differs from the first cell's %x", what, i, c.Attributes, cells[0].Attributes)
		}
	}
}

// TestTitleButtons_AllWindowTypes pins the look of the title-bar buttons of
// every kind of window and dialog: the classic style (and a text terminal,
// whatever style is selected) keeps "[x]"; the rounded style on a graphical
// backend has no square brackets, and the button is still 3 cells wide.
func TestTitleButtons_AllWindowTypes(t *testing.T) {
	styles := []struct {
		name    string
		backend string
		style   GlyphStyle
		rounded bool
	}{
		{"classic style, graphical backend", "gogpu", GlyphStyleClassic, false},
		{"rounded style, text terminal", "", GlyphStyleRounded, false},
		{"rounded style, graphical backend", "gogpu", GlyphStyleRounded, true},
	}
	for _, st := range styles {
		t.Run(st.name, func(t *testing.T) {
			roundedButtonEnv(t, st.backend, st.style)
			left, right := string(UIStrings.CloseBrackets[0]), string(UIStrings.CloseBrackets[1])
			if st.rounded {
				left, right = " ", " "
			}
			for _, w := range titleButtonWindows(t) {
				scr := NewSilentScreenBuf()
				scr.AllocBuf(100, 40)
				w.show(scr)

				closeX := w.x2 - w.offset
				zoomX := closeX
				if w.showClose {
					zoomX -= 3
					checkTitleButton(t, w.name+" close button", titleCells(scr, closeX, w.y1),
						[3]string{left, string(EffectiveCloseSymbol()), right})
				}
				if w.showZoom {
					checkTitleButton(t, w.name+" zoom button", titleCells(scr, zoomX, w.y1),
						[3]string{left, string(UIStrings.ZoomSymbol), right})
				}
				if !w.showClose && !w.showZoom {
					t.Fatalf("%s: case shows no title button", w.name)
				}
			}
		})
	}
}

func TestTitleButtons_WindowNumber(t *testing.T) {
	cases := []struct {
		name    string
		backend string
		style   GlyphStyle
		rounded bool
	}{
		{"classic", "gogpu", GlyphStyleClassic, false},
		{"rounded graphical", "gogpu", GlyphStyleRounded, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			roundedButtonEnv(t, c.backend, c.style)
			SetDefaultPalette()
			win := NewWindow(5, 5, 45, 12, " Window ")
			win.Number = 3
			scr := NewSilentScreenBuf()
			scr.AllocBuf(100, 40)
			win.Show(scr)
			left, right := string(UIStrings.CloseBrackets[0]), string(UIStrings.CloseBrackets[1])
			if c.rounded {
				left, right = " ", " "
			}
			checkTitleButton(t, "window number", titleCells(scr, 7, 5), [3]string{left, "3", right})
		})
	}
}

func TestTitleButtonText(t *testing.T) {
	roundedButtonEnv(t, "gogpu", GlyphStyleClassic)
	if got, want := TitleButtonText("x"), "[x]"; got != want {
		t.Errorf("classic: got %q, want %q", got, want)
	}
	SetGlyphStyle(GlyphStyleRounded)
	if got, want := TitleButtonText("x"), " x "; got != want {
		t.Errorf("rounded: got %q, want %q", got, want)
	}
	SetActiveBackend("")
	if got, want := TitleButtonText("x"), "[x]"; got != want {
		t.Errorf("rounded on a text terminal: got %q, want %q", got, want)
	}
}
