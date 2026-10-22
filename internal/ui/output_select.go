package ui

import (
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/PopupMenu"
)

// The Live Output monitor's output selector (EasyWorship's
// ToolButton_LiveFoldBackOuputSelect with PopupMenu_Outputs): a dropdown of
// radio items Live Output · Alternate Output (locked) · — · FoldBack Output
// that chooses what the monitor previews. The caption beside it names the
// chosen output. Ids follow EasyWorship's menu item tags.
const (
	outputLive     = 0
	outputAlt      = 1
	outputFoldback = 2
)

// Captions of the monitor bar for each output.
const (
	LiveOutputCaption     = "Live Output"
	FoldbackOutputCaption = "Foldback Output"
)

type outputSelect struct {
	menu     PopupMenu.Instance
	caption  Label.Instance
	foldback bool
}

// wireOutputSelect turns the swap button into the output selector.
func (w *MainWindow) wireOutputSelect(swap *miniButton, caption Label.Instance) {
	s := &outputSelect{caption: caption}
	s.menu = PopupMenu.New()
	s.menu.AsNode().SetName("OutputSelect")
	s.menu.MoreArgs().AddRadioCheckItem(LiveOutputCaption, outputLive, 0)
	s.menu.MoreArgs().AddRadioCheckItem("Alternate Output", outputAlt, 0)
	s.menu.SetItemDisabled(s.menu.GetItemIndex(outputAlt), true) // EasyWorship: Locked
	s.menu.AddSeparator()
	s.menu.MoreArgs().AddRadioCheckItem(FoldbackOutputCaption, outputFoldback, 0)
	s.menu.OnIdPressed(func(id int) {
		if id != outputLive && id != outputFoldback {
			return
		}
		if w.OnMonitorOutput != nil {
			w.OnMonitorOutput(id == outputFoldback)
		}
	})
	swap.AsNode().AddChild(s.menu.AsNode())
	swap.AsBaseButton().OnPressed(func() { popupBelow(s.menu, swap.AsControl()) })
	w.outSel = s
	s.render()
}

// SetMonitorOutput shows which output the Live Output monitor previews (the
// app swaps the monitor's content itself).
func (w *MainWindow) SetMonitorOutput(foldback bool) {
	if w.outSel == nil || w.outSel.foldback == foldback {
		return
	}
	w.outSel.foldback = foldback
	w.outSel.render()
}

// MonitorCaption is the Live Output monitor bar's caption.
func (w *MainWindow) MonitorCaption() string {
	if w.outSel == nil {
		return ""
	}
	return w.outSel.caption.Text()
}

func (s *outputSelect) render() {
	s.menu.SetItemChecked(s.menu.GetItemIndex(outputLive), !s.foldback)
	s.menu.SetItemChecked(s.menu.GetItemIndex(outputFoldback), s.foldback)
	if s.foldback {
		s.caption.SetText(FoldbackOutputCaption)
	} else {
		s.caption.SetText(LiveOutputCaption)
	}
}
