package vtui

import "github.com/unxed/vtui/vreactive"

// BindText binds the edit's text to a property in both directions: the text
// follows the property, and typing updates it. The property's current value is
// applied at once. The returned function undoes the binding and restores the
// edit's previous OnTextChange handler. Notification echoes between the two
// sides are suppressed by vreactive.TwoWayBind.
func (e *Edit) BindText(prop vreactive.Property[string]) func() {
	if e == nil || prop == nil {
		return func() {}
	}
	previous := e.OnTextChange
	return vreactive.TwoWayBind(prop, e.GetText, e.SetText, func(onChange func(string)) func() {
		e.OnTextChange = func(text string) {
			if previous != nil {
				previous(text)
			}
			onChange(text)
		}
		return func() { e.OnTextChange = previous }
	})
}

// BindChecked binds the checkbox to a boolean property in both directions:
// the box is checked while the property is true, and clicking it updates the
// property. The undefined state of a three-state checkbox reads as false. The
// returned function undoes the binding and restores the previous OnChange.
func (cb *Checkbox) BindChecked(prop vreactive.Property[bool]) func() {
	if cb == nil || prop == nil {
		return func() {}
	}
	previous := cb.OnChange
	return vreactive.TwoWayBind(prop,
		func() bool { return cb.State == 1 },
		func(checked bool) {
			if checked {
				cb.State = 1
			} else {
				cb.State = 0
			}
			cb.NotifyChange()
		},
		func(onChange func(bool)) func() {
			cb.OnChange = func(state int) {
				if previous != nil {
					previous(state)
				}
				onChange(state == 1)
			}
			return func() { cb.OnChange = previous }
		})
}

// BindText makes the text follow a property: the current value is applied at
// once and every later change is shown. The returned function undoes the
// binding.
func (t *Text) BindText(prop vreactive.Property[string]) func() {
	if t == nil || prop == nil {
		return func() {}
	}
	return vreactive.BindTo(prop, t.SetText)
}

// BindSelected binds the selected item of the radio group to an int property
// in both directions: the item with the property's index is selected (and
// gets the focus mark), and choosing an item updates the property. An index
// outside the items leaves no item selected and the focus where it was, as
// SetData does. The returned function undoes the binding and restores the
// previous OnChange.
func (rg *RadioGroup) BindSelected(prop vreactive.Property[int]) func() {
	if rg == nil || prop == nil {
		return func() {}
	}
	previous := rg.OnChange
	return vreactive.TwoWayBind(prop,
		func() int { return rg.Selected },
		func(index int) {
			rg.Selected = index
			if index >= 0 && index < len(rg.Items) {
				rg.focusIdx = index
			}
			rg.NotifyChange()
		},
		func(onChange func(int)) func() {
			rg.OnChange = func(index int) {
				if previous != nil {
					previous(index)
				}
				onChange(index)
			}
			return func() { rg.OnChange = previous }
		})
}

// BindPercent makes the progress bar follow an int property (0..100, clamped
// by SetPercent): the current value is shown at once and every later change
// too. Combined with Property.SetBehavior the bar animates; from a background
// goroutine change the property with vreactive.SafeSet so the bar is updated
// on the UI thread. The returned function undoes the binding.
func (pb *ProgressBar) BindPercent(prop vreactive.Property[int]) func() {
	if pb == nil || prop == nil {
		return func() {}
	}
	return vreactive.BindTo(prop, pb.SetPercent)
}

// BindMask binds the options of the check group to a bit mask property in both
// directions: option i is checked while bit i of the property is set (the
// layout GetData and SetData use, so at most 32 options), and toggling an
// option updates the property. The returned function undoes the binding and
// restores the previous OnChange.
func (cg *CheckGroup) BindMask(prop vreactive.Property[uint32]) func() {
	if cg == nil || prop == nil {
		return func() {}
	}
	previous := cg.OnChange
	return vreactive.TwoWayBind(prop,
		func() uint32 { return cg.GetData().(uint32) },
		func(mask uint32) {
			cg.SetData(mask)
			cg.NotifyChange()
		},
		func(onChange func(uint32)) func() {
			cg.OnChange = func(mask uint32) {
				if previous != nil {
					previous(mask)
				}
				onChange(mask)
			}
			return func() { cg.OnChange = previous }
		})
}

// BindText binds the combo box text to a property in both directions: the
// text follows the property, and typing or choosing an item from the list
// updates it. The property's current value is applied at once. The returned
// function undoes the binding and restores the previous Edit.OnTextChange and
// Menu.OnAction handlers.
func (cb *ComboBox) BindText(prop vreactive.Property[string]) func() {
	if cb == nil || prop == nil {
		return func() {}
	}
	previousText := cb.Edit.OnTextChange
	previousAction := cb.Menu.OnAction
	return vreactive.TwoWayBind(prop, cb.Edit.GetText, cb.Edit.SetText, func(onChange func(string)) func() {
		cb.Edit.OnTextChange = func(text string) {
			if previousText != nil {
				previousText(text)
			}
			onChange(text)
		}
		// Choosing an item sets the text without OnTextChange, so it is
		// reported here, once the previous handler has put the item in the edit.
		cb.Menu.OnAction = func(idx int) {
			if previousAction != nil {
				previousAction(idx)
			}
			onChange(cb.Edit.GetText())
		}
		return func() {
			cb.Edit.OnTextChange = previousText
			cb.Menu.OnAction = previousAction
		}
	})
}
