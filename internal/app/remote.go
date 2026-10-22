package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"reflect"
	"time"

	"freebeams/internal/app/core"
	"freebeams/internal/control"
	"freebeams/internal/remote"
	"freebeams/internal/ui"
)

// The LAN remote control (internal/remote, docs/REMOTE.md). The server runs
// beside the app: phone actions arrive as control.Commands on its Commands
// channel and are drained in Process like the control API's, so a tap on the
// phone takes the same event path as the operator's own click or hotkey. The
// phone's view is a projection of the presenter and the schedule, published
// from render (remote.Server.Publish sends only what changed). Whether the
// server runs is a setting (remote.enabled, remote.port); the approved
// devices are persisted as token hashes (remote.devices).

// remoteApp is the App's remote control state.
type remoteApp struct {
	srv     *remote.Server
	name    string // what phones show as this computer ("FreeBeams on …")
	enabled bool   // the setting: run the server
	port    int    // the setting: its port
	err     string // why the server could not start ("" when it runs or is off)
	dialog  bool   // the pairing dialog is up

	status remote.Status // the server's status, refreshed on Changed and once a second
	saved  []byte        // the devices as last persisted (JSON)
	frames int           // frames since the last status refresh

	// The Options dialog's snapshot (Cancel restores it).
	snapEnabled bool
	snapPort    int
}

// remoteRefreshFrames is how often the status is re-read while the server
// runs, so waiting requests expire and "Seen 5 min ago" moves on.
const remoteRefreshFrames = 60

// initRemote creates the server, loads its settings and devices and starts it
// when it is enabled (Ready, after loadSettings).
func (a *App) initRemote() {
	host, _ := os.Hostname()
	a.rm.name = core.RemoteServerName(host)
	opt := remote.Options{Name: a.rm.name}
	if st := a.st; st != nil {
		opt.SearchSongs = func(ctx context.Context, q string) ([]remote.SongHit, error) {
			rows, err := st.SearchSongs(ctx, q, 50)
			if err != nil {
				return nil, err
			}
			hits := make([]remote.SongHit, 0, len(rows))
			for _, r := range rows {
				hits = append(hits, remote.SongHit{ID: r.ID, Title: r.Title, Author: r.Author})
			}
			return hits, nil
		}
	}
	a.rm.srv = remote.New(opt)
	a.rm.port = remote.DefaultPort
	if a.st != nil {
		get := func(key string, dst any) {
			if _, err := a.st.GetSetting(a.ctx(), key, dst); err != nil {
				a.fail("read setting "+key, err)
			}
		}
		get(core.SettingRemoteEnabled, &a.rm.enabled)
		get(core.SettingRemotePort, &a.rm.port)
		var devices []remote.Device
		get(core.SettingRemoteDevices, &devices)
		a.rm.srv.SetDevices(devices)
		a.rm.saved, _ = json.Marshal(a.rm.srv.StoredDevices())
	}
	if core.ValidRemotePort(a.rm.port) != nil {
		a.rm.port = remote.DefaultPort
	}
	a.syncRemoteServer()
	a.rm.status = a.rm.srv.Status()
	log.Printf("[freebeams] remote control %s (port %d, %d paired device(s))",
		map[bool]string{true: "on", false: "off"}[a.rm.enabled], a.rm.port, len(a.rm.status.Devices))
}

// syncRemoteServer starts or stops the server to match the setting. A port
// change restarts it.
func (a *App) syncRemoteServer() {
	srv := a.rm.srv
	if srv == nil {
		return
	}
	if !a.rm.enabled {
		srv.Stop()
		a.rm.err = ""
		return
	}
	if srv.Running() && srv.Status().Port == a.rm.port {
		return
	}
	srv.Stop()
	if err := srv.Start(a.rm.port); err != nil {
		a.rm.err = err.Error()
		a.fail("remote control", err)
		return
	}
	a.rm.err = ""
}

// stopRemote stops the server and persists the devices (shutdown, before the
// library closes; the async save path is already cancelled).
func (a *App) stopRemote() {
	if a.rm.srv == nil {
		return
	}
	a.rm.srv.Stop()
	if a.st == nil {
		return
	}
	devices := a.rm.srv.StoredDevices()
	if b, _ := json.Marshal(devices); string(b) != string(a.rm.saved) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := a.st.SetSetting(ctx, core.SettingRemoteDevices, devices); err != nil {
			log.Printf("[freebeams] save remote devices: %v", err)
		}
	}
}

// processRemote drains the phones' commands and the server's change signal
// (Process, every frame).
func (a *App) processRemote() {
	srv := a.rm.srv
	if srv == nil {
		return
	}
	for drained := false; !drained; {
		select {
		case cmd := <-srv.Commands():
			a.event(func() { a.handleCommand(cmd) })
		default:
			drained = true
		}
	}
	changed := false
	select {
	case <-srv.Changed():
		changed = true
		a.saveRemoteDevices()
	default:
	}
	if a.rm.frames++; changed || (a.rm.frames >= remoteRefreshFrames && (a.rm.status.Running || a.rm.dialog)) {
		a.rm.frames = 0
		a.refreshRemoteStatus()
	}
}

// refreshRemoteStatus re-reads the server's status; a change re-renders.
func (a *App) refreshRemoteStatus() {
	st := a.rm.srv.Status()
	if !reflect.DeepEqual(st, a.rm.status) {
		a.rm.status = st
		a.settingsVer++ // the Options page shows the status line
		a.dirty = true
	}
}

// saveRemoteDevices persists the approved devices when they changed.
func (a *App) saveRemoteDevices() {
	devices := a.rm.srv.StoredDevices()
	b, err := json.Marshal(devices)
	if err != nil || string(b) == string(a.rm.saved) {
		return
	}
	a.rm.saved = b
	a.saveSetting(core.SettingRemoteDevices, devices)
}

// ---- events -------------------------------------------------------------------------

// setRemoteEnabled turns the remote control on or off (persisted).
func (a *App) setRemoteEnabled(on bool) {
	if a.rm.enabled != on {
		a.rm.enabled = on
		a.saveSetting(core.SettingRemoteEnabled, on)
	}
	a.settingsVer++
	a.syncRemoteServer()
	a.refreshRemoteStatus()
}

// setRemotePort changes the port (persisted); a running server moves to it.
func (a *App) setRemotePort(port int) error {
	if err := core.ValidRemotePort(port); err != nil {
		return err
	}
	if a.rm.port != port {
		a.rm.port = port
		a.saveSetting(core.SettingRemotePort, port)
	}
	a.settingsVer++
	a.syncRemoteServer()
	a.refreshRemoteStatus()
	return nil
}

// openRemoteDialog shows the pairing dialog (re-reading the LAN addresses:
// the Wi-Fi may have changed since the server started).
func (a *App) openRemoteDialog() {
	a.rm.dialog = true
	if a.rm.srv != nil {
		a.rm.srv.RefreshAddresses()
		a.refreshRemoteStatus()
	}
}

func (a *App) remoteApprove(id string) error {
	if _, err := a.rm.srv.Approve(id); err != nil {
		return err
	}
	a.refreshRemoteStatus()
	return nil
}

func (a *App) remoteDeny(id string) error {
	if err := a.rm.srv.Deny(id); err != nil {
		return err
	}
	a.refreshRemoteStatus()
	return nil
}

func (a *App) remoteRevoke(id string) error {
	if err := a.rm.srv.Revoke(id); err != nil {
		return err
	}
	a.saveRemoteDevices()
	a.refreshRemoteStatus()
	return nil
}

func (a *App) remoteNewCode() {
	a.rm.srv.NewCode()
	a.refreshRemoteStatus()
}

// wireRemote connects the pairing dialog (called from buildUI).
func (a *App) wireRemote() {
	d := a.win.Remote
	d.OnEnabled = func(on bool) { a.event(func() { a.setRemoteEnabled(on) }) }
	d.OnNewCode = func() { a.event(a.remoteNewCode) }
	d.OnApprove = func(id string) { a.event(func() { a.fail("remote approve", a.remoteApprove(id)) }) }
	d.OnDeny = func(id string) { a.event(func() { a.fail("remote deny", a.remoteDeny(id)) }) }
	d.OnRevoke = func(id string) { a.event(func() { a.fail("remote revoke", a.remoteRevoke(id)) }) }
	d.OnClose = func() { a.event(func() { a.rm.dialog = false }) }

	s := a.win.Settings
	s.OnRemoteEnabled = func(on bool) { a.event(func() { a.setRemoteEnabled(on) }) }
	s.OnRemotePort = func(p int) { a.event(func() { a.fail("remote port", a.setRemotePort(p)) }) }
	s.OnShowRemote = func() { a.event(a.openRemoteDialog) }
}

// ---- view -------------------------------------------------------------------------

// remoteStatusText is the status line of the dialog and the Options page.
func (a *App) remoteStatusText() (string, bool) {
	return core.RemoteStatusText(a.rm.enabled, a.rm.status, a.rm.err)
}

// remoteValues is the pairing dialog's projection of the remote state.
func (a *App) remoteValues() ui.RemoteValues {
	st := a.rm.status
	text, isErr := a.remoteStatusText()
	v := ui.RemoteValues{Enabled: a.rm.enabled, Running: st.Running, Status: text, Error: isErr,
		URL: st.URL, Code: core.FormatPairingCode(st.Code), Pending: []ui.RemoteRow{}, Devices: []ui.RemoteRow{}}
	if st.Running && st.URL != "" {
		if side, px, err := remote.QRPixels(st.URL, 1); err == nil {
			v.QR, v.QRSide = px, side
		} else {
			log.Printf("[freebeams] remote QR code: %v", err) // render never writes state
		}
	}
	now := time.Now()
	for _, p := range st.Pending {
		v.Pending = append(v.Pending, ui.RemoteRow{ID: p.ID, Name: p.Name, Detail: core.RemoteDeviceKind(p.Kind) + " · " + p.Address})
	}
	for _, d := range st.Devices {
		v.Devices = append(v.Devices, ui.RemoteRow{ID: d.ID, Name: d.Name, Detail: core.RemoteDeviceKind(d.Kind) + " · " + core.RemoteDeviceStatus(d, now)})
	}
	return v
}

// remoteView is what renderRemote last applied.
type remoteView struct {
	values  ui.RemoteValues
	menuOn  bool
	applied bool
}

// renderRemote projects the remote state onto the pairing dialog and the
// toolbar menu, and publishes the phone's state (part of render).
func (a *App) renderRemote(v *remoteView) {
	if a.rm.srv == nil {
		return
	}
	w := a.win
	if a.rm.dialog || w.Remote.Visible() {
		vals := a.remoteValues()
		vals.QR = nil // compared without the pixels (they follow the URL)
		if !v.applied || !reflect.DeepEqual(vals, v.values) {
			v.values, v.applied = vals, true
			w.Remote.SetValues(a.remoteValues())
		}
	}
	switch {
	case a.rm.dialog && !w.Remote.Visible():
		w.Remote.Open()
	case !a.rm.dialog && w.Remote.Visible():
		w.Remote.Hide()
	}
	if on := a.rm.enabled; on != v.menuOn || !a.view.init {
		v.menuOn = on
		w.Toolbar.SetRemoteOn(on)
	}
	if a.rm.status.Running {
		a.rm.srv.Publish(remote.BuildState(remote.StateInput{
			Name: a.rm.name, Presenter: a.pres.State, Schedule: a.sched, Selected: a.schedSel,
			Audio: a.audioPhoneDoc(),
		}))
	}
}

// remoteStateDoc is /state.remote.
func (a *App) remoteStateDoc() core.RemoteState {
	r := core.RemoteState{Enabled: a.rm.enabled, ConfiguredPort: a.rm.port, Error: a.rm.err}
	if a.rm.dialog {
		r.Dialog = "pairing"
	}
	if a.rm.srv != nil {
		r.Status = a.rm.srv.Status()
	}
	return r
}

// ---- control -----------------------------------------------------------------------

// handleRemoteCommand runs the remote control actions; false when the action
// is not one of them.
//
//	remote {on, port}            turn the server on / off, move it to a port
//	remote-dialog {on}           show (default) or close the pairing dialog
//	remote-approve {device | name}  approve a waiting phone (neither: the first one)
//	remote-deny {device | name}     refuse it
//	remote-revoke {device | name}   remove a paired device (its connections close)
//	remote-new-code              replace the pairing code
//	remote-live-item {index}     (phones) preview schedule row index and go live
//	remote-add-song {id}         (phones) add library song id to the schedule
func (a *App) handleRemoteCommand(cmd *control.Command, args actionArgs) bool {
	if a.rm.srv == nil {
		switch cmd.Action {
		case "remote", "remote-dialog", "remote-approve", "remote-deny", "remote-revoke", "remote-new-code",
			"remote-live-item", "remote-add-song":
			cmd.RespondErr(errors.New("the remote control is not available"))
			return true
		}
		return false
	}
	switch cmd.Action {
	case "remote":
		if args.Port != nil {
			if err := a.setRemotePort(*args.Port); err != nil {
				cmd.RespondErr(err)
				return true
			}
		}
		if args.On != nil {
			a.setRemoteEnabled(*args.On)
		}
		if a.rm.err != "" {
			a.answer(cmd, errors.New(a.rm.err))
			return true
		}
		a.answer(cmd, nil)
	case "remote-dialog":
		if args.On == nil || *args.On {
			a.openRemoteDialog()
		} else {
			a.rm.dialog = false
		}
		a.answer(cmd, nil)
	case "remote-approve", "remote-deny":
		st := a.rm.srv.Status()
		i := core.FindRemoteRequest(st.Pending, args.Device, args.Name)
		if i < 0 {
			cmd.RespondErr(fmt.Errorf("%s: no device is waiting that matches %s", cmd.Action, string(cmd.Data)))
			return true
		}
		if cmd.Action == "remote-approve" {
			a.answer(cmd, a.remoteApprove(st.Pending[i].ID))
		} else {
			a.answer(cmd, a.remoteDeny(st.Pending[i].ID))
		}
	case "remote-revoke":
		st := a.rm.srv.Status()
		i := core.FindRemoteDevice(st.Devices, args.Device, args.Name)
		if i < 0 {
			cmd.RespondErr(fmt.Errorf("remote-revoke: no paired device matches %s", string(cmd.Data)))
			return true
		}
		a.answer(cmd, a.remoteRevoke(st.Devices[i].ID))
	case "remote-new-code":
		a.remoteNewCode()
		a.answer(cmd, nil)
	case "remote-live-item":
		if args.Index == nil || *args.Index < 0 || *args.Index >= len(a.sched.Items) {
			cmd.RespondErr(fmt.Errorf("remote-live-item: no schedule item %s", string(cmd.Data)))
			return true
		}
		a.selectScheduleItem(*args.Index)
		a.goLive()
		a.answer(cmd, nil)
	case "remote-add-song":
		a.remoteAddSong(cmd, args.ID)
	default:
		return false
	}
	return true
}

// remoteAddSong loads library song id and adds it to the end of the schedule.
func (a *App) remoteAddSong(cmd *control.Command, id int64) {
	if a.st == nil {
		cmd.RespondErr(errNoLibrary)
		return
	}
	if id <= 0 {
		cmd.RespondErr(errArgs)
		return
	}
	st := a.st
	a.async(cmd, func(ctx context.Context) func() error {
		song, err := st.GetSong(ctx, id)
		return func() error {
			if err != nil {
				return fmt.Errorf("add song %d: %w", id, err)
			}
			a.addSongToSchedule(song)
			return nil
		}
	})
}
