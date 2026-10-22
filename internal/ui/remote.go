package ui

import (
	"reflect"

	"graphics.gd/classdb/Button"
	"graphics.gd/classdb/CanvasItem"
	"graphics.gd/classdb/CheckBox"
	"graphics.gd/classdb/Control"
	"graphics.gd/classdb/Image"
	"graphics.gd/classdb/ImageTexture"
	"graphics.gd/classdb/Label"
	"graphics.gd/classdb/TextServer"
	"graphics.gd/classdb/Texture2D"
	"graphics.gd/classdb/TextureRect"
	"graphics.gd/classdb/Theme"
	"graphics.gd/classdb/VBoxContainer"
	"graphics.gd/classdb/Window"
	"graphics.gd/variant/Object"
	"graphics.gd/variant/Vector2"
)

// RemoteRow is a phone waiting for approval or a paired device in the Remote
// dialog.
type RemoteRow struct {
	ID     string
	Name   string
	Detail string // "Phone · 192.168.1.20", "Phone · Connected"
}

// RemoteValues is what the Remote dialog shows (a projection of the app's
// remote state).
type RemoteValues struct {
	Enabled bool   // the setting
	Running bool   // the server is listening
	Status  string // one line under the switch
	Error   bool   // Status is an error
	URL     string // what the QR code encodes
	Code    string // the pairing code, spaced ("123 456")
	// QR is the QR code as opaque RGBA8 pixels, QRSide × QRSide (one pixel
	// per module, quiet zone included); nil while the remote is off.
	QR      []byte
	QRSide  int
	Pending []RemoteRow
	Devices []RemoteRow
}

// RemoteDialog is the remote control's pairing dialog (toolbar Remote ▾ ▸
// Show Pairing Code…): the on/off switch, a QR code with the address, the
// pairing code, the phones waiting for approval and the paired devices. It is
// a dumb view: SetValues projects the app's state; Approve, Deny, Remove, New
// Code and the switch are reported through the On… callbacks.
type RemoteDialog struct {
	OnEnabled func(on bool)
	OnNewCode func()
	OnApprove func(id string)
	OnDeny    func(id string)
	OnRevoke  func(id string)
	OnClose   func() // Close, Esc or the window's close button

	dlg       *dialogWindow
	enable    CheckBox.Instance
	status    Label.Instance
	qr        TextureRect.Instance
	url       Label.Instance
	code      Label.Instance
	newCode   Button.Instance
	pairing   Control.Instance // the QR code and the instructions
	pending   VBoxContainer.Instance
	devices   VBoxContainer.Instance
	noPending Label.Instance
	noDevices Label.Instance

	values    RemoteValues
	qrURL     string // what the QR texture shows
	rows      []Control.Instance
	drawnRows [2][]RemoteRow
	rendering bool
}

// NewRemoteDialog builds the dialog. MainWindow adds AsWindow() to the tree.
func NewRemoteDialog() *RemoteDialog {
	d := &RemoteDialog{}
	d.dlg = newDialogWindow("Remote Control", 620, 600)
	d.dlg.onOK = d.dlg.close
	d.dlg.onClose = func() {
		if d.OnClose != nil {
			d.OnClose()
		}
	}

	d.enable = CheckBox.New()
	d.enable.AsButton().SetText("Allow phones and tablets on this network to control FreeBeams")
	d.enable.AsBaseButton().OnToggled(func(on bool) {
		if !d.rendering && d.OnEnabled != nil {
			d.OnEnabled(on)
		}
	})
	d.status = wrapLabel("", vHint)

	// The QR code on white (its quiet zone is white too), drawn with nearest
	// filtering so the modules stay sharp at any scale.
	d.qr = TextureRect.New()
	d.qr.SetExpandMode(TextureRect.ExpandIgnoreSize)
	d.qr.SetStretchMode(TextureRect.StretchKeepAspectCentered)
	d.qr.AsCanvasItem().SetTextureFilter(CanvasItem.TextureFilterNearest)
	d.qr.AsControl().SetCustomMinimumSize(Vector2.New(200, 200))
	d.url = wrapLabel("", vTitle)
	d.code = label("", vTitle)
	d.code.SetClipText(false)
	d.code.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
	d.code.AsControl().AddThemeFontSizeOverride("font_size", 30)
	d.newCode = Button.New()
	d.newCode.SetText("New Code")
	d.newCode.AsBaseButton().OnPressed(func() {
		if d.OnNewCode != nil {
			d.OnNewCode()
		}
	})
	codeRow := hbox(12)
	add(codeRow, d.code.AsControl(), d.newCode.AsControl(), spacer())
	steps := vbox(6)
	steps.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	add(steps,
		wrapLabel("1. On a phone or tablet on the same Wi-Fi, scan the code or open:", "").AsControl(),
		d.url.AsControl(),
		wrapLabel("2. Enter this pairing code:", "").AsControl(),
		codeRow.AsControl(),
		wrapLabel("3. Approve the device below. Each code pairs one device; a new code is made after every approval.", vHint).AsControl(),
	)
	pairRow := hbox(16)
	add(pairRow, d.qr.AsControl(), steps.AsControl())
	d.pairing = pairRow.AsControl()

	d.pending = vbox(4)
	d.noPending = label("No device is waiting.", vDim)
	add(d.pending, d.noPending.AsControl())
	d.devices = vbox(4)
	d.noDevices = label("No devices are paired yet.", vDim)
	add(d.devices, d.noDevices.AsControl())

	add(d.dlg.body,
		d.enable.AsControl(), d.status.AsControl(), fixed(0, 4), d.pairing, fixed(0, 6),
		label("Waiting for approval", vTitle).AsControl(), d.pending.AsControl(), fixed(0, 6),
		label("Paired devices", vTitle).AsControl(), d.devices.AsControl(),
	)
	d.dlg.ok.SetText("Close")
	add(d.dlg.footer, spacer(), d.dlg.ok.AsControl())
	Object.Free(d.dlg.cancel) // Close only: the Cancel button never joins the tree
	d.dlg.cancel = Button.Nil
	d.dlg.win.AsNode().SetName("RemoteDialog")
	d.render()
	return d
}

// wrapLabel is a label that wraps instead of clipping.
func wrapLabel(text, variation string) Label.Instance {
	l := label(text, variation)
	l.SetAutowrapMode(TextServer.AutowrapWordSmart)
	l.SetClipText(false)
	l.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
	l.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	return l
}

// AsWindow returns the dialog window node.
func (d *RemoteDialog) AsWindow() Window.Instance { return d.dlg.win }

// SetTheme sets the dialog's theme (separate OS windows do not inherit it).
func (d *RemoteDialog) SetTheme(th Theme.Instance) { d.dlg.setTheme(th) }

// Open shows the dialog.
func (d *RemoteDialog) Open() {
	d.render()
	d.dlg.popup()
}

// Hide closes the dialog without calling OnClose.
func (d *RemoteDialog) Hide() { d.dlg.hide() }

// Visible reports whether the dialog is on screen.
func (d *RemoteDialog) Visible() bool { return d.dlg.win.Visible() }

// SetValues projects the app's remote state onto the dialog.
func (d *RemoteDialog) SetValues(v RemoteValues) {
	d.values = v
	d.render()
}

func (d *RemoteDialog) render() {
	d.rendering = true
	defer func() { d.rendering = false }()
	v := d.values
	d.enable.AsBaseButton().SetPressedNoSignal(v.Enabled)
	d.status.SetText(v.Status)
	if v.Error {
		d.status.AsControl().SetThemeTypeVariation(vError)
	} else {
		d.status.AsControl().SetThemeTypeVariation(vHint)
	}
	d.pairing.AsCanvasItem().SetVisible(v.Running)
	d.url.SetText(v.URL)
	d.code.SetText(v.Code)
	if v.URL != d.qrURL {
		d.qrURL = v.URL
		d.setQR(v.QR, v.QRSide)
	}
	d.renderRows()
	flushCalls(d.enable.AsControl()) // the no-signal setter lands before rendering is reset
}

// setQR shows the QR pixels (nil clears the code). The texture is owned by
// the TextureRect; no Go reference is kept.
func (d *RemoteDialog) setQR(px []byte, side int) {
	if side <= 0 || len(px) != side*side*4 {
		d.qr.SetTexture(Texture2D.Nil)
		return
	}
	img := Image.CreateFromData(side, side, false, Image.FormatRgba8, px)
	d.qr.SetTexture(ImageTexture.CreateFromImage(img).AsTexture2D())
}

// renderRows rebuilds the pending and device rows when they changed.
func (d *RemoteDialog) renderRows() {
	rows := [2][]RemoteRow{d.values.Pending, d.values.Devices}
	if reflect.DeepEqual(rows, d.drawnRows) && d.rows != nil {
		return
	}
	d.drawnRows = rows
	for _, r := range d.rows {
		r.AsNode().QueueFree()
	}
	d.rows = []Control.Instance{}
	for _, p := range d.values.Pending {
		id := p.ID
		row := d.row(p,
			d.button("Approve", func() {
				if d.OnApprove != nil {
					d.OnApprove(id)
				}
			}),
			d.button("Deny", func() {
				if d.OnDeny != nil {
					d.OnDeny(id)
				}
			}))
		add(d.pending, row)
	}
	for _, dev := range d.values.Devices {
		id := dev.ID
		row := d.row(dev, d.button("Remove", func() {
			if d.OnRevoke != nil {
				d.OnRevoke(id)
			}
		}))
		add(d.devices, row)
	}
	d.noPending.AsCanvasItem().SetVisible(len(d.values.Pending) == 0)
	d.noDevices.AsCanvasItem().SetVisible(len(d.values.Devices) == 0)
}

// row is one device line: its name, the detail and the buttons.
func (d *RemoteDialog) row(r RemoteRow, buttons ...Control.Instance) Control.Instance {
	p := panel(vBk1)
	h := hbox(8)
	name := label(r.Name, "")
	name.AsControl().SetSizeFlagsHorizontal(Control.SizeExpandFill)
	detail := label(r.Detail, vDim)
	detail.SetClipText(false)
	detail.SetTextOverrunBehavior(TextServer.OverrunNoTrimming)
	add(h, name.AsControl(), detail.AsControl())
	add(h, buttons...)
	add(p, padded(h.AsControl(), 8, 4, 6, 4))
	d.rows = append(d.rows, p.AsControl())
	return p.AsControl()
}

func (d *RemoteDialog) button(text string, fn func()) Control.Instance {
	b := Button.New()
	b.SetText(text)
	b.AsControl().SetCustomMinimumSize(Vector2.New(75, 25))
	b.AsBaseButton().OnPressed(fn)
	return b.AsControl()
}
